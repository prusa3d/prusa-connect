package session

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/platform/auth"
)

// errNoVerifier is the cause behind a refusal from a server that was wired
// without a verifier. It reaches a log line and never a client.
var errNoVerifier = errors.New("session: no verifier is wired")

// CookieName carries a session credential in the browser. The handler that sets
// it marks it HttpOnly and SameSite=Lax, so no script can read it.
const CookieName = "pcl_session"

// BearerCookieName is the cookie the Prusa Connect web client keeps its Prusa
// Account token in. It is read only when neither the Authorization header nor
// the session cookie carries anything, and whatever it holds is judged as a
// bearer token and never as a session. This server does not set it, so it
// cannot vouch for its flags.
const BearerCookieName = "auth.access_token" //nolint:gosec // a cookie name, not a credential

// bearerPrefix is the scheme RFC 6750 section 2.1 defines. The comparison is
// case-insensitive, as the RFC's grammar requires.
const bearerPrefix = "bearer "

// Bearer resolves a token another issuer signed, which internal/platform/auth
// does through an identity plugin and a pairing. It is an interface here so the
// composition root can hand in one that reads the running identity instance.
type Bearer interface {
	Resolve(ctx context.Context, token auth.Token, on auth.Transport) (auth.Credential, error)
}

// Principals resolves an HTTP or SSE request to the subject behind it. It
// satisfies the port internal/transport/http/httpapi declares, by having its
// method, so this package does not import transport.
type Principals struct {
	verifier *auth.Verifier
	bearer   Bearer
	logger   *slog.Logger
	// streams are the paths this server serves an event stream on, from the
	// package that owns the routes. See TransportOf for why they are here.
	streams []string
}

// NewPrincipals returns a resolver over a verifier. A nil logger discards.
//
// streamPaths are the request paths this server serves an event stream on. A
// credential presented on one is judged as SSE whatever the request's own
// headers say, so the caller cannot choose the acceptance rule.
func NewPrincipals(verifier *auth.Verifier, logger *slog.Logger, streamPaths ...string) *Principals {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Principals{verifier: verifier, logger: logger, streams: streamPaths}
}

// WithBearer returns a resolver that also admits a token another issuer signed,
// through bearer. Without one, a token of that shape is refused the way rubbish
// is. The verifier for this server's own tokens is unchanged. The token's shape
// decides which of the two checks it, never the caller.
func (p *Principals) WithBearer(bearer Bearer) *Principals {
	next := *p
	next.bearer = bearer

	return &next
}

// Principal returns the subject a request authenticates as, or the one refusal
// this package ever gives.
func (p *Principals) Principal(r *http.Request) (string, error) {
	cred, err := p.Resolve(r)
	if err != nil {
		return "", err
	}

	return cred.Subject, nil
}

// Resolve returns the credential a request carries, or the one refusal this
// package ever gives. The transport is read from the request and not
// configured, because the same handler chain serves both.
//
// It returns the whole row and not the subject alone, because the kind says how
// the subject is spelled. A session names a bare id the composition root types,
// and an api credential names the graph subject it was minted for, which may
// not be a person.
func (p *Principals) Resolve(r *http.Request) (auth.Credential, error) {
	on := p.TransportOf(r)
	token := TokenFrom(r)

	var (
		cred auth.Credential
		err  error
	)

	// A token another issuer signed is shaped unlike one this server minted,
	// and the shape picks the check. A bearer token on a server with no bearer
	// path falls through to the verifier, which refuses it as rubbish.
	if p.bearer != nil && auth.IsBearerToken(token) {
		cred, err = p.bearer.Resolve(r.Context(), token, on)
	} else {
		cred, err = Require(r.Context(), p.verifier, token, on)
	}

	if err != nil {
		// A request that presented nothing is logged at debug only: a browser
		// asks who it is before anybody signs in, and the probe on
		// /api/v1/readiness carries no credential and reads that endpoint
		// every ten seconds. The refusal is unchanged: an absent credential is
		// still refused and the caller still gets the one worded fault.
		if token == "" {
			p.logger.DebugContext(r.Context(), "no credential presented",
				slog.String("path", r.URL.Path),
				slog.String("transport", on.String()))
		} else {
			p.logger.WarnContext(r.Context(), "credential refused",
				slog.String("path", r.URL.Path),
				slog.String("transport", on.String()),
				slog.Any("cause", auth.UnwrapCause(err)))
		}

		return auth.Credential{}, err
	}

	return cred, nil
}

