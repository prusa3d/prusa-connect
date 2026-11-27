package printerpairing_test

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing"
	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing/printerpairingtest"
)

func anyAddress(netip.Addr) bool { return true }

func printerAt(t *testing.T, fake *printerpairingtest.Printer) printerpairing.Printer {
	t.Helper()

	printer, err := printerpairing.ParsePrinter(fake.Address())
	require.NoError(t, err)

	return printer
}

func request(t *testing.T) printerpairing.Request {
	t.Helper()

	code, err := printerpairing.NewCode()
	require.NoError(t, err)

	return printerpairing.Request{
		Code:    code,
		Name:    "Workshop",
		Connect: printerpairing.Connect{Host: "192.168.1.50", Port: 8080},
	}
}

func TestAPrinterAsksThePersonAndSaysYesOnce(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	client := printerpairing.NewClient(anyAddress)
	asked := request(t)

	reached, err := client.Start(t.Context(), printerAt(t, fake), asked)
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("127.0.0.1"), reached,
		"the address the request reached is what the printer's registration is matched against")

	sent := fake.Requests()
	require.Len(t, sent, 1)
	require.Equal(t, "Prusa Connect", sent[0].Client)
	require.Equal(t, asked.Code, sent[0].Code)
	require.Equal(t, "192.168.1.50", sent[0].Connect.Host)
	require.Equal(t, 8080, sent[0].Connect.Port)
	require.NotEmpty(t, sent[0].APIKey,
		"firmware without pairing answers a request with no key with a digest challenge")

	state, err := client.Poll(t.Context(), printerAt(t, fake), asked.Code)
	require.NoError(t, err)
	require.Equal(t, printerpairing.Waiting, state)

	fake.Accept()

	state, err = client.Poll(t.Context(), printerAt(t, fake), asked.Code)
	require.NoError(t, err)
	require.Equal(t, printerpairing.Approved, state)

	state, err = client.Poll(t.Context(), printerAt(t, fake), asked.Code)
	require.NoError(t, err)
	require.Equal(t, printerpairing.Unknown, state, "the printer answers an approval once")
}

func TestAPersonWhoCancelsIsARefusal(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	client := printerpairing.NewClient(anyAddress)
	asked := request(t)

	_, err := client.Start(t.Context(), printerAt(t, fake), asked)
	require.NoError(t, err)

	fake.Cancel()

	state, err := client.Poll(t.Context(), printerAt(t, fake), asked.Code)
	require.NoError(t, err)
	require.Equal(t, printerpairing.Refused, state)
}

func TestAWithdrawalTakesTheQuestionOffTheScreen(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	client := printerpairing.NewClient(anyAddress)
	asked := request(t)

	_, err := client.Start(t.Context(), printerAt(t, fake), asked)
	require.NoError(t, err)
	require.True(t, fake.Showing())

	require.NoError(t, client.Withdraw(t.Context(), printerAt(t, fake), asked.Code))
	require.False(t, fake.Showing())
	require.Equal(t, []string{asked.Code}, fake.Withdrawn())

	require.NoError(t, client.Withdraw(t.Context(), printerAt(t, fake), asked.Code),
		"a question already gone is gone either way")
}

