package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// The namespace and the instance every case here uses. The namespace is the
// shape extension.NamespaceOf renders for github.com/acme/planner, so a case that
// passes here passes for a real module path.
const (
	moduleNamespace = "com.github.aux.acme.planner"
	moduleInstance  = "sink:planner"
)

// fakeModulePlane is one module claiming one namespace, and a root mount when
// mount is set.
type fakeModulePlane struct {
	namespace string
	instance  ref.Ref
	handler   http.Handler
	openapi   []byte
	mount     string
	// unavailable is why the claimed prefix is not being served, for the case
	// where a module is declared, switched on, and its files are not installed.
	unavailable string
	// modulesOff is a prefix whose installed module the operator's key
	// keeps off.
	modulesOff bool
	// safeMode makes the held prefix a safe-mode one.
	safeMode bool
	// public is the routes the operator approved to answer with no caller.
	public []manifest.Route
	// staged is the routes the operator approved to stage, with their limits.
	staged []manifest.StagedRoute
	// title and summary are the module's own, from its manifest.
	title, summary string
	// themes is the stylesheet of each theme the bundle offers, by id.
	themes map[string]string
}

func (p fakeModulePlane) Module(namespace string) (ModuleSurface, bool) {
	if namespace != p.namespace {
		return ModuleSurface{}, false
	}

	return ModuleSurface{
		Instance: p.instance, Handler: p.handler, OpenAPI: p.openapi, Mount: p.mount, Public: p.public,
		Staged: p.staged, Title: p.title, Summary: p.summary, Themes: p.themes,
	}, true
}

func (p fakeModulePlane) Root(path string) (ModuleSurface, bool) {
	if p.mount == "" || (path != p.mount && !strings.HasPrefix(path, p.mount+"/")) {
		return ModuleSurface{}, false
	}

	if p.unavailable != "" || p.modulesOff {
		return ModuleSurface{
			Instance:    p.instance,
			Mount:       p.mount,
			Unavailable: p.unavailable,
			ModulesOff:  p.modulesOff,
			SafeMode:    p.safeMode,
		}, true
	}

	return ModuleSurface{
		Instance: p.instance, Handler: p.handler, OpenAPI: p.openapi, Mount: p.mount, Public: p.public,
		Staged: p.staged, Title: p.title, Summary: p.summary,
	}, true
}

// fakeModuleAccess is the graph as this mount reads it: it permits one action
// on one instance for one subject, and refuses everything else the way
// authz.Store does. The lock guards asked, because parallel subtests may call
// one server that holds this fake at the same time.
type fakeModuleAccess struct {
	mu       sync.Mutex
	who      string
	instance ref.Ref
	actions  []authz.Action
	asked    []authz.Action
}

func (a *fakeModuleAccess) RequireResource(
	_ context.Context, who authz.Subject, action authz.Action, instance ref.Ref,
) error {
	a.mu.Lock()
	a.asked = append(a.asked, action)
	a.mu.Unlock()

	if who.String() == a.who && instance == a.instance && slices.Contains(a.actions, action) {
		return nil
	}

	return fault.New(authz.CodeAccessDenied(), "the subject holds no grant reaching this action")
}

// moduleEcho answers with the path and the principal it was handed, which is
// what proves the group prefix came off and the host's own answer about the
// caller went in.
func moduleEcho() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"path":      r.URL.Path,
			"principal": ModulePrincipalOf(r.Context()),
			"query":     r.URL.RawQuery,
		})
	})
}

func moduleServer(t *testing.T, plane ModulePlane, access ModuleAccess, who string) *Server {
	t.Helper()

	server, err := New(Config{
		Version:      "test",
		Principals:   fixedPrincipal(who),
		Resources:    seededStore(),
		Presets:      seededPresets(),
		Endpoints:    seededEndpoints(),
		Grants:       &fakeGrantStore{},
		Sessions:     newFakeSessions(),
		Accounts:     &fakeAccounts{info: AuthInfo{Provider: "local", Credentials: true}},
		Readiness:    &fakeReadiness{report: readyReport()},
		Devices:      noPrinters{},
		Stores:       noStorages{},
		Events:       noEvents{},
		Modules:      plane,
		ModuleAccess: access,
	})
	require.NoError(t, err)

	return server
}

