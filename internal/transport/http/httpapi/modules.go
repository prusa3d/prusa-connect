package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// modulesPathPrefix is the subtree every module's own surface is served under.
// A module claims nothing outside it, except a root prefix an operator names
// for it. The group is a static prefix this server writes, the namespace is
// one path segment, and everything after it is the module's.
const modulesPathPrefix = "/api/v1/modules"

// modulesRoutePattern is the single route the mount registers. It is one
// pattern and not one per module. A module is declared configuration, and an
// operator adds and removes one while the process runs, so a route per module
// would need the router rebuilt each time.
//
// The namespace is a path parameter and not a literal, so no module string
// reaches chi's pattern table and no module can register a pattern.
const modulesRoutePattern = modulesPathPrefix + "/{namespace}/*"

// namespaceParam is the segment naming which module a request is for.
const namespaceParam = "namespace"

// ModuleSurface is one module's HTTP surface as the plane hands it over: the
// handler that answers, and the configured instance whose grants decide who may
// reach it.
//
// Instance is required. A surface carrying no instance has nothing to
// authorize on, and the mount refuses to serve it.
type ModuleSurface struct {
	// Instance is the declared instance the module runs as. The authorization
	// check names it. It is the host's own record of the module, and nothing
	// the module said about itself.
	Instance ref.Ref
	// Handler answers the request with the group prefix taken off, so a module
	// serving /spools is reached at /api/v1/modules/<namespace>/spools and
	// writes its own routes as though it owned the origin.
	Handler http.Handler
	// OpenAPI is the module's fragment rebased under its mount, as the plane
	// rendered it at launch, and nil for a module that shipped none. It is
	// served at ModuleOpenAPIPath and never by the module.
	OpenAPI []byte
	// Mount is the root prefix an operator named for the module, such as
	// /graphql, and empty for a module reached under its namespace. A module
	// with a Mount serves its routes only there. The namespace path still
	// serves its document, so a client finds it at one address whatever the
	// operator chose.
	Mount string
	// Unavailable is why a module that claimed Mount is not answering on it,
	// and empty for one that is. It is set for an instance that is declared and
	// not serving: switched on and never installed, failed to launch, or
	// switched off after it had claimed the prefix.
	//
	// A claimed prefix with nothing behind it answers 503 and not 404, so an
	// integration can tell "switched off here" from "wrong server". The
	// reason is the host's own words and names no path on disk.
	Unavailable string
	// ModulesOff is set for a prefix whose installed module the operator's
	// core.modules_enabled: false keeps off. The prefix then answers
	// MODULES_DISABLED instead of the 503: nothing changes until the operator
	// restarts the server, so a client must not retry.
	ModulesOff bool
	// SafeMode says the key is safe mode, so the prefix answers SAFE_MODE
	// instead of MODULES_DISABLED.
	SafeMode bool
	// Public is the routes the operator approved from the manifest's
	// routes.public. A request on one reaches the module with no caller
	// resolved and no permission checked, and with an empty principal
	// (ADR-20261120-xys rules 1 to 3). Every other route is held to the
	// instance's grants.
	Public []manifest.Route
	// Staged is the routes whose request body the host writes into the
	// module's data directory, each with the limit the operator approved. It
	// is empty for a module granted no data directory, whose bodies go on the
	// wire. The body limit middleware holds a request on one of them to that
	// limit, and every other module request to the document bound
	// (ADR-20261120-xys rule 6).
	Staged []manifest.StagedRoute
	// Title and Summary are the module's own, from its manifest. A browser
	// that opens the module's address with no credential reads them on the
	// page the core answers instead of the problem document.
	Title   string
	Summary string
	// Themes is the stylesheet of each theme the module's bundle offers, by
	// theme id, as a path inside the bundle such as ui/ocean.css. A theme is
	// drawn on the sign-in page too, so a stylesheet, and the fonts and
	// pictures in its directory, are served with no caller resolved.
	Themes map[string]string
}

// ModulePlane answers which module claims one namespace. It is the seam between
// this transport and the supervision plane. The mount knows a namespace and a
// path, and no more about what a module is.
//
// The host derives the namespace from the module's own identity and never
// reads it from the module, so two modules cannot claim one prefix and no
// module can name another's.
type ModulePlane interface {
	// Module returns the surface mounted under namespace, and false when
	// nothing claims it. False is answered as an unrouted path, so a caller
	// cannot enumerate which modules a deployment runs.
	Module(namespace string) (ModuleSurface, bool)
	// Root returns the surface whose root mount path is under, with Mount set,
	// and false when no module claimed one there. The router asks it only for
	// a path no route of its own answers, so a module's claim never shadows
	// an operation whatever the plane says.
	Root(path string) (ModuleSurface, bool)
}

