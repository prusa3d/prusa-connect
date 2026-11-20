// Package certfile holds the TLS keypair a listener presents, read from a
// certificate file and a key file, and read again when either file changes.
//
// Something outside the process renews a certificate and replaces the files
// under a running server: certbot, a mounted secret, an operator. A listener
// that read them once at startup needs a restart to present the new pair, and
// a restart drops every connection the listener holds.
package certfile

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"sync"
	"time"
)

// DefaultPoll is how often a handshake looks at the files when Options.Poll is
// zero. A handshake that finds the last look younger than this presents what
// the Keypair already holds and touches no file, so a mount that has stopped
// answering costs one handshake a poll rather than every client that dials
// while it is stuck.
const DefaultPoll = time.Second

// ErrNoLoad is returned by New when Options.Load is nil.
var ErrNoLoad = errors.New("certfile: Options.Load is nil")

// Load reads the keypair and returns the leaf certificate a client is shown.
// It is also where a caller refuses a pair its listener must not serve. An
// error at startup is returned by New. An error after a file changed leaves
// the pair already held in place.
//
// ctx is New's context at startup and the handshake's context on a reload.
type Load func(ctx context.Context) (tls.Certificate, *x509.Certificate, error)

// Options names the files to watch and how to read them.
type Options struct {
	// Certificate and Key are the files whose modification times decide when
	// Load runs again. Both are watched, because a renewal is two writes.
	Certificate string
	Key         string

	// Load reads the pair. It is required.
	Load Load

	// Refused is told when a changed file did not load, with the context of
	// the handshake that looked. It is called once per change, not once per
	// handshake. Nil ignores it.
	Refused func(ctx context.Context, err error)

	// Poll is how often a handshake looks at the files. Zero is DefaultPoll.
	Poll time.Duration
}

// Keypair is the certificate a listener presents now. Its GetCertificate is
// the tls.Config field of the same name.
type Keypair struct {
	files   [2]string
	load    Load
	refused func(context.Context, error)
	poll    time.Duration

	// mu guards the fields below. The stat that decides whether to read runs
	// outside it, so a file system that has stopped answering holds up the
	// one handshake that went looking and not every handshake behind it.
	mu     sync.Mutex
	looked time.Time
	stamp  time.Time
	pair   tls.Certificate
	leaf   *x509.Certificate
}

// New reads the pair a listener starts with. An error from Load is returned
// as it is, so the listener refuses to start on a pair it could not serve.
func New(ctx context.Context, opts Options) (*Keypair, error) {
	if opts.Load == nil {
		return nil, ErrNoLoad
	}

	poll := opts.Poll
	if poll <= 0 {
		poll = DefaultPoll
	}

	k := &Keypair{
		files:   [2]string{opts.Certificate, opts.Key},
		load:    opts.Load,
		refused: opts.Refused,
		poll:    poll,
	}
	k.stamp = k.newestChange()

	pair, leaf, err := k.load(ctx)
	if err != nil {
		return nil, err
	}

	k.pair, k.leaf = pair, leaf

	return k, nil
}

// GetCertificate answers a ClientHello with the current pair. A reload it
// causes gets the handshake's context.
//
// Leave tls.Config.Certificates empty when using it. Go consults
// GetCertificate only when Certificates is empty or the ClientHello carried a
// server name, so filling both serves the pair read at startup to every
// client that dials by address.
func (k *Keypair) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	pair, _ := k.Current(helloContext(hello))

	return &pair, nil
}

// Current is the pair to present now and its leaf, read again first when
// either file changed since the last look.
//
// A file that cannot be read is no reason to stop presenting the pair already
// held: clients still verify it. So a stat that fails and a load that fails
// both leave the held pair in place.
func (k *Keypair) Current(ctx context.Context) (tls.Certificate, *x509.Certificate) {
	if !k.lookDue() {
		return k.held()
	}

	stamp := k.newestChange()

	k.mu.Lock()
	defer k.mu.Unlock()

	if stamp.IsZero() || !stamp.After(k.stamp) {
		return k.pair, k.leaf
	}

	// Recorded whether or not the load below succeeds, so a replacement that
	// does not load is reported once and not once per handshake. It covers
	// both files, so a renewal that lands the certificate first and the key a
	// moment later is read again when the key arrives.
	k.stamp = stamp

	pair, leaf, err := k.load(ctx)
	if err != nil {
		if k.refused != nil {
			k.refused(ctx, err)
		}

		return k.pair, k.leaf
	}

	k.pair, k.leaf = pair, leaf

	return k.pair, k.leaf
}

// lookDue reports whether the poll interval has passed since the files were
// last looked at, and records the look when it has.
func (k *Keypair) lookDue() bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	now := time.Now()
	if now.Sub(k.looked) < k.poll {
		return false
	}

	k.looked = now

	return true
}

// held is the pair already read, for a handshake with no reason to look at
// the files.
func (k *Keypair) held() (tls.Certificate, *x509.Certificate) {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.pair, k.leaf
}

// newestChange is the later of the two files' modification times, or the zero
// time when either cannot be read at all. A file that is missing for a moment
// during a replacement then leads to no load, rather than to a load that fails.
func (k *Keypair) newestChange() time.Time {
	var newest time.Time

	for _, path := range k.files {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}
		}

		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}

	return newest
}

// helloContext is the context of the handshake a ClientHello belongs to.
// crypto/tls gives every handshake one, and a ClientHelloInfo built by hand has
// none, which is the one case that gets an empty context.
func helloContext(hello *tls.ClientHelloInfo) context.Context {
	if hello == nil || hello.Context() == nil {
		return context.Background()
	}

	return hello.Context()
}
