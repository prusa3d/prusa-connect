// Package printerpairing adds a Prusa printer from its own screen. The server
// asks the printer to pair over the printer's local API, the printer shows a
// code and a question, and a person at the printer answers it. A printer that
// says yes stores this server's address and registers with it over the xBuddy
// wire, and the registration that follows is approved with no second step.
//
// The printer's half is the pairing endpoints of Buddy firmware's local API:
//
//	POST   /api/v1/pairing         {code, client, name, connect} in, 202 while the question is on the screen
//	GET    /api/v1/pairing/{code}  202 waiting, 200 approved, 403 refused, 404 unknown or expired
//	DELETE /api/v1/pairing/{code}  204 withdrawn, 404 nothing to withdraw
//
// See ADR-20261126-qca for which registration a pairing approves, and why.
package printerpairing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prusa3d/prusa-connect/internal/kernel/machinetext"
)

const (
	pairingPath = "/api/v1/pairing"

	// clientName is the client the printer names on its question. The
	// firmware accepts this value and "PrusaLink", and nothing else.
	clientName = "Prusa Connect"

	// legacyKeyHeader and legacyKeyValue are sent because firmware without
	// pairing checks the API key before it routes a path. Without a key that
	// firmware answers with a digest challenge, and with any key it answers a
	// plain 401, which is what tells this client the firmware is too old.
	legacyKeyHeader = "X-Api-Key"
	legacyKeyValue  = "pairing"

	// requestTimeout bounds one call to the printer. A board on the local
	// network answers in well under a second, and one that takes this long is
	// busy enough that the person would not see the question either.
	requestTimeout = 5 * time.Second

	// maxAnswerBytes is what this client reads of an answer. The firmware's
	// longest answer is a short JSON object, and a body that is not one is
	// not read further.
	maxAnswerBytes = 1 << 10

	// defaultPrinterPort is the port the printer's local API listens on.
	defaultPrinterPort = 80

	// MaxNameBytes and MaxConnectHostBytes are the firmware's limits on the
	// two strings a request carries. A longer value is refused by the printer
	// with 422, so it is refused here first, with a sentence.
	MaxNameBytes        = 20
	MaxConnectHostBytes = 35
)

// The answers a printer gives that are not a success. A caller inspects them
// with errors.Is, and LockedOutError with errors.As.
var (
	// ErrBusy is a printer that is printing, or that shows another dialog.
	ErrBusy = errors.New("printerpairing: the printer is busy")
	// ErrPending is a printer already showing another pairing question.
	ErrPending = errors.New("printerpairing: the printer is already showing a pairing question")
	// ErrLockedOut is a printer that takes no pairing request for a while.
	// The error is a *LockedOutError, which says for how long.
	ErrLockedOut = errors.New("printerpairing: the printer takes no pairing request for now")
	// ErrDisabled is a printer that takes no pairing request until it
	// restarts, because a person cancelled two in a row.
	ErrDisabled = errors.New("printerpairing: pairing is off on the printer until it restarts")
	// ErrUnsupported is firmware with no pairing endpoints.
	ErrUnsupported = errors.New("printerpairing: the printer's firmware cannot pair")
	// ErrInvalid is a request the printer refused as malformed: the code, the
	// name, or the address of this server. A *InvalidError carries what the
	// printer said.
	ErrInvalid = errors.New("printerpairing: the printer refused the request")
	// ErrUnreachable is a printer this client could not get an answer from.
	ErrUnreachable = errors.New("printerpairing: the printer did not answer")
	// ErrAddressRefused is an address this client does not dial: one that is
	// not on the local network.
	ErrAddressRefused = errors.New("printerpairing: a printer is paired only on the local network")
	// ErrUnexpected is an answer the pairing endpoints never give.
	ErrUnexpected = errors.New("printerpairing: the printer gave an answer pairing does not have")
)

// LockedOutError is ErrLockedOut with the wait the printer asked for.
type LockedOutError struct {
	// RetryAfter is the printer's Retry-After, at least one second.
	RetryAfter time.Duration
}

func (e *LockedOutError) Error() string {
	return fmt.Sprintf("%s for %s", ErrLockedOut, e.RetryAfter)
}

// Unwrap makes errors.Is(err, ErrLockedOut) hold.
func (e *LockedOutError) Unwrap() error { return ErrLockedOut }

