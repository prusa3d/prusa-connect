package session

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/platform/auth"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// TestPrincipalResolvesASessionAndRefusesABrokerCredential is the cross-transport
// rule at the surface a request actually reaches, worded the same as an invalid
// token on both HTTP and SSE.
func TestPrincipalResolvesASessionAndRefusesABrokerCredential(t *testing.T) {
	t.Parallel()

	store := auth.NewMemoryStore()
	principals := NewPrincipals(auth.NewVerifier(store, nil), nil, streamPath)

	session, _, err := auth.Issue(t.Context(), store, "operator", auth.KindSession, time.Now())
	require.NoError(t, err)

	broker, _, err := auth.Issue(t.Context(), store, "operator", auth.KindMQTT, time.Now())
	require.NoError(t, err)

	apiToken, _, err := auth.IssueAPI(
		t.Context(),
		store,
		auth.APISpec{Subject: "operator", Name: "script", Scopes: []string{"read"}},
		time.Now(),
	)
	require.NoError(t, err)

	invalid, _, _, err := auth.Mint()
	require.NoError(t, err)

	cases := []struct {
		name     string
		token    auth.Token
		asStream bool
		admitted bool
	}{
		{name: "session on http", token: session, admitted: true},
		{name: "session on sse", token: session, asStream: true, admitted: true},
		{name: "api on http", token: apiToken, admitted: true},
		{name: "api on sse", token: apiToken, asStream: true, admitted: true},
		{name: "mqtt on http", token: broker},
		{name: "mqtt on sse", token: broker, asStream: true},
		{name: "invalid on http", token: invalid},
		{name: "invalid on sse", token: invalid, asStream: true},
		{name: "nothing at all on http", token: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			subject, resolveErr := principals.Principal(requestWith(t, tc.token, tc.asStream))
			if tc.admitted {
				require.NoError(t, resolveErr)
				require.Equal(t, "operator", subject)

				return
			}

			require.Error(t, resolveErr)
			require.Empty(t, subject)
			require.True(t, auth.Refused(resolveErr))
		})
	}
}

// TestAnAPIKeyResolvesAsTheSameCredentialAsItsBearerToken is the API key
// header of ADR-20261119-w4q: a client that cannot send a bearer token is
// the same caller with the same credential, and a revoked or invented key is
// refused as a bearer token is.
func TestAnAPIKeyResolvesAsTheSameCredentialAsItsBearerToken(t *testing.T) {
	t.Parallel()

	store := auth.NewMemoryStore()
	principals := NewPrincipals(auth.NewVerifier(store, nil), nil, streamPath)

	apiToken, _, err := auth.IssueAPI(
		t.Context(),
		store,
		auth.APISpec{Subject: "operator", Name: "slicer", Scopes: []string{"read"}},
		time.Now(),
	)
	require.NoError(t, err)

	invalid, _, _, err := auth.Mint()
	require.NoError(t, err)

	keyed := func(token auth.Token) *http.Request {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
		request.Header.Set(APIKeyHeader, string(token))

		return request
	}

	byKey, err := principals.Resolve(keyed(apiToken))
	require.NoError(t, err)

	byBearer, err := principals.Resolve(requestWith(t, apiToken, false))
	require.NoError(t, err)
	require.Equal(t, byBearer.PublicID, byKey.PublicID)
	require.Equal(t, byBearer.Subject, byKey.Subject)
	require.Equal(t, byBearer.Scopes, byKey.Scopes)

	_, err = principals.Resolve(keyed(invalid))
	require.True(t, auth.Refused(err))
}