// theFarm is the plane, the graph and the server every reachable case is built
// from: one module claiming one namespace, and a caller holding both resource
// actions on its instance.
func theFarm(t *testing.T) (*Server, *fakeModuleAccess) {
	t.Helper()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	access := &fakeModuleAccess{
		who:      "user:operator",
		instance: instance,
		actions:  []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
	}

	plane := fakeModulePlane{namespace: moduleNamespace, instance: instance, handler: moduleEcho()}

	return moduleServer(t, plane, access, "user:operator"), access
}

// ask sends one request at the server with no client in the way, so a path a
// client would normalise still arrives as it was written.
func ask(t *testing.T, server *Server, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request := httptest.NewRequest(method, target, reader)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	return recorder
}

// TestAModuleRouteIsMountedUnderItsNamespaceAndAuthorized is the gate ADR-20260827-wmv
// rules 12 and 13 owe.
//
// It is one case and not two because the two halves are one property: the
// address a module is reached at, and the fact that reaching it took a
// permission. A mount proved without the refusal beside it is the shape planner
// mode shipped, which is the thing rule 13 exists to refuse.
func TestAModuleRouteIsMountedUnderItsNamespaceAndAuthorized(t *testing.T) {
	t.Parallel()

	server, access := theFarm(t)

	// Reached, at the address rule 12 names, with the group taken off.
	answered := ask(t, server, http.MethodGet,
		"/api/v1/modules/"+moduleNamespace+"/spools?colour=black", "")
	require.Equal(t, http.StatusOK, answered.Code, answered.Body.String())

	var echoed map[string]string
	require.NoError(t, json.Unmarshal(answered.Body.Bytes(), &echoed))

	require.Equal(t, "/spools", echoed["path"],
		"the module was handed the group prefix, so it cannot write its own routes")
	require.Equal(t, "colour=black", echoed["query"], "the query was dropped on the way in")
	require.Equal(t, "user:operator", echoed["principal"],
		"the module was not told who the host authorized")

	// Authorized, on the module's own instance, with the action the method
	// asks for.
	require.Equal(t, []authz.Action{authz.ActionViewResource}, access.asked,
		"a read did not ask the graph for view_resource on the module's instance")

	written := ask(t, server, http.MethodPost, "/api/v1/modules/"+moduleNamespace+"/spools", `{}`)
	require.Equal(t, http.StatusOK, written.Code, written.Body.String())
	require.Equal(t, authz.ActionOperateResource, access.asked[len(access.asked)-1],
		"a write did not ask the graph for operate_resource")
}

// TestAModuleRouteIsRefusedForACallerWhoHoldsNothing is the refusal, on the
// same mount and the same module.
//
// The module must not run at all. A server that answered the module and then
// refused would have handed the request to code the caller may not reach, and
// on a module in its own process that work is already done by then.
func TestAModuleRouteIsRefusedForACallerWhoHoldsNothing(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse("sink:planner")
	require.NoError(t, err)

	var reached bool

	plane := fakeModulePlane{
		namespace: moduleNamespace,
		instance:  instance,
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}),
	}

	// A graph that permits the operator and nobody else.
	access := &fakeModuleAccess{
		who:      "user:operator",
		instance: instance,
		actions:  []authz.Action{authz.ActionViewResource},
	}

	server := moduleServer(t, plane, access, "user:stranger")

	refused := ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/spools", "")
	require.Equal(t, http.StatusForbidden, refused.Code, refused.Body.String())
	require.Contains(t, refused.Body.String(), "ACCESS_DENIED")
	require.False(t, reached, "the module ran for a caller who holds nothing on it")
}

