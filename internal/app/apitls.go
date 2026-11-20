package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/kernel/certfile"
)

// apiTLS is the certificate the API listener presents, or nil for plain HTTP.
//
// TLS 1.3 is the minimum. A browser reaches this listener, and every browser
// in use supports 1.3. No device connects here, so no older version is
// needed.
//
// The pair is read again when either file changes, so a renewed certificate
// reaches browsers without a restart, which would drop every printer
// connected to this process.
func apiTLS(ctx context.Context, logger *slog.Logger, cfg config.TLS) (*tls.Config, error) {
	if !cfg.Enabled() {
		return nil, nil //nolint:nilnil // no certificate is not an error: it is plain HTTP.
	}

	keypair, err := certfile.New(ctx, certfile.Options{
		Certificate: cfg.Certificate,
		Key:         cfg.Key,
		Load: func(ctx context.Context) (tls.Certificate, *x509.Certificate, error) {
			return loadAPIKeypair(ctx, logger, cfg)
		},
		Refused: func(ctx context.Context, err error) {
			logger.WarnContext(ctx,
				"the certificate file changed and the API listener kept the one it had, so browsers "+
					"still see the previous certificate",
				"certificate", cfg.Certificate,
				"reason", err.Error())
		},
	})
	if err != nil {
		return nil, err
	}

	// Certificates stays empty: certfile.Keypair.GetCertificate says why.
	return &tls.Config{
		GetCertificate: keypair.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{"h2", "http/1.1"},
	}, nil
}

// loadAPIKeypair reads the pair and its leaf, and says in the log which
// certificate the listener now serves.
func loadAPIKeypair(
	ctx context.Context, logger *slog.Logger, cfg config.TLS,
) (tls.Certificate, *x509.Certificate, error) {
	pair, err := cfg.Load()
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	leaf, err := config.Leaf(pair)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: core.api.tls.certificate is %q", err, cfg.Certificate)
	}

	// The names and the issuer tell a renewal apart from a leftover
	// certificate. The keys are the ones the device listener's line uses.
	logger.InfoContext(ctx, "the API listener serves TLS",
		"certificate", cfg.Certificate, "min_version", "1.3",
		"dns_names", leaf.DNSNames,
		"ip_addresses", leaf.IPAddresses,
		"not_after", leaf.NotAfter.Format("2006-01-02"),
		"issued_by", leaf.Issuer.String())

	warnAboutExpiry(ctx, logger, "core.api.tls.certificate", cfg.Certificate, leaf)

	return pair, leaf, nil
}

// warnAboutExpiry says, when a certificate is read, that it is inside the last stretch
// of its validity. It is a warning and not a refusal: a browser reaches this
// listener, and a person can still click through an expired certificate.
func warnAboutExpiry(ctx context.Context, logger *slog.Logger, key, path string, leaf *x509.Certificate) {
	left := time.Until(leaf.NotAfter)
	if left > config.ExpiryWarning {
		return
	}

	logger.WarnContext(ctx, "the certificate this listener presents expires soon, and every client that "+
		"verifies it fails from that moment",
		"key", key,
		"certificate", path,
		"expires", leaf.NotAfter.UTC().Format(time.RFC3339),
		"remaining", left.Round(time.Hour).String(),
		"remedy", "reissue with scripts/generate_certificates.sh before that date")
}
