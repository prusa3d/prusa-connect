package xbuddy

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/prusa3d/prusa-connect/internal/kernel/certfile"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// firmwareCipherSuite is the one suite xBuddy firmware offers. It is written
// here rather than left to Go's defaults because Go's default TLS 1.2 list is a
// library decision that has changed before and answers to nobody's firmware,
// and because a listener offering more than the printer speaks is a listener
// whose failures are somebody else's to explain.
//
// See docs/src/content/docs/prusa-xbuddy-websocket-protocol.md, "TLS behavior
// in firmware".
const firmwareCipherSuite = tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256

// defaultHandshake bounds a printer's TLS handshake on this dialect's own
// socket. Go takes the handshake budget from the smallest of the server read
// and write timeouts, and the host's ten seconds on the API listener is not
// enough: a CORE One running phase stepping spends most of its processor time
// elsewhere and intermittently loses the handshake at that bound, reported as a
// connection reset with nothing else to go on.
const defaultHandshake = 60 * time.Second

// expiryWarning is how long before a certificate's NotAfter this dialect starts
// saying so. A leaf from scripts/generate_certificates.sh is good for 825 days,
// so a month of notice is quiet until it is not, and replacing a leaf signed by
// the same CA needs no visit to any printer.
const expiryWarning = 30 * 24 * time.Hour

// certificatePoll is how often a handshake looks at the files behind the
// keypair. A handshake that finds the last look younger than this presents what
// the listener already holds and touches no file at all, so a mount that has
// stopped answering costs one handshake a second rather than every printer that
// dials while it is stuck.
const certificatePoll = time.Second

// Listener is the socket this dialect asks the host for: the device-facing
// listener, with its own certificate and its own timeouts.
//
// It is separate from the host's own listener because the two answer different
// clients and cannot be satisfied at once. A printer verifies against one
// certificate loaded from a USB stick and speaks TLS 1.2 with a single ECDSA
// cipher suite; a browser verifies against its own trust store and should be
// held to TLS 1.3. One socket serving both means the printer's floor becomes
// everybody's floor, and the whole API answers on whatever network the printers
// are on.
//
// An empty address is the default and asks for no second socket: the host's own
// listener carries the device wires as well.
type Listener struct {
	// Addr is a host:port accepted by net.Listen. Empty asks for no socket.
	Addr string `json:"addr,omitempty"`
	// TLS is the certificate printers verify. It must be ECDSA, because the
	// firmware offers one cipher suite and that suite needs an ECDSA key.
	TLS ListenerTLS `json:"tls,omitzero"`
	// HandshakeTimeout bounds the TLS handshake a printer starts, as
	// time.ParseDuration reads it. Empty takes defaultHandshake. It is a key
	// rather than a constant because what is on the far side of it is a printer
	// whose processor time belongs to the print.
	HandshakeTimeout string `json:"handshake_timeout,omitempty"`
}

// ListenerTLS is the keypair that socket presents. Both empty is a socket that
// serves plain HTTP, which is what a deployment with no certificate runs on.
type ListenerTLS struct {
	// Certificate is a PEM file holding the leaf, and any certificate between
	// it and the CA the printer holds as connect.der. The printer reads the
	// whole Certificate message into a 1024-byte buffer, so in practice it is
	// the leaf alone: a leaf with an intermediate does not fit, and the
	// printer drops the connection with no alert.
	Certificate string `json:"certificate,omitempty"`
	// Key is the PEM private key for that leaf.
	Key string `json:"key,omitempty"`
}

// enabled reports whether this deployment asks for a socket of its own.
func (l Listener) enabled() bool { return l.Addr != "" }

// enabled reports whether that socket was given a certificate.
func (t ListenerTLS) enabled() bool { return t.Certificate != "" || t.Key != "" }

// DeviceListener is the socket this driver asks the host for. An empty address
// asks for none, and the host then serves the device routes on its own
// listener.
func (d *Driver) DeviceListener() connectorapi.DeviceListener { return d.listener }

var _ connectorapi.ServesItsOwnSocket = (*Driver)(nil)

// WithDeviceListener gives the driver the socket it asks the host for. The zero
// value asks for none.
func WithDeviceListener(listener connectorapi.DeviceListener) Option {
	return func(d *Driver) { d.listener = listener }
}