// TestAServerWithModulesAndNoGraphDoesNotStart is the same rule one layer
// earlier. A mount that authorizes on nothing must not be constructible, so the
// hole cannot be opened by leaving a field out.
func TestAServerWithModulesAndNoGraphDoesNotStart(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse("sink:planner")
	require.NoError(t, err)

	_, err = New(Config{
		Version:    "test",
		Principals: fixedPrincipal("user:operator"),
		Resources:  seededStore(),
		Presets:    seededPresets(),
		Endpoints:  seededEndpoints(),
		Grants:     &fakeGrantStore{},
		Sessions:   newFakeSessions(),
		Accounts:   &fakeAccounts{info: AuthInfo{Provider: "local", Credentials: true}},
		Readiness:  &fakeReadiness{report: readyReport()},
		Devices:    noPrinters{},
		Stores:     noStorages{},
		Events:     noEvents{},
		Modules: fakeModulePlane{
			namespace: moduleNamespace, instance: instance, handler: moduleEcho(),
		},
	})

	require.ErrorIs(t, err, ErrNoModuleAccess)
}

// TestAModuleReachesNothingOutsideItsOwnGroup is the containment half: every
// way a module could be reached at an address that is not its own, and every
// way a caller could reach past the group into this server's own paths.
//
// Each case answers as an unrouted path and never as a refusal that says a
// module was looked for, so the set of installed modules is not enumerable from
// outside.
//
//nolint:tparallel // the cases share one recorder and the assertions after the loop read what every case did.
func TestAModuleReachesNothingOutsideItsOwnGroup(t *testing.T) {
	t.Parallel()

	server, access := theFarm(t)

	cases := map[string]string{
		"a namespace nothing claims":            "/api/v1/modules/com.github.aux.acme.other/spools",
		"the core namespace":                    "/api/v1/modules/core/spools",
		"a single label, which is no namespace": "/api/v1/modules/acme/spools",
		"a namespace with an illegal label":     "/api/v1/modules/com.github.aux.Acme.planner/spools",
		"a traversal out of the group":          "/api/v1/modules/" + moduleNamespace + "/../../health",
		"a traversal that would reach the document": "/api/v1/modules/" + moduleNamespace +
			"/../../../openapi.json",
		"a dot segment inside the group": "/api/v1/modules/" + moduleNamespace + "/./spools",
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			answered := ask(t, server, http.MethodGet, target, "")

			require.Equal(t, http.StatusNotFound, answered.Code, answered.Body.String())
			require.Contains(t, answered.Body.String(), "ROUTE_NOT_FOUND",
				"the refusal is not the one an unrouted path answers with")
		})
	}

	require.Empty(t, access.asked,
		"a path outside the group reached the graph, so something was resolved to a module")

	// The core paths those traversals named still answer as themselves.
	require.Equal(t, http.StatusOK, ask(t, server, http.MethodGet, "/api/v1/health", "").Code)
	require.Equal(t, http.StatusOK, ask(t, server, http.MethodGet, "/openapi.json", "").Code)
}

// TestAModuleThatPanicsWhileServingDoesNotTakeTheServerDown is what a module
// costs when it is wrong. The request fails, the process serves, and the next
// request through the same mount is answered.
func TestAModuleThatPanicsWhileServingDoesNotTakeTheServerDown(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse("sink:planner")
	require.NoError(t, err)

	plane := fakeModulePlane{
		namespace: moduleNamespace,
		instance:  instance,
		handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/panic") {
				panic("the module panicked while serving")
			}
		}),
	}

	access := &fakeModuleAccess{
		who: "user:operator", instance: instance,
		actions: []authz.Action{authz.ActionViewResource},
	}

	server := moduleServer(t, plane, access, "user:operator")

	panicked := ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/panic", "")
	require.Equal(t, http.StatusInternalServerError, panicked.Code)
	require.NotContains(t, panicked.Body.String(), "the module panicked",
		"the module's own words reached the client")

	require.Equal(t, http.StatusOK, ask(t, server, http.MethodGet, "/api/v1/health", "").Code,
		"a panicking module took the server with it")
	require.Equal(t, http.StatusOK,
		ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/spools", "").Code,
		"the mount stopped answering after one module panicked")
}

