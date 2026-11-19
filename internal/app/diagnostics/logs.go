package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/platform/observability"
)

const maxFrontendBytes = 256 << 10

// maxLineBytes is the longest message a bundle line carries, the length of one
// module line. The patterns scale with the text, and a joined stack trace or
// base64 blob says nothing past its first few kilobytes.
const maxLineBytes = 8 << 10

// keptKeys carry values that name nothing of the operator's: ids the server
// minted, enums, counts and correlation ids. `printer` is always a device id;
// a printer's name is logged as printer_name and hashed. `blocked` and
// `instances` list kind:name instance refs, which modules.json shows anyway. A
// connector instance, logged as connector or driver, is a name the operator
// chose and is hashed; its dialect is logged as type.
var keptKeys = map[string]bool{
	"trace_id":                    true,
	"op":                          true,
	"kind":                        true,
	"state":                       true,
	"status":                      true,
	"code":                        true,
	"resource":                    true,
	"instance":                    true,
	"device_id":                   true,
	"transfer_id":                 true,
	"method":                      true,
	"version":                     true,
	"type":                        true,
	"model":                       true,
	"firmware":                    true,
	"level":                       true,
	observability.ModuleStderrKey: true,
	"operation":                   true,
	"printer":                     true,
	"theme":                       true,
	"language":                    true,
	"blocked":                     true,
	"instances":                   true,
}

// deniedKeys never leave the process, whatever their value looks like.
var deniedKeys = map[string]bool{
	"setup_code": true, "fingerprint": true, "username": true, "subject": true,
	"identity": true, "gcode": true, "password": true, "token": true, "secret": true,
	"authorization": true, "cookie": true, "dsn": true,
}

// textKeys are free text a developer reads: scrubbed, not hashed, because a
// hashed error message says nothing.
var textKeys = map[string]bool{"error": true, "err": true, "cause": true, "stack": true, "panic": true, "detail": true,
	"reason": true, "result": true}