// parseListener reads the listener block and returns what the host binds.
//
// Everything that can be checked before a socket is opened is checked here: the
// address, the handshake budget, and the certificate down to the public key it
// carries and the dates it is valid between. A configuration that would serve
// every printer a refusal is refused at start, where it costs one line in a
// log, rather than at the bench, where it costs an afternoon.
func parseListener(cfg Listener, log *slog.Logger) (connectorapi.DeviceListener, error) {
	if !cfg.enabled() {
		return connectorapi.DeviceListener{}, refuseStrayListenerKeys(cfg)
	}

	if err := validAddr(cfg.Addr); err != nil {
		return connectorapi.DeviceListener{}, err
	}

	handshake, err := parsePositive("listener.handshake_timeout", cfg.HandshakeTimeout, defaultHandshake)
	if err != nil {
		return connectorapi.DeviceListener{}, err
	}

	profile, err := firmwareTLS(cfg.TLS, log)
	if err != nil {
		return connectorapi.DeviceListener{}, err
	}

	return connectorapi.DeviceListener{Addr: cfg.Addr, TLS: profile, HandshakeTimeout: handshake}, nil
}

// refuseStrayListenerKeys refuses a certificate or a budget configured for a
// socket with no address.
//
// Silence there is how an operator discovers on the next outage that the TLS
// they configured has not been running.
func refuseStrayListenerKeys(cfg Listener) error {
	if cfg.TLS.enabled() {
		return fmt.Errorf("%w: listener.tls names a certificate and listener.addr is empty, so "+
			"nothing would serve it", ErrInvalidConfig)
	}

	if cfg.HandshakeTimeout != "" {
		return fmt.Errorf("%w: listener.handshake_timeout is %s and listener.addr is empty, so no "+
			"handshake would be bounded by it", ErrInvalidConfig, cfg.HandshakeTimeout)
	}

	return nil
}

// validAddr holds the listener address to something net.Listen can bind.
//
// A numeric port only. A named one resolves through /etc/services, which makes
// whether the process starts depend on a file nobody deploying it looked at.
func validAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%w: listener.addr is %q, expected host:port", ErrInvalidConfig, addr)
	}

	if port == "" {
		return fmt.Errorf("%w: listener.addr is %q, expected a port", ErrInvalidConfig, addr)
	}

	if _, err = strconv.Atoi(port); err != nil {
		return fmt.Errorf("%w: listener.addr is %q, and %q is not a port number",
			ErrInvalidConfig, addr, port)
	}

	return nil
}

// firmwareTLS is the profile the device socket serves, or nil for plain HTTP.
//
// TLS 1.2 is both floor and ceiling and the suite list is one entry, because
// that is what the firmware in the field enforces. Raising the floor gives a
// listener no printer in the field can reach; leaving the ceiling off gives one
// whose advertised profile is not the profile it serves, because Go ignores
// CipherSuites under TLS 1.3 and a 1.3-capable client walks straight past the
// pin. This socket serves the device wires to one client whose profile is
// frozen in shipped firmware, so a ceiling costs nothing that dials it, and
// anything that is not a printer belongs on the host's own listener at its own
// floor.
func firmwareTLS(cfg ListenerTLS, log *slog.Logger) (*tls.Config, error) {
	if !cfg.enabled() {
		return nil, nil //nolint:nilnil // no certificate is not an error: it is plain HTTP.
	}

	source, err := newCertificateSource(cfg, log)
	if err != nil {
		return nil, err
	}

	// The ceiling is the point rather than an oversight: without it Go
	// negotiates TLS 1.3 and ignores the suite list, which is a profile no
	// printer in the field speaks.
	//
	// Certificates is left empty on purpose. Go consults GetCertificate only
	// when Certificates is empty or the ClientHello carried a server name, so
	// filling both would serve the certificate read at startup to every
	// printer that dials by address and sends no name.
	return &tls.Config{ //nolint:gosec // xBuddy firmware speaks TLS 1.2 and nothing else.
		GetCertificate:     source.GetCertificate,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		CipherSuites:       []uint16{firmwareCipherSuite},
		GetConfigForClient: reportUnanswerableHellos(log, cfg, source),
	}, nil
}

// newCertificateSource reads the keypair a listener starts with, refusing the
// same certificates parseListener refused before a port is bound, and reads it
// again when a file behind it changes.
//
// A deployment mounts the keypair from a secret that something outside the
// process rotates, and the file is replaced under a running server. A restart
// to pick it up would drop every printer's connection.
//
// connectorapi.Spec carries no context, so the load at startup starts from an
// empty one. A reload gets the context of the handshake that caused it.
func newCertificateSource(cfg ListenerTLS, log *slog.Logger) (*certfile.Keypair, error) {
	return certfile.New(context.Background(), certfile.Options{
		Certificate: cfg.Certificate,
		Key:         cfg.Key,
		Poll:        certificatePoll,
		Load: func(ctx context.Context) (tls.Certificate, *x509.Certificate, error) {
			pair, leaf, err := loadKeypair(cfg)
			if err != nil {
				return tls.Certificate{}, nil, err
			}

			if err = refuseCertificateFirmwareRejects(cfg, leaf, time.Now()); err != nil {
				return tls.Certificate{}, nil, err
			}

			report(ctx, log, cfg, leaf)

			return pair, leaf, nil
		},
		Refused: func(ctx context.Context, err error) {
			if log == nil {
				return
			}

			log.WarnContext(ctx,
				"the certificate file changed and the listener kept the one it had, so every printer "+
					"still verifies the previous certificate",
				"certificate", cfg.Certificate,
				"reason", err.Error())
		},
	})
}

