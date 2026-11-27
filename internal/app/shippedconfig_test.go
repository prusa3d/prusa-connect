package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTheShippedConfigurationLeavesSignInAndHTTPSToTheWizard is CL-388. The
// wizard cannot change a key the operator's file sets, even to an empty value
// (ADR-20261015-ejz), and a release hands config/config.yaml to a first run
// unchanged. With the sign-in keys written out as "" a new install could not
// choose a sign-in provider, and with the certificate keys written out it
// could not turn HTTPS on. The shipped file shows those defaults in comments
// instead.
//
// The file still sets other keys the wizard asks, on purpose or not, so this
// test names only the keys the sign-in and encryption steps send.
func TestTheShippedConfigurationLeavesSignInAndHTTPSToTheWizard(t *testing.T) {
	t.Parallel()

	wizardSets := []string{
		"core.auth.issuer",
		"core.auth.client_id",
		"core.auth.redirect_uri",
		"core.auth.local",
		"core.principal",
		"core.api.tls.certificate",
		"core.api.tls.key",
	}

	contract, err := setupContract(filepath.Join("..", "..", "config", "config.yaml"))
	require.NoError(t, err)

	locked := map[string]bool{}

	for _, key := range contract.Keys {
		locked[key.Path] = key.Locked
	}

	var set []string

	for _, path := range wizardSets {
		isLocked, asked := locked[path]
		require.True(t, asked, "the wizard no longer asks %s", path)

		if isLocked {
			set = append(set, path)
		}
	}

	require.Empty(t, set, "config/config.yaml sets keys the first-run wizard's sign-in and "+
		"encryption steps then cannot change. Comment out a key that only repeats its default.")
}
