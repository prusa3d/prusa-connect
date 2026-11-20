package app

import (
	"crypto/tls"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/kernel/certfile"
)

// TestARenewedAPICertificateIsServedWithoutARestart replaces the certificate
// and key under a running API listener, as certbot or a mounted secret does.
// A restart to pick the new pair up would drop every printer connected to
// this process.
func TestARenewedAPICertificateIsServedWithoutARestart(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		first := issue(t, dir)

		api, err := apiTLS(t.Context(), discard(), config.TLS{Certificate: first.certificate, Key: first.key})
		require.NoError(t, err)
		require.Empty(t, api.Certificates,
			"a pair in Certificates is served to every client that sends no server name, "+
				"whatever GetCertificate holds")

		before := servedLeaf(t, api)

		// The renewal is dated a minute after the first pair, because two
		// writes inside one clock tick of the file system carry the same
		// modification time, and the clock inside a synctest bubble reads the
		// year 2000.
		firstWritten, err := os.Stat(first.certificate)
		require.NoError(t, err)

		renewed := issue(t, dir)
		renewedAt := firstWritten.ModTime().Add(time.Minute)

		for _, path := range []string{renewed.certificate, renewed.key} {
			require.NoError(t, os.Chtimes(path, renewedAt, renewedAt))
		}

		time.Sleep(certfile.DefaultPoll)

		require.NotEqual(t, before, servedLeaf(t, api),
			"the listener presents the renewed certificate once the poll has passed")
	})
}

// servedLeaf is the DER of the leaf the listener answers a ClientHello with.
func servedLeaf(t *testing.T, api *tls.Config) []byte {
	t.Helper()

	pair, err := api.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotEmpty(t, pair.Certificate)

	return pair.Certificate[0]
}
