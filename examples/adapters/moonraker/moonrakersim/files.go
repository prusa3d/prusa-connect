package moonrakersim

import (
	"encoding/json"
	"io"
	"net/http"
	pathpkg "path"
	"strings"
)

func (h *Host) gcodesPermissions() string {
	if h.gcodesReadOnly {
		return "r"
	}

	return "rw"
}

func (h *Host) filesRoots(id uint64) map[string]any {
	if h.withoutGcodes {
		return resultFrame(id, []any{})
	}

	return resultFrame(id, []map[string]any{{
		"name":        "gcodes",
		"path":        "/home/printer/printer_data/gcodes",
		"permissions": h.gcodesPermissions(),
	}})
}

func (h *Host) filesGetDirectory(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &req)

	clean := strings.Trim(req.Path, "/")
	if clean != "gcodes" && !strings.HasPrefix(clean, "gcodes/") {
		return errorFrame(id, requestRefused, "Directory not found")
	}

	relDir := strings.TrimPrefix(clean, "gcodes")
	relDir = strings.Trim(relDir, "/")

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.dirs[relDir] {
		return errorFrame(id, requestRefused, "Directory does not exist")
	}

	perms := h.gcodesPermissions()
	var dirs []map[string]any
	var files []map[string]any

	for dir := range h.dirs {
		if dir == "" || pathpkg.Dir(dir) != orElse(relDir, ".") {
			continue
		}

		dirs = append(dirs, map[string]any{
			"dirname":     pathpkg.Base(dir),
			"modified":    simModifiedTime,
			"size":        simDirSize,
			"permissions": perms,
		})
	}

	for name, data := range h.files {
		if pathpkg.Dir(name) != orElse(relDir, ".") {
			continue
		}

		files = append(files, map[string]any{
			"filename":    pathpkg.Base(name),
			"modified":    simModifiedTime,
			"size":        len(data),
			"permissions": perms,
		})
	}

	return resultFrame(id, map[string]any{
		"dirs":  dirs,
		"files": files,
		"disk_usage": map[string]any{
			"total": simDiskTotal,
			"used":  simDiskUsed,
			"free":  simDiskFree,
		},
		"root_info": map[string]any{
			"name":        "gcodes",
			"permissions": perms,
		},
	})
}

func (h *Host) filesMetadata(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Filename string `json:"filename"`
	}
	_ = json.Unmarshal(params, &req)

	clean := strings.Trim(req.Filename, "/")

	h.mu.Lock()
	data, ok := h.files[clean]

	var thumbs []map[string]any
	if ok {
		parent := pathpkg.Dir(clean)
		stem := strings.TrimSuffix(pathpkg.Base(clean), pathpkg.Ext(clean))
		relThumb := ".thumbs/" + stem + "-300x300.png"
		fullThumb := relThumb
		if parent != "." {
			fullThumb = parent + "/" + relThumb
		}

		if thumbBytes, hasThumb := h.files[fullThumb]; hasThumb {
			thumbs = append(thumbs, map[string]any{
				"width":         simThumbSize,
				"height":        simThumbSize,
				"size":          len(thumbBytes),
				"relative_path": relThumb,
			})
		}
	}
	h.mu.Unlock()

	if !ok {
		return errorFrame(id, requestRefused, "Metadata not available for requested file")
	}

	reply := map[string]any{
		"size":        len(data),
		"modified":    simModifiedTime,
		"permissions": h.gcodesPermissions(),
	}
	if len(thumbs) > 0 {
		reply["thumbnails"] = thumbs
	}

	return resultFrame(id, reply)
}

func (h *Host) serveGcodesFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	rel := strings.TrimPrefix(r.URL.Path, "/server/files/gcodes/")
	data, ok := h.FileBytes(rel)
	if !ok {
		http.NotFound(w, r)

		return
	}

	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func (h *Host) filesPostDirectory(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &req)

	rel := strings.Trim(strings.TrimPrefix(strings.Trim(req.Path, "/"), "gcodes/"), "/")
	if rel == "" {
		return errorFrame(id, requestRefused, "Invalid directory path")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.dirs[rel] {
		return errorFrame(id, requestRefused, "Directory already exists")
	}

	h.dirs[rel] = true

	return resultFrame(id, map[string]any{"item": map[string]any{"path": req.Path}})
}

func (h *Host) filesDeleteFile(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &req)

	rel := strings.Trim(strings.TrimPrefix(strings.Trim(req.Path, "/"), "gcodes/"), "/")

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.files[rel]; !ok {
		return errorFrame(id, requestRefused, "File not found")
	}

	delete(h.files, rel)

	return resultFrame(id, map[string]any{"item": map[string]any{"path": req.Path}})
}

func (h *Host) filesDeleteDirectory(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Path  string `json:"path"`
		Force bool   `json:"force"`
	}
	_ = json.Unmarshal(params, &req)

	rel := strings.Trim(strings.TrimPrefix(strings.Trim(req.Path, "/"), "gcodes/"), "/")
	if rel == "" {
		return errorFrame(id, requestRefused, "Cannot delete root")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.dirs[rel] {
		return errorFrame(id, requestRefused, "Directory not found")
	}

	prefix := rel + "/"
	for name := range h.files {
		if strings.HasPrefix(name, prefix) {
			if !req.Force {
				return errorFrame(id, requestRefused, "Directory is not empty")
			}

			delete(h.files, name)
		}
	}

	for dir := range h.dirs {
		if dir == rel || strings.HasPrefix(dir, prefix) {
			delete(h.dirs, dir)
		}
	}

	return resultFrame(id, map[string]any{"item": map[string]any{"path": req.Path}})
}

func (h *Host) startPrint(id uint64, params json.RawMessage) map[string]any {
	var req struct {
		Filename string `json:"filename"`
	}
	_ = json.Unmarshal(params, &req)

	clean := strings.Trim(req.Filename, "/")

	h.mu.Lock()
	_, ok := h.files[clean]
	if ok {
		h.startedFiles = append(h.startedFiles, clean)
	}
	h.mu.Unlock()

	if !ok {
		return errorFrame(id, requestRefused, "File not found")
	}

	return resultFrame(id, "ok")
}

func (h *Host) serveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	if h.gcodesReadOnly {
		http.Error(w, "storage is read only", http.StatusForbidden)

		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	var (
		subDir   string
		fileName string
		data     []byte
		hasFile  bool
	)

	for {
		part, partErr := mr.NextPart()
		if partErr == io.EOF {
			break
		}

		if partErr != nil {
			http.Error(w, partErr.Error(), http.StatusBadRequest)

			return
		}

		partData, readErr := io.ReadAll(io.LimitReader(part, maxUploadBytes+1))
		_ = part.Close()
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)

			return
		}

		if int64(len(partData)) > maxUploadBytes {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)

			return
		}

		switch part.FormName() {
		case "path":
			subDir = strings.Trim(string(partData), "/")
		case "file":
			fileName = part.FileName()
			data = partData
			hasFile = true
		}
	}

	if !hasFile || fileName == "" {
		http.Error(w, "missing file part", http.StatusBadRequest)

		return
	}

	if h.corruptUploadSize && len(data) > 0 {
		data = data[:len(data)-1]
	}

	rel := fileName
	if subDir != "" {
		rel = subDir + "/" + fileName
	}

	h.PutFile(rel, data)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"item": map[string]string{"path": rel, "root": "gcodes"},
	})
}
