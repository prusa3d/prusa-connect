package catalog

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PartDraft is what a caller sends to make or to change a part. A field left
// out keeps what is stored, and a field sent empty clears it.
//
// Revision is the version of the record the caller read. A change that names
// one is refused when somebody else changed the part since, so two people
// editing one description do not overwrite each other without knowing.
type PartDraft struct {
	Name        *string   `json:"name,omitempty"`
	Folder      *int      `json:"folder,omitempty"`
	Summary     *string   `json:"summary,omitempty"`
	Description *string   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Cover       *string   `json:"cover,omitempty"`
	Revision    *uint64   `json:"revision,omitempty"`
}

// Picture is the file a listing draws for a part: a picture file of its own,
// or a print file with a thumbnail inside it.
type Picture struct {
	Storage string `json:"storage"`
	Path    string `json:"path"`
	// Kind says how a page reads it: image is the file itself, thumbnail is
	// the picture a slicer wrote into it.
	Kind string `json:"kind"`
}

// The two kinds of picture.
const (
	PictureImage     = "image"
	PictureThumbnail = "thumbnail"
)

// imageExtensions are the files a browser draws as they are.
var imageExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}

// thumbnailExtensions are the files a slicer writes a picture into.
var thumbnailExtensions = []string{".bgcode", ".gcode", ".3mf"}

// pictureOf is the file that stands for a part: the cover a person chose
// while a version still holds it, else the first picture file of the newest
// version, else the first print file that carries a thumbnail.
func pictureOf(cover string, latest *Version) *Picture {
	if latest == nil {
		return nil
	}

	kindOf := func(name string) string {
		extension := strings.ToLower(path.Ext(name))

		switch {
		case slices.Contains(imageExtensions, extension):
			return PictureImage
		case slices.Contains(thumbnailExtensions, extension):
			return PictureThumbnail
		}

		return ""
	}

	// A print file the storage found no picture in cannot stand for the part.
	draws := func(file File) bool { return file.Thumbnail == nil || *file.Thumbnail }

	if cover != "" {
		for _, file := range latest.Files {
			if file.Path == cover && kindOf(file.Name) != "" && draws(file) {
				return &Picture{Storage: file.Storage, Path: file.Path, Kind: kindOf(file.Name)}
			}
		}
	}

	for _, wanted := range []string{PictureImage, PictureThumbnail} {
		for _, file := range latest.Files {
			if kindOf(file.Name) == wanted && draws(file) {
				return &Picture{Storage: file.Storage, Path: file.Path, Kind: wanted}
			}
		}
	}

	return nil
}

// Latest is what a listing shows of a part's newest version.
type Latest struct {
	Number  int       `json:"number"`
	Files   int       `json:"files"`
	Bytes   int64     `json:"bytes"`
	Created time.Time `json:"created"`
}

// PartRow is one part as a listing shows it: the record without its long
// text, and what its versions and comments add up to.
type PartRow struct {
	ID        int       `json:"id"`
	Folder    int       `json:"folder"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary,omitempty"`
	Tags      []string  `json:"tags"`
	Versions  int       `json:"versions"`
	Comments  int       `json:"comments"`
	Latest    *Latest   `json:"latest,omitempty"`
	Picture   *Picture  `json:"picture,omitempty"`
	Changed   time.Time `json:"changed"`
	ChangedBy string    `json:"changed_by"`
}

// PartDetail is one part with everything a part's own page draws.
type PartDetail struct {
	Part

	// Tags shadows the record's own field, which is left out when empty. The
	// page reads the tags of every part it opens, so this one is always sent.
	Tags []string `json:"tags"`
	// Revision is the version of the record. A change sends it back.
	Revision uint64    `json:"revision"`
	Versions []Version `json:"versions"`
	Comments []Comment `json:"comments"`
	Picture  *Picture  `json:"picture,omitempty"`
	// Dir is the folder in the storage this part's new files go into.
	Dir string `json:"dir"`
	// Storage is the storage new files go into.
	Storage string `json:"storage"`
}

// Query narrows a listing. The zero value lists every part.
type Query struct {
	// Folder keeps the parts in one folder. Nil keeps every folder.
	Folder *int
	// Deep also keeps the parts in every folder under Folder.
	Deep bool
	// Text keeps the parts whose name, summary, tags, description or file
	// names hold it, ignoring case.
	Text string
	// Tag keeps the parts that carry one tag.
	Tag string
	// Order is name or changed. Changed puts the newest first.
	Order string
	// Offset and Limit cut one page out of what matched.
	Offset int
	Limit  int
}

// The orders a listing may be in.
const (
	OrderName    = "name"
	OrderChanged = "changed"
)

// MaxLimit is the most parts one page answers.
const MaxLimit = 200

