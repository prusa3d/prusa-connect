package httpapi

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// The core's own app is served like a module's bundle of browser files. It is
// compiled into the full build and served at the origin root through the /*
// mount, with the cache and content-type rules ModuleUIHandler applies to a
// module's bundle.
const (
	// appEntry is the document every address that names no file answers with,
	// so a client route survives a reload.
	appEntry = "index.html"
	// appAssetDir is the subtree the build writes hashed file names into. A
	// name that is not there is a stale reference, never a route, so it
	// answers 404 and not the entry document.
	appAssetDir = "assets/"
	// appEntryCacheControl keeps the entry document revalidated. Its name
	// never changes and its body names the hashed files, so a browser with a
	// cached copy would keep loading the previous release.
	appEntryCacheControl = "no-cache"
	// appAssetCacheControl belongs to a name that carries its own content
	// hash. A new build writes a new name, so the old one can be kept for a
	// year without ever being wrong.
	//nolint:gosec // G101 reads the year in seconds as an entropy score. It is a cache directive.
	appAssetCacheControl = "public, max-age=31536000, immutable"
	// appFileCacheControl is for every other file: an hour, with an entity
	// tag that changes with the build. The app's own files are the same
	// for everyone, so any cache may keep them.
	appFileCacheControl = "public, max-age=3600"
)

// appMountReason is why the origin root is not an operation. A browser
// navigation is not a call a generated client makes, and the addresses under it
// are the app's own router's, not this server's.
const appMountReason = "the core's own app, compiled into the full build and served at the origin root. " +
	"A browser navigation is not an operation a client calls, and the addresses below it belong to the " +
	"app's router"

// AppMount is the /* mount that serves the core's app, and false for a build
// that carries none.
//
// A build without the app returns false and not a mount that answers 404.
// With no mount the root falls through to a module's root mount and then to
// this server's own refusal.
func AppMount(opts AppOptions) (Mount, bool) {
	files, ok := appBundle()
	if !ok {
		return Mount{}, false
	}

	return Mount{
		Pattern: rootCatchAll,
		Handler: AppHandlerWith(files, opts),
		Reason:  appMountReason,
	}, true
}

// AppHandler serves one bundle of browser files at the origin root.
//
// A name must pass fs.ValidPath, which refuses an empty element, a dot element
// and a leading slash. It must also hold no backslash, which a valid URL path
// never carries. A name under the asset directory that names no file answers
// 404. Every other name that names no file answers the entry document, so
// reloading a client route works.
//
// The method is not checked here. The mount is wrapped in navigations before
// it reaches the router. That wrapper refuses a request that is not a
// navigation, and this handler is reached only through it.
func AppHandler(files fs.FS) http.Handler {
	return AppHandlerWith(files, AppOptions{})
}

