package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// committedSpecPath is the frozen core document: everything compiled into this
// binary, with no extension loaded.
const committedSpecPath = "openapi.json"

// publicOperations is every operation that answers without a credential. It is
// the honest part of the identity gate: the exceptions live here, in one
// reviewed list, and adding to it is a line in a diff rather than a decision
// nobody sees.
//
//   - health is what a watchdog reads, and a wedged process cannot be asked to
//     authenticate first
//   - readiness is what a load balancer routes on, and a load balancer holds no
//     credential
//   - the login operations are how a caller with no credential gets one, and
//     the first-run pair is how a server nobody has set up yet gets its
//     administrator. The login report names no account and counts none
//   - the two first-run operations are how an install that has recorded nothing
//     about identity records it. Until it does, this server cannot say whether
//     a caller needs a credential, so asking for one would be asking a question
//     nobody has defined
//   - the app settings are what this server's own page reads before it paints
//     and before anybody has signed in. They name no account, no printer and no
//     module: a title, a language and a theme
//   - the vocabulary operations publish closed sets that are identical in every
//     deployment of a build and name no principal, grant or instance. A client
//     branching on an error code and an author writing a plugin both need them
//     before they hold a credential, and the same bytes are on the public
//     documentation site (ADR-20260825-bhe rule 10)
//   - the drawing a device-model row links to is public with the row. The
//     driver that declared it is in the binary, so it too is the same in every
//     deployment of a build (ADR-20261016-qrp rule 6)
var publicOperations = []string{
	"POST " + appPairingsPath,
	"POST " + appPairingTokenPath,
	"GET " + callbackPath,
	"GET " + loginPath,
	"POST " + loginPath,
	"GET " + authInfoPath,
	"POST " + setupPath,
	"GET " + firstRunPath,
	"POST " + firstRunPath,
	"GET " + setupConfigurationPath,
	"POST " + setupStoragePath,
	"POST " + setupOIDCPath,
	"POST " + setupCertificatesPath,
	"GET " + healthPath,
	"GET " + readinessPath,
	"GET " + appSettingsPath,
	"GET " + vocabulariesPath,
	"GET " + vocabularyErrorCodesPath,
	"GET " + vocabularyExtensionsPath,
	"GET " + vocabularyCapabilitiesPath,
	"GET " + vocabularyResourceKindsPath,
	"GET " + vocabularyAuthorizationPath,
	"GET " + vocabularyEventTypesPath,
	"GET " + vocabularyDeviceIssuesPath,
	"GET " + vocabularyDeviceModelsPath,
	"GET " + vocabularyDeviceModelArtworkPath,
}

// TestTheDocumentIsServedWithoutCaching covers the handler Huma's own would
// have been: the version that caches in an unsynchronised closure and never
// invalidates.
func TestTheDocumentIsServedWithoutCaching(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for path, mediaType := range map[string]string{
		specJSONPath: "application/openapi+json",
		specYAMLPath: "application/openapi+yaml",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, mediaType, rec.Header().Get("Content-Type"))
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			require.NotEmpty(t, rec.Body.Bytes())
		})
	}
}

