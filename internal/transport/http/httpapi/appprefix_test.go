package httpapi

import (
	"bytes"
	"compress/gzip"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

const prefixedDocument = `<!doctype html><html><head><base href="/" /><script type="importmap">` +
	`{"imports":{"svelte":"./assets/svelte-1.js"}}</script>` +
	`<script type="module" src="./assets/index-1.js"></script></head><body></body></html>`

// The Supervisor reaches an app from this address on its own network.
const ingressPeer = "172.30.32.2:41234"

func prefixBundle(t *testing.T, compressed bool) fstest.MapFS {
	t.Helper()

	files := fstest.MapFS{"assets/index-1.js": {Data: []byte("export {};")}}
	if !compressed {
		files["index.html"] = &fstest.MapFile{Data: []byte(prefixedDocument)}

		return files
	}

	var stored bytes.Buffer

	writer := gzip.NewWriter(&stored)
	_, err := writer.Write([]byte(prefixedDocument))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	files["index.html"+appCompressedSuffix] = &fstest.MapFile{Data: stored.Bytes()}

	return files
}

func prefixHandler(t *testing.T, compressed bool) http.Handler {
	t.Helper()

	return AppHandlerWith(prefixBundle(t, compressed), AppOptions{
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("172.30.32.2/32")},
	})
}

func prefixRequest(handler http.Handler, path, peer string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = peer

	maps.Copy(req.Header, header)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func baseOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	body := rec.Body.String()
	if rec.Header().Get("Content-Encoding") == "gzip" {
		body = gunzip(t, rec.Body.Bytes())
	}

	start := strings.Index(body, `<base href="`)
	require.NotEqual(t, -1, start, "the document carries no base element")

	href, _, found := strings.Cut(body[start+len(`<base href="`):], `"`)
	require.True(t, found, "the base element is not closed")

	return href
}

// TestADeclaredProxyServesTheAppUnderItsPrefix: the Home Assistant Ingress
// serves the app at /api/hassio_ingress/<token>/, and the document it gets
// names that prefix, on the root and on a client route.
func TestADeclaredProxyServesTheAppUnderItsPrefix(t *testing.T) {
	t.Parallel()

	for _, compressed := range []bool{false, true} {
		handler := prefixHandler(t, compressed)

		for _, path := range []string{"/", "/printers/abc"} {
			rec := prefixRequest(handler, path, ingressPeer, http.Header{
				"X-Ingress-Path":  {"/api/hassio_ingress/Gp28Z_hF6J9miKuP434PkxdSyMSWI-E7jCD99LSh5Tc"},
				"Accept-Encoding": {"gzip"},
			})

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "/api/hassio_ingress/Gp28Z_hF6J9miKuP434PkxdSyMSWI-E7jCD99LSh5Tc/", baseOf(t, rec))
			require.Contains(t, rec.Header().Values("Vary"), "X-Ingress-Path, X-Forwarded-Prefix")
			require.Equal(t, appEntryCacheControl, rec.Header().Get("Cache-Control"))
		}
	}
}

// TestTheForwardedPrefixHeaderIsReadToo: Traefik and others name the prefix
// in X-Forwarded-Prefix.
func TestTheForwardedPrefixHeaderIsReadToo(t *testing.T) {
	t.Parallel()

	rec := prefixRequest(prefixHandler(t, false), "/", ingressPeer, http.Header{"X-Forwarded-Prefix": {"/connect/"}})

	require.Equal(t, "/connect/", baseOf(t, rec))
}

// TestAPrefixFromAnUndeclaredClientIsIgnored: the header is only a proxy's
// word, so from any other client the document stays at the root.
func TestAPrefixFromAnUndeclaredClientIsIgnored(t *testing.T) {
	t.Parallel()

	handler := prefixHandler(t, false)
	rec := prefixRequest(handler, "/", "192.168.1.20:5000", http.Header{"X-Ingress-Path": {"/elsewhere"}})

	require.Equal(t, "/", baseOf(t, rec))

	plain := AppHandler(prefixBundle(t, false))
	rec = prefixRequest(plain, "/", ingressPeer, http.Header{"X-Ingress-Path": {"/elsewhere"}})

	require.Equal(t, "/", baseOf(t, rec), "a deployment that declared no proxy took a prefix")
}

// TestAPrefixThatIsNotAPlainPathIsIgnored: the value is written into an
// attribute and becomes the base of every address, so anything but a path of
// plain segments is refused.
func TestAPrefixThatIsNotAPlainPathIsIgnored(t *testing.T) {
	t.Parallel()

	handler := prefixHandler(t, false)

	for _, value := range []string{
		"//evil.example/x", "https://evil.example", "relative/path", "/a/../b", "/a/./b", "/a//b",
		`/a"onload="x`, "/a b", "/a%2f", "/<script>", "/" + strings.Repeat("a", prefixMaxBytes+1), "/",
	} {
		rec := prefixRequest(handler, "/", ingressPeer, http.Header{"X-Ingress-Path": {value}})

		require.Equalf(t, "/", baseOf(t, rec), "%q was taken as a prefix", value)
	}
}

// TestEachPrefixHasItsOwnEntityTag: a cache must not answer one prefix with
// the document of another, nor the root with a prefixed one.
func TestEachPrefixHasItsOwnEntityTag(t *testing.T) {
	t.Parallel()

	handler := prefixHandler(t, false)

	root := prefixRequest(handler, "/", ingressPeer, nil).Header().Get("ETag")
	first := prefixRequest(handler, "/", ingressPeer, http.Header{"X-Ingress-Path": {"/one"}})
	second := prefixRequest(handler, "/", ingressPeer, http.Header{"X-Ingress-Path": {"/two"}})

	require.NotEmpty(t, first.Header().Get("ETag"))
	require.NotEqual(t, root, first.Header().Get("ETag"))
	require.NotEqual(t, first.Header().Get("ETag"), second.Header().Get("ETag"))

	again := prefixRequest(handler, "/", ingressPeer, http.Header{
		"X-Ingress-Path": {"/one"},
		"If-None-Match":  {first.Header().Get("ETag")},
	})
	require.Equal(t, http.StatusNotModified, again.Code)
}

// TestTheAppsOwnFilesDoNotChangeUnderAPrefix: only the entry document names
// the prefix. A script is the same bytes whichever way it is asked for.
func TestTheAppsOwnFilesDoNotChangeUnderAPrefix(t *testing.T) {
	t.Parallel()

	rec := prefixRequest(prefixHandler(t, false), "/assets/index-1.js", ingressPeer,
		http.Header{"X-Ingress-Path": {"/one"}})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "export {};", rec.Body.String())
}

func TestCleanPrefix(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{
		"/api/hassio_ingress/abc": "/api/hassio_ingress/abc",
		"/connect/":               "/connect",
		"/a.b~c_d-e":              "/a.b~c_d-e",
	} {
		got, ok := cleanPrefix(raw)
		require.Truef(t, ok, "%q was refused", raw)
		require.Equal(t, want, got)
	}
}
