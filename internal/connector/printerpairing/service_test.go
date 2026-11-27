package printerpairing_test

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing"
	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing/printerpairingtest"
)

// clock is a time a case moves by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// admissions records what the service approved.
type admissions struct {
	mu   sync.Mutex
	got  []string
	by   []string
	fail error
}

func (a *admissions) admit(_ context.Context, code, by string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.fail != nil {
		return "", a.fail
	}

	a.got = append(a.got, code)
	a.by = append(a.by, by)

	return "device-" + code, nil
}

func (a *admissions) approved() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.got...)
}

type rig struct {
	fake    *printerpairingtest.Printer
	service *printerpairing.Service
	clock   *clock
	admit   *admissions
}

// newRig is a service whose own poll never ticks, so a case moves a pairing
// with Step and the clock alone.
func newRig(t *testing.T) rig {
	t.Helper()

	r := rig{
		fake:  printerpairingtest.New(t),
		clock: &clock{now: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)},
		admit: &admissions{},
	}

	r.service = printerpairing.NewService(printerpairing.Options{
		Client:       printerpairing.NewClient(anyAddress),
		Admit:        r.admit.admit,
		Now:          r.clock.Now,
		PollInterval: time.Hour,
	})
	t.Cleanup(r.service.Close)

	return r
}

func (r rig) start(t *testing.T) printerpairing.Pairing {
	t.Helper()

	started, err := r.service.Start(t.Context(), printerpairing.StartRequest{
		Printer: r.fake.Address(),
		Connect: printerpairing.Connect{Host: "192.168.1.50", Port: 8080},
		By:      "user:alice",
	})
	require.NoError(t, err)
	require.Equal(t, printerpairing.StateWaiting, started.State)

	return started
}

func (r rig) state(t *testing.T, id string) printerpairing.State {
	t.Helper()

	read, err := r.service.Get(id)
	require.NoError(t, err)

	return read.State
}

var loopback = netip.MustParseAddr("127.0.0.1")

func arrival(code string, from netip.Addr) printerpairing.Arrival {
	return printerpairing.Arrival{Connector: "prusa-xbuddy", Code: code, Serial: "SN1", Model: "1.3.4", From: from}
}

func TestARegistrationAfterTheYesIsApprovedByWhoeverPaired(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	require.True(t, r.service.Step(t.Context(), started.ID), "a question nobody answered still waits")

	r.fake.Accept()
	require.False(t, r.service.Step(t.Context(), started.ID))
	require.Equal(t, printerpairing.StateApproved, r.state(t, started.ID))

	require.True(t, r.service.Arrived(t.Context(), arrival("REGCODE001", loopback)))
	require.Equal(t, []string{"REGCODE001"}, r.admit.approved())
	require.Equal(t, []string{"user:alice"}, r.admit.by,
		"the printer is admitted by the person who paired it, so it is theirs")

	read, err := r.service.Get(started.ID)
	require.NoError(t, err)
	require.Equal(t, printerpairing.StateAdmitted, read.State)
	require.Equal(t, "device-REGCODE001", read.Admitted.DeviceID)
	require.Equal(t, "SN1", read.Admitted.Serial)
}

func TestARegistrationBeforeTheYesWaitsForIt(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	require.True(t, r.service.Arrived(t.Context(), arrival("EARLY00001", loopback)))
	require.Empty(t, r.admit.approved(), "nobody at the printer has said yes yet")

	r.fake.Accept()
	r.service.Step(t.Context(), started.ID)

	require.Equal(t, []string{"EARLY00001"}, r.admit.approved())
	require.Equal(t, printerpairing.StateAdmitted, r.state(t, started.ID))
}

func TestARegistrationBeforeACancelIsNeverApproved(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	require.True(t, r.service.Arrived(t.Context(), arrival("EARLY00002", loopback)))

	r.fake.Cancel()
	r.service.Step(t.Context(), started.ID)

	require.Equal(t, printerpairing.StateRefused, r.state(t, started.ID))
	require.Empty(t, r.admit.approved(), "a person at the printer said no")
}

func TestAQuestionThatClosedUnansweredApprovesNothing(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	require.True(t, r.service.Arrived(t.Context(), arrival("EARLY00003", loopback)))

	// The printer forgets a question it timed out, and forgets an approval it
	// already answered. The two look the same from here.
	r.fake.TimeOut()
	r.service.Step(t.Context(), started.ID)

	require.Equal(t, printerpairing.StateExpired, r.state(t, started.ID))
	require.Empty(t, r.admit.approved())
}

func TestARegistrationFromAnotherAddressIsNotThePairings(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	r.fake.Accept()
	r.service.Step(t.Context(), started.ID)

	require.False(t, r.service.Arrived(t.Context(), arrival("STRANGER01", netip.MustParseAddr("192.168.1.99"))))
	require.False(t, r.service.Arrived(t.Context(), arrival("NOWHERE001", netip.Addr{})))
	require.Empty(t, r.admit.approved())
	require.Equal(t, printerpairing.StateApproved, r.state(t, started.ID),
		"the pairing still waits for its own printer")

	require.True(t, r.service.Arrived(t.Context(), arrival("MAPPED0001", netip.MustParseAddr("::ffff:127.0.0.1"))),
		"an IPv4 address seen on an IPv6 socket is the same printer")
	require.Equal(t, []string{"MAPPED0001"}, r.admit.approved())
}

func TestAPairingApprovesOneRegistration(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	r.fake.Accept()
	r.service.Step(t.Context(), started.ID)

	require.True(t, r.service.Arrived(t.Context(), arrival("FIRST00001", loopback)))
	require.False(t, r.service.Arrived(t.Context(), arrival("SECOND0001", loopback)),
		"a second registration from the address waits for a person")
	require.Equal(t, []string{"FIRST00001"}, r.admit.approved())
}