// TestNoModuleMovesWhatTheDocumentPublishes is ADR-20260827-wmv rule 14 at this layer.
// A server with a module mounted publishes exactly what a server without one
// does, so installing a module changes no generated client and uninstalling one
// costs no breaking change.
func TestNoModuleMovesWhatTheDocumentPublishes(t *testing.T) {
	t.Parallel()

	withModule, _ := theFarm(t)

	// The same wiring with no module plane, so the only difference between the
	// two servers is the module.
	without := moduleServer(t, nil, nil, "user:operator")

	require.Equal(t, publishedOperations(without.api), publishedOperations(withModule.api),
		"mounting a module moved the published operation set")

	// The core publishes a few operations under the group, and every one of
	// them is the core's own: the document address, the settings a module's
	// pane reads and writes, the three that install a module and say what
	// there is to install, the marketplace switch, and the databases modules
	// left behind. They are there on both
	// servers, parametrised by the
	// namespace where they name one, and no module's own route joins them.
	core := []string{
		http.MethodGet + " " + moduleOpenAPIPath,
		http.MethodGet + " " + moduleStatePath,
		http.MethodGet + " " + moduleStateKeyPath,
		http.MethodPut + " " + moduleStateKeyPath,
		http.MethodDelete + " " + moduleStateKeyPath,
		http.MethodPost + " " + modulesPathPrefix,
		http.MethodDelete + " " + moduleNamespacePath,
		http.MethodGet + " " + moduleIndexPath,
		http.MethodPost + " " + modulePlanPath,
		http.MethodGet + " " + moduleRemovalPath,
		http.MethodGet + " " + moduleWhyPath,
		http.MethodGet + " " + moduleWhyNotPath,
		http.MethodGet + " " + moduleOutdatedPath,
		http.MethodGet + " " + moduleMemoryPath,
		http.MethodGet + " " + moduleMarketplacePath,
		http.MethodPut + " " + moduleMarketplacePath,
		http.MethodGet + " " + moduleDatabasesPath,
		http.MethodDelete + " " + moduleDatabasePath,
	}

	for _, path := range publishedOperations(withModule.api) {
		if slices.Contains(core, path) {
			continue
		}

		require.NotContains(t, path, modulesPathPrefix,
			"a module's own subtree reached the OpenAPI document")
	}
}

// TestAServerWithNoModulePlaneAnswersTheGroupAsUnrouted keeps the address
// behaving the same however the process was wired. A build with no module plane
// answers what a build with one answers for a module that is not installed.
func TestAServerWithNoModulePlaneAnswersTheGroupAsUnrouted(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	answered := ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/spools", "")
	require.Equal(t, http.StatusNotFound, answered.Code)
	require.Contains(t, answered.Body.String(), "ROUTE_NOT_FOUND")
}

// greedyPlane claims every namespace it is asked about. It exists so the
// mount's own grammar check is what the case below reads, rather than a plane
// that happened to hold only one namespace.
type greedyPlane struct {
	instance ref.Ref
	asked    []string
}

func (p *greedyPlane) Module(namespace string) (ModuleSurface, bool) {
	p.asked = append(p.asked, namespace)

	return ModuleSurface{Instance: p.instance, Handler: moduleEcho()}, true
}

func (p *greedyPlane) Root(string) (ModuleSurface, bool) { return ModuleSurface{}, false }

