package moonraker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	pathpkg "path"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

const (
	gcodesStorage     = "gcodes"
	thumbsDir         = ".thumbs"
	uploadChunkBytes  = 32 * 1024
	progressStepBytes = 128 * 1024
	maxThumbBytes     = 2 * 1024 * 1024
	maxErrorBodyBytes = 4096
)

var (
	_ connectorapi.Filesystem  = (*Session)(nil)
	_ connectorapi.FileManager = (*Session)(nil)
	_ connectorapi.Transfers   = (*Session)(nil)
)

// readInitialStorages inspects Moonraker's file roots during handshake.
func (s *Session) readInitialStorages(ctx context.Context) (hasGcodes, writable bool, err error) {
	var roots []fileRoot
	if err = s.callSync(ctx, methodFilesRoots, nil, &roots); err != nil {
		if errors.Is(err, ErrRefused) {
			return false, false, nil
		}

		return false, false, err
	}

	var found *fileRoot

	for i := range roots {
		if roots[i].Name == gcodesStorage {
			found = &roots[i]

			break
		}
	}

	if found == nil {
		return false, false, nil
	}

	writable = strings.Contains(found.Permissions, "w")

	storage := connectorapi.Storage{
		Mountpoint: "/" + gcodesStorage,
		Name:       gcodesStorage,
		Type:       "LOCAL",
		ReadOnly:   !writable,
	}

	var dir directoryReply
	if dirErr := s.callSync(
		ctx,
		methodFilesGetDirectory,
		map[string]any{"path": gcodesStorage, "extended": false},
		&dir,
	); dirErr == nil && dir.DiskUsage != nil {
		storage.FreeSpace = dir.DiskUsage.Free
		storage.FreeSpaceKnown = true
	}

	s.storagesMu.Lock()
	s.storages = []connectorapi.Storage{storage}
	s.gcodesWritable = writable
	s.storagesMu.Unlock()

	return true, writable, nil
}

// Storages returns the Moonraker gcodes storage, refreshing free space when
// the session is open.
func (s *Session) Storages(ctx context.Context) ([]connectorapi.Storage, error) {
	s.storagesMu.Lock()
	cached := slices.Clone(s.storages)
	s.storagesMu.Unlock()

	if len(cached) == 0 || !s.caps.Has(VerbSendInfo) {
		return cached, nil
	}

	select {
	case <-s.done:
		return cached, nil
	default:
	}

	if result, refusal, err := s.call(
		ctx,
		methodFilesGetDirectory,
		map[string]any{"path": gcodesStorage, "extended": false},
	); err == nil && refusal == nil {
		var dir directoryReply
		if json.Unmarshal(result, &dir) == nil && dir.DiskUsage != nil {
			s.setGcodesFreeSpace(dir.DiskUsage)
			s.storagesMu.Lock()
			cached = slices.Clone(s.storages)
			s.storagesMu.Unlock()
		}
	}

	return cached, nil
}

func (s *Session) setGcodesFreeSpace(usage *diskUsage) {
	if usage == nil {
		return
	}

	s.storagesMu.Lock()
	defer s.storagesMu.Unlock()

	for i := range s.storages {
		if s.storages[i].Name == gcodesStorage {
			s.storages[i].FreeSpace = usage.Free
			s.storages[i].FreeSpaceKnown = true
		}
	}
}