func TestARegistrationAfterTheWindowWaitsForAPerson(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	r.fake.Accept()
	r.service.Step(t.Context(), started.ID)

	r.clock.Add(printerpairing.RegistrationWindow + time.Second)

	require.False(t, r.service.Arrived(t.Context(), arrival("LATE000001", loopback)))
	require.Empty(t, r.admit.approved())
	require.Equal(t, printerpairing.StateExpired, r.state(t, started.ID))
}

func TestAQuestionNobodyAnswersRunsOut(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	started := r.start(t)

	r.clock.Add(printerpairing.QuestionWindow + time.Second)

	require.False(t, r.service.Step(t.Context(), started.ID))
	require.Equal(t, printerpairing.StateExpired, r.state(t, started.ID))
	require.False(t, r.service.Arrived(t.Context(), arrival("LATE000002", loopback)))
}

func TestAWithdrawnPairingApprovesNothing(t *testing.T) {
	t.Parallel()

	t.Run("while the question is on the screen", func(t *testing.T) {
		t.Parallel()

		r := newRig(t)
		started := r.start(t)

		cancelled, err := r.service.Cancel(t.Context(), started.ID)
		require.NoError(t, err)
		require.Equal(t, printerpairing.StateWithdrawn, cancelled.State)
		require.False(t, r.fake.Showing(), "the question was left on the printer's screen")
		require.False(t, r.service.Arrived(t.Context(), arrival("AFTER00001", loopback)))
		require.Empty(t, r.admit.approved())
	})

	t.Run("after the yes", func(t *testing.T) {
		t.Parallel()

		r := newRig(t)
		started := r.start(t)

		r.fake.Accept()
		r.service.Step(t.Context(), started.ID)

		cancelled, err := r.service.Cancel(t.Context(), started.ID)
		require.NoError(t, err)
		require.Equal(t, printerpairing.StateWithdrawn, cancelled.State)
		require.False(t, r.service.Arrived(t.Context(), arrival("AFTER00002", loopback)))
		require.Empty(t, r.admit.approved())
	})
}

func TestAnApprovalThatFailsLeavesTheRegistrationToAPerson(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.admit.fail = errors.New("the store is gone")
	started := r.start(t)

	r.fake.Accept()
	r.service.Step(t.Context(), started.ID)

	require.True(t, r.service.Arrived(t.Context(), arrival("FAILS00001", loopback)))
	require.Equal(t, printerpairing.StateApproved, r.state(t, started.ID),
		"the pairing did not admit anything, so it does not say it did")
}

func TestOnePrinterHasOnePairingOpen(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.start(t)

	_, err := r.service.Start(t.Context(), printerpairing.StartRequest{
		Printer: r.fake.Address(),
		Connect: printerpairing.Connect{Host: "192.168.1.50", Port: 8080},
	})
	require.ErrorIs(t, err, printerpairing.ErrOpen)
}

func TestAServerAddressThePrinterCannotUseIsRefusedBeforeItIsSent(t *testing.T) {
	t.Parallel()

	r := newRig(t)

	for _, connect := range []printerpairing.Connect{
		{Host: "", Port: 8080},
		{Host: "localhost", Port: 8080},
		{Host: "127.0.0.1", Port: 8080},
		{Host: "0.0.0.0", Port: 8080},
		{Host: "192.168.1.50:8080", Port: 8080},
		{Host: "a-very-long-name.for-a-workshop.example", Port: 8080},
		{Host: "192.168.1.50", Port: 0},
	} {
		_, err := r.service.Start(t.Context(), printerpairing.StartRequest{Printer: r.fake.Address(), Connect: connect})
		require.ErrorIs(t, err, printerpairing.ErrConnect, connect.Host)
	}

	require.Empty(t, r.fake.Requests(), "a request the printer would refuse was sent")
}

func TestAnUnknownPairingIsNotFound(t *testing.T) {
	t.Parallel()

	r := newRig(t)

	_, err := r.service.Get("nothing")
	require.ErrorIs(t, err, printerpairing.ErrUnknownPairing)

	_, err = r.service.Cancel(t.Context(), "nothing")
	require.ErrorIs(t, err, printerpairing.ErrUnknownPairing)
}

// TestThePollHearsTheYesOnItsOwn runs the service's own timer against the
// fake, so the loop that calls Step is covered too.
func TestThePollHearsTheYesOnItsOwn(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	admitted := &admissions{}

	service := printerpairing.NewService(printerpairing.Options{
		Client:       printerpairing.NewClient(anyAddress),
		Admit:        admitted.admit,
		PollInterval: 10 * time.Millisecond,
	})
	t.Cleanup(service.Close)

	started, err := service.Start(t.Context(), printerpairing.StartRequest{
		Printer: fake.Address(),
		Connect: printerpairing.Connect{Host: "192.168.1.50", Port: 8080},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return fake.Polls() > 0 }, 5*time.Second, 5*time.Millisecond)

	fake.Accept()

	require.Eventually(t, func() bool {
		read, readErr := service.Get(started.ID)

		return readErr == nil && read.State == printerpairing.StateApproved
	}, 5*time.Second, 5*time.Millisecond)

	polls := fake.Polls()

	require.True(t, service.Arrived(t.Context(), arrival("LOOPED0001", loopback)))
	require.Equal(t, []string{"LOOPED0001"}, admitted.approved())
	require.Equal(t, polls, fake.Polls(), "the printer was asked again after it answered")
}