// TestTheDocumentStatesWhatTheMiddlewareEnforces is the gate on a document that
// said every operation was open. components.securitySchemes was nil and no
// operation carried a requirement, and OpenAPI 3.1 reads an absent root
// security as no requirement at all, so a generated client sent no credentials
// to a server that refuses callers it cannot identify.
func TestTheDocumentStatesWhatTheMiddlewareEnforces(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, specJSONPath, nil))

	var document struct {
		Components struct {
			SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
		} `json:"components"`
		Security []map[string][]string `json:"security"`
		// publishedPath drops the members of a path item that are not
		// operations, which a described path item carries.
		Paths map[string]publishedPath `json:"paths"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &document))
	require.Contains(t, document.Components.SecuritySchemes, bearerScheme,
		"a client cannot send a credential the document does not name")
	require.Contains(t, document.Components.SecuritySchemes, cookieScheme,
		"a browser session is a credential this server accepts, so the document names it")

	// Two entries and not one entry naming two schemes. The root list is read
	// as alternatives, and a single entry holding both would say a request has
	// to carry a bearer token and a cookie at once.
	require.Equal(t,
		[]map[string][]string{{bearerScheme: {}}, {cookieScheme: {}}},
		document.Security)

	health := document.Paths[healthPath][strings.ToLower(http.MethodGet)]
	require.NotNil(t, health.Security, "health is public, and the document has to say so explicitly")
	require.Empty(t, *health.Security)

	// Every operation the middleware gates must be left to inherit the root
	// requirement, so what is public is exactly what declares it is.
	var public []string

	for path, item := range document.Paths {
		for method, op := range item {
			if op.Security == nil {
				continue
			}

			require.Emptyf(t, *op.Security, "%s %s declares a requirement of its own", method, path)

			public = append(public, strings.ToUpper(method)+" "+path)
		}
	}

	require.ElementsMatch(t, publicOperations, public,
		"the operations reachable without an identity are the ones listed in publicOperations")
}

// TestEveryPublicOperationIsOneTheMiddlewareTreatsAsPublic ties the document to
// the code that reads it. isPublic is the only thing standing between a
// published empty requirement and an unauthenticated route.
func TestEveryPublicOperationIsOneTheMiddlewareTreatsAsPublic(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for path, item := range server.api.OpenAPI().Paths {
		for method, op := range operationsOfPath(item) {
			if op == nil {
				continue
			}

			require.Equalf(t, slices.Contains(publicOperations, method+" "+path), isPublic(op),
				"%s %s and the middleware disagree about whether it needs an identity", method, path)
		}
	}
}

// TestTheServedDocumentPublishesEveryOperation reads the bytes a client would,
// rather than the object they were rendered from.
func TestTheServedDocumentPublishesEveryOperation(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, specJSONPath, nil))

	var document struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &document))
	require.Equal(t, "3.1.0", document.OpenAPI)
	require.Contains(t, document.Paths, healthPath)
	require.Contains(t, document.Paths, principalPath)
}

// New must not render the document. Rendering it takes about 27 MiB for a
// moment, which is enough to get the server killed on a small device while it
// starts.
func TestNewDoesNotRenderTheDocument(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	require.Zero(t, server.spec.renders.Load(), "New rendered the OpenAPI document")
}

// The first request renders the document, and every later request is answered
// with the same bytes.
func TestTheDocumentIsRenderedOnTheFirstRequestAndKept(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	get := func(path string) string {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, "GET %s", path)

		return rec.Body.String()
	}

	first := get(specJSONPath)
	require.EqualValues(t, 1, server.spec.renders.Load(), "the first JSON request renders the document")
	require.Equal(t, first, get(specJSONPath))

	firstYAML := get(specYAMLPath)
	require.Contains(t, firstYAML, "openapi: 3.1.0")
	require.YAMLEq(t, firstYAML, get(specYAMLPath))
	require.YAMLEq(t, first, firstYAML, "the YAML says what the JSON says")
	require.EqualValues(t, 1, server.spec.renders.Load(), "the YAML is made from the rendered JSON")

	rendered := renderedDocument(t, server)
	require.Equal(t, first, string(rendered), "the served bytes are the rendered ones")

	again := renderedDocument(t, server)
	require.Same(t, &rendered[0], &again[0], "the bytes are rendered once and shared")
}

// A first request for the YAML renders the JSON it is made from, and the JSON
// is not rendered again after that.
func TestAFirstYAMLRequestRendersTheDocumentOnce(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for _, path := range []string{specYAMLPath, specJSONPath, specYAMLPath} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, "GET %s", path)
	}

	require.EqualValues(t, 1, server.spec.renders.Load())
}

// Huma's own document cache races when two first requests arrive together.
// This one renders the document once whatever the number of first requests,
// for either form.
func TestConcurrentFirstRequestsRenderTheDocumentOnce(t *testing.T) {
	t.Parallel()

	const callers = 8

	server := newTestServer(t)

	bodies := make([]string, callers*2)

	var wg sync.WaitGroup

	for i := range bodies {
		path := specJSONPath
		if i%2 == 1 {
			path = specYAMLPath
		}

		wg.Go(func() {
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			bodies[i] = rec.Body.String()
		})
	}

	wg.Wait()

	require.EqualValues(t, 1, server.spec.renders.Load(), "the document is rendered once")

	for i := 2; i < len(bodies); i++ {
		require.Equal(t, bodies[i%2], bodies[i], "every request for one form gets the same bytes")
	}
}

// renderedDocument returns the JSON document the server serves, and fails the
// test when it cannot be rendered.
func renderedDocument(t *testing.T, server *Server) []byte {
	t.Helper()

	body, err := server.spec.document()
	require.NoError(t, err)

	return body
}
