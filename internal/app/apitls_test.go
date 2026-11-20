package app

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log/slog"
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

// TestTheAPIListenerNamesTheCertificateItServes reads the line an operator
// checks after a renewal: which names the certificate covers and who issued
// it, so a leftover self-signed certificate is told apart from the renewed one.
func TestTheAPIListenerNamesTheCertificateItServes(t *testing.T) {
	t.Parallel()

	pki := issue(t, t.TempDir())

	var out bytes.Buffer

	_, err := apiTLS(t.Context(), slog.New(slog.NewJSONHandler(&out, nil)),
		config.TLS{Certificate: pki.certificate, Key: pki.key})
	require.NoError(t, err)

	var line struct {
		Msg         string   `json:"msg"`
		DNSNames    []string `json:"dns_names"`
		IPAddresses []string `json:"ip_addresses"`
		IssuedBy    string   `json:"issued_by"`
	}

	require.NoError(t, json.Unmarshal(bytes.SplitN(out.Bytes(), []byte("\n"), 2)[0], &line))
	require.Equal(t, "the API listener serves TLS", line.Msg)
	require.Equal(t, []string{printerName, "127.0.0.1"}, line.DNSNames)
	require.Equal(t, []string{"127.0.0.1"}, line.IPAddresses)
	require.Equal(t, "CN=Prusa Connect Test Root CA", line.IssuedBy)
}

// servedLeaf is the DER of the leaf the listener answers a ClientHello with.
func servedLeaf(t *testing.T, api *tls.Config) []byte {
	t.Helper()

	pair, err := api.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotEmpty(t, pair.Certificate)

	return pair.Certificate[0]
}