// Require resolves a token on a transport, or refuses. No path through it
// admits a caller whose credential did not verify. An empty token, a store that
// cannot answer and an unreachable issuer all reach the same refusal, worded
// the same way.
func Require(
	ctx context.Context,
	verifier *auth.Verifier,
	token auth.Token,
	on auth.Transport,
) (auth.Credential, error) {
	if verifier == nil {
		// A server wired with no verifier refuses every request. That is what
		// fail-closed means here.
		return auth.Credential{}, auth.Refuse(errNoVerifier)
	}

	cred, err := verifier.Verify(ctx, token, on)
	if err != nil {
		return auth.Credential{}, err
	}

	return cred, nil
}

// APIKeyHeader carries a credential this server minted, for a client that
// cannot send an Authorization header with a bearer token. PrusaSlicer 2 sends
// the API key of a PrusaLink printer in it (ADR-20261119-w4q). A browser sends
// it to another site only after a CORS preflight, so a page on another site
// cannot make a browser send it.
const APIKeyHeader = "X-Api-Key" //nolint:gosec // a header name, not a credential

// TokenFrom reads the credential a request carries. A Bearer header wins over
// the API key header, and both win over a cookie, so a caller holding several
// says which one it means. The session cookie wins over the Prusa Account one.
//
// The API key header is read only for a credential this server minted, with
// its prefix. A token another issuer signed travels as a bearer token only.
//
// The Prusa Account cookie is admitted only for text shaped like a bearer
// token. This server never sets that cookie, so a minted token found in it was
// put there by something else. Reading it would accept a session from a cookie
// that lacks the flags the real session cookie carries.
func TokenFrom(r *http.Request) auth.Token {
	header := r.Header.Get("Authorization")
	if len(header) > len(bearerPrefix) && strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return auth.Token(strings.TrimSpace(header[len(bearerPrefix):]))
	}

	if key := auth.Token(strings.TrimSpace(r.Header.Get(APIKeyHeader))); isMinted(key) {
		return key
	}

	if cookie, err := r.Cookie(CookieName); err == nil {
		return auth.Token(cookie.Value)
	}

	if cookie, err := r.Cookie(BearerCookieName); err == nil && auth.IsBearerToken(auth.Token(cookie.Value)) {
		return auth.Token(cookie.Value)
	}

	return ""
}

// isMinted reports whether text is shaped like a credential this server
// minted: it carries the token prefix.
func isMinted(token auth.Token) bool {
	return strings.HasPrefix(string(token), auth.Prefix)
}

// TransportOf reports which transport a request is.
//
// The route decides it, not a header the caller controls. Acceptance depends on
// the transport, so a caller able to choose which transport they are judged as
// could choose the rule that admits them. The paths come from the package that
// registers the routes, through the composition root.
//
// Accept is read as well, and only ever to add a stream. A request asking for
// text/event-stream on any route is judged as a stream, which covers a stream
// route nobody named here.
func (p *Principals) TransportOf(r *http.Request) auth.Transport {
	path := strings.TrimSuffix(r.URL.Path, "/")

	for _, stream := range p.streams {
		if path == strings.TrimSuffix(stream, "/") {
			return auth.TransportSSE
		}
	}

	for accepted := range strings.SplitSeq(r.Header.Get("Accept"), ",") {
		if strings.EqualFold(strings.TrimSpace(strings.Split(accepted, ";")[0]), "text/event-stream") {
			return auth.TransportSSE
		}
	}

	return auth.TransportHTTP
}