// List returns the storage root, a gcodes directory listing, or a single file's
// metadata entry.
func (s *Session) List(ctx context.Context, rawPath string) (connectorapi.Listing, error) {
	if refusal := connectorapi.RefusePathEscape(rawPath); refusal != nil {
		return connectorapi.Listing{}, refusal
	}

	if !s.caps.Has(VerbSendFileInfo) {
		return connectorapi.Listing{}, connectorapi.CapabilityNotSupported.
			New("the printer does not have that capability").
			With(fault.StringParam("verb", VerbSendFileInfo.String()))
	}

	clean := strings.Trim(rawPath, "/")
	if clean == "" {
		return s.rootListing(), nil
	}

	if clean != gcodesStorage && !strings.HasPrefix(clean, gcodesStorage+"/") {
		return connectorapi.Listing{}, listingRefused(rawPath, "only gcodes storage is available")
	}

	result, refusal, err := s.call(
		ctx,
		methodFilesGetDirectory,
		map[string]any{"path": clean, "extended": true},
	)
	if err != nil {
		return connectorapi.Listing{}, err
	}

	if refusal == nil {
		var reply directoryReply
		if err = decodeResult(methodFilesGetDirectory, result, &reply); err != nil {
			return connectorapi.Listing{}, err
		}

		s.setGcodesFreeSpace(reply.DiskUsage)

		return directoryListing(clean, reply, !s.gcodesWritable), nil
	}

	if clean == gcodesStorage {
		return connectorapi.Listing{}, listingRefused(rawPath, refusal.Message)
	}

	rel, _ := relativeToGcodes(clean)

	metaResult, metaRefusal, metaErr := s.call(ctx, methodFilesMetadata, map[string]any{"filename": rel})
	if metaErr != nil {
		return connectorapi.Listing{}, metaErr
	}

	if metaRefusal != nil {
		return connectorapi.Listing{}, listingRefused(rawPath, metaRefusal.Message)
	}

	var meta fileMetadataReply
	if err = decodeResult(methodFilesMetadata, metaResult, &meta); err != nil {
		return connectorapi.Listing{}, err
	}

	preview := s.fetchThumbnail(ctx, rel, meta.Thumbnails)
	name := pathpkg.Base(clean)

	return connectorapi.Listing{
		Directory: connectorapi.File{
			Path:        clean,
			Name:        name,
			DisplayName: name,
			Folder:      false,
			Size:        meta.Size,
			Modified:    int64(meta.Modified),
			ReadOnly:    isReadOnly(meta.Permissions),
			Preview:     preview,
		},
	}, nil
}

func (s *Session) rootListing() connectorapi.Listing {
	s.storagesMu.Lock()
	storages := slices.Clone(s.storages)
	s.storagesMu.Unlock()

	entries := make([]connectorapi.File, 0, len(storages))
	for _, st := range storages {
		entries = append(entries, connectorapi.File{
			Path:        st.Name,
			Name:        st.Name,
			DisplayName: st.Name,
			Folder:      true,
			ReadOnly:    st.ReadOnly,
		})
	}

	return connectorapi.Listing{
		Directory: connectorapi.File{Folder: true},
		Entries:   entries,
	}
}

func directoryListing(clean string, reply directoryReply, defaultReadOnly bool) connectorapi.Listing {
	dirName := pathpkg.Base(clean)
	dirReadOnly := defaultReadOnly
	if reply.RootInfo != nil && reply.RootInfo.Permissions != "" {
		dirReadOnly = isReadOnly(reply.RootInfo.Permissions)
	}

	entries := make([]connectorapi.File, 0, len(reply.Dirs)+len(reply.Files))

	for _, dir := range reply.Dirs {
		if dir.Dirname == thumbsDir {
			continue
		}

		entries = append(entries, connectorapi.File{
			Path:        clean + "/" + dir.Dirname,
			Name:        dir.Dirname,
			DisplayName: dir.Dirname,
			Folder:      true,
			Modified:    int64(dir.Modified),
			ReadOnly:    isReadOnly(dir.Permissions),
		})
	}

	for _, file := range reply.Files {
		entries = append(entries, connectorapi.File{
			Path:        clean + "/" + file.Filename,
			Name:        file.Filename,
			DisplayName: file.Filename,
			Folder:      false,
			Size:        file.Size,
			Modified:    int64(file.Modified),
			ReadOnly:    isReadOnly(file.Permissions),
		})
	}

	return connectorapi.Listing{
		Directory: connectorapi.File{
			Path:        clean,
			Name:        dirName,
			DisplayName: dirName,
			Folder:      true,
			ReadOnly:    dirReadOnly,
		},
		Entries: entries,
	}
}

// CreateFolder creates a subdirectory inside gcodes storage.
func (s *Session) CreateFolder(ctx context.Context, rawPath string) error {
	clean, err := s.validateManagePath(VerbCreateFolder, rawPath)
	if err != nil {
		return err
	}

	_, refusal, callErr := s.call(ctx, methodFilesPostDirectory, map[string]any{"path": clean})
	if callErr != nil {
		return callErr
	}

	if refusal != nil {
		return fileChangeRefused(rawPath, refusal.Message)
	}

	return nil
}

