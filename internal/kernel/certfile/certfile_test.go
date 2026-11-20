package certfile_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/kernel/certfile"
)

func TestNewRefusesAPairThatDoesNotLoad(t *testing.T) {
	t.Parallel()

	refused := errors.New("not this one")

	_, err := certfile.New(t.Context(), certfile.Options{
		Load: func(context.Context) (tls.Certificate, *x509.Certificate, error) {
			return tls.Certificate{}, nil, refused
		},
	})
	require.ErrorIs(t, err, refused, "a listener must not start on a pair its Load refused")

	_, err = certfile.New(t.Context(), certfile.Options{})
	require.ErrorIs(t, err, certfile.ErrNoLoad)
}

func TestARenewedPairIsPresentedOnceThePollHasPassed(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		files := writePair(t, t.TempDir(), "first")
		keypair, _ := newKeypair(t, files)

		require.Equal(t, "first", commonName(t, keypair))

		writePairTo(t, files, "renewed")
		touch(t, files.certificate, files.key)

		require.Equal(t, "first", commonName(t, keypair),
			"a handshake inside the poll interval touches no file")

		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "renewed", commonName(t, keypair),
			"the renewed pair is presented once the poll has passed, with no restart")

		served, err := keypair.GetCertificate(nil)
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(served.Certificate[0])
		require.NoError(t, err)
		require.Equal(t, "renewed", leaf.Subject.CommonName, "GetCertificate serves what Current holds")
	})
}

func TestAReplacementThatDoesNotLoadKeepsTheHeldPairAndIsReportedOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		files := writePair(t, t.TempDir(), "first")
		keypair, refusals := newKeypair(t, files)

		require.NoError(t, os.WriteFile(files.certificate, []byte("not a certificate"), 0o600))
		touch(t, files.certificate)
		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "first", commonName(t, keypair), "clients still verify the pair already held")
		require.Len(t, *refusals, 1)

		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "first", commonName(t, keypair))
		require.Len(t, *refusals, 1, "a file that did not change is not reported again")
	})
}

func TestACertificateAheadOfItsKeyIsReadAgainWhenTheKeyArrives(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		files := writePair(t, dir, "first")
		keypair, refusals := newKeypair(t, files)

		next := writePair(t, filepath.Join(dir, "next"), "renewed")

		copyFile(t, next.certificate, files.certificate)
		touch(t, files.certificate)
		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "first", commonName(t, keypair), "a certificate with the old key does not load")
		require.Len(t, *refusals, 1)

		copyFile(t, next.key, files.key)
		touch(t, files.key)
		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "renewed", commonName(t, keypair), "the key's arrival is a change of its own")
	})
}

func TestAMissingFileKeepsTheHeldPairWithoutAReport(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		files := writePair(t, t.TempDir(), "first")
		keypair, refusals := newKeypair(t, files)

		require.NoError(t, os.Remove(files.key))
		time.Sleep(certfile.DefaultPoll)

		require.Equal(t, "first", commonName(t, keypair))
		require.Empty(t, *refusals, "a file missing during a replacement is not a failed load")
	})
}

// handshakeKey marks the context a server handshake runs under, so the test
// can tell it apart from the one New was given.
type handshakeKey struct{}

func TestAReloadGetsTheContextOfTheHandshakeThatCausedIt(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		files := writePair(t, t.TempDir(), "first")

		var seen []any

		keypair, err := certfile.New(t.Context(), certfile.Options{
			Certificate: files.certificate,
			Key:         files.key,
			Load: func(ctx context.Context) (tls.Certificate, *x509.Certificate, error) {
				seen = append(seen, ctx.Value(handshakeKey{}))

				pair, err := tls.LoadX509KeyPair(files.certificate, files.key)
				if err != nil {
					return tls.Certificate{}, nil, err
				}

				leaf, err := x509.ParseCertificate(pair.Certificate[0])

				return pair, leaf, err
			},
		})
		require.NoError(t, err)

		writePairTo(t, files, "renewed")
		touch(t, files.certificate, files.key)
		time.Sleep(certfile.DefaultPoll)

		serverSide, clientSide := net.Pipe()
		defer func() { _ = serverSide.Close() }()
		defer func() { _ = clientSide.Close() }()

		server := tls.Server(serverSide, &tls.Config{
			GetCertificate: keypair.GetCertificate,
			MinVersion:     tls.VersionTLS13,
		})
		handshook := make(chan error, 1)

		go func() {
			handshook <- server.HandshakeContext(context.WithValue(t.Context(), handshakeKey{}, "the handshake"))
		}()

		client := tls.Client(clientSide, &tls.Config{
			InsecureSkipVerify: true, // the test reads the certificate's name, not whether it is trusted
			MinVersion:         tls.VersionTLS13,
		})
		require.NoError(t, client.HandshakeContext(t.Context()))
		require.NoError(t, <-handshook)

		require.Equal(t, "renewed", client.ConnectionState().PeerCertificates[0].Subject.CommonName)
		require.Equal(t, []any{nil, "the handshake"}, seen,
			"the load at startup gets New's context, and the reload gets the handshake's")
	})
}

type pairFiles struct {
	certificate string
	key         string
}

// newKeypair reads files the way a listener does, and collects what Refused
// is told.
func newKeypair(t *testing.T, files pairFiles) (*certfile.Keypair, *[]error) {
	t.Helper()

	refusals := &[]error{}

	keypair, err := certfile.New(t.Context(), certfile.Options{
		Certificate: files.certificate,
		Key:         files.key,
		Load: func(context.Context) (tls.Certificate, *x509.Certificate, error) {
			pair, err := tls.LoadX509KeyPair(files.certificate, files.key)
			if err != nil {
				return tls.Certificate{}, nil, err
			}

			leaf, err := x509.ParseCertificate(pair.Certificate[0])

			return pair, leaf, err
		},
		Refused: func(_ context.Context, err error) { *refusals = append(*refusals, err) },
	})
	require.NoError(t, err)

	return keypair, refusals
}

func commonName(t *testing.T, keypair *certfile.Keypair) string {
	t.Helper()

	_, leaf := keypair.Current(t.Context())
	require.NotNil(t, leaf)

	return leaf.Subject.CommonName
}

func writePair(t *testing.T, dir, name string) pairFiles {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o700))

	files := pairFiles{certificate: filepath.Join(dir, "cert.pem"), key: filepath.Join(dir, "key.pem")}
	writePairTo(t, files, name)

	return files
}

// writePairTo writes a self-signed certificate named name and its key.
func writePairTo(t *testing.T, files pairFiles, name string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(files.certificate,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(files.key,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()

	data, err := os.ReadFile(from)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(to, data, 0o600))
}

// touch moves each file's modification time a minute past where it is. Inside
// a synctest bubble the clock reads the year 2000, so a time taken from it would
// move the file back rather than forward.
func touch(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		info, err := os.Stat(path)
		require.NoError(t, err)

		later := info.ModTime().Add(time.Minute)
		require.NoError(t, os.Chtimes(path, later, later))
	}
}
