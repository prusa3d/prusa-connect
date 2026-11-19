package httpapi

import (
	"errors"
	"net/http"
)

// ErrWildcardOrigin reports a Config that allows every origin. The
// configuration loader refuses the same value with a message naming the key;
// this is the sealed constructor refusing it from any other caller.
var ErrWildcardOrigin = errors.New("httpapi: Config.AllowedOrigins may not carry *")

// What a cross-origin request may carry and what a client reads back. Both
// lists are written out by hand, because the browser checks its preflight
// against them. A request header missing here is a request the browser never
// sends, and a response header missing here is one a page never sees.
//
// The request headers are the ones an operation binds: the credential, the
// content type, the four conditional headers the resource and file operations
// take, the id an EventSource resumes from, and the trace a client may start.
// The response headers are the two a client reads after a write: the version
// a resource answered with and where a created one lives. The trace id and the
// stream cursor are not headers, so neither is exposed: the id travels in the
// problem document and the cursor in the stream's own id line.
const (
	corsAllowMethods = "GET, HEAD, POST, PUT, PATCH, DELETE"
	corsAllowHeaders = "Accept, Authorization, Content-Type, If-Match, If-Modified-Since, " +
		"If-None-Match, Last-Event-ID, Traceparent"
	corsExposeHeaders = "ETag, Location"
	// corsMaxAge is how long a browser may hold one preflight answer. Ten
	// minutes is the longest Chromium honours whatever the value says.
	corsMaxAge = "600"
)

// corsMiddleware answers a page served from one of the allowed origins the way
// the browser's same-origin policy requires before it lets that page read a
// response, and answers everybody else nothing.
//
// It is only on the API router and never on the device one. A printer sends no
// Origin header and reads no CORS header, and the printer listener serves only
// the printer's wire, which a browser cannot use.
//
// The origin is matched exactly and echoed, never wildcarded, because every
// browser refuses a credentialed request when the answer is *. An origin off
// the list gets no header at all, so its preflight and its request are
// answered as if no list were set. Vary: Origin goes on every response once a
// list is set, allowed or not. The response depends on a request header, and
// a cache without Vary would hand one origin's answer to another.
//
// The list is read for every request, because a person changes it in Settings
// and an extension adds to it while the server runs. A list that is empty at
// the time of a request sends nothing, Vary included.
func corsMiddleware(origins allowedOrigins) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !origins.Any() {
				next.ServeHTTP(w, r)

				return
			}

			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if !origins.Allows(origin) {
				next.ServeHTTP(w, r)

				return
			}

			header := w.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")

			if isPreflight(r) {
				header.Set("Access-Control-Allow-Methods", corsAllowMethods)
				header.Set("Access-Control-Allow-Headers", corsAllowHeaders)
				header.Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)

				return
			}

			header.Set("Access-Control-Expose-Headers", corsExposeHeaders)

			next.ServeHTTP(w, r)
		})
	}
}

// allowedOrigins is the allow-list as the middleware asks it.
type allowedOrigins interface {
	Any() bool
	Allows(origin string) bool
}

// fixedOrigins is the allow-list of a server built with a list and no store.
type fixedOrigins map[string]struct{}

func (f fixedOrigins) Any() bool { return len(f) > 0 }

func (f fixedOrigins) Allows(origin string) bool {
	_, ok := f[origin]

	return ok
}

// originsOf is the allow-list the middleware reads: the store when the
// composition root wired one, else the fixed list.
func originsOf(cfg Config) allowedOrigins {
	if cfg.Origins != nil {
		return cfg.Origins
	}

	fixed := make(fixedOrigins, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		fixed[origin] = struct{}{}
	}

	return fixed
}

// isPreflight reports whether this is the browser asking before it sends,
// which is an OPTIONS naming the method it wants to send. An OPTIONS without
// that header is a request in its own right and goes to the router.
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}