// loadKeypair reads both halves and parses the leaf a client is shown.
//
// The leaf is parsed from the loaded chain rather than taken from
// tls.Certificate.Leaf, because what the checks below do with it must not
// depend on which release of the standard library populated that field.
func loadKeypair(cfg ListenerTLS) (tls.Certificate, *x509.Certificate, error) {
	if cfg.Certificate == "" {
		return tls.Certificate{}, nil, fmt.Errorf("%w: listener.tls.key is set and "+
			"listener.tls.certificate is empty, so there is nothing to present", ErrInvalidConfig)
	}

	if cfg.Key == "" {
		return tls.Certificate{}, nil, fmt.Errorf("%w: listener.tls.certificate is set and "+
			"listener.tls.key is empty, so nothing can sign the handshake", ErrInvalidConfig)
	}

	for name, path := range map[string]string{
		"listener.tls.certificate": cfg.Certificate,
		"listener.tls.key":         cfg.Key,
	} {
		if _, err := os.Stat(path); err != nil {
			return tls.Certificate{}, nil, fmt.Errorf("%w: %s is %q: %w", ErrInvalidConfig, name, path, err)
		}
	}

	pair, err := tls.LoadX509KeyPair(cfg.Certificate, cfg.Key)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: read the certificate at %s and the key at %s: %w",
			ErrInvalidConfig, cfg.Certificate, cfg.Key, err)
	}

	if len(pair.Certificate) == 0 {
		return tls.Certificate{}, nil, fmt.Errorf("%w: listener.tls.certificate is %q and holds no "+
			"certificate", ErrInvalidConfig, cfg.Certificate)
	}

	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: parse the leaf in listener.tls.certificate %q: %w",
			ErrInvalidConfig, cfg.Certificate, err)
	}

	return pair, leaf, nil
}

// refuseCertificateFirmwareRejects stops a certificate every printer would
// refuse, before a port is bound.
//
// The firmware offers one cipher suite,
// TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, so an RSA certificate cannot
// complete a handshake with it however well formed it is. It also verifies the
// dates, so a listener holding an expired leaf serves every printer a refusal
// while the process reports itself healthy, and that is the only failure this
// socket has where nothing looks wrong anywhere.
//
// The clock is named in the refusal because a machine with no battery-backed
// clock is the other way to reach it, and on that machine the certificate is
// fine and the reading is not.
func refuseCertificateFirmwareRejects(cfg ListenerTLS, leaf *x509.Certificate, now time.Time) error {
	if _, ecdsaKey := leaf.PublicKey.(*ecdsa.PublicKey); !ecdsaKey {
		return fmt.Errorf("%w: listener.tls.certificate is %q, whose public key is %T. xBuddy "+
			"firmware offers one cipher suite, TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, so only an "+
			"ECDSA certificate can complete a handshake with it",
			ErrInvalidConfig, cfg.Certificate, leaf.PublicKey)
	}

	if now.After(leaf.NotAfter) {
		return fmt.Errorf("%w: listener.tls.certificate is %q, which expired on %s, and the clock here "+
			"reads %s. xBuddy firmware verifies the certificate, so every printer would fail the "+
			"handshake", ErrInvalidConfig, cfg.Certificate, stamp(leaf.NotAfter), stamp(now))
	}

	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("%w: listener.tls.certificate is %q, which is not valid until %s, and the "+
			"clock here reads %s. xBuddy firmware verifies the certificate, so every printer would "+
			"fail the handshake", ErrInvalidConfig, cfg.Certificate, stamp(leaf.NotBefore), stamp(now))
	}

	return nil
}

