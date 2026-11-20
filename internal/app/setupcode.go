package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The setup code the server makes itself. Twelve symbols of a 32-letter
// alphabet are 60 random bits. The alphabet is Crockford's base 32, which
// leaves out I, L, O and U, so no symbol looks like another.
const (
	setupCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	setupCodeSymbols  = 12
	setupCodeGroup    = 4
	setupCodeBits     = 5
	setupCodeMask     = 1<<setupCodeBits - 1
)

// The limit on wrong codes. One address may send five in fifteen minutes and
// is then refused until the fifteen minutes from its first wrong code are over.
// At that rate, one address needs more than a billion years to try half of the
// 2^60 codes.
const (
	setupCodeFree    = 5
	setupCodeWindow  = 15 * time.Minute
	setupCodeClients = 1024
	// setupCodeFullLogEvery is how often a full table, a full network or wrong
	// codes nobody counts are logged.
	setupCodeFullLogEvery = time.Minute
	// setupCodeNetworkSlots is how many addresses of one IPv6 /64 the table
	// counts. A host holds a whole /64, so without it one host could fill the
	// table and lock every new address out.
	setupCodeNetworkSlots = 16
	// setupCodeIPv6Network is the prefix one IPv6 host is given.
	setupCodeIPv6Network = 64
)

// setupGate holds the setup code of one process and the wrong codes each
// client address has sent. The code is kept as a SHA-256 digest and compared
// with crypto/subtle, so a comparison takes as long whichever symbol is wrong.
type setupGate struct {
	logger *slog.Logger
	now    func() time.Time

	mu       sync.Mutex
	digest   [sha256.Size]byte
	live     bool
	failures map[string]codeFailures
	// fullLogged is when a full table was last logged.
	fullLogged time.Time
	// networkLogged is when a full /64 was last logged.
	networkLogged time.Time
	// uncountedLogged is when wrong codes from loopback were last logged, and
	// uncounted is how many came since.
	uncountedLogged time.Time
	uncounted       int
	// proxyWarned says the warning about an unlisted proxy was written.
	proxyWarned bool
}

// codeFailures is what one client address has spent in its window.
type codeFailures struct {
	count int
	first time.Time
	// network is the /64 of an IPv6 address, and empty for IPv4.
	network string
}

// newSetupGate holds the code core.auth.setup_code sets, or makes a new one
// when the key is empty. It returns the code to print, which is empty for a
// configured code: the operator already knows it, and a log is read by more
// people than the file is.
func newSetupGate(configured string, logger *slog.Logger, now func() time.Time) (*setupGate, string, error) {
	shown := ""
	code := configured

	if code == "" {
		made, err := makeSetupCode()
		if err != nil {
			return nil, "", err
		}

		code, shown = made, made
	}

	gate := &setupGate{
		logger:   logger,
		now:      now,
		digest:   sha256.Sum256([]byte(config.NormalSetupCode(code))),
		live:     true,
		failures: make(map[string]codeFailures),
	}

	return gate, shown, nil
}

// makeSetupCode draws a new code and writes it in groups of four, the way a
// person reads it aloud.
func makeSetupCode() (string, error) {
	var raw [8]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("draw a setup code: %w", err)
	}

	bits := binary.BigEndian.Uint64(raw[:])

	var out strings.Builder

	for i := range setupCodeSymbols {
		if i > 0 && i%setupCodeGroup == 0 {
			out.WriteByte('-')
		}

		out.WriteByte(setupCodeAlphabet[bits&setupCodeMask])
		bits >>= setupCodeBits
	}

	return out.String(), nil
}

