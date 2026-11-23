package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// fakeOrigins is the allow-list as the routes and the CORS answer see it: a
// fixed part that stands for the file and the extensions, and the stored one.
type fakeOrigins struct {
	mu     sync.Mutex
	fixed  []AllowedOrigin
	stored []string
	sets   int
}

func (f *fakeOrigins) Any() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.fixed)+len(f.stored) > 0
}

func (f *fakeOrigins) Allows(origin string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Contains(f.stored, origin) ||
		slices.ContainsFunc(f.fixed, func(one AllowedOrigin) bool { return one.Origin == origin })
}

func (f *fakeOrigins) AllowedOrigins(context.Context) (AllowedOriginList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.listed(), nil
}

func (f *fakeOrigins) SetAllowedOrigins(
	_ context.Context, origins []string, pre Precondition,
) (AllowedOriginList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if current := f.version(); !pre.Holds(true, current) {
		return AllowedOriginList{}, httpfault.VersionConflict(AllowedOriginsReference, current)
	}

	f.stored = origins
	f.sets++

	return f.listed(), nil
}

func (f *fakeOrigins) version() string { return versionOf(f.stored) }

// versionOf is the version the server answers for a stored list: a digest of
// it, so the version does not depend on the order the samples run in.
func versionOf(stored []string) string {
	sum := sha256.Sum256([]byte(strings.Join(stored, "\n")))

	return hex.EncodeToString(sum[:8])
}

func (f *fakeOrigins) listed() AllowedOriginList {
	out := make([]AllowedOrigin, 0, len(f.stored)+len(f.fixed))
	for _, origin := range f.stored {
		out = append(out, AllowedOrigin{Origin: origin, Source: OriginFromSettings})
	}

	return AllowedOriginList{Items: append(out, f.fixed...), Version: f.version()}
}

