package diagnostics

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/platform/observability"
)

func entry(msg string, attrs ...slog.Attr) observability.LogEntry {
	return observability.LogEntry{Time: time.Unix(0, 0).UTC(), Level: slog.LevelWarn, Message: msg, Attrs: attrs}
}

func TestLogKeysAreKeptDeniedScrubbedOrHashed(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines := project(t, []observability.LogEntry{entry("dial failed",
		slog.String("trace_id", "0123456789abcdef0123456789abcdef"),
		slog.String("setup_code", "ABCD-1234"),
		slog.String("error", "post https://hooks.example.com/T/x failed"),
		slog.String("principal", "user:42"),
		slog.Int("attempt", 3),
	)}, s, false)

	require.Len(t, lines, 1)
	attrs := lines[0].Attrs
	require.Equal(t, "0123456789abcdef0123456789abcdef", attrs["trace_id"])
	require.NotContains(t, attrs, "setup_code")
	require.Equal(t, "post https://<"+s.Hash("hooks.example.com")+"> failed", attrs["error"])
	require.Equal(t, s.Hash("user:42"), attrs["principal"])
	require.EqualValues(t, 3, attrs["attempt"])
}

func TestModuleStderrIsLeftOutUnlessAskedAndRejoinedWhenSplit(t *testing.T) {
	s := NewScrubber([]byte("salt"), []string{"tok_SPLITSECRET"})
	module := func(msg string, split bool) observability.LogEntry {
		attrs := []slog.Attr{slog.String("instance", "cam"), slog.Bool(observability.ModuleStderrKey, true)}
		if split {
			attrs = append(attrs, slog.Bool("split", true))
		}

		return entry(msg, attrs...)
	}
	entries := []observability.LogEntry{module("auth tok_SPLIT", true), module("SECRET ok", false), entry("core line")}

	require.Len(t, project(t, entries, s, false), 1)

	lines := project(t, entries, s, true)
	require.Len(t, lines, 2)

	rendered, err := json.Marshal(lines)
	require.NoError(t, err)
	require.NotContains(t, string(s.Sweep(rendered)), "SPLITSECRET")
}

func TestFrontendBodyIsBoundedAndScrubbed(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	_, state := frontendLines(nil, s)
	require.Equal(t, "none", state)

	_, state = frontendLines([]byte("not json"), s)
	require.Equal(t, "unreadable", state)

	_, state = frontendLines([]byte(`"`+strings.Repeat("x", maxFrontendBytes)+`"`), s)
	require.Equal(t, "too_large", state)

	lines, state := frontendLines([]byte(`[{"time":"t","level":"error","scope":"api","message":"request failed",
		"fields":{"operation":"get-device","traceId":"abc","status":500,"stack":"at https://pcl.lan/x.js","email":"a@b.cz"}}]`), s)
	require.Equal(t, "included", state)
	require.Equal(t, "get-device", lines[0].Fields["operation"])
	require.Equal(t, "abc", lines[0].Fields["traceId"])
	require.EqualValues(t, 500, lines[0].Fields["status"])
	require.Equal(t, "at https://<"+s.Hash("pcl.lan")+">", lines[0].Fields["stack"])
	require.Equal(t, s.Hash("a@b.cz"), lines[0].Fields["email"])
}

func TestInterleavedModuleInstancesAreRejoinedSeparately(t *testing.T) {
	s := NewScrubber([]byte("salt"), []string{"tok_SPLITSECRET"})
	rec := func(instance, msg string, split bool) observability.LogEntry {
		attrs := []slog.Attr{slog.String("instance", instance), slog.Bool(observability.ModuleStderrKey, true)}
		if split {
			attrs = append(attrs, slog.Bool("split", true))
		}

		return entry(msg, attrs...)
	}

	lines := project(t, []observability.LogEntry{
		rec("a", "auth tok_SPLIT", true), rec("b", "hello", false), rec("a", "SECRET ok", false),
	}, s, true)

	require.Len(t, lines, 2)
	require.Equal(t, "hello", lines[0].Msg)
	require.Equal(t, "b", lines[0].Attrs["instance"])
	require.Equal(t, "auth <secret> ok", lines[1].Msg)
	require.Equal(t, "a", lines[1].Attrs["instance"])
}

func TestUnterminatedSplitIsFlushed(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)
	e := entry(
		"cut off",
		slog.String("instance", "a"),
		slog.Bool(observability.ModuleStderrKey, true),
		slog.Bool("split", true),
	)

	lines := project(t, []observability.LogEntry{e}, s, true)
	require.Len(t, lines, 1)
	require.Equal(t, "cut off", lines[0].Msg)
}