// TestTheMountHoldsTheNamespaceGrammarBeforeAnythingIsAsked is the guard that a
// plane cannot be trusted to hold.
//
// The namespace is a path segment a caller writes, and every reader of it keys
// on it: the plane looks a module up by it, and the mount builds the prefix it
// strips out of it. A segment no module could have been given must be refused
// where the request arrives, so that a plane which answered generously, or a
// second plane written later, cannot open the group to a name the extension
// host would never derive.
//
// The plane here claims everything, so a case that passes proves the refusal
// was the mount's and not the plane's.
//
//nolint:tparallel // the cases share one recorder and the assertion after the loop reads what every case did.
func TestTheMountHoldsTheNamespaceGrammarBeforeAnythingIsAsked(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	plane := &greedyPlane{instance: instance}

	access := &fakeModuleAccess{
		who: "user:operator", instance: instance,
		actions: []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
	}

	server := moduleServer(t, plane, access, "user:operator")

	refused := map[string]string{
		"the core namespace, which only the main module holds": "core",
		"a single label, which is not a collision domain":      "acme",
		"a label that is not DNS-1123":                         "com.github.aux.Acme.planner",
		"an empty label":                                       "com..planner",
		"a namespace longer than a vocabulary row carries":     strings.Repeat("ab.", 40) + "planner",
	}

	for name, namespace := range refused {
		t.Run(name, func(t *testing.T) {
			answered := ask(t, server, http.MethodGet, "/api/v1/modules/"+namespace+"/spools", "")

			require.Equal(t, http.StatusNotFound, answered.Code, answered.Body.String())
		})
	}

	require.Empty(t, plane.asked,
		"a namespace no module could have been given was looked up, so the grammar is the plane's "+
			"to hold rather than the mount's: %v", plane.asked)

	// The arming half. A namespace the extension host does derive reaches the
	// same greedy plane and is served, so the cases above are refusals and not
	// a mount that answers nothing.
	served := ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/spools", "")
	require.Equal(t, http.StatusOK, served.Code, served.Body.String())
	require.Equal(t, []string{moduleNamespace}, plane.asked)
}

// TestAPublicRouteReachesTheModuleWithNoPrincipal is the gate ADR-20261120-xys
// rules 1 to 3 owe. A route the operator approved under routes.public is
// reached by a caller who presents nothing, the graph is never asked, and the
// module reads an empty principal even when the caller is signed in. Every
// other route of the same module still refuses that caller, and the mount's
// path checks still hold on the public one.
func TestAPublicRouteReachesTheModuleWithNoPrincipal(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	token := manifest.Route{Method: http.MethodPost, Path: "/hooks/announce/"}
	group := "/api/v1/modules/" + moduleNamespace

	for _, mount := range []string{"", "/connect"} {
		prefix := group
		if mount != "" {
			prefix = mount
		}

		t.Run("mounted at "+prefix, func(t *testing.T) {
			t.Parallel()

			var reached int

			plane := fakeModulePlane{
				namespace: moduleNamespace, instance: instance, mount: mount, public: []manifest.Route{token},
				handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					reached++
					moduleEcho().ServeHTTP(w, r)
				}),
			}

			access := &fakeModuleAccess{
				who: "user:operator", instance: instance,
				actions: []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
			}

			for _, who := range []string{"", "user:operator"} {
				server := moduleServer(t, plane, access, who)

				answered := ask(t, server, http.MethodPost, prefix+"/hooks/announce/", "grant_type=x")
				require.Equal(t, http.StatusOK, answered.Code, answered.Body.String())

				var echoed map[string]string
				require.NoError(t, json.Unmarshal(answered.Body.Bytes(), &echoed))
				require.Equal(t, "/hooks/announce/", echoed["path"])
				require.Empty(t, echoed["principal"], "a public route resolved a caller for %q", who)
			}

			require.Equal(t, 2, reached)
			require.Empty(t, access.asked, "a public route asked the graph")

			// The same server with no caller, on everything that is not the
			// public route: another method, another path, and a path that
			// leaves the mount on the way to the public one.
			server := moduleServer(t, plane, access, "")

			for _, refused := range []struct{ method, path string }{
				{http.MethodGet, "/hooks/announce/"},
				{http.MethodPost, "/hooks/announce"},
				{http.MethodPost, "/hooks/announce/extra"},
				{http.MethodPost, "/spools"},
			} {
				answered := ask(t, server, refused.method, prefix+refused.path, "")
				require.Equal(t, http.StatusUnauthorized, answered.Code, "%s %s", refused.method, refused.path)
			}

			escaped := ask(t, server, http.MethodPost, prefix+"/x/../hooks/announce/", "")
			require.Equal(t, http.StatusNotFound, escaped.Code, escaped.Body.String())

			require.Equal(t, 2, reached, "the module ran on a route that is not public")
		})
	}
}