// Page is one page of a listing.
type Page struct {
	Parts []PartRow `json:"parts"`
	// Matched is how many parts the query kept, over every page.
	Matched int `json:"matched"`
	// Held is how many parts the catalogue holds, whatever the query kept.
	Held int `json:"held"`
}

// searchText is what a listing searches in one part, lower-cased once when
// the part or its files change.
type searchText struct {
	// name is the name alone, which a listing also sorts by.
	name string
	// all is every field a person would look in, joined by a character no
	// field and no query holds, so a match never spans two fields.
	all string
}

// fieldBreak joins the fields of a searchText.
const fieldBreak = "\x00"

// index builds the search text of one part from its record and the names of
// its files. The caller holds the write lock.
func (c *Catalog) index(id int) {
	part, found := c.parts[id]
	if !found {
		delete(c.search, id)

		return
	}

	fields := []string{part.row.Name, part.row.Summary, part.row.Description}
	fields = append(fields, part.row.Tags...)

	for _, version := range c.versions[id] {
		for _, file := range version.row.Files {
			fields = append(fields, file.Name)
		}
	}

	c.search[id] = searchText{
		name: strings.ToLower(part.row.Name),
		all:  strings.ToLower(strings.Join(fields, fieldBreak)),
	}
}

// match is one part a listing kept, with what it is ordered by.
type match struct {
	id      int
	name    string
	changed time.Time
}

// List answers one page of the parts that match.
//
// It orders the parts that matched by what the order needs and renders only
// the page it answers, so a long listing costs one row per part on the page.
func (c *Catalog) List(query Query) Page {
	needle := strings.ToLower(strings.TrimSpace(query.Text))
	tag := strings.ToLower(strings.TrimSpace(query.Tag))

	c.mu.RLock()
	defer c.mu.RUnlock()

	// A query holding the character the fields are joined by would match
	// across two of them, and no field holds it.
	if strings.Contains(needle, fieldBreak) {
		return Page{Parts: []PartRow{}, Held: len(c.parts)}
	}

	matched := make([]match, 0, len(c.parts))

	for id, part := range c.parts {
		text := c.search[id]

		if c.inFolder(part.row, query) && (tag == "" || slices.Contains(part.row.Tags, tag)) &&
			(needle == "" || strings.Contains(text.all, needle)) {
			matched = append(matched, match{id: id, name: text.name, changed: c.changedOf(part.row)})
		}
	}

	slices.SortFunc(matched, func(a, b match) int {
		if query.Order == OrderChanged && !a.changed.Equal(b.changed) {
			return b.changed.Compare(a.changed)
		}

		if order := strings.Compare(a.name, b.name); order != 0 {
			return order
		}

		return a.id - b.id
	})

	limit := query.Limit
	if limit <= 0 || limit > MaxLimit {
		limit = MaxLimit
	}

	start := min(max(query.Offset, 0), len(matched))
	end := min(start+limit, len(matched))

	rows := make([]PartRow, 0, end-start)
	for _, kept := range matched[start:end] {
		rows = append(rows, c.rowOf(c.parts[kept.id].row))
	}

	return Page{Parts: rows, Matched: len(matched), Held: len(c.parts)}
}

// changedOf is when a part last changed as a listing reads it: its own
// record, or its newest version when that is later. The caller holds the
// read lock.
func (c *Catalog) changedOf(part Part) time.Time {
	versions := c.versions[part.ID]
	if len(versions) > 0 && versions[len(versions)-1].row.Created.After(part.Changed) {
		return versions[len(versions)-1].row.Created
	}

	return part.Changed
}

// inFolder reports whether a part is in the folder a query names, or under it
// for a deep query. A query naming no folder keeps every part. The caller
// holds the read lock.
func (c *Catalog) inFolder(part Part, query Query) bool {
	switch {
	case query.Folder == nil:
		return true
	case query.Deep:
		return c.within(part.Folder, *query.Folder)
	}

	return part.Folder == *query.Folder
}

// rowOf renders one part for a listing. The caller holds the read lock.
func (c *Catalog) rowOf(part Part) PartRow {
	versions := c.versions[part.ID]
	row := PartRow{
		ID:        part.ID,
		Folder:    part.Folder,
		Name:      part.Name,
		Summary:   part.Summary,
		Tags:      part.Tags,
		Versions:  len(versions),
		Comments:  len(c.comments[part.ID]),
		Changed:   part.Changed,
		ChangedBy: part.ChangedBy,
	}

	if row.Tags == nil {
		row.Tags = []string{}
	}

	if len(versions) > 0 {
		latest := versions[len(versions)-1].row
		summary := Latest{Number: latest.Number, Files: len(latest.Files), Created: latest.Created}

		for _, file := range latest.Files {
			summary.Bytes += file.Size
		}

		row.Latest = &summary
		row.Picture = pictureOf(part.Cover, &latest)

		// A new version is a change a listing ordered by change shows first.
		if latest.Created.After(row.Changed) {
			row.Changed = latest.Created
			row.ChangedBy = latest.CreatedBy
		}
	}

	return row
}