func TestEveryRefusalThePrinterGivesIsNamed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prepare func(*printerpairingtest.Printer)
		change  func(*printerpairing.Request)
		want    error
	}{
		{name: "printing", prepare: (*printerpairingtest.Printer).Busy, want: printerpairing.ErrBusy},
		{
			name:    "disabled until reboot",
			prepare: (*printerpairingtest.Printer).Disable,
			want:    printerpairing.ErrDisabled,
		},
		{
			name:    "firmware without pairing",
			prepare: (*printerpairingtest.Printer).Legacy,
			want:    printerpairing.ErrUnsupported,
		},
		{
			name:   "a server name the printer cannot store",
			change: func(r *printerpairing.Request) { r.Connect.Host = "printers.workshop.example.net" },
			want:   printerpairing.ErrInvalid,
		},
		{
			name:   "a code in the wrong shape",
			change: func(r *printerpairing.Request) { r.Code = "ABCDEFGH" },
			want:   printerpairing.ErrInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := printerpairingtest.New(t)
			if tc.prepare != nil {
				tc.prepare(fake)
			}

			asked := request(t)
			if tc.change != nil {
				tc.change(&asked)
			}

			_, err := printerpairing.NewClient(anyAddress).Start(t.Context(), printerAt(t, fake), asked)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestALockedPrinterSaysForHowLong(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	fake.LockFor(240)

	_, err := printerpairing.NewClient(anyAddress).Start(t.Context(), printerAt(t, fake), request(t))
	require.ErrorIs(t, err, printerpairing.ErrLockedOut)

	locked, ok := errors.AsType[*printerpairing.LockedOutError](err)
	require.True(t, ok)
	require.Equal(t, 240*time.Second, locked.RetryAfter)
}

func TestASecondRequestWhileTheFirstIsOnTheScreenIsPending(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)
	client := printerpairing.NewClient(anyAddress)

	_, err := client.Start(t.Context(), printerAt(t, fake), request(t))
	require.NoError(t, err)

	_, err = client.Start(t.Context(), printerAt(t, fake), request(t))
	require.ErrorIs(t, err, printerpairing.ErrPending)
}

func TestTheClientDialsOnlyTheLocalNetwork(t *testing.T) {
	t.Parallel()

	fake := printerpairingtest.New(t)

	_, err := printerpairing.NewClient(nil).Start(t.Context(), printerAt(t, fake), request(t))
	require.ErrorIs(t, err, printerpairing.ErrAddressRefused,
		"loopback reaches the services beside this server, and a printer is never there")
	require.Empty(t, fake.Requests())

	for _, tc := range []struct {
		addr  string
		local bool
	}{
		{"192.168.1.20", true},
		{"10.0.0.7", true},
		{"172.16.4.4", true},
		{"169.254.10.10", true},
		{"100.101.102.103", true},
		{"fd00::1", true},
		{"::ffff:192.168.1.20", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"8.8.8.8", false},
		{"0.0.0.0", false},
		{"224.0.0.251", false},
	} {
		require.Equal(t, tc.local, printerpairing.LocalNetwork(netip.MustParseAddr(tc.addr)), tc.addr)
	}
}

func TestAPrinterAddressIsAHostAndAPort(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		typed string
		want  string
		bad   bool
	}{
		{typed: "192.168.1.20", want: "192.168.1.20:80"},
		{typed: " 192.168.1.20:8080 ", want: "192.168.1.20:8080"},
		{typed: "mk4.local", want: "mk4.local:80"},
		{typed: "[fd00::1]:81", want: "[fd00::1]:81"},
		{typed: "", bad: true},
		{typed: "http://192.168.1.20", bad: true},
		{typed: "192.168.1.20/api", bad: true},
		{typed: "user@192.168.1.20", bad: true},
		{typed: "192.168.1.20:0", bad: true},
		{typed: "192.168.1.20:99999", bad: true},
	} {
		printer, err := printerpairing.ParsePrinter(tc.typed)
		if tc.bad {
			require.ErrorIs(t, err, printerpairing.ErrAddressRefused, tc.typed)

			continue
		}

		require.NoError(t, err, tc.typed)
		require.Equal(t, tc.want, printer.String())
	}
}

func TestACodeIsOneTheFirmwareTakes(t *testing.T) {
	t.Parallel()

	// The firmware's own alphabet, from lib/WUI/link_content/pairing.cpp.
	const firmware = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	seen := map[string]bool{}

	for range 200 {
		code, err := printerpairing.NewCode()
		require.NoError(t, err)
		require.Len(t, code, 9)
		require.Equal(t, byte('-'), code[4])

		for i, c := range code {
			if i == 4 {
				continue
			}

			require.True(t, strings.ContainsRune(firmware, c), "%q is not in the firmware's alphabet", c)
			require.False(t, strings.ContainsRune("01OIL", c), "%q reads as another character", c)
		}

		seen[code] = true
	}

	require.Len(t, seen, 200, "two of two hundred codes were equal")
}