// DeleteFile deletes a file inside gcodes storage.
func (s *Session) DeleteFile(ctx context.Context, rawPath string) error {
	clean, err := s.validateManagePath(VerbDeleteFile, rawPath)
	if err != nil {
		return err
	}

	_, refusal, callErr := s.call(ctx, methodFilesDeleteFile, map[string]any{"path": clean})
	if callErr != nil {
		return callErr
	}

	if refusal != nil {
		return fileChangeRefused(rawPath, refusal.Message)
	}

	return nil
}

// DeleteFolder deletes a subdirectory inside gcodes storage.
func (s *Session) DeleteFolder(ctx context.Context, rawPath string) error {
	clean, err := s.validateManagePath(VerbDeleteFolder, rawPath)
	if err != nil {
		return err
	}

	// Pass force: true so non-empty folders delete recursively, matching
	// Prusa Connect's folder deletion expectations.
	_, refusal, callErr := s.call(
		ctx,
		methodFilesDeleteDirectory,
		map[string]any{"path": clean, "force": true},
	)
	if callErr != nil {
		return callErr
	}

	if refusal != nil {
		return fileChangeRefused(rawPath, refusal.Message)
	}

	return nil
}

func (s *Session) validateManagePath(verb connectorapi.Capability, rawPath string) (string, error) {
	if refusal := connectorapi.RefusePathEscape(rawPath); refusal != nil {
		return "", refusal
	}

	if !s.caps.Has(verb) {
		return "", connectorapi.CapabilityNotSupported.
			New("the printer does not have that capability").
			With(fault.StringParam("verb", verb.String()))
	}

	clean := strings.Trim(rawPath, "/")
	if _, ok := relativeToGcodes(clean); !ok {
		return "", connectorapi.PathNotAllowed.
			New("path must name an entry inside gcodes storage").
			With(fault.StringParam("path", rawPath))
	}

	return clean, nil
}

// Send streams a print file to Moonraker over multipart HTTP POST and confirms
// the landed file size via server.files.metadata.
func (s *Session) Send(ctx context.Context, up connectorapi.Upload) error {
	if refusal := connectorapi.RefusePathEscape(up.Path); refusal != nil {
		return refusal
	}

	if !s.caps.Has(VerbFileUpload) {
		return connectorapi.CapabilityNotSupported.
			New("the printer does not have that capability").
			With(fault.StringParam("verb", VerbFileUpload.String()))
	}

	clean := strings.Trim(up.Path, "/")
	rel, ok := relativeToGcodes(clean)
	if !ok {
		return connectorapi.PathNotAllowed.
			New("upload path must be inside gcodes storage").
			With(fault.StringParam("path", up.Path))
	}

	if !s.transferMu.TryLock() {
		return connectorapi.TransferBusy.
			New("the printer is already running a transfer").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferBusy)
	}
	defer s.transferMu.Unlock()

	if up.Content == nil {
		return connectorapi.TransferFailed.
			New("upload has no content").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferFailed)
	}

	total := up.Content.Size()

	reader, err := up.Content.Open(ctx, 0)
	if err != nil {
		return fmt.Errorf("moonraker: open upload content: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if up.Progress != nil {
		up.Progress(connectorapi.Progress{Total: total, Sent: 0})
	}

	subDir, fileName := pathpkg.Split(rel)
	subDir = strings.TrimSuffix(subDir, "/")

	pipeReader, pipeWriter := io.Pipe()
	mpWriter := multipart.NewWriter(pipeWriter)
	writeErrCh := make(chan error, 1)

	go func() {
		writeErrCh <- streamMultipart(mpWriter, pipeWriter, reader, fileName, subDir, total, up.Progress)
	}()

	uploadURL := strings.TrimRight(s.httpBaseURL, "/") + "/server/files/upload"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		<-writeErrCh

		return fmt.Errorf("moonraker: build upload request: %w", err)
	}

	req.Header.Set("Content-Type", mpWriter.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		_ = pipeReader.Close()
		<-writeErrCh

		return connectorapi.TransferFailed.
			New("upload to Moonraker did not finish").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferFailed)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	_ = resp.Body.Close()

	if writeErr := <-writeErrCh; writeErr != nil {
		return connectorapi.TransferFailed.
			New("failed while streaming upload to Moonraker").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferFailed)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}

		return connectorapi.TransferRefused.
			New("Moonraker refused the upload").
			With(
				fault.StringParam("path", up.Path),
				fault.StringParam("machine_reason", msg),
			).
			Wrap(connectorapi.ErrTransferRefused)
	}

	metaResult, metaRefusal, metaErr := s.call(ctx, methodFilesMetadata, map[string]any{"filename": rel})
	if metaErr != nil || metaRefusal != nil {
		return connectorapi.TransferNotOnStorage.
			New("Moonraker did not confirm the uploaded file on storage").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferFailed)
	}

	var meta fileMetadataReply
	if err = decodeResult(methodFilesMetadata, metaResult, &meta); err != nil || meta.Size != total {
		return connectorapi.TransferNotOnStorage.
			New("uploaded file size did not match on Moonraker storage").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferFailed)
	}

	if up.Progress != nil {
		up.Progress(connectorapi.Progress{
			Total:    total,
			Sent:     total,
			Received: meta.Size,
			Known:    true,
		})
	}

	return nil
}