// admit lets a first run through when it sent the code, and charges a wrong
// one to the address it came from. The code sent is never logged.
//
// Wrong codes are counted per full address, so one host on a home network
// cannot use up the attempts of the others behind the same IPv6 /64. The table
// bound is what stops a host that holds many addresses: 1024 addresses times 5
// codes is 5120 guesses per window against 2^60 codes, and one /64 takes at
// most 16 of those addresses. A caller on loopback is not counted, so the owner
// at the machine is never refused.
func (g *setupGate) admit(ctx context.Context, client httpapi.SetupClient, presented string) error {
	key, loopback := setupClientKey(client.Address)
	network := setupNetworkOf(key)
	now := g.now()
	sent := sha256.Sum256([]byte(config.NormalSetupCode(presented)))

	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.live {
		return fault.AuthSetupClosed.New("this server has already recorded what it does about identity")
	}

	g.sweep(now)
	g.warnUnlistedProxy(ctx, client)

	record, held := g.failures[key]

	if !loopback {
		if held && record.count >= setupCodeFree {
			return setupCodeLimited(record.first.Add(setupCodeWindow).Sub(now),
				"this address sent too many wrong setup codes")
		}

		// A full table refuses an address it has not seen, rather than forget
		// one it has: forgetting would give that address a new allowance.
		if !held && len(g.failures) >= setupCodeClients {
			wait := g.firstFreeSlot("").Sub(now)
			g.logThrottled(ctx, &g.fullLogged, now,
				"the setup code refused a new address because too many addresses sent wrong codes",
				"client", client.Address, "addresses", len(g.failures), "wait", wait.Round(time.Second).String())

			return setupCodeLimited(wait, "too many addresses sent wrong setup codes")
		}

		if !held && network != "" && g.countNetwork(network) >= setupCodeNetworkSlots {
			wait := g.firstFreeSlot(network).Sub(now)
			g.logThrottled(ctx, &g.networkLogged, now,
				"the setup code refused a new address because its IPv6 network sent too many wrong codes",
				"client", client.Address, "network", network, "wait", wait.Round(time.Second).String())

			return setupCodeLimited(wait, "too many addresses of this network sent wrong setup codes")
		}
	}

	if subtle.ConstantTimeCompare(sent[:], g.digest[:]) == 1 {
		return nil
	}

	if loopback {
		g.logUncounted(ctx, now)
	} else {
		if !held {
			record = codeFailures{first: now, network: network}
		}

		record.count++
		g.failures[key] = record

		g.logger.WarnContext(ctx, "a first run sent a wrong setup code", "client", client.Address,
			"wrong_codes", record.count, "allowed", setupCodeFree)

		if record.count == setupCodeFree {
			g.logger.WarnContext(ctx, "a client address sent too many wrong setup codes and is refused for a while",
				"client", client.Address, "until", record.first.Add(setupCodeWindow).Format(time.RFC3339))
		}
	}

	if presented == "" {
		return fault.AuthSetupCodeRefused.New("the request carried no setup code")
	}

	return fault.AuthSetupCodeRefused.New("the setup code does not match the one this server holds")
}

// logUncounted logs wrong codes from loopback at most once a minute, with how
// many came since the last line. Nothing limits them, so a line for each one
// would let a caller behind a proxy on this machine fill the log, push the
// code out of `docker logs` and wear out an SD card. The caller holds the lock.
func (g *setupGate) logUncounted(ctx context.Context, now time.Time) {
	g.uncounted++

	if !g.uncountedLogged.IsZero() && now.Sub(g.uncountedLogged) < setupCodeFullLogEvery {
		return
	}

	g.logger.WarnContext(ctx, "wrong setup codes came from this machine, which is not limited",
		"wrong_codes", g.uncounted, "since", setupCodeFullLogEvery.String())

	g.uncountedLogged = now
	g.uncounted = 0
}

// warnUnlistedProxy warns once that a proxy on this machine forwards requests
// that core.trusted_proxies does not list. Every client behind it then counts
// as this machine, which is never limited. The caller holds the lock.
func (g *setupGate) warnUnlistedProxy(ctx context.Context, client httpapi.SetupClient) {
	if !client.Forwarded || g.proxyWarned {
		return
	}

	g.proxyWarned = true
	g.logger.WarnContext(ctx, "a proxy on this machine forwards requests and core.trusted_proxies does not list it, "+
		"so wrong setup codes from every client behind it go unlimited; list the proxy in core.trusted_proxies",
		"client", client.Address)
}

// logThrottled writes one line at most once a minute, so a flood of refusals
// fills neither the table nor the log. The caller holds the lock.
func (g *setupGate) logThrottled(ctx context.Context, last *time.Time, now time.Time, msg string, args ...any) {
	if !last.IsZero() && now.Sub(*last) < setupCodeFullLogEvery {
		return
	}

	*last = now
	g.logger.WarnContext(ctx, msg, args...)
}

// countNetwork is how many counted addresses belong to one /64. The caller
// holds the lock.
func (g *setupGate) countNetwork(network string) int {
	n := 0

	for _, record := range g.failures {
		if record.network == network {
			n++
		}
	}

	return n
}

// setupNetworkOf is the /64 of an IPv6 address, and empty for anything else.
func setupNetworkOf(key string) string {
	parsed, err := netip.ParseAddr(key)
	if err != nil || !parsed.Is6() {
		return ""
	}

	network, err := parsed.Prefix(setupCodeIPv6Network)
	if err != nil {
		return ""
	}

	return network.String()
}

// firstFreeSlot is when the oldest counted address of one /64 leaves the
// table, or of the whole table when network is empty. The caller holds the
// lock.
func (g *setupGate) firstFreeSlot(network string) time.Time {
	var earliest time.Time

	for _, record := range g.failures {
		if network != "" && record.network != network {
			continue
		}

		if earliest.IsZero() || record.first.Before(earliest) {
			earliest = record.first
		}
	}

	return earliest.Add(setupCodeWindow)
}

