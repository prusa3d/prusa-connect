package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
)

// repoRoot is this package's distance from the top of the tree. The language
// test below reads a file the app owns, because the point of it is that the core
// and the app cannot drift apart quietly.
const repoRoot = "../../.."

// TestTheAppIsServedUnlessItIsSwitchedOff is the owner's rule of 2026-09-14: a
// server nobody configured serves its own app. A file with no core.frontend
// block and a file that names the block and nothing under it both read as on.
func TestTheAppIsServedUnlessItIsSwitchedOff(t *testing.T) {
	t.Parallel()

	for name, file := range map[string]string{
		"no block at all":       "core:\n  site: local\n",
		"the block and nothing": "core:\n  frontend:\n",
		"every key but enabled": "core:\n  frontend:\n    title: Workshop\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := config.Load(write(t, file))
			require.NoError(t, err)
			require.True(t, core.Frontend.On(), "the app was off on a file that never switched it off")
			require.NoError(t, core.Validate())
		})
	}
}

// TestEveryFrontendKeyIsRead covers the block the page reads before it paints.
func TestEveryFrontendKeyIsRead(t *testing.T) {
	t.Parallel()

	core, err := config.Load(write(t, `
core:
  frontend:
    enabled: false
    title: "Workshop floor"
    default_language: cs
    theme: dark
    printer_address: https://shop.example.net
    support_url: https://portal.example/beta
`))
	require.NoError(t, err)

	require.False(t, core.Frontend.On())
	require.Equal(t, "Workshop floor", core.Frontend.Title)
	require.Equal(t, "cs", core.Frontend.DefaultLanguage)
	require.Equal(t, "dark", core.Frontend.Theme)
	require.Equal(t, "https://shop.example.net", core.Frontend.PrinterAddress)
	require.Equal(t, "https://portal.example/beta", core.Frontend.SupportURL)
	require.NoError(t, core.Validate())
}

// TestAValueTheAppCannotDrawIsRefused is why these keys are validated at all.
// Each one is handed to a browser, and a language with no catalogue is a page of
// missing labels rather than an error anybody sees.
func TestAValueTheAppCannotDrawIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		block string
		says  string
	}{
		"a language the app ships no catalogue for": {
			block: "    default_language: kl\n", says: "default_language",
		},
		"a theme that is not one of the three": {block: "    theme: sepia\n", says: "theme"},
		"a title nobody could read": {
			block: "    title: \"" + strings.Repeat("x", config.MaxTitleBytes+1) + "\"\n", says: "title",
		},
		"a printer address with a path": {
			block: "    printer_address: https://shop.example.net/p\n", says: "printer_address",
		},
		"a printer address with no scheme": {
			block: "    printer_address: shop.example.net\n", says: "printer_address",
		},
		"a support link that is not http": {
			block: "    support_url: ftp://x\n", says: "support_url",
		},
		"a printer address only this machine answers on": {
			block: "    printer_address: http://localhost:8080\n", says: "printer_address",
		},
		"a printer address on a loopback IP": {
			block: "    printer_address: http://127.0.0.1:8080\n", says: "printer_address",
		},
		"a printer address on the unspecified address": {
			block: "    printer_address: http://0.0.0.0:8080\n", says: "printer_address",
		},
		"a printer host the firmware cuts short": {
			block: "    printer_address: https://printers.workshop.example.net\n", says: "printer_address",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := config.Load(write(t, "core:\n  frontend:\n"+testCase.block))
			require.NoError(t, err)

			err = core.Validate()
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), testCase.says)
		})
	}
}

// TestARetiredBuildKeyIsRefusedByName is the other half of retiring
// server.api.frontend and documentation.enabled. Both were read by the build
// tool and by nothing else, so a file that still carries one would build the
// app and never say which key stopped being read.
func TestARetiredBuildKeyIsRefusedByName(t *testing.T) {
	t.Parallel()

	for name, file := range map[string]string{
		"the retired server block":        "server:\n  api:\n    frontend: true\n",
		"the retired documentation block": "documentation:\n  enabled: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(write(t, file))
			require.ErrorIs(t, err, config.ErrInvalid)
		})
	}
}

// TestTheLanguagesAreTheCataloguesTheAppShips holds the core's list to the app's
// own. The core serves a language the page then has to have a catalogue for, and
// the binary carries no catalogue to check against at runtime.
func TestTheLanguagesAreTheCataloguesTheAppShips(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join(repoRoot, "packages/svelte/src/i18n/locale.svelte.ts"))
	require.NoError(t, err)

	declared := regexp.MustCompile(`LOCALES\s*=\s*\[([^\]]*)\]`).FindSubmatch(source)
	require.Len(t, declared, 2, "the app's LOCALES list has moved; this test reads it by name")

	shipped := quoted(string(declared[1]))
	require.NotEmpty(t, shipped)

	for _, language := range shipped {
		require.NoErrorf(t, languageAccepted(language),
			"the app ships a %s catalogue and core.frontend.default_language refuses it", language)
	}
}

// languageAccepted asks the validator rather than a list, so the test holds the
// refusal an operator meets and not a variable beside it.
func languageAccepted(language string) error {
	core := config.Default().WithDefaults("config/config.yaml")
	core.Frontend.DefaultLanguage = language

	return core.Validate()
}

// quoted reads the string literals out of one TypeScript array body.
func quoted(body string) []string {
	var out []string

	for _, match := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(body, -1) {
		out = append(out, match[1])
	}

	slices.Sort(out)

	return slices.Compact(out)
}