// Tag is one tag and how many parts carry it.
type Tag struct {
	Tag   string `json:"tag"`
	Parts int    `json:"parts"`
}

// Tags answers every tag the catalogue uses, the most used first.
func (c *Catalog) Tags() []Tag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	counts := map[string]int{}

	for _, part := range c.parts {
		for _, tag := range part.row.Tags {
			counts[tag]++
		}
	}

	out := make([]Tag, 0, len(counts))
	for tag, count := range counts {
		out = append(out, Tag{Tag: tag, Parts: count})
	}

	slices.SortFunc(out, func(a, b Tag) int {
		if a.Parts != b.Parts {
			return b.Parts - a.Parts
		}

		return strings.Compare(a.Tag, b.Tag)
	})

	return out
}

// Detail answers one part with its versions and its comments.
func (c *Catalog) Detail(id int) (PartDetail, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	part, found := c.parts[id]
	if !found {
		return PartDetail{}, fmt.Errorf("%w: %d", ErrPartNotFound, id)
	}

	detail := PartDetail{
		Part:     part.row,
		Tags:     part.row.Tags,
		Revision: part.version,
		Versions: make([]Version, 0, len(c.versions[id])),
		Comments: make([]Comment, 0, len(c.comments[id])),
		Dir:      PartDir(c.place.Root, id),
		Storage:  c.place.Storage,
	}

	if detail.Tags == nil {
		detail.Tags = []string{}
	}

	for _, version := range c.versions[id] {
		detail.Versions = append(detail.Versions, version.row)
	}

	for _, comment := range c.comments[id] {
		detail.Comments = append(detail.Comments, comment.row)
	}

	if count := len(detail.Versions); count > 0 {
		detail.Picture = pictureOf(part.row.Cover, &detail.Versions[count-1])
	}

	return detail, nil
}

// AddPart writes one new part, with no version yet.
func (c *Catalog) AddPart(ctx context.Context, draft PartDraft, by string) (Part, error) {
	c.writing.Lock()
	defer c.writing.Unlock()

	now := c.now().UTC()
	part := Part{V: Format, Created: now, CreatedBy: by, Changed: now, ChangedBy: by}

	if err := c.applyPart(&part, draft, true); err != nil {
		return Part{}, err
	}

	if part.Cover != "" {
		return Part{}, invalid("a new part has no file yet, so it has no cover")
	}

	db, err := c.store()
	if err != nil {
		return Part{}, err
	}

	id, err := db.addPart(ctx, part)
	if err != nil {
		return Part{}, err
	}

	part.ID = id

	c.mu.Lock()
	defer c.mu.Unlock()

	c.parts[id] = held[Part]{row: part, version: 1}
	c.index(id)

	return part, nil
}

