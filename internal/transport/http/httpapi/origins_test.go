package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
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

func (f *fakeOrigins) AllowedOrigins(context.Context) ([]AllowedOrigin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.listed(), nil
}

func (f *fakeOrigins) SetAllowedOrigins(_ context.Context, origins []string) ([]AllowedOrigin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stored = origins
	f.sets++

	return f.listed(), nil
}

func (f *fakeOrigins) listed() []AllowedOrigin {
	out := make([]AllowedOrigin, 0, len(f.stored)+len(f.fixed))
	for _, origin := range f.stored {
		out = append(out, AllowedOrigin{Origin: origin, Source: OriginFromSettings})
	}

	return append(out, f.fixed...)
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
	rec := call(t, originsHandler(t, grants, "user:viewer", origins),
		http.MethodPut, allowedOriginsPath, `{"origins": ["https://printers.example"]}`)
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

	rec := call(t, handler, http.MethodPut, allowedOriginsPath, `{"origins": ["https://printers.example"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []AllowedOrigin{{Origin: farmOrigin, Source: OriginFromSettings}, extension},
		listedOrigins(t, rec.Body.Bytes()))

	rec = call(t, handler, http.MethodPut, allowedOriginsPath, `{"origins": []}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []AllowedOrigin{extension}, listedOrigins(t, rec.Body.Bytes()))
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

	rec = call(t, built.Handler(), http.MethodPut, allowedOriginsPath, `{"origins": []}`)
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

	_, err := origins.SetAllowedOrigins(t.Context(), []string{farmOrigin})
	require.NoError(t, err)

	rec = callFrom(handler, http.MethodGet, readinessPath, farmOrigin, nil)
	require.Equal(t, farmOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))

	rec = callFrom(handler, http.MethodGet, readinessPath, otherOrigin, nil)
	requireNoCORS(t, rec.Header())
	require.Contains(t, rec.Header().Values("Vary"), "Origin")

	_, err = origins.SetAllowedOrigins(t.Context(), nil)
	require.NoError(t, err)

	rec = callFrom(handler, http.MethodGet, readinessPath, farmOrigin, nil)
	requireNoCORS(t, rec.Header())
}
