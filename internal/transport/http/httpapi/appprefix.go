package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// The app can be served under a path prefix that a reverse proxy adds, such as
// the Home Assistant Supervisor's Ingress, which serves an app at
// /api/hassio_ingress/<token>/ and removes that part before it forwards the
// request. The core's routes do not change: the API, the printers' /p/
// addresses and the app's own files stay at the origin root of this server.
// Only the browser, which talks to the proxy, has to know the prefix. The entry
// document tells it with its <base> element, and the app builds every address
// it requests from that element.
//
// The prefix is read from a request header, so it is taken only from a proxy
// the operator declared in core.trusted_proxies. From any other client the
// header is ignored and the document says "/", as it always did.
const (
	// prefixHeaderIngress is the header the Home Assistant Supervisor's Ingress
	// sets on every request it forwards to an app.
	prefixHeaderIngress = "X-Ingress-Path"
	// prefixHeaderForwarded is the header Traefik and other reverse proxies set
	// when they strip a prefix.
	prefixHeaderForwarded = "X-Forwarded-Prefix"
	// prefixMaxBytes bounds the prefix. Ingress writes about 60 bytes.
	prefixMaxBytes = 256
	// appBaseElement is the element the build writes into the entry document.
	// The served document carries it as it is, or with the prefix in its href.
	appBaseElement = `<base href="/" />`
)

// AppOptions is how the core's app is served.
type AppOptions struct {
	// TrustedProxies are the networks a reverse proxy reaches this server from,
	// core.trusted_proxies. Only a request from one of them may set the prefix
	// the entry document names.
	TrustedProxies []netip.Prefix
}

// appPrefix is the path prefix a declared proxy serves this app under, without
// a trailing slash, or "" when the request did not come through one. A header
// value that is not a plain absolute path is ignored, so the value can never
// name another origin or break out of the attribute it is written into.
func appPrefix(r *http.Request, trusted []netip.Prefix) string {
	if len(trusted) == 0 {
		return ""
	}

	peer, readable := proxyPeerOf(r.RemoteAddr)
	if !readable || !proxyTrusted(trusted, peer) {
		return ""
	}

	for _, name := range []string{prefixHeaderIngress, prefixHeaderForwarded} {
		if prefix, ok := cleanPrefix(r.Header.Get(name)); ok {
			return prefix
		}
	}

	return ""
}

// cleanPrefix accepts a path of one or more segments of unreserved URL
// characters, and returns it without a trailing slash. "/" alone is the root,
// which is no prefix.
func cleanPrefix(raw string) (string, bool) {
	prefix := strings.TrimRight(raw, "/")
	if prefix == "" || len(prefix) > prefixMaxBytes || !strings.HasPrefix(prefix, "/") {
		return "", false
	}

	for segment := range strings.SplitSeq(prefix[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}

		for _, char := range segment {
			if !unreservedPathChar(char) {
				return "", false
			}
		}
	}

	return prefix, true
}

// unreservedPathChar is RFC 3986's unreserved set. It holds no quote, no angle
// bracket and no percent sign, so a prefix needs no escaping in HTML or a URL.
func unreservedPathChar(char rune) bool {
	switch {
	case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		return true
	default:
		return char == '-' || char == '.' || char == '_' || char == '~'
	}
}

func proxyPeerOf(remote string) (netip.Addr, bool) {
	addrPort, err := netip.ParseAddrPort(remote)
	if err != nil {
		return netip.Addr{}, false
	}

	return addrPort.Addr().Unmap().WithZone(""), true
}

func proxyTrusted(trusted []netip.Prefix, addr netip.Addr) bool {
	for _, network := range trusted {
		if network.Contains(addr) {
			return true
		}
	}

	return false
}

// appEntryDocument reads the entry document once, from the plain file or from
// its stored gzip copy, so a prefixed answer can be written from it.
func appEntryDocument(files fs.FS, file appFile) func() ([]byte, error) {
	return sync.OnceValues(func() ([]byte, error) {
		if file.plain {
			return fs.ReadFile(files, file.name)
		}

		stored, err := files.Open(file.name + appCompressedSuffix)
		if err != nil {
			return nil, err
		}

		defer func() { _ = stored.Close() }()

		reader, err := gzip.NewReader(stored)
		if err != nil {
			return nil, err
		}

		defer func() { _ = reader.Close() }()

		return io.ReadAll(reader)
	})
}

// servedUnderPrefix answers the entry document for a request a declared proxy
// sent under a prefix, and reports whether it did. With a proxy declared, every
// answer of the entry document says that it varies by the two headers, so a
// cache keeps one copy per prefix and one for the root. With none, the
// document never varies and the answer stays as it was.
func servedUnderPrefix(
	w http.ResponseWriter, r *http.Request, opts AppOptions, entry func() ([]byte, error), digest func() string,
) bool {
	if len(opts.TrustedProxies) == 0 {
		return false
	}

	w.Header().Add("Vary", prefixHeaderIngress+", "+prefixHeaderForwarded)

	prefix := appPrefix(r, opts.TrustedProxies)
	if prefix == "" {
		return false
	}

	document, err := entry()
	if err != nil {
		writeProblem(w, r, httpfault.Unexpected("read the core's app document").Wrap(err))

		return true
	}

	writePrefixedEntry(w, r, document, digest(), prefix)

	return true
}

// writePrefixedEntry sends the entry document with the prefix in its <base>
// element. It is a few kilobytes and is sent plain. Its entity tag names the
// prefix, so a cache never answers one prefix with the document of another.
func writePrefixedEntry(w http.ResponseWriter, r *http.Request, document []byte, digest, prefix string) {
	body := bytes.Replace(document, []byte(appBaseElement), []byte(`<base href="`+prefix+`/" />`), 1)

	sum := sha256.Sum256([]byte(prefix))
	etag := etagOf(digest + "-" + hex.EncodeToString(sum[:8]))

	w.Header().Set("Content-Type", contentTypeOf(appEntry))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", appEntryCacheControl)
	w.Header().Set("ETag", etag)

	if notModified(r, etag) {
		w.WriteHeader(http.StatusNotModified)

		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)

	if headRequest(r) {
		return
	}

	//nolint:gosec // G705: the prefix passed cleanPrefix, which keeps unreserved URL characters only.
	_, _ = w.Write(body)
}
