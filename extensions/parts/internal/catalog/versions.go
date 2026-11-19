package catalog

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/storageapi"
)

// FileDraft names one file a new version holds. Storage is left out for a
// file the page has just uploaded, which is in the storage the settings name.
// A file kept from an earlier version is sent with the storage it was
// recorded under.
type FileDraft struct {
	Storage string `json:"storage,omitempty"`
	Path    string `json:"path"`
}

// VersionDraft is what a caller sends to add a version.
type VersionDraft struct {
	Note  string      `json:"note,omitempty"`
	Files []FileDraft `json:"files"`
}

// AddVersion writes a new version of a part, with the files the draft names.
//
// Every file is looked up in the storage first, as the person the host named,
// and the size and digest the storage reports are what the version records.
// So a version never names a file that was not there when it was written,
// and a person cannot record a file they may not read.
//
// The lookups run before the write lock is taken, so a slow storage holds up
// no other change. The part and its count are checked again under the lock.
func (c *Catalog) AddVersion(
	ctx context.Context, part int, draft VersionDraft, by string,
) (Version, error) {
	note, err := block("note", draft.Note, MaxNoteBytes, false)
	if err != nil {
		return Version{}, err
	}

	c.mu.RLock()
	_, found := c.parts[part]
	place := c.place
	known := map[string]File{}

	for _, version := range c.versions[part] {
		for _, file := range version.row.Files {
			known[file.Storage+"\x00"+file.Path] = file
		}
	}
	c.mu.RUnlock()

	if !found {
		return Version{}, fmt.Errorf("%w: %d", ErrPartNotFound, part)
	}

	files, err := c.filesOf(ctx, part, draft.Files, place, known, by)
	if err != nil {
		return Version{}, err
	}

	c.writing.Lock()
	defer c.writing.Unlock()

	c.mu.RLock()
	owner, found := c.parts[part]
	count := len(c.versions[part])
	c.mu.RUnlock()

	if !found {
		return Version{}, fmt.Errorf("%w: %d", ErrPartNotFound, part)
	}

	if count >= MaxVersions {
		return Version{}, fmt.Errorf("%w: a part keeps at most %d versions", ErrFull, MaxVersions)
	}

	// The number after the highest one the part ever gave out, and not after
	// the highest it holds now. A deleted newest version keeps its number.
	next := owner.row.LastVersion + 1

	now := c.now().UTC()
	version := Version{
		V: Format, Part: part, Number: next, Note: note, Files: files,
		Created: now, CreatedBy: by, Changed: now, ChangedBy: by,
	}

	db, err := c.store()
	if err != nil {
		return Version{}, err
	}

	if err = db.addVersion(ctx, version); err != nil {
		return Version{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.versions[part] = append(c.versions[part], held[Version]{row: version})
	sortedVersions(c.versions[part])

	owner.row.LastVersion = next
	c.parts[part] = owner
	c.index(part)

	return version, nil
}

// filesOf turns the files a draft names into the files a version records.
func (c *Catalog) filesOf(
	ctx context.Context, part int, drafts []FileDraft, place Place, known map[string]File, by string,
) ([]File, error) {
	switch {
	case len(drafts) == 0:
		return nil, invalid("a version holds at least one file")
	case len(drafts) > MaxFilesPerVersion:
		return nil, fmt.Errorf("%w: a version holds at most %d files, and %d were sent",
			ErrFull, MaxFilesPerVersion, len(drafts))
	}

	files := make([]File, 0, len(drafts))
	seen := make([]string, 0, len(drafts))

	for _, draft := range drafts {
		storage := draft.Storage
		if storage == "" {
			storage = place.Storage
		}

		identity := storage + "\x00" + draft.Path
		if slices.Contains(seen, identity) {
			return nil, invalid("the file %q is named twice", draft.Path)
		}

		seen = append(seen, identity)

		// A file an earlier version recorded is kept as it was recorded, even
		// when the settings have moved the place new files go. Anything else
		// has to be in this part's own folder under the current place.
		var name string
		if earlier, found := known[identity]; found {
			name = earlier.Name
		} else {
			if storage != place.Storage {
				return nil, invalid("the file %q is in storage %q, and new files go into %q",
					draft.Path, storage, place.Storage)
			}

			checked, err := filePath(place.Root, part, draft.Path)
			if err != nil {
				return nil, err
			}

			name = checked
		}

		info, err := c.files.Stat(ctx, storageapi.StatParams{Principal: by, Storage: storage, Path: draft.Path})
		if err != nil {
			return nil, statFailed(draft.Path, err)
		}

		if info.IsDir {
			return nil, invalid("%q is a folder, and a version holds files", draft.Path)
		}

		file := File{Name: name, Storage: storage, Path: draft.Path, Size: info.Size, SHA256: info.SHA256}
		// The host reads a print file's picture while it describes the file, and
		// leaves Print out when it found neither details nor a picture.
		if slices.Contains(thumbnailExtensions, strings.ToLower(path.Ext(name))) {
			carries := info.Print != nil && info.Print.ThumbnailWidth > 0
			file.Thumbnail = &carries
		}

		files = append(files, file)
	}

	return files, nil
}

// statFailed sorts a refusal of the storage door into what the person can
// act on. A file that is not there and one they may not read are one answer,
// as the storage door itself answers them.
func statFailed(written string, err error) error {
	switch {
	case errors.Is(err, storageapi.ErrNoFile), errors.Is(err, storageapi.ErrDenied),
		errors.Is(err, storageapi.ErrInvalidPath):
		return fmt.Errorf("%w: %s", ErrFileMissing, written)
	case errors.Is(err, storageapi.ErrUnreadable):
		return invalid("%q cannot be read as a file", written)
	}

	return storeFailed("look up "+written, err)
}

// ChangeVersion rewrites the note of one version. The files of a version do
// not change: a different set of files is a new version.
func (c *Catalog) ChangeVersion(ctx context.Context, part, number int, note, by string) (Version, error) {
	written, err := block("note", note, MaxNoteBytes, false)
	if err != nil {
		return Version{}, err
	}

	c.writing.Lock()
	defer c.writing.Unlock()

	c.mu.RLock()
	row, err := c.versionAt(part, number)
	c.mu.RUnlock()

	if err != nil {
		return Version{}, err
	}

	version := row.row
	version.V = Format
	version.Note = written
	version.Changed = c.now().UTC()
	version.ChangedBy = by

	db, err := c.store()
	if err != nil {
		return Version{}, err
	}

	if err = db.saveVersion(ctx, version); err != nil {
		return Version{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for at, kept := range c.versions[part] {
		if kept.row.Number == number {
			c.versions[part][at] = held[Version]{row: version}
		}
	}

	return version, nil
}

// DeleteVersion removes one version, and answers the files no other version
// of the part names, which the page then deletes from the storage. The part
// keeps the number, so the next version does not take it again.
func (c *Catalog) DeleteVersion(ctx context.Context, part, number int) (Removed, error) {
	c.writing.Lock()
	defer c.writing.Unlock()

	c.mu.RLock()
	row, err := c.versionAt(part, number)
	place := c.place
	c.mu.RUnlock()

	if err != nil {
		return Removed{}, err
	}

	db, err := c.store()
	if err != nil {
		return Removed{}, err
	}

	if err = db.deleteVersion(ctx, part, number); err != nil {
		return Removed{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.versions[part] = slices.DeleteFunc(c.versions[part], func(kept held[Version]) bool {
		return kept.row.Number == number
	})

	if owner, found := c.parts[part]; found {
		owner.row.LastVersion = max(owner.row.LastVersion, number)
		c.parts[part] = owner
	}

	c.index(part)

	removed := Removed{Storage: place.Storage, Dirs: []Dir{}, Files: []File{}}

	for _, file := range row.row.Files {
		if !c.holdsFile(part, file) {
			removed.Files = append(removed.Files, file)
		}
	}

	return removed, nil
}

// versionAt finds one version of a part. The caller holds the read lock.
func (c *Catalog) versionAt(part, number int) (held[Version], error) {
	if _, found := c.parts[part]; !found {
		return held[Version]{}, fmt.Errorf("%w: %d", ErrPartNotFound, part)
	}

	for _, row := range c.versions[part] {
		if row.row.Number == number {
			return row, nil
		}
	}

	return held[Version]{}, fmt.Errorf("%w: part %d has no version %d", ErrVersionNotFound, part, number)
}
