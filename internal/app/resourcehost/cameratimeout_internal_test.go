package resourcehost

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
)

// A generic camera's timeout is checked as the rtsp driver's is, against the
// camera types this build ships. Each block is checked as both types, and the
// two answers must agree: the same value is accepted by both, and the same
// value is refused by both for the same reason.
func TestAGenericCamerasTimeoutIsCheckedAsTheRTSPDriversIs(t *testing.T) {
	t.Parallel()

	types, err := builtin.Registry()
	if err != nil {
		t.Fatal(err)
	}

	// Each block also names the stream, so the rtsp driver's required url is
	// written and only the timeout is judged.
	cases := map[string]struct {
		timeout string
		want    error
	}{
		"a duration":                 {timeout: `"10s"`},
		"a duration under a second":  {timeout: `"300ms"`},
		"a long duration":            {timeout: `"10m"`},
		"text that is no duration":   {timeout: `"soon"`},
		"zero":                       {timeout: `"0s"`},
		"a negative duration":        {timeout: `"-5s"`},
		"a bare number":              {timeout: `7`},
		"a block where a value goes": {timeout: `{"read":"10s"}`, want: ErrSettingsShape},
		"a list where a value goes":  {timeout: `["10s"]`, want: ErrSettingsShape},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for typeName, block := range map[string]string{
				"generic": `{"printer":"p1","stream_url":"rtsp://192.0.2.1/live","timeout":` + tc.timeout + `}`,
				"rtsp":    `{"printer":"p1","url":"rtsp://192.0.2.1/live","timeout":` + tc.timeout + `}`,
			} {
				checked := builtinSettings(types, builtin.HostKeys(ref.KindCamera), ref.KindCamera, typeName,
					json.RawMessage(block))

				if tc.want == nil && checked != nil {
					t.Errorf("%s refused %s: %v", typeName, tc.timeout, checked)
				}

				if tc.want != nil && !errors.Is(checked, tc.want) {
					t.Errorf("%s: got %v, want %v", typeName, checked, tc.want)
				}
			}
		})
	}
}

// A key a generic camera does not read is still refused, and the refusal now
// lists timeout among the keys it does read.
func TestAMisspeltTimeoutOnAGenericCameraIsRefusedByName(t *testing.T) {
	t.Parallel()

	types, err := builtin.Registry()
	if err != nil {
		t.Fatal(err)
	}

	checked := builtinSettings(types, builtin.HostKeys(ref.KindCamera), ref.KindCamera, "generic",
		json.RawMessage(`{"stream_url":"rtsp://192.0.2.1/live","timeut":"10s"}`))
	if !errors.Is(checked, ErrSettingsUnknown) {
		t.Fatalf("got %v, want %v", checked, ErrSettingsUnknown)
	}

	for _, said := range []string{`"timeut"`, `"timeout"`} {
		if !strings.Contains(checked.Error(), said) {
			t.Errorf("the refusal %q does not name %s", checked, said)
		}
	}
}