// TestABrokerCredentialIsRefusedInTheSameWordsAsRubbish compares the two
// refusals field by field, which is the only way to know they are the same.
func TestABrokerCredentialIsRefusedInTheSameWordsAsRubbish(t *testing.T) {
	t.Parallel()

	store := auth.NewMemoryStore()
	principals := NewPrincipals(auth.NewVerifier(store, nil), nil, streamPath)

	broker, _, err := auth.Issue(t.Context(), store, "operator", auth.KindMQTT, time.Now())
	require.NoError(t, err)

	rubbish, _, _, err := auth.Mint()
	require.NoError(t, err)

	for _, asStream := range []bool{false, true} {
		transport := "http"
		if asStream {
			transport = "sse"
		}

		t.Run(transport, func(t *testing.T) {
			t.Parallel()

			_, brokerErr := principals.Principal(requestWith(t, broker, asStream))
			_, rubbishErr := principals.Principal(requestWith(t, rubbish, asStream))

			require.Error(t, brokerErr)
			require.Error(t, rubbishErr)

			var brokerFault, rubbishFault *fault.Fault
			require.ErrorAs(t, brokerErr, &brokerFault)
			require.ErrorAs(t, rubbishErr, &rubbishFault)

			require.Equal(t, brokerFault.Code(), rubbishFault.Code())
			require.Equal(t, brokerFault.Status(), rubbishFault.Status())
			require.Equal(t, brokerFault.Title(), rubbishFault.Title())
			require.Equal(t, brokerFault.Message(), rubbishFault.Message())
			require.True(t, brokerFault.Redacted())

			t.Logf("%s: broker %q / rubbish %q, both %s %d",
				transport, brokerFault.Message(), rubbishFault.Message(),
				brokerFault.Code(), brokerFault.Status())
		})
	}
}

// TestRequireIsFailClosed is the property the whole layer rests on. Every way a
// check can fail to run is a refusal, including the two that are easiest to get
// backwards: no verifier at all, and a store that cannot answer.
func TestRequireIsFailClosed(t *testing.T) {
	t.Parallel()

	store := auth.NewMemoryStore()
	verifier := auth.NewVerifier(store, nil)

	token, _, err := auth.Issue(t.Context(), store, "operator", auth.KindSession, time.Now())
	require.NoError(t, err)

	cred, err := Require(t.Context(), verifier, token, auth.TransportHTTP)
	require.NoError(t, err, "the good case is good, so the refusals below are the rule")
	require.Equal(t, "operator", cred.Subject)

	t.Run("no verifier wired", func(t *testing.T) {
		t.Parallel()

		_, refusal := Require(t.Context(), nil, token, auth.TransportHTTP)
		require.Error(t, refusal, "a server with no verifier must refuse every request")
		require.True(t, auth.Refused(refusal))
		require.ErrorIs(t, auth.UnwrapCause(refusal), errNoVerifier)
	})

	t.Run("store cannot answer", func(t *testing.T) {
		broken := auth.NewMemoryStore()
		brokenVerifier := auth.NewVerifier(broken, nil)

		brokenToken, _, issueErr := auth.Issue(t.Context(), broken, "operator", auth.KindSession, time.Now())
		require.NoError(t, issueErr)

		broken.Break(errors.New("the credential store is unreachable"))

		_, refusal := Require(t.Context(), brokenVerifier, brokenToken, auth.TransportHTTP)
		require.Error(t, refusal, "an unreachable store must refuse, not admit")
		require.True(t, auth.Refused(refusal))
	})

	t.Run("unknown transport", func(t *testing.T) {
		t.Parallel()

		_, refusal := Require(t.Context(), verifier, token, auth.TransportUnknown)
		require.Error(t, refusal, "a transport nobody named accepts nothing")
	})
}