// AppHandlerWith is AppHandler with the options of a deployment. A declared
// proxy may then serve the app under a path prefix (appprefix.go).
func AppHandlerWith(files fs.FS, opts AppOptions) http.Handler {
	digest := sync.OnceValue(func() string { return appDigest(files) })
	etags := sync.OnceValue(func() appETags { return appETagsOf(digest()) })
	slots := make(chan struct{}, appDecompressSlots)

	var entry func() ([]byte, error)
	if found, err := appFileStored(files, appEntry); err == nil && found.name != "" {
		entry = appEntryDocument(files, found)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A trailing slash is how a browser writes a route's directory form,
		// and an fs name never carries one.
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if name == "" {
			name = appEntry
		}

		if !fs.ValidPath(name) || strings.Contains(name, `\`) {
			notFound(w, r)

			return
		}

		// Whether the address is an asset reference is read from the address
		// the client wrote, not from the name derived from it. /assets/ is a
		// reference to a file the build never wrote, and the trimmed name
		// "assets" does not show that.
		asset := strings.HasPrefix(r.URL.Path, "/"+appAssetDir)

		served, err := openAppFile(files, name, asset)
		if err != nil {
			writeProblem(w, r, httpfault.Unexpected("open a file of the core's app").Wrap(err))

			return
		}

		if served.name == "" {
			notFound(w, r)

			return
		}

		if served.name == appEntry && entry != nil && servedUnderPrefix(w, r, opts, entry, digest) {
			return
		}

		writeAppFile(w, r, files, served, etags(), slots)
	})
}

// Decompression for a client without gzip holds about 100 KB for as long as
// the client takes to read, which a slow client stretches to the write
// timeout. Two at a time keeps that small against a printer's memory. Two
// slow clients can hold both places for most of a minute, so Retry-After asks
// a refused client to wait long enough for one to be likely free.
const (
	appDecompressSlots      = 2
	appDecompressWait       = 2 * time.Second
	appDecompressRetryAfter = "30"
)

// appCompressedSuffix names the gzip copy the build stores in place of a text
// file. The build removes the plain file, so the binary carries each text file
// once, and the copy goes to the browser as it is stored.
const appCompressedSuffix = ".gz"

// appFile is one file of the bundle as it is stored. The name is the one the
// browser asked for, and it decides the type and the cache rule. The file is
// stored as it is, as a gzip copy, or both.
type appFile struct {
	name       string
	plain      bool
	compressed bool
}

// appETags are the entity tags of the two forms a file can be sent in. They
// differ because the two forms are different bytes, and one tag over both
// would let a cache give gzip bytes to a client that cannot read them.
type appETags struct {
	plain      string
	compressed string
}

func appETagsOf(digest string) appETags {
	if digest == "" {
		return appETags{}
	}

	return appETags{plain: etagOf(digest), compressed: etagOf(digest + "-gzip")}
}

// openAppFile decides which file answers the request: the name itself when it
// is a file, the entry document when it is not and the address is a route, and
// no file when nothing may answer.
func openAppFile(files fs.FS, name string, asset bool) (appFile, error) {
	switch found, err := appFileStored(files, name); {
	case err != nil:
		return appFile{}, err
	case found.name != "":
		return found, nil
	case asset:
		// A hashed name the build never wrote. Answering a document here
		// would be a 200 the browser cannot use, and a module script that
		// receives HTML fails with a syntax error nothing explains.
		return appFile{}, nil
	}

	return appFileStored(files, appEntry)
}

// appFileStored reports how one name is stored in the bundle. A name ending in
// .gz is never a file of the app: it is the stored copy of another name, and
// it is sent only under that name.
func appFileStored(files fs.FS, name string) (appFile, error) {
	if strings.HasSuffix(name, appCompressedSuffix) {
		return appFile{}, nil
	}

	plain, err := appFileExists(files, name)
	if err != nil {
		return appFile{}, err
	}

	compressed, err := appFileExists(files, name+appCompressedSuffix)
	if err != nil {
		return appFile{}, err
	}

	if !plain && !compressed {
		return appFile{}, nil
	}

	return appFile{name: name, plain: plain, compressed: compressed}, nil
}

// appFileExists reports whether one name is a regular file of the bundle. A
// directory is not one: nothing here lists a directory.
func appFileExists(files fs.FS, name string) (bool, error) {
	info, err := fs.Stat(files, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			return false, nil
		}

		return false, err
	}

	return !info.IsDir(), nil
}

// writeAppFile sends one file with the headers for its place in the bundle.
//
// A client that accepts gzip gets the stored copy with Content-Encoding. A
// client that does not gets the plain file, which is decompressed while it is
// sent when the build stored only the copy. Every browser accepts gzip, so
// that path costs processor time only for a rare tool such as curl.
//
// A Range header is ignored, and the whole file is sent with 200. RFC 9110
// allows that, and the standard library does the same for a response that
// has a Content-Encoding.
func writeAppFile(
	w http.ResponseWriter, r *http.Request, files fs.FS, file appFile, etags appETags, slots chan struct{},
) {
	sendCompressed := file.compressed && acceptsGzip(r)
	decompress := !sendCompressed && !file.plain

	stored := file.name
	if sendCompressed || decompress {
		stored += appCompressedSuffix
	}

	body, err := files.Open(stored)
	if err != nil {
		writeProblem(w, r, httpfault.Unexpected("open a file of the core's app").Wrap(err))

		return
	}

	defer func() { _ = body.Close() }()

	etag := etags.plain
	if sendCompressed {
		etag = etags.compressed
	}

	w.Header().Set("Content-Type", contentTypeOf(file.name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", appCacheControl(file.name))

	// On the 304 and on the plain answer as well: a cache that stored a
	// response without it would give the gzip bytes to the next client that
	// cannot read them.
	if file.compressed {
		w.Header().Add("Vary", "Accept-Encoding")
	}

	if etag != "" {
		w.Header().Set("ETag", etag)
	}

	if notModified(r, etag) {
		w.WriteHeader(http.StatusNotModified)

		return
	}

	if decompress {
		writeDecompressed(w, r, body, slots)

		return
	}

	if sendCompressed {
		w.Header().Set("Content-Encoding", "gzip")
	}

	if info, statErr := body.Stat(); statErr == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}

	w.WriteHeader(http.StatusOK)

	if headRequest(r) {
		return
	}

	_, _ = io.Copy(w, body)
}

// headRequest reports whether the client asked for the headers alone. The
// server answers HEAD through the GET handler, so the original method is read
// from the request context. Sending no body spares the decompression.
func headRequest(r *http.Request) bool {
	return r.Method == http.MethodHead || headOf(r.Context())
}

// writeDecompressed sends a stored gzip copy as the plain file. It streams, so
// a request holds the decompressor's window and never the whole file. The
// length is not known before the end, so the answer carries no Content-Length.
//
// A slow client holds its decompressor until the write timeout, so only
// appDecompressSlots requests decompress at once. A request that finds no free
// slot within appDecompressWait answers 503.
func writeDecompressed(w http.ResponseWriter, r *http.Request, stored io.Reader, slots chan struct{}) {
	if headRequest(r) {
		w.WriteHeader(http.StatusOK)

		return
	}

	if !takeSlot(r, slots) {
		dropCacheHeaders(w)
		w.Header().Set("Retry-After", appDecompressRetryAfter)
		writeProblem(w, r, httpfault.AppFileBusy(appDecompressSlots))

		return
	}

	defer func() { <-slots }()

	reader, err := gzip.NewReader(stored)
	if err != nil {
		dropCacheHeaders(w)
		writeProblem(w, r, httpfault.Unexpected("decompress a file of the core's app").Wrap(err))

		return
	}

	defer func() { _ = reader.Close() }()

	w.WriteHeader(http.StatusOK)

	// The status is already sent, so a problem document cannot replace it.
	// Aborting the connection tells the client the file is incomplete. After
	// a normal end the client would take a truncated script for the whole one.
	//nolint:gosec // G110: the stored copy is compiled into the binary by the build, never sent by a client.
	if _, copyErr := io.Copy(w, reader); copyErr != nil {
		panic(http.ErrAbortHandler)
	}
}

// takeSlot waits for a free decompression slot, for at most appDecompressWait
// and never after the client has gone.
func takeSlot(r *http.Request, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
	}

	wait := time.NewTimer(appDecompressWait)
	defer wait.Stop()

	select {
	case slots <- struct{}{}:
		return true
	case <-wait.C:
		return false
	case <-r.Context().Done():
		return false
	}
}

// dropCacheHeaders removes what belongs to the file from an answer that
// became a problem document. A cache must not keep the error under the file's
// tag, and Vary means nothing on it.
func dropCacheHeaders(w http.ResponseWriter) {
	for _, name := range []string{"Cache-Control", "ETag", "Vary"} {
		w.Header().Del(name)
	}
}

// appCacheControl is how long one file of the bundle may be kept.
func appCacheControl(name string) string {
	switch {
	case name == appEntry:
		return appEntryCacheControl
	case strings.HasPrefix(name, appAssetDir):
		return appAssetCacheControl
	default:
		return appFileCacheControl
	}
}

// appDigest identifies the bundle by its bytes, so every file carries an
// entity tag that changes only when the build changes.
//
// A module's bundle arrives with a recorded digest. This one is compiled in
// and has none, so the hash is computed here. It is taken once, on the first
// request that needs it, over every name and every body in sorted order.
func appDigest(files fs.FS) string {
	sum := sha256.New()

	var names []string

	if err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() {
			names = append(names, name)
		}

		return nil
	}); err != nil {
		return ""
	}

	sort.Strings(names)

	for _, name := range names {
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return ""
		}

		_, _ = io.WriteString(sum, name)
		_, _ = sum.Write(body)
	}

	return hex.EncodeToString(sum.Sum(nil))
}

// appBundleFS roots one embedded tree at the directory the build copies into,
// and reports false when that directory holds no entry document. A checkout
// that has not built the app carries the placeholder alone, and a binary built
// from it serves no page.
func appBundleFS(tree fs.FS, root string) (fs.FS, bool) {
	files, err := fs.Sub(tree, root)
	if err != nil {
		return nil, false
	}

	found, err := appFileStored(files, appEntry)
	if err != nil || found.name == "" {
		return nil, false
	}

	return files, true
}