// stamp renders an instant the way every other timestamp an operator compares
// it against is rendered.
func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// report writes the one startup line an operator reads the socket's profile
// off, and the two warnings that name a certificate which is valid today and
// will not serve every printer.
func report(ctx context.Context, log *slog.Logger, cfg ListenerTLS, leaf *x509.Certificate) {
	if log == nil {
		return
	}

	log.InfoContext(ctx, "the device listener serves TLS",
		"certificate", cfg.Certificate,
		"min_version", "1.2",
		"max_version", "1.2",
		"cipher_suite", tls.CipherSuiteName(firmwareCipherSuite),
		"dns_names", leaf.DNSNames,
		"ip_addresses", addresses(leaf.IPAddresses),
		"not_after", leaf.NotAfter.Format(time.DateOnly),
		// The subject a printer's connect.der must carry, so the one failure no
		// certificate of ours can check is two commands to settle:
		// openssl x509 -inform DER -in connect.der -noout -subject.
		"issued_by", leaf.Issuer.String())

	warnAboutAddressNames(ctx, log, cfg, leaf)
	warnAboutExpiry(ctx, log, cfg, leaf)
}

// warnAboutExpiry says at startup when a certificate is inside the last stretch
// of its validity. Past NotAfter is refused before a port is bound, so what is
// left to warn about is the month before that.
func warnAboutExpiry(ctx context.Context, log *slog.Logger, cfg ListenerTLS, leaf *x509.Certificate) {
	left := time.Until(leaf.NotAfter)
	if left > expiryWarning {
		return
	}

	log.WarnContext(ctx, "the certificate this listener presents expires soon, and every printer that "+
		"verifies it fails from that moment",
		"key", "listener.tls.certificate",
		"certificate", cfg.Certificate,
		"expires", stamp(leaf.NotAfter),
		"remaining", left.Round(time.Hour).String(),
		"remedy", "reissue with scripts/generate_certificates.sh before that date")
}

// warnAboutAddressNames says at startup when a certificate carries an address a
// printer configured by that address cannot verify.
//
// mbedTLS 2.x compares the string handed to mbedtls_ssl_set_hostname() against
// DNS SAN entries only, so a certificate with IP:192.168.1.50 and no
// DNS:192.168.1.50 beside it fails every IP-configured printer at once, with
// verify_result 0x04 and nothing naming the certificate. Earlier research
// records this as frozen.
//
// It is a warning and not a refusal: a deployment whose printers are configured
// by DNS name needs no IP entry at all, and the certificate is only wrong for
// the printers that use the address.
func warnAboutAddressNames(
	ctx context.Context, log *slog.Logger, cfg ListenerTLS, leaf *x509.Certificate,
) {
	for _, ip := range leaf.IPAddresses {
		if slices.Contains(leaf.DNSNames, ip.String()) {
			continue
		}

		log.WarnContext(ctx, "the printer certificate carries an IP address with no matching DNS name, "+
			"so a printer configured by that address will fail the handshake with 0x04",
			"certificate", cfg.Certificate,
			"address", ip.String(),
			"missing", "DNS:"+ip.String(),
			"remedy", "regenerate with scripts/generate_certificates.sh prusa, which emits both forms")
	}
}

// reportUnanswerableHellos names a printer's own hostname when this certificate
// cannot answer the ClientHello it sent.
//
// It is the only part of a certificate failure the server sees directly. The
// rest arrives as a fatal alert once the printer has already refused, and an
// alert says which check failed and never which value failed it. mbedTLS passes
// whatever is in prusa_printer_settings.ini to mbedtls_ssl_set_hostname, so a
// printer configured by an address sends that address in SNI and this is the
// string that has to appear as a DNS entry on the certificate.
//
// The callback returns nil, which keeps the configuration it was called on
// (crypto/tls: "If GetConfigForClient is nil or returns nil, then the original
// Config will be used"). It observes and refuses nothing, because what the
// printer will accept is the printer's verdict and not this listener's.
func reportUnanswerableHellos(
	log *slog.Logger, cfg ListenerTLS, source *certfile.Keypair,
) func(*tls.ClientHelloInfo) (*tls.Config, error) {
	if log == nil {
		return nil
	}

	return func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		pair, leaf := source.Current(hello.Context())

		if err := hello.SupportsCertificate(&pair); err != nil {
			log.WarnContext(hello.Context(),
				"a printer opened a handshake this certificate cannot answer, so it will refuse what "+
					"it is sent",
				"certificate", cfg.Certificate,
				"server_name", hello.ServerName,
				"remote", remote(hello),
				"reason", err.Error(),
				"dns_names", leaf.DNSNames)
		}

		return nil, nil
	}
}

// remote is the address a ClientHello arrived from, or empty when there is no
// connection behind it.
func remote(hello *tls.ClientHelloInfo) string {
	if hello.Conn == nil {
		return ""
	}

	return hello.Conn.RemoteAddr().String()
}

// addresses renders the IP entries of a certificate for one log line.
func addresses(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}

	return out
}