type logLine struct {
	Time  string         `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

func logLines(ctx context.Context, entries []observability.LogEntry, s *Scrubber, modules bool) ([]logLine, error) {
	lines := make([]logLine, 0, len(entries))
	// Each module instance has its own mirror goroutine, so records of
	// different instances interleave; a split line is rejoined per instance.
	pending := map[string]observability.LogEntry{}

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		module, split := e.Flag(observability.ModuleStderrKey), e.Flag("split")
		if module && !modules {
			continue
		}

		if module {
			// A long line arrives as several records. Joined, a token cut in
			// two is whole again and the sweep finds it.
			key := instanceOf(e.Attrs)
			// Past twice the clip a fragment is dropped: the clip keeps none of
			// it, and the doubled length is what lets the sweep still find a
			// secret written across the clip.
			if head, ok := pending[key]; ok {
				if len(head.Message) < 2*maxLineBytes {
					e.Message = head.Message + e.Message
				} else {
					e.Message = head.Message
				}

				e.Time, e.Attrs = head.Time, head.Attrs
				delete(pending, key)
			}

			if split {
				pending[key] = e

				continue
			}
		}

		lines = append(lines, projectLine(e, s))
	}

	// A split sequence cut off by the end of the buffer still holds text.
	for _, key := range slices.Sorted(maps.Keys(pending)) {
		lines = append(lines, projectLine(pending[key], s))
	}

	return lines, nil
}

func projectLine(e observability.LogEntry, s *Scrubber) logLine {
	return logLine{
		Time:  e.Time.UTC().Format(time.RFC3339Nano),
		Level: e.Level.String(),
		Msg:   s.Text(s.clip(e.Message)),
		Attrs: attrsOf(e.Attrs, s),
	}
}

func instanceOf(attrs []slog.Attr) string {
	for _, a := range attrs {
		if a.Key == "instance" {
			return a.Value.String()
		}
	}

	return ""
}

func attrsOf(attrs []slog.Attr, s *Scrubber) map[string]any {
	out := map[string]any{}

	for _, a := range attrs {
		key := a.Key
		if deniedKeys[key] || key == "split" {
			continue
		}

		switch a.Value.Kind() {
		case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool, slog.KindDuration:
			out[key] = a.Value.Any()
		case slog.KindTime:
			out[key] = a.Value.Time().UTC().Format(time.RFC3339Nano)
		case slog.KindString, slog.KindAny, slog.KindGroup, slog.KindLogValuer:
			out[key] = stringAttr(key, a.Value.String(), s)
		}
	}

	return out
}

func stringAttr(key, value string, s *Scrubber) string {
	switch {
	case keptKeys[key]:
		return value
	case textKeys[key]:
		return s.Text(s.clip(value))
	default:
		return s.Hash(value)
	}
}

type frontendLine struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Scope   string         `json:"scope"`
	Route   string         `json:"route,omitempty"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// frontendKept are fields the browser buffer writes that name nothing of the
// operator's. Every other string is scrubbed.
var frontendKept = map[string]bool{"operation": true, "method": true, "traceId": true}

// errorCodePattern is the shape of a server error code, which names no one.
var errorCodePattern = regexp.MustCompile(`^[a-z0-9_.-]+/[A-Z0-9_]+$`)

// paramKeys are the error params whose values are machine identifiers: a
// verb, an argument, a state. IDENTIFIER_PARAMS in frontend/src/lib/client.ts
// is the same list and changes with this one: the browser sends the value of
// these keys alone, and the server keeps it only in the shape of an
// identifier.
var paramKeys = map[string]bool{
	"verb": true, "argument": true, "field": true, "state": true, "kind": true, "operation": true,
	"code": true, "reason_code": true, "machine_reason": true, "capability": true, "unit": true,
	"type": true, "action": true, "method": true, "op": true, "suggested_verb": true,
	"printer_model": true, "firmware": true, "version": true, "extension": true, "event": true,
	"device_id": true, "transfer_id": true, "command_id": true,
}

var paramValuePattern = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,64}$`)

// routePattern is the shape of a router template: static segments and
// :params. The app sends the template, never the address; the shape alone
// cannot tell a lowercase address from a template.
var routePattern = regexp.MustCompile(`^(/[a-z0-9-]*|/:[a-z_]+)+$`)

var frontendLevels = map[string]bool{"error": true, "warn": true, "info": true, "debug": true, "trace": true}

func frontendLines(body []byte, s *Scrubber) ([]frontendLine, string) {
	switch {
	case len(body) == 0 || string(bytes.TrimSpace(body)) == "null":
		return nil, "none"
	case len(body) > maxFrontendBytes:
		return nil, "too_large"
	}

	var lines []frontendLine
	if err := json.Unmarshal(body, &lines); err != nil {
		return nil, "unreadable"
	}

	if len(lines) == 0 {
		return nil, "none"
	}

	for i := range lines {
		lines[i].Time = s.Text(lines[i].Time)
		if !frontendLevels[lines[i].Level] {
			lines[i].Level = ""
		}

		lines[i].Scope = s.Text(lines[i].Scope)
		if lines[i].Route != "" && !routePattern.MatchString(lines[i].Route) {
			lines[i].Route = s.Hash(lines[i].Route)
		}

		lines[i].Message = s.Text(lines[i].Message)
		lines[i].Fields = frontendFields(lines[i].Fields, s)
	}

	return lines, "included"
}

func frontendString(key, v string, s *Scrubber) string {
	switch {
	case frontendKept[key], key == "code" && errorCodePattern.MatchString(v),
		key == "route" && routePattern.MatchString(v):
		return v
	case textKeys[key]:
		return s.Text(v)
	default:
		return s.Hash(v)
	}
}

func frontendFields(fields map[string]any, s *Scrubber) map[string]any {
	out := map[string]any{}

	for key, value := range fields {
		if deniedKeys[key] {
			continue
		}

		switch v := value.(type) {
		case string:
			out[key] = frontendString(key, v, s)
		case float64, bool, nil:
			out[key] = v
		case []any:
			scrubbed := make([]string, 0, len(v))
			for _, item := range v {
				if text, ok := item.(string); ok {
					scrubbed = append(scrubbed, frontendString(key, text, s))
				}
			}

			out[key] = scrubbed
		case map[string]any:
			out[key] = frontendParams(v, s)
		}
	}

	return out
}

func frontendParams(params map[string]any, s *Scrubber) map[string]any {
	out := make(map[string]any, len(params))

	for key, value := range params {
		switch v := value.(type) {
		case nil:
			out[key] = nil
		case string:
			if paramKeys[key] && paramValuePattern.MatchString(v) {
				out[key] = v
			} else {
				out[key] = s.Hash(v)
			}
		case float64, bool:
			if paramKeys[key] {
				out[key] = v
			} else {
				out[key] = s.Hash(fmt.Sprint(v))
			}
		default:
			out[key] = s.Hash(fmt.Sprint(v))
		}
	}

	return out
}