// TestTokenFromReadsEveryCarrier is the wire contract: a Bearer header for an
// api credential, the X-Api-Key header for a client that cannot send one, and
// an HttpOnly cookie for a session one.
func TestTokenFromReadsEveryCarrier(t *testing.T) {
	t.Parallel()

	// A token another issuer signed, and text of no credential's shape.
	const (
		signedJWT = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.c2ln"
		shapeless = "0123456789abcdef"
	)

	cases := []struct {
		name   string
		set    func(*http.Request)
		expect auth.Token
	}{
		{name: "nothing", set: func(*http.Request) {}, expect: ""},
		{
			name:   "bearer",
			set:    func(r *http.Request) { r.Header.Set("Authorization", "Bearer pcl_abc") },
			expect: "pcl_abc",
		},
		{
			name:   "bearer in lower case",
			set:    func(r *http.Request) { r.Header.Set("Authorization", "bearer pcl_abc") },
			expect: "pcl_abc",
		},
		{
			name:   "cookie",
			set:    func(r *http.Request) { r.AddCookie(&http.Cookie{Name: CookieName, Value: "pcl_cookie"}) },
			expect: "pcl_cookie",
		},
		{
			name: "bearer wins over cookie",
			set: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer pcl_header")
				r.AddCookie(&http.Cookie{Name: CookieName, Value: "pcl_cookie"})
			},
			expect: "pcl_header",
		},
		{
			name:   "api key",
			set:    func(r *http.Request) { r.Header.Set(APIKeyHeader, "pcl_key") },
			expect: "pcl_key",
		},
		{
			name: "bearer wins over api key",
			set: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer pcl_header")
				r.Header.Set(APIKeyHeader, "pcl_key")
			},
			expect: "pcl_header",
		},
		{
			name: "api key wins over cookie",
			set: func(r *http.Request) {
				r.Header.Set(APIKeyHeader, "pcl_key")
				r.AddCookie(&http.Cookie{Name: CookieName, Value: "pcl_cookie"})
			},
			expect: "pcl_key",
		},
		{
			name:   "an api key another issuer signed is not read",
			set:    func(r *http.Request) { r.Header.Set(APIKeyHeader, signedJWT) },
			expect: "",
		},
		{
			name:   "an api key of another shape is not read",
			set:    func(r *http.Request) { r.Header.Set(APIKeyHeader, shapeless) },
			expect: "",
		},
		{
			name:   "another scheme is not a token",
			set:    func(r *http.Request) { r.Header.Set("Authorization", "Basic dXNlcjpwYXNz") },
			expect: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
			tc.set(request)
			require.Equal(t, tc.expect, TokenFrom(request))
		})
	}
}

// TestTransportOfReadsTheRouteThenTheAcceptHeader covers both signals. The
// route is the one that cannot be chosen by the caller, and Accept is read
// after it and only ever to refuse more.
func TestTransportOfReadsTheRouteThenTheAcceptHeader(t *testing.T) {
	t.Parallel()

	principals := NewPrincipals(nil, nil, streamPath)

	cases := []struct {
		name   string
		path   string
		accept string
		want   auth.Transport
	}{
		{name: "a call with no accept", path: callPath, want: auth.TransportHTTP},
		{name: "a call asking for json", path: callPath, accept: "application/json", want: auth.TransportHTTP},
		{name: "the stream route with no accept", path: streamPath, want: auth.TransportSSE},
		{
			name:   "the stream route asking for json",
			path:   streamPath,
			accept: "application/json",
			want:   auth.TransportSSE,
		},
		{name: "the stream route with a trailing slash", path: streamPath + "/", want: auth.TransportSSE},
		{name: "a call asking for a stream", path: callPath, accept: "text/event-stream", want: auth.TransportSSE},
		{
			name:   "a call asking for a stream with a charset",
			path:   callPath,
			accept: "text/event-stream; charset=utf-8",
			want:   auth.TransportSSE,
		},
		{
			name:   "a call listing a stream among others",
			path:   callPath,
			accept: "application/json, text/event-stream",
			want:   auth.TransportSSE,
		},
		{name: "a call shouting for a stream", path: callPath, accept: "TEXT/EVENT-STREAM", want: auth.TransportSSE},
		{
			name:   "a call asking for something else",
			path:   callPath,
			accept: "text/event-streamer",
			want:   auth.TransportHTTP,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.accept != "" {
				request.Header.Set("Accept", tc.accept)
			}

			require.Equal(t, tc.want, principals.TransportOf(request))
		})
	}
}

