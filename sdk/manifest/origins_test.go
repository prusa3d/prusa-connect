package manifest_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

func parseOrigins(t *testing.T, origins ...string) (manifest.Manifest, error) {
	t.Helper()

	listed, err := json.Marshal(origins)
	if err != nil {
		t.Fatal(err)
	}

	document := fmt.Sprintf(`{"name":"slicer","kind":"asset","version":"1.0.0","license":"MIT",`+
		`"allowed_origins":%s}`, listed)

	return manifest.Parse("module.yaml", []byte(document), json.Unmarshal)
}

func TestAllowedOriginsAreRead(t *testing.T) {
	t.Parallel()

	held, err := parseOrigins(t, "https://slicer.example", "https://slicer.example:8443")
	if err != nil {
		t.Fatal(err)
	}

	if len(held.AllowedOrigins) != 2 || held.AllowedOrigins[0] != "https://slicer.example" {
		t.Fatalf("AllowedOrigins = %v", held.AllowedOrigins)
	}
}

func TestAnOriginABrowserNeverSendsIsRefused(t *testing.T) {
	t.Parallel()

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
		_, err := parseOrigins(t, origin)
		if !errors.Is(err, manifest.ErrInvalid) || !strings.Contains(err.Error(), "allowed_origins") {
			t.Errorf("%q: err = %v, expected ErrInvalid naming allowed_origins", origin, err)
		}
	}
}

func TestAllowedOriginsAreBounded(t *testing.T) {
	t.Parallel()

	if _, err := parseOrigins(t, "https://a.example", "https://a.example"); !errors.Is(err, manifest.ErrInvalid) {
		t.Errorf("a repeated origin: err = %v", err)
	}

	many := make([]string, 0, manifest.MaxAllowedOrigins+1)
	for i := range manifest.MaxAllowedOrigins + 1 {
		many = append(many, fmt.Sprintf("https://s%d.example", i))
	}

	if _, err := parseOrigins(t, many...); !errors.Is(err, manifest.ErrInvalid) {
		t.Errorf("over the limit: err = %v", err)
	}
}

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