// putOrigins replaces the stored list, quoting version in If-Match when it is
// not empty.
func putOrigins(handler http.Handler, version, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, allowedOriginsPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if version != "" {
		req.Header.Set("If-Match", `"`+version+`"`)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func originsHandler(t *testing.T, grants GrantStore, principal string, origins BrowserOrigins) http.Handler {
	t.Helper()

	built, err := New(Config{
		Version:    "test",
		Principals: fixedPrincipal(principal),
		Resources:  seededStore(),
		Presets:    seededPresets(),
		Endpoints:  seededEndpoints(),
		Grants:     grants,
		Sessions:   newFakeSessions(),
		Accounts:   &fakeAccounts{},
		Readiness:  &fakeReadiness{report: readyReport()},
		Devices:    noPrinters{},
		Stores:     noStorages{},
		Events:     noEvents{},
		Origins:    origins,
	})
	require.NoError(t, err)

	return built.Handler()
}

func listedOrigins(t *testing.T, body []byte) []AllowedOrigin {
	t.Helper()

	var answer struct {
		Items []AllowedOrigin `json:"items"`
	}

	require.NoError(t, json.Unmarshal(body, &answer))

	return answer.Items
}

func TestAViewerReadsTheAllowedOrigins(t *testing.T) {
	t.Parallel()

	grants := &orgRoleGrants{}
	origins := &fakeOrigins{fixed: []AllowedOrigin{{Origin: farmOrigin, Source: OriginFromFile}}}

	rec := call(t, originsHandler(t, grants, "user:viewer", origins), http.MethodGet, allowedOriginsPath, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, origins.fixed, listedOrigins(t, rec.Body.Bytes()))
	require.Equal(t, []authz.Object{linkKindObject(t)}, grants.objects,
		"the read is decided on the link resource kind")
}

// An allowed origin acts with the rights of whoever opens its page, so the
// list is the organization's to change and a viewer is refused.
func TestAViewerMayNotChangeTheAllowedOrigins(t *testing.T) {
	t.Parallel()

	link, err := ref.New(ref.KindLink, "upstream")
	require.NoError(t, err)

	origins := &fakeOrigins{}
	grants := &orgRoleGrants{fakeGrantStore: fakeGrantStore{refuse: authz.Denied(authz.ActionAdminister, link)}}
	rec := putOrigins(originsHandler(t, grants, "user:viewer", origins), versionOf(nil),
		`{"origins": ["https://printers.example"]}`)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Zero(t, origins.sets, "a refused caller changed the list")
}

func TestAnAdministratorReplacesTheStoredOrigins(t *testing.T) {
	t.Parallel()

	extension := AllowedOrigin{
		Origin: otherOrigin, Source: OriginFromExtension, Extension: "com.github.acme.slicer", Title: "Web slicer",
	}
	origins := &fakeOrigins{fixed: []AllowedOrigin{extension}}
	handler := originsHandler(t, &orgRoleGrants{admin: true}, "user:admin", origins)

	rec := putOrigins(handler, versionOf(nil), `{"origins": ["https://printers.example"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, `"`+versionOf([]string{farmOrigin})+`"`, rec.Header().Get("ETag"))
	require.Equal(t, []AllowedOrigin{{Origin: farmOrigin, Source: OriginFromSettings}, extension},
		listedOrigins(t, rec.Body.Bytes()))

	rec = putOrigins(handler, versionOf([]string{farmOrigin}), `{"origins": []}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []AllowedOrigin{extension}, listedOrigins(t, rec.Body.Bytes()))
}

// A write quotes the version it read, so a person working from a list another
// person changed since is refused and does not undo that change. A write that
// quotes none is refused before it is tried.
func TestAWriteFromAnOldReadIsRefused(t *testing.T) {
	t.Parallel()

	origins := &fakeOrigins{}
	handler := originsHandler(t, &orgRoleGrants{admin: true}, "user:admin", origins)

	rec := putOrigins(handler, versionOf(nil), `{"origins": ["https://printers.example"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = putOrigins(handler, versionOf(nil), `{"origins": ["https://elsewhere.example"]}`)
	require.Equal(t, http.StatusPreconditionFailed, rec.Code, rec.Body.String())

	rec = putOrigins(handler, "", `{"origins": ["https://elsewhere.example"]}`)
	require.Equal(t, http.StatusPreconditionRequired, rec.Code, rec.Body.String())
	require.Equal(t, []string{farmOrigin}, origins.stored, "a refused write changed the list")
}

func TestAServerWithNoOriginStoreListsNoneAndRefusesAChange(t *testing.T) {
	t.Parallel()

	built, err := New(Config{
		Version:    "test",
		Principals: fixedPrincipal("user:admin"),
		Resources:  seededStore(),
		Presets:    seededPresets(),
		Endpoints:  seededEndpoints(),
		Grants:     &orgRoleGrants{admin: true},
		Sessions:   newFakeSessions(),
		Accounts:   &fakeAccounts{},
		Readiness:  &fakeReadiness{report: readyReport()},
		Devices:    noPrinters{},
		Stores:     noStorages{},
		Events:     noEvents{},
	})
	require.NoError(t, err)

	rec := call(t, built.Handler(), http.MethodGet, allowedOriginsPath, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Empty(t, listedOrigins(t, rec.Body.Bytes()))

	rec = putOrigins(built.Handler(), versionOf(nil), `{"origins": []}`)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// The CORS answer reads the list for every request: an origin stored while
// the server runs is answered by the next request, and one taken away is not.
func TestAStoredOriginIsAnsweredWithNoRestart(t *testing.T) {
	t.Parallel()

	origins := &fakeOrigins{}
	handler := originsHandler(t, &orgRoleGrants{admin: true}, "user:admin", origins)

	rec := callFrom(handler, http.MethodGet, readinessPath, farmOrigin, nil)
	requireNoCORS(t, rec.Header())
	require.Empty(t, rec.Header().Values("Vary"), "an empty list sends nothing, Vary included")

	_, err := origins.SetAllowedOrigins(t.Context(), []string{farmOrigin},
		Precondition{IfMatch: []string{versionOf(nil)}})
	require.NoError(t, err)

	rec = callFrom(handler, http.MethodGet, readinessPath, farmOrigin, nil)
	require.Equal(t, farmOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))

	rec = callFrom(handler, http.MethodGet, readinessPath, otherOrigin, nil)
	requireNoCORS(t, rec.Header())
	require.Contains(t, rec.Header().Values("Vary"), "Origin")

	_, err = origins.SetAllowedOrigins(t.Context(), nil,
		Precondition{IfMatch: []string{versionOf([]string{farmOrigin})}})
	require.NoError(t, err)

	rec = callFrom(handler, http.MethodGet, readinessPath, farmOrigin, nil)
	requireNoCORS(t, rec.Header())
}