func TestReasonAndResultAreScrubbedAsText(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines := project(t, []observability.LogEntry{entry("x",
		slog.String("reason", "the printer would not say what is in /home/jan/prints"),
		slog.String("result", "see /home/jan/prints"),
	)}, s, false)

	require.NotContains(t, lines[0].Attrs["reason"], "/home/jan/prints")
	require.NotContains(t, lines[0].Attrs["result"], "/home/jan/prints")
}

func TestFrontendUnlistedFieldsAreHashedAndCodesKeptByShape(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines, _ := frontendLines([]byte(`[{"time":"a@b.cz","level":"tok_x 10.0.0.1","scope":"s","message":"m","fields":{
		"name":"Bob's MK4","code":"ABCD-1234"}},
		{"level":"warn","fields":{"code":"core.auth/LOGIN_REFUSED"}}]`), s)

	require.Equal(t, s.Hash("Bob's MK4"), lines[0].Fields["name"])
	require.Equal(t, s.Hash("ABCD-1234"), lines[0].Fields["code"])
	require.Equal(t, "<email>", lines[0].Time)
	require.Empty(t, lines[0].Level)
	require.Equal(t, "core.auth/LOGIN_REFUSED", lines[1].Fields["code"])
	require.Equal(t, "warn", lines[1].Level)
}

func TestFrontendNullAndEmptyArrayAreNone(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	for _, body := range []string{"null", "[]"} {
		_, state := frontendLines([]byte(body), s)
		require.Equal(t, "none", state, body)
	}
}

func TestServerKeysThatNameNothingAreKept(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines := project(t, []observability.LogEntry{entry("readiness",
		slog.String("printer", "d1"),
		slog.String("printer_name", "Bob's MK4"),
		slog.String("theme", "dark"),
		slog.String("language", "cs"),
		slog.Any("blocked", []string{"camera:front"}),
		slog.Any("instances", []string{"camera:front=serving/required"}),
	)}, s, false)

	attrs := lines[0].Attrs
	require.Equal(t, "d1", attrs["printer"])
	require.Equal(t, s.Hash("Bob's MK4"), attrs["printer_name"])
	require.Equal(t, "dark", attrs["theme"])
	require.Equal(t, "cs", attrs["language"])
	require.Equal(t, "[camera:front]", attrs["blocked"])
	require.Equal(t, "[camera:front=serving/required]", attrs["instances"])
}

func TestFrontendParamsKeepOnlySafeValuesOfListedKeys(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines, _ := frontendLines([]byte(`[{"level":"error","fields":{"params":{
		"verb":"set_temperature","argument":"target","code":"core.validation/ARGUMENT_MISSING",
		"state":"two words","printer_name":"Bob","min":3,"name":null}}}]`), s)

	params, ok := lines[0].Fields["params"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"verb": "set_temperature", "argument": "target", "code": "core.validation/ARGUMENT_MISSING",
		"state": s.Hash("two words"), "printer_name": s.Hash("Bob"), "min": s.Hash("3"), "name": nil,
	}, params)
}

func TestFrontendEntryWithNoRouteKeepsNone(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	lines, _ := frontendLines([]byte(`[{"level":"info"}]`), s)
	require.Empty(t, lines[0].Route)
}

func TestFrontendRouteIsKeptOnlyAsATemplate(t *testing.T) {
	s := NewScrubber([]byte("salt"), nil)

	for route, want := range map[string]string{
		"/printers/:id":              "/printers/:id",
		"/":                          "/",
		"/storage/usb/folder/Benchy": s.Hash("/storage/usb/folder/Benchy"),
		"/home/planted/file.gcode":   s.Hash("/home/planted/file.gcode"),
		"/printers/:id?token=x":      s.Hash("/printers/:id?token=x"),
		// The shape is all the server checks. The app sends the router's
		// template, so a lowercase address reaches it only from a body the
		// admin's own browser did not write.
		"/printers/mk4-bob": "/printers/mk4-bob",
	} {
		lines, _ := frontendLines([]byte(`[{"level":"info","route":"`+route+`","fields":{"route":"`+route+`"}}]`), s)
		require.Equal(t, want, lines[0].Route, route)
		require.Equal(t, want, lines[0].Fields["route"], route)
	}
}

func project(t *testing.T, entries []observability.LogEntry, s *Scrubber, modules bool) []logLine {
	t.Helper()

	lines, err := logLines(t.Context(), entries, s, modules)
	require.NoError(t, err)

	return lines
}