func streamMultipart(
	mpWriter *multipart.Writer,
	pipeWriter *io.PipeWriter,
	src io.Reader,
	fileName, subDir string,
	total int64,
	progress func(connectorapi.Progress),
) error {
	closeErr := func(err error) error {
		_ = mpWriter.Close()
		_ = pipeWriter.CloseWithError(err)

		return err
	}

	if err := mpWriter.WriteField("root", gcodesStorage); err != nil {
		return closeErr(err)
	}

	if subDir != "" {
		if err := mpWriter.WriteField("path", subDir); err != nil {
			return closeErr(err)
		}
	}

	part, err := mpWriter.CreateFormFile("file", fileName)
	if err != nil {
		return closeErr(err)
	}

	buf := make([]byte, uploadChunkBytes)

	var sent, nextReport int64
	nextReport = progressStepBytes

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := part.Write(buf[:n]); writeErr != nil {
				return closeErr(writeErr)
			}

			sent += int64(n)
			if progress != nil && sent >= nextReport {
				progress(connectorapi.Progress{Total: total, Sent: min(sent, total)})
				nextReport = sent + progressStepBytes
			}
		}

		if readErr == io.EOF {
			break
		}

		if readErr != nil {
			return closeErr(readErr)
		}
	}

	if closePipeErr := mpWriter.Close(); closePipeErr != nil {
		_ = pipeWriter.CloseWithError(closePipeErr)

		return closePipeErr
	}

	return pipeWriter.Close()
}

func (s *Session) fetchThumbnail(ctx context.Context, gcodeRelPath string, thumbs []moonrakerThumbnail) []byte {
	if len(thumbs) == 0 || s.httpBaseURL == "" {
		return nil
	}

	best := thumbs[0]
	for _, candidate := range thumbs[1:] {
		if candidate.Width*candidate.Height > best.Width*best.Height {
			best = candidate
		}
	}

	if best.RelativePath == "" {
		return nil
	}

	thumbRel := strings.TrimLeft(best.RelativePath, "/")
	if parent := pathpkg.Dir(gcodeRelPath); parent != "." && parent != "" {
		thumbRel = pathpkg.Join(parent, thumbRel)
	} else {
		thumbRel = pathpkg.Clean(thumbRel)
	}

	if thumbRel == "." || connectorapi.PathEscapes(thumbRel) {
		return nil
	}

	segments := strings.Split(thumbRel, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}

	thumbURL := strings.TrimRight(s.httpBaseURL, "/") +
		"/server/files/" + gcodesStorage + "/" + strings.Join(segments, "/")

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return nil
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxThumbBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxThumbBytes {
		return nil
	}

	return data
}

func relativeToGcodes(clean string) (string, bool) {
	trimmed := strings.Trim(clean, "/")
	prefix := gcodesStorage + "/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}

	rel := strings.TrimPrefix(trimmed, prefix)
	if rel == "" {
		return "", false
	}

	return rel, true
}

func isReadOnly(permissions string) bool {
	return permissions != "" && !strings.Contains(permissions, "w")
}

func listingRefused(rawPath, reason string) error {
	return connectorapi.ListingRefused.
		New(reason).
		With(
			fault.StringParam("path", rawPath),
			fault.StringParam("machine_reason", reason),
			fault.StringParam("reason", reason),
		).
		Wrap(connectorapi.ErrListingRefused)
}

func fileChangeRefused(rawPath, reason string) error {
	return connectorapi.FileChangeRefused.
		New(reason).
		With(
			fault.StringParam("path", rawPath),
			fault.StringParam("machine_reason", reason),
			fault.StringParam("reason", reason),
		).
		Wrap(connectorapi.ErrFileChangeRefused)
}
