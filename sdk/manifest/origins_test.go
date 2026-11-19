package manifest_test

import (
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

func TestAnOriginIsHeldToWhatABrowserSendsOverHTTPS(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"https://slicer.example", "https://slicer.example:8443"} {
		if !manifest.IsAllowedOrigin(origin) {
			t.Errorf("%q was refused", origin)
		}
	}

	for _, origin := range []string{
		"*",
		"https://*.example",
		"http://slicer.example",
		"https://slicer.example/",
		"https://slicer.example/slice",
		"https://slicer.example:443",
		"https://Slicer.example",
		"https://user@slicer.example",
		"https://",
		"slicer.example",
	} {
		if manifest.IsAllowedOrigin(origin) {
			t.Errorf("%q was taken", origin)
		}
	}
}