// setupClientKey is the key one client is counted under, and whether it is on
// loopback. The whole address is the key; an address this cannot parse is kept
// as it arrived, which is what a unix socket peer is.
func setupClientKey(client string) (string, bool) {
	parsed, err := netip.ParseAddr(client)
	if err != nil {
		return client, false
	}

	parsed = parsed.Unmap().WithZone("")

	return parsed.String(), parsed.IsLoopback()
}

// consume ends the code. It runs once the first run has recorded its decision,
// so the code works one time and is gone from memory afterwards.
func (g *setupGate) consume() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.live = false
	g.digest = [sha256.Size]byte{}
	clear(g.failures)
}

// sweep drops the addresses whose window is over, so the table holds only what
// still counts.
func (g *setupGate) sweep(now time.Time) {
	for key, record := range g.failures {
		if now.Sub(record.first) >= setupCodeWindow {
			delete(g.failures, key)
		}
	}
}

// setupCodeLimited is the refusal for a client that has to wait. The message
// names the wait in minutes, so a page shows it as it is, and the param carries
// the whole seconds that Retry-After copies.
func setupCodeLimited(wait time.Duration, why string) *fault.Fault {
	seconds := max(int64(math.Ceil(wait.Seconds())), 1)
	minutes := max(int64(math.Ceil(wait.Minutes())), 1)

	unit := "minutes"
	if minutes == 1 {
		unit = "minute"
	}

	return fault.AuthSetupCodeLimited.New(fmt.Sprintf("%s. Try again in %d %s.", why, minutes, unit)).
		With(fault.IntParam(httpapi.RetryAfterParam, seconds))
}

// announceSetupCode prints the code on standard error in a block a person
// finds in `docker logs` or `journalctl`, whatever the log level says. The log
// line beside it carries the same code for a log collector.
//
// A server that serves no web app, a build without one or one with
// core.frontend.enabled off, has no page to open: the block names the API
// door instead, so nobody is sent to an address that answers 404.
func announceSetupCode(out io.Writer, code string, where []string, door string, webApp bool) {
	open := "  This server is not set up yet. Open it in a browser and enter\n" +
		"  this setup code:\n"

	switch {
	case !webApp:
		open = "  This server is not set up yet, and it serves no web app. Finish the\n" +
			"  first run over the API, at " + door + ", with this setup code:\n"
	case len(where) > 0:
		var addresses strings.Builder
		for _, address := range where {
			addresses.WriteString("      " + address + "\n")
		}

		open = "  This server is not set up yet. Open one of these addresses in a\n" +
			"  browser:\n\n" + addresses.String() + "\n" +
			"  and enter this setup code:\n"
	}

	// A write to standard error that fails has nowhere to report itself, and
	// the log line carries the same code.
	_, _ = fmt.Fprintf(out, "\n%s\n      %s\n\n  The code works once. A restart prints a new one.\n\n", open, code)
}

// announceOrSpend spends the setup code when no door is open, and otherwise
// returns what prints it. A recorded decision with an empty account store still
// has a door: POST /api/v1/auth/setup creates the first administrator, and it
// takes the same code.
//
// The print is returned for the start to run once every port is open. A copy
// started on a port another copy holds never serves, and the browser reaches
// the copy that does, which refuses every code but its own.
func announceOrSpend(
	ctx context.Context, first *firstRun, code *setupGate, shown string,
	core config.Core, opts Options, logger *slog.Logger,
) (func(), error) {
	needsAccount, err := first.needsAccount(ctx)
	if err != nil {
		return nil, err
	}

	if !first.Pending() && !needsAccount {
		code.consume()

		return func() {}, nil
	}

	_, carried := httpapi.AppMount(httpapi.AppOptions{})

	return func() {
		announcePending(ctx, logger, opts.stderr(), shown, first.Pending(), browserAddresses(core.API),
			carried && core.Frontend.On())
	}, nil
}

// announcePending tells the operator the server waits for its first
// administrator, which path takes the code, and where the code comes from. A
// code this process made is printed; a code from core.auth.setup_code is only
// named, because whoever wrote it knows it.
//
// pending is an install with no decision, whose door is POST /api/v1/setup.
// Otherwise the decision is recorded and no account exists, and the door is
// POST /api/v1/auth/setup.
func announcePending(
	ctx context.Context, logger *slog.Logger, stderr io.Writer, shown string, pending bool, where []string,
	webApp bool,
) {
	state := "this server has recorded nothing about identity; it serves the first run and nothing else"
	door := "POST " + httpapi.SetupPath()

	if !pending {
		state = "this server has no administrator yet"
		door = "POST /api/v1/auth/setup"
	}

	if shown == "" {
		logger.WarnContext(ctx, state+", and asks for the setup code core.auth.setup_code sets", "setup", door)

		return
	}

	logger.WarnContext(ctx, state+", and asks for this setup code", "setup", door, "setup_code", shown)

	announceSetupCode(stderr, shown, where, door, webApp)
}