// TestAnEncodedPathCannotTurnAPublicRouteIntoAnother sends, with no caller,
// paths that match a public route as the host decodes them and another route
// once they are parsed again: an encoded ? or # cuts the path short there, and
// an encoded % is decoded twice. The host refuses every one before the module
// runs, so no route that needs a caller is reached without one.
func TestAnEncodedPathCannotTurnAPublicRouteIntoAnother(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	announce := manifest.Route{Method: http.MethodPost, Path: "/hooks/{device}/announce"}
	group := "/api/v1/modules/" + moduleNamespace

	for _, mount := range []string{"", "/connect"} {
		prefix := group
		if mount != "" {
			prefix = mount
		}

		t.Run("mounted at "+prefix, func(t *testing.T) {
			t.Parallel()

			var reached []string

			plane := fakeModulePlane{
				namespace: moduleNamespace, instance: instance, mount: mount, public: []manifest.Route{announce},
				handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					reached = append(reached, r.URL.Path)
					moduleEcho().ServeHTTP(w, r)
				}),
			}

			access := &fakeModuleAccess{
				who: "user:operator", instance: instance,
				actions: []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
			}
			server := moduleServer(t, plane, access, "")

			answered := ask(t, server, http.MethodPost, prefix+"/hooks/x/announce", "")
			require.Equal(t, http.StatusOK, answered.Code, answered.Body.String())

			for _, path := range []string{
				"/hooks/x%3F/announce",
				"/hooks/x%23/announce",
				"/hooks/x%253F/announce",
				"/hooks/x%25/announce",
				"/hooks/x%00/announce",
				"/hooks/x%0A/announce",
			} {
				refused := ask(t, server, http.MethodPost, prefix+path, "")
				require.Equal(t, http.StatusNotFound, refused.Code, "%s: %s", path, refused.Body.String())
			}

			require.Equal(t, []string{"/hooks/x/announce"}, reached, "the module ran on an encoded path")
			require.Empty(t, access.asked, "a public route asked the graph")
		})
	}
}

// TestALargeBodyReachesAModuleThroughTheServer sends a body over the document
// bound through the whole server, middleware included, to a staged route of a
// module under its namespace and under a root prefix. The route's approved
// limit is above the body, so the server's document bound must not cut it
// off first (ADR-20261120-xys rule 6).
func TestALargeBodyReachesAModuleThroughTheServer(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	body := strings.Repeat("G", 3<<20)

	for _, mount := range []string{"", "/connect"} {
		prefix := "/api/v1/modules/" + moduleNamespace
		if mount != "" {
			prefix = mount
		}

		var got int

		plane := fakeModulePlane{
			namespace: moduleNamespace,
			instance:  instance,
			mount:     mount,
			staged: []manifest.StagedRoute{
				{Route: manifest.Route{Method: http.MethodPut, Path: "/files/{name}"}, MaxMiB: 4},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				read, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					http.Error(w, readErr.Error(), http.StatusUnprocessableEntity)

					return
				}

				got = len(read)

				w.WriteHeader(http.StatusOK)
			}),
		}

		access := &fakeModuleAccess{
			who: "user:operator", instance: instance,
			actions: []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
		}

		answered := ask(t, moduleServer(t, plane, access, "user:operator"), http.MethodPut, prefix+"/files/raw", body)
		require.Equal(t, http.StatusOK, answered.Code, "%s: %s", prefix, answered.Body.String())
		require.Equal(t, len(body), got, "%s: the module read a cut body", prefix)
	}
}