// ModuleAccess is the authorization graph as a module's mount reads it.
// *authz.Store satisfies it with no adapter, so a module route is decided by
// the same call, against the same projected row, as every other path under
// /api/v1.
type ModuleAccess interface {
	// RequireResource reports nil when who may perform action on instance, and
	// a fault carrying ACCESS_DENIED otherwise.
	RequireResource(ctx context.Context, who authz.Subject, action authz.Action, instance ref.Ref) error
}

// moduleAction is the permission a request needs on the module's instance.
//
// The rest of the resource surface makes the same split. A safe method reads,
// which is view_resource. Anything else may change what the module holds,
// which is operate_resource. configure_resource is not reachable here.
// Rewriting a module's declaration is the resource API's own operation, and a
// module route must not be a second way to do it.
func moduleAction(method string) authz.Action {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return authz.ActionViewResource
	}

	return authz.ActionOperateResource
}

// mountModules registers the one route every module's surface is reached
// through.
//
// It is registered after every operation, and a module cannot shadow one. chi
// prefers the literal segments of /api/v1/health to this pattern, and this
// pattern cannot match outside its own subtree. It is registered whatever a
// deployment wired, so the address behaves the same on a build with no module
// plane as on one with a module that is not installed.
func (s *Server) mountModules(cfg Config) {
	s.mux.Handle(modulesRoutePattern, s.moduleHandler(cfg))
}

// moduleHandler makes every check between a caller and a module.
//
// The order matters. The namespace is checked against the grammar first. A
// segment no module could have is an unrouted path, and the answer must not
// say a module was looked for. The claim is resolved next, and an unclaimed
// namespace answers exactly as an unrouted path does. Identity and the
// permission come last, before anything of the module's runs, so a caller who
// holds nothing never reaches the module. A module reached before the check
// would already have read the request and done the work.
func (s *Server) moduleHandler(cfg Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		namespace := chi.URLParam(r, namespaceParam)
		if extension.Namespace(namespace).Validate() != nil {
			notFound(w, r)

			return
		}

		if cfg.Modules == nil {
			notFound(w, r)

			return
		}

		surface, claimed := cfg.Modules.Module(namespace)
		if !claimed || surface.Handler == nil || surface.Instance.IsZero() {
			notFound(w, r)

			return
		}

		// A module an operator mounted at a root prefix is served there and
		// not here. Its document is still served under the namespace, by the
		// operation registered for it, which chi prefers to this pattern.
		if surface.Mount != "" {
			notFound(w, r)

			return
		}

		inner, ok := mountedRequest(ModuleMount(namespace), r)
		if !ok {
			notFound(w, r)

			return
		}

		caller, allowed := s.moduleCaller(w, r, inner, cfg, surface)
		if !allowed {
			return
		}

		// The principal is on the context, so the plane forwards the host's
		// own answer about who is calling and the module does not read a
		// header the caller wrote. The scopes are on it too, so the plane
		// limits what the module asks for on this person's behalf to them.
		surface.Handler.ServeHTTP(w, inner.WithContext(withCaller(inner.Context(), caller)))
	})
}

// moduleCaller is who the module serves on this request: nobody on a public
// route, and the caller allowModule resolved on any other. inner is the
// request with the mount taken off, which is the path a route names.
//
// A public route resolves no caller at all, so a session cookie or a token
// the request carries changes nothing there and the module reads an empty
// principal. Answering a public route as the person who happens to be signed
// in would let a module tell two callers apart on a route the operator
// approved as one that knows nobody.
func (s *Server) moduleCaller(
	w http.ResponseWriter, r, inner *http.Request, cfg Config, surface ModuleSurface,
) (Caller, bool) {
	for _, route := range surface.Public {
		if route.Matches(inner.Method, inner.URL.Path) {
			return Caller{}, true
		}
	}

	if themeFile(inner, surface.Themes) {
		return Caller{}, true
	}

	return s.allowModule(w, r, cfg, surface)
}