// InvalidError is ErrInvalid with the printer's own words, which name the
// field it refused.
type InvalidError struct {
	// Status is 400 for a malformed code and 422 for any other field.
	Status int
	// Said is the start of the printer's answer, as text.
	Said string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("%s (%d): %s", ErrInvalid, e.Status, e.Said)
}

// Unwrap makes errors.Is(err, ErrInvalid) hold.
func (e *InvalidError) Unwrap() error { return ErrInvalid }

// Connect is the address of this server as the printer stores it.
type Connect struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  bool   `json:"tls"`
}

// Request is what a printer is asked.
type Request struct {
	Code    string
	Name    string
	Connect Connect
}

type wireRequest struct {
	Code    string  `json:"code"`
	Client  string  `json:"client"`
	Name    string  `json:"name,omitempty"`
	Connect Connect `json:"connect"`
}

// Poll is what a printer says about one pairing question.
type Poll int

// The answers to a poll.
const (
	// Waiting is a question still on the screen.
	Waiting Poll = iota
	// Approved is a person who pressed yes. The printer says it once and then
	// forgets the question.
	Approved
	// Refused is a person who pressed cancel.
	Refused
	// Unknown is a question the printer does not hold: it timed out, it was
	// withdrawn, or the approval was already collected.
	Unknown
)

// Printer is where a printer's local API answers.
type Printer struct {
	// Host is a name or an address, as a person typed it.
	Host string
	Port int
}

// ParsePrinter reads an address a person typed: a name or an IP address, with
// an optional port. A scheme or a path is refused, because the pairing
// endpoints are at fixed paths of plain http.
func ParsePrinter(text string) (Printer, error) {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "/?#@ \t") {
		return Printer{}, fmt.Errorf("%w: %q is not a printer address", ErrAddressRefused, text)
	}

	host, port := text, defaultPrinterPort

	if split, portText, err := net.SplitHostPort(text); err == nil {
		number, convErr := strconv.Atoi(portText)
		if convErr != nil || number < 1 || number > 65535 {
			return Printer{}, fmt.Errorf("%w: %q has no usable port", ErrAddressRefused, text)
		}

		host, port = split, number
	}

	host = strings.Trim(host, "[]")
	if host == "" {
		return Printer{}, fmt.Errorf("%w: %q names no host", ErrAddressRefused, text)
	}

	return Printer{Host: host, Port: port}, nil
}

// String is the address as host:port.
func (p Printer) String() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

// Client asks printers to pair. The zero value is not usable: build one with
// NewClient.
type Client struct {
	http *http.Client
}

// NewClient returns a client that dials only the addresses permit accepts. A
// nil permit takes LocalNetwork.
//
// The address is checked after the name is resolved, at the moment of the
// dial, so a name that resolves to this machine or to the internet is refused
// however it is spelled. No redirect is followed and no proxy is read from the
// environment, because either would carry the request to an address the
// check did not see.
func NewClient(permit func(netip.Addr) bool) *Client {
	if permit == nil {
		permit = LocalNetwork
	}

	dialer := &net.Dialer{Timeout: requestTimeout, Control: dialGuard(permit)}

	return &Client{http: &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		},
	}}
}

// LocalNetwork reports an address a printer on the local network can have: a
// private address, a link-local one, or one from the shared range a carrier or
// an overlay network such as Tailscale hands out. Loopback is refused, because
// a printer is never this machine, and the address would reach the services
// this server runs beside.
func LocalNetwork(ip netip.Addr) bool {
	ip = ip.Unmap()

	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || sharedRange.Contains(ip)
}

// sharedRange is RFC 6598's shared address space.
//
//nolint:gochecknoglobals // a constant prefix; netip has no constant form.
var sharedRange = netip.MustParsePrefix("100.64.0.0/10")

func dialGuard(permit func(netip.Addr) bool) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrAddressRefused, err)
		}

		ip, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrAddressRefused, err)
		}

		if !permit(ip.Unmap()) {
			return fmt.Errorf("%w: %s", ErrAddressRefused, ip)
		}

		return nil
	}
}