// TestTheRouteDecidesTheTransportAndNotTheCaller is the defect the paths close.
// A caller who can choose which of the two transports they are judged as has
// chosen the acceptance rule: a Bearer credential once opened the event stream
// by leaving Accept off, and the refusal beside it went on passing.
//
// The kind that was refused on the stream then is accepted there now, so the
// probe is the one kind that is still refused on both, and what the case holds
// is that nothing in the request can move the judgement off the route. A kind a
// later release admits on one transport and not the other is admitted on the
// route's say-so and never on the caller's.
func TestTheRouteDecidesTheTransportAndNotTheCaller(t *testing.T) {
	t.Parallel()

	store := auth.NewMemoryStore()
	principals := NewPrincipals(auth.NewVerifier(store, nil), nil, streamPath)

	broker, _, err := auth.Issue(t.Context(), store, "operator", auth.KindMQTT, time.Now())
	require.NoError(t, err)

	sessionToken, _, err := auth.Issue(t.Context(), store, "operator", auth.KindSession, time.Now())
	require.NoError(t, err)

	cases := []struct {
		name     string
		token    auth.Token
		accept   string
		admitted bool
	}{
		{name: "a broker credential on the stream route with no accept", token: broker},
		{name: "a broker credential on the stream route asking for json", token: broker, accept: "application/json"},
		{
			name:   "a broker credential on the stream route asking for a stream",
			token:  broker,
			accept: "text/event-stream",
		},
		{name: "a session credential on the stream route with no accept", token: sessionToken, admitted: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, streamPath, nil).WithContext(t.Context())
			request.Header.Set("Authorization", "Bearer "+string(tc.token))

			if tc.accept != "" {
				request.Header.Set("Accept", tc.accept)
			}

			require.Equal(t, auth.TransportSSE, principals.TransportOf(request),
				"the stream route was judged as something other than a stream")

			subject, resolveErr := principals.Principal(request)
			if tc.admitted {
				require.NoError(t, resolveErr)
				require.Equal(t, "operator", subject)

				return
			}

			require.Error(t, resolveErr, "the stream route admitted a credential its kind is refused on")
			require.Empty(t, subject)
			require.True(t, auth.Refused(resolveErr))
		})
	}
}

// The two routes the cases above use. streamPath is the one this server serves
// an event stream on, and callPath is any other operation.
const (
	streamPath = "/api/v1/events"
	callPath   = "/api/v1/devices"
)

// TestOnlyAPresentedCredentialIsLogged holds the line count on a server nobody
// is signed in to. The readiness probe reads its endpoint every ten seconds and
// holds no credential, so a warning per refusal wrote six lines a minute saying
// a token was malformed where no token was read.
func TestOnlyAPresentedCredentialIsLogged(t *testing.T) {
	t.Parallel()

	rubbish, _, _, err := auth.Mint()
	require.NoError(t, err)

	cases := []struct {
		name   string
		token  auth.Token
		logged bool
	}{
		{name: "nothing presented"},
		{name: "a token that does not resolve", token: rubbish, logged: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var written bytes.Buffer

			logger := slog.New(slog.NewJSONHandler(&written, nil))
			principals := NewPrincipals(auth.NewVerifier(auth.NewMemoryStore(), nil), logger, streamPath)

			_, resolveErr := principals.Principal(requestWith(t, tc.token, false))
			require.Error(t, resolveErr, "the refusal is the same either way")
			require.True(t, auth.Refused(resolveErr))

			require.Equal(t, tc.logged, strings.Contains(written.String(), "credential refused"))
		})
	}
}

func requestWith(t *testing.T, token auth.Token, asStream bool) *http.Request {
	t.Helper()

	path := callPath
	if asStream {
		path = streamPath
	}

	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(t.Context())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+string(token))
	}

	return request
}
