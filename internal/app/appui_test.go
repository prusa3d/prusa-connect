package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// TestTheAppIsMountedOnlyWhereTheConfigurationAsksForIt is the owner's rule of
// 2026-09-14 at the seam that carries it: a server nobody configured serves its
// own app, and core.frontend.enabled false leaves the origin root exactly as a
// build carrying no app leaves it.
//
// The two answers a caller sees are the same 404, and the two states are
// reported apart, because an operator who switched the app off and one who
// built a binary without it need different things done about it.
func TestTheAppIsMountedOnlyWhereTheConfigurationAsksForIt(t *testing.T) {
	t.Parallel()

	_, carried := httpapi.AppMount()

	beside := []httpapi.Mount{{Pattern: "/p/ws", Handler: nil, Reason: "a device wire"}}

	on := config.Default().Frontend
	off := on
	off.Enabled = new(false)

	mountedOn, stateOn := withCoreApp(beside, on)
	mountedOff, stateOff := withCoreApp(beside, off)

	require.Len(t, mountedOff, len(beside), "the app was mounted on a deployment that switched it off")

	if !carried {
		require.Equal(t, httpapi.AppAbsent, stateOn, "a build carrying no app reported one")
		require.Equal(t, httpapi.AppAbsent, stateOff)
		require.Len(t, mountedOn, len(beside))

		return
	}

	require.Equal(t, httpapi.AppServed, stateOn)
	require.Equal(t, httpapi.AppOff, stateOff)
	require.Len(t, mountedOn, len(beside)+1, "the app was not mounted on a build that carries it")
}

// TestThePageIsToldHowToLookWhateverTheAnswerIs covers what GET /api/v1/ui
// sends. Enabled is where the app stands and not what the file says, so a
// client reading it from a build with no app is told the page is not here
// rather than told to expect one. The three appearance keys travel either way:
// a deployment serving its own page still reads the name and the theme this
// server was configured with.
func TestThePageIsToldHowToLookWhateverTheAnswerIs(t *testing.T) {
	t.Parallel()

	frontend := config.Default().Frontend
	frontend.Title = "Workshop floor"

	for state, enabled := range map[string]bool{
		httpapi.AppServed: true,
		httpapi.AppOff:    false,
		httpapi.AppAbsent: false,
	} {
		settings := appSettings(frontend, config.Diagnostics{}, state)

		require.Equalf(t, enabled, settings.Enabled, "state %s", state)
		require.Equal(t, "Workshop floor", settings.Title)
		require.Equal(t, config.DefaultLanguage, settings.DefaultLanguage)
		require.Equal(t, config.DefaultTheme, settings.Theme)
		require.Equal(t, config.DefaultDiagnosticsSendURL, settings.DiagnosticsSendURL)
	}
}

// TestAnEmptySendAddressHidesSendToPrusa covers the one way an operator turns
// the upload off: the key written empty. The page then gets no address and
// offers the download alone.
func TestAnEmptySendAddressHidesSendToPrusa(t *testing.T) {
	t.Parallel()

	off := appSettings(config.Default().Frontend,
		config.Diagnostics{SendURL: new("")}, httpapi.AppServed)
	require.Empty(t, off.DiagnosticsSendURL)

	own := appSettings(config.Default().Frontend,
		config.Diagnostics{SendURL: new("https://support.example/")}, httpapi.AppServed)
	require.Equal(t, "https://support.example", own.DiagnosticsSendURL)
}