// Start puts the pairing question on the printer's screen. It returns the
// address the request reached, which is the address the printer's own
// registration is expected from.
func (c *Client) Start(ctx context.Context, printer Printer, request Request) (netip.Addr, error) {
	body, err := json.Marshal(wireRequest{
		Code:    request.Code,
		Client:  clientName,
		Name:    request.Name,
		Connect: request.Connect,
	})
	if err != nil {
		return netip.Addr{}, fmt.Errorf("encode the pairing request: %w", err)
	}

	answer, reached, err := c.call(ctx, http.MethodPost, printer, pairingPath, body)
	if err != nil {
		return netip.Addr{}, err
	}

	switch answer.status {
	case http.StatusAccepted:
		return reached, nil
	case http.StatusConflict:
		if answer.reason() == "busy" {
			return netip.Addr{}, ErrBusy
		}

		return netip.Addr{}, ErrPending
	case http.StatusTooManyRequests:
		if answer.reason() == "disabled" {
			return netip.Addr{}, ErrDisabled
		}

		return netip.Addr{}, &LockedOutError{RetryAfter: answer.retryAfter}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return netip.Addr{}, &InvalidError{Status: answer.status, Said: answer.text()}
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed:
		// Firmware with no pairing checks the key first, and answers 401. A
		// build that routes /api/v1 without the pairing paths answers 404.
		return netip.Addr{}, fmt.Errorf("%w: it answered %d", ErrUnsupported, answer.status)
	default:
		return netip.Addr{}, fmt.Errorf("%w: %d to the request", ErrUnexpected, answer.status)
	}
}

// Poll asks what became of one pairing question.
//
// The body of an approval is drained and never read. Firmware that runs
// PrusaLink puts its API key there, and this server has no use for it: the
// registration that follows issues a credential of its own.
func (c *Client) Poll(ctx context.Context, printer Printer, code string) (Poll, error) {
	answer, _, err := c.call(ctx, http.MethodGet, printer, pairingPath+"/"+url.PathEscape(code), nil)
	if err != nil {
		return Unknown, err
	}

	switch answer.status {
	case http.StatusAccepted:
		return Waiting, nil
	case http.StatusOK:
		return Approved, nil
	case http.StatusForbidden:
		return Refused, nil
	case http.StatusNotFound:
		return Unknown, nil
	default:
		return Unknown, fmt.Errorf("%w: %d to a poll", ErrUnexpected, answer.status)
	}
}

// Withdraw takes the question off the printer's screen. A printer that no
// longer holds it is not an error, because the question is gone either way.
func (c *Client) Withdraw(ctx context.Context, printer Printer, code string) error {
	answer, _, err := c.call(ctx, http.MethodDelete, printer, pairingPath+"/"+url.PathEscape(code), nil)
	if err != nil {
		return err
	}

	switch answer.status {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("%w: %d to a withdrawal", ErrUnexpected, answer.status)
	}
}

// answer is what this client keeps of a response.
type answer struct {
	status     int
	body       []byte
	retryAfter time.Duration
}

func (a answer) reason() string {
	var said struct {
		Reason string `json:"reason"`
	}

	if json.Unmarshal(a.body, &said) != nil {
		return ""
	}

	return said.Reason
}

// text is the printer's answer as one short line, for a log and for the
// person who typed a value it refused.
func (a answer) text() string {
	const maxSaid = 120

	return machinetext.Clip(strings.ToValidUTF8(strings.Join(strings.Fields(string(a.body)), " "), ""), maxSaid)
}

func (c *Client) call(
	ctx context.Context, method string, printer Printer, path string, body []byte,
) (answer, netip.Addr, error) {
	target := url.URL{Scheme: "http", Host: printer.String(), Path: path}

	var reached netip.Addr

	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		if addr, err := netip.ParseAddrPort(info.Conn.RemoteAddr().String()); err == nil {
			reached = addr.Addr().Unmap()
		}
	}}

	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace),
		method, target.String(), bytes.NewReader(body))
	if err != nil {
		return answer{}, netip.Addr{}, fmt.Errorf("%w: %w", ErrAddressRefused, err)
	}

	request.Header.Set(legacyKeyHeader, legacyKeyValue)

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, ErrAddressRefused) {
			return answer{}, netip.Addr{}, fmt.Errorf("%w: %s", ErrAddressRefused, printer.Host)
		}

		return answer{}, netip.Addr{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}

	defer func() {
		// The body is read to its bound before this runs, and a failure to
		// close a response the client has finished with changes nothing.
		_ = response.Body.Close()
	}()

	read, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return answer{}, netip.Addr{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}

	return answer{
		status:     response.StatusCode,
		body:       read,
		retryAfter: retryAfterOf(response.Header.Get("Retry-After")),
	}, reached, nil
}

// retryAfterOf is a Retry-After in seconds, and one second when the printer
// sent none it could be read from: the firmware sends at least one.
func retryAfterOf(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 1 {
		return time.Second
	}

	return time.Duration(seconds) * time.Second
}