// allowModule resolves the caller and asks the graph, writing the refusal
// itself and reporting whether the module may be reached.
//
// A server with a module plane and no graph refuses everything. New will not
// build such a server, and the check is here anyway, because without it a
// missing graph would open the module surface to anybody.
func (s *Server) allowModule(
	w http.ResponseWriter, r *http.Request, cfg Config, surface ModuleSurface,
) (Caller, bool) {
	caller, err := cfg.Principals.Caller(r)
	if err != nil {
		refusal := identityFault(err)

		logRefusal(r.Context(), s.logger, r.URL.Path, err)

		if refusal.Status() == http.StatusUnauthorized && wantsAddressPage(r) {
			writeModuleAddressPage(w, r, surface, refusal.Status())

			return Caller{}, false
		}

		writeProblem(w, r, refusal)

		return Caller{}, false
	}

	if cfg.ModuleAccess == nil {
		writeProblem(w, r, httpfault.Unexpected("no authorization graph stands behind this module"))

		return Caller{}, false
	}

	// The principal is parsed and not assumed to be a user. The Principals
	// implementation renders the subject the graph is keyed by, type
	// included, so adding a prefix here would name a principal nobody is.
	who, err := authz.ParseSubject(caller.Principal)
	if err != nil {
		writeProblem(w, r, moduleFault(err))

		return Caller{}, false
	}

	if err = cfg.ModuleAccess.RequireResource(
		authz.WithScopes(r.Context(), caller.Scopes), who, moduleAction(r.Method), surface.Instance,
	); err != nil {
		writeProblem(w, r, moduleFault(err))

		return Caller{}, false
	}

	return caller, true
}

// mountedRequest takes the mount prefix off, and reports false for a path that
// tried to leave it. The prefix is the namespace group or the root mount an
// operator named; the module sees the same relative path under either.
//
// Nothing is cleaned or rewritten. A dot segment in the remainder is refused
// and not resolved, because a module handler may map a path onto a file name,
// and a traversal that never reaches the module cannot trick it. chi matches
// literal segments, so such a path reaches no other route here either. A path
// plainPath refuses is not served either, because the module could read it
// as another route than the one the host authorized.
func mountedRequest(prefix string, r *http.Request) (*http.Request, bool) {
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == r.URL.Path {
		return nil, false
	}

	if rest == "" {
		rest = "/"
	}

	if !strings.HasPrefix(rest, "/") {
		return nil, false
	}

	for segment := range strings.SplitSeq(rest, "/") {
		if segment == "." || segment == ".." {
			return nil, false
		}
	}

	if !plainPath(rest) {
		return nil, false
	}

	// Clone deep-copies the URL, so the rewrite below is on this request alone
	// and the address the outer layers logged is the one the caller sent.
	inner := r.Clone(r.Context())
	inner.URL.Path = rest
	inner.URL.RawPath = ""

	return inner, true
}

// plainPath reports whether a decoded path reads the same when it is parsed
// as a URL again. The host matches public and staged routes against this path,
// and a module built with an older SDK parses it again before its router sees
// it. A ? or # the caller sent encoded would start a query or a fragment
// there, a % would be decoded a second time, and a control character is
// refused by the parse. Any of them lets the host and the module read two
// different routes, so a path that holds one reaches no module at all.
func plainPath(path string) bool {
	return !strings.ContainsAny(path, "?#%") &&
		!strings.ContainsFunc(path, func(r rune) bool { return r < ' ' || r == '\x7f' })
}

// moduleFault renders what the graph reported. A refusal the graph raised
// carries its own code and status. Anything else is a failure of this server
// and is answered as one, without the cause in the body.
func moduleFault(err error) *fault.Fault {
	if f, ok := errors.AsType[*fault.Fault](err); ok {
		return f
	}

	return httpfault.Unexpected("the authorization graph could not decide")
}

// WriteProblem renders a Fault as this server's own problem document, for a
// module plane that has to refuse a request before the module is reached.
//
// It is exported so that a refusal has one rendering. A plane that wrote its
// own would answer in a format no client of this API can decode.
func WriteProblem(w http.ResponseWriter, r *http.Request, f *fault.Fault) {
	writeProblem(w, r, f)
}

// ModulePrincipalOf returns the principal the mount resolved and authorized,
// for a plane forwarding the request into a module.
//
// It is the host's answer to who is calling, and it is empty on any request
// that did not come through the mount. A module reads it and does not trust a
// header, because the caller writes a header and cannot write this.
func ModulePrincipalOf(ctx context.Context) string { return principalFrom(ctx) }
