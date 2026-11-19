package hostfake

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/prusa3d/prusa-connect/sdk/storageapi"
)

// Files is the host's storage door in memory. It answers storage.stat for the
// files a test put there, and storageapi.ErrNoFile for every other path, the
// way the host answers a file it does not hold.
//
// The zero value holds no file, ready to use.
type Files struct {
	mu   sync.Mutex
	rows map[string]storageapi.FileInfo

	// Asked records the principal of every stat, so a test reads that the
	// module asked as the person it was serving and not as itself.
	Asked []string

	// Fail is called before every stat. A non-nil answer is what the call
	// returns, which is how a refusal of the host is reached.
	Fail func(params storageapi.StatParams) error
}

// Put places one file in a storage. A print file carries a picture, as the
// host reports one it found; PutBare places one that carries none.
func (f *Files) Put(storage, file string, size int64, sha256 string) {
	f.put(storage, file, size, sha256, true)
}

// PutBare places a file the way the host describes a print file with neither
// details nor a picture: Print is left out.
func (f *Files) PutBare(storage, file string, size int64, sha256 string) {
	f.put(storage, file, size, sha256, false)
}

// thumbnailSide is any non-zero size: a test only asks whether a picture exists.
const thumbnailSide = 16

func (f *Files) put(storage, file string, size int64, sha256 string, picture bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.rows == nil {
		f.rows = map[string]storageapi.FileInfo{}
	}

	row := storageapi.FileInfo{Name: file, Path: file, Size: size, SHA256: sha256}
	if picture && slices.Contains([]string{".bgcode", ".gcode", ".3mf"}, strings.ToLower(path.Ext(file))) {
		row.Print = &storageapi.PrintInfo{ThumbnailWidth: thumbnailSide, ThumbnailHeight: thumbnailSide}
	}

	f.rows[storage+"\x00"+file] = row
}

// Stat answers one file the test put in a storage.
func (f *Files) Stat(_ context.Context, params storageapi.StatParams) (storageapi.FileInfo, error) {
	f.mu.Lock()
	f.Asked = append(f.Asked, params.Principal)
	row, found := f.rows[params.Storage+"\x00"+params.Path]
	f.mu.Unlock()

	if f.Fail != nil {
		if err := f.Fail(params); err != nil {
			return storageapi.FileInfo{}, err
		}
	}

	if !found {
		return storageapi.FileInfo{}, fmt.Errorf("%w: %s", storageapi.ErrNoFile, params.Path)
	}

	return row, nil
}