// TestABodyOverItsModuleLimitIsRefusedBeforeTheModuleRuns sends a body over
// the approved limit of a staged route, and the same body to a route that is
// not staged and so takes the document bound. The server refuses both with
// BODY_TOO_LARGE, naming the limit, and the module never runs
// (ADR-20261120-xys rule 6).
func TestABodyOverItsModuleLimitIsRefusedBeforeTheModuleRuns(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	body := strings.Repeat("G", 3<<20)

	for _, tc := range []struct {
		name, mount, method, path string
		limit                     int64
	}{
		{"staged under the namespace", "", http.MethodPut, "/files/raw", 2 << 20},
		{"staged under a root prefix", "/connect", http.MethodPut, "/files/raw", 2 << 20},
		{"not staged", "", http.MethodPost, "/files/raw", 1 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prefix := "/api/v1/modules/" + moduleNamespace
			if tc.mount != "" {
				prefix = tc.mount
			}

			var ran atomic.Bool

			plane := fakeModulePlane{
				namespace: moduleNamespace,
				instance:  instance,
				mount:     tc.mount,
				staged: []manifest.StagedRoute{
					{Route: manifest.Route{Method: http.MethodPut, Path: "/files/{name}"}, MaxMiB: 2},
				},
				handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					ran.Store(true)
					w.WriteHeader(http.StatusOK)
				}),
			}

			access := &fakeModuleAccess{
				who: "user:operator", instance: instance,
				actions: []authz.Action{authz.ActionViewResource, authz.ActionOperateResource},
			}

			answered := ask(t, moduleServer(t, plane, access, "user:operator"), tc.method, prefix+tc.path, body)
			require.Equal(t, http.StatusRequestEntityTooLarge, answered.Code, answered.Body.String())
			require.Contains(t, answered.Body.String(), "BODY_TOO_LARGE")
			require.Contains(t, answered.Body.String(), strconv.FormatInt(tc.limit, 10))
			require.False(t, ran.Load(), "the module ran on a refused body")
		})
	}
}

// TestNoStagedRouteWidensACoreOperation walks every operation the core serves
// under a module's namespace, such as PUT .../state/{key}, for a module whose
// staged routes match any path of one or two segments. The body bound runs
// before chi routes, and a staged limit there would let up to 4 GiB into an
// operation that reads its body into memory.
func TestNoStagedRouteWidensACoreOperation(t *testing.T) {
	t.Parallel()

	instance, err := ref.Parse(moduleInstance)
	require.NoError(t, err)

	plane := fakeModulePlane{namespace: moduleNamespace, instance: instance}
	for _, method := range []string{http.MethodPut, http.MethodPost, http.MethodPatch, http.MethodGet} {
		for _, path := range []string{"/{a}", "/{a}/{b}"} {
			plane.staged = append(plane.staged,
				manifest.StagedRoute{Route: manifest.Route{Method: method, Path: path}, MaxMiB: 4096})
		}
	}

	under := ModuleMount("{"+namespaceParam+"}") + "/"

	var core []string

	err = chi.Walk(newTestServer(t).mux,
		func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if !strings.HasPrefix(pattern, under) || pattern == modulesRoutePattern {
				return nil
			}

			core = append(core, method+" "+pattern)

			path := concretePath(strings.Replace(pattern, "{"+namespaceParam+"}", moduleNamespace, 1))
			req := httptest.NewRequest(method, path, strings.NewReader("{}"))

			if _, _, toModule := moduleTarget(req, plane); toModule {
				t.Errorf("%s %s takes a module's staged limit", method, pattern)
			}

			return nil
		})
	require.NoError(t, err)
	require.NotEmpty(t, core, "this gate walked no core operation under a module's namespace")

	// The module's own paths still reach it.
	req := httptest.NewRequest(http.MethodPut, ModuleMount(moduleNamespace)+"/files/raw", strings.NewReader("{}"))
	_, _, toModule := moduleTarget(req, plane)
	require.True(t, toModule)
}