// ChangePart writes the fields a draft carries onto a part.
func (c *Catalog) ChangePart(ctx context.Context, id int, draft PartDraft, by string) (Part, error) {
	c.writing.Lock()
	defer c.writing.Unlock()

	c.mu.RLock()
	row, found := c.parts[id]
	c.mu.RUnlock()

	if !found {
		return Part{}, fmt.Errorf("%w: %d", ErrPartNotFound, id)
	}

	if draft.Revision != nil && *draft.Revision != row.version {
		return Part{}, fmt.Errorf("%w: part %d is at revision %d and the change was made against %d",
			ErrConflict, id, row.version, *draft.Revision)
	}

	part := row.row
	if err := c.applyPart(&part, draft, false); err != nil {
		return Part{}, err
	}

	part.V = Format
	part.Changed = c.now().UTC()
	part.ChangedBy = by

	db, err := c.store()
	if err != nil {
		return Part{}, err
	}

	revision, err := db.savePart(ctx, part, row.version)
	if err != nil {
		return Part{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.parts[id] = held[Part]{row: part, version: revision}
	c.index(id)

	return part, nil
}

// Removed is what deleting a part or a version leaves for the page to clean
// up in the storage: the module records files and never deletes one, because
// deleting a file takes write_storage, which the person holds and the module
// does not.
type Removed struct {
	// Storage is the storage new files go into, and the storage of Dir.
	Storage string `json:"storage"`
	// Dir is the part's whole folder in Storage, set when the part itself
	// went and every folder the part used is that one. A part whose files
	// sit in several folders, after the settings moved, leaves it empty, and
	// Dirs names each folder.
	Dir string `json:"dir,omitempty"`
	// Dirs is every folder the part's files were kept in, each in its own
	// storage, set when the part itself went. It holds the folder new files
	// go into, where an upload no version recorded yet may still be.
	Dirs []Dir `json:"dirs"`
	// Files are the files no version of the part names any more. When the
	// part itself went, these are every file it recorded.
	Files []File `json:"files"`
}

// Dir is one folder in one storage.
type Dir struct {
	Storage string `json:"storage"`
	Dir     string `json:"dir"`
}

// DeletePart removes a part with its versions and its comments, in one
// transaction.
func (c *Catalog) DeletePart(ctx context.Context, id int) (Removed, error) {
	c.writing.Lock()
	defer c.writing.Unlock()

	c.mu.RLock()
	_, found := c.parts[id]
	versions := slices.Clone(c.versions[id])
	place := c.place
	c.mu.RUnlock()

	if !found {
		return Removed{}, fmt.Errorf("%w: %d", ErrPartNotFound, id)
	}

	db, err := c.store()
	if err != nil {
		return Removed{}, err
	}

	if err = db.deletePart(ctx, id); err != nil {
		return Removed{}, err
	}

	c.mu.Lock()
	delete(c.parts, id)
	delete(c.versions, id)
	delete(c.comments, id)
	delete(c.search, id)
	c.mu.Unlock()

	removed := Removed{Storage: place.Storage, Files: []File{}}

	for _, version := range versions {
		removed.Files = append(removed.Files, version.row.Files...)
	}

	removed.Dirs = dirsOf(id, place, removed.Files)
	if len(removed.Dirs) == 1 {
		removed.Dir = removed.Dirs[0].Dir
	}

	return removed, nil
}

// dirsOf answers every folder a part's files were kept in, read from the
// paths they were recorded under, with the folder new files go into first.
// A path not of the shape <dir>/<upload>/<file> under a folder named for the
// part is left out, so a delete never takes a folder that is not the part's.
func dirsOf(part int, place Place, files []File) []Dir {
	out := []Dir{{Storage: place.Storage, Dir: PartDir(place.Root, part)}}
	suffix := "/" + strconv.Itoa(part)

	for _, file := range files {
		dir := path.Dir(path.Dir(file.Path))
		if !strings.HasSuffix(dir, suffix) || dir == suffix || strings.HasPrefix(dir, ".") {
			continue
		}

		held := Dir{Storage: file.Storage, Dir: dir}
		if !slices.Contains(out, held) {
			out = append(out, held)
		}
	}

	return out
}

// applyPart writes a draft onto a part and holds the result to the rules.
//
// The caller holds the write lock, so a folder it finds is still there when
// the part is written into it.
func (c *Catalog) applyPart(part *Part, draft PartDraft, creating bool) error {
	if err := applyWords(part, draft, creating); err != nil {
		return err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if draft.Folder != nil {
		if *draft.Folder != Root {
			if _, found := c.folders[*draft.Folder]; !found {
				return fmt.Errorf("%w: %d, the folder named", ErrFolderNotFound, *draft.Folder)
			}
		}

		part.Folder = *draft.Folder
	}

	if draft.Cover != nil {
		cover := strings.TrimSpace(*draft.Cover)
		if cover != "" && !c.holdsPath(part.ID, cover) {
			return invalid("the cover %q is not a file of part %d", cover, part.ID)
		}

		part.Cover = cover
	}

	return nil
}

// applyWords writes what a person typed onto a part: the name, the summary,
// the description and the tags.
func applyWords(part *Part, draft PartDraft, creating bool) error {
	if draft.Name != nil || creating {
		written := ""
		if draft.Name != nil {
			written = *draft.Name
		}

		name, err := line("name", written, MaxNameBytes, true)
		if err != nil {
			return err
		}

		part.Name = name
	}

	if draft.Summary != nil {
		summary, err := line("summary", *draft.Summary, MaxSummaryBytes, false)
		if err != nil {
			return err
		}

		part.Summary = summary
	}

	if draft.Description != nil {
		description, err := block("description", *draft.Description, MaxDescriptionBytes, false)
		if err != nil {
			return err
		}

		part.Description = description
	}

	if draft.Tags != nil {
		held, err := tags(*draft.Tags)
		if err != nil {
			return err
		}

		part.Tags = held
	}

	return nil
}

// holdsPath reports whether any version of a part names a path, in any
// storage. A cover is named by its path alone. The caller holds the read
// lock.
func (c *Catalog) holdsPath(part int, written string) bool {
	for _, version := range c.versions[part] {
		for _, file := range version.row.Files {
			if file.Path == written {
				return true
			}
		}
	}

	return false
}

// holdsFile reports whether any version of a part names one file: the same
// path in the same storage. The same path in another storage is another
// file. The caller holds the read lock.
func (c *Catalog) holdsFile(part int, held File) bool {
	for _, version := range c.versions[part] {
		for _, file := range version.row.Files {
			if file.Storage == held.Storage && file.Path == held.Path {
				return true
			}
		}
	}

	return false
}
