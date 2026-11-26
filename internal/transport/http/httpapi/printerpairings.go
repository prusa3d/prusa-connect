package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The printer pairing paths. A pairing is addressed by an id this server
// mints, and never by the code the printer shows: anybody standing at the
// printer can read the code.
const (
	printerPairingObject = "printer pairing"

	printerPairingsPath = "/api/v1/printer-pairings"
	printerPairingPath  = "/api/v1/printer-pairings/{pairing_id}"
)

// The errors a PrinterPairings implementation returns, which this surface
// turns into its faults.
var (
	ErrPrinterPairingUnknown     = errors.New("httpapi: no printer pairing has that id")
	ErrPrinterPairingBusy        = errors.New("httpapi: the printer is busy")
	ErrPrinterPairingPending     = errors.New("httpapi: the printer is already showing a pairing question")
	ErrPrinterPairingOpen        = errors.New("httpapi: a pairing with this printer is already open")
	ErrPrinterPairingDisabled    = errors.New("httpapi: pairing is off on the printer until it restarts")
	ErrPrinterPairingUnsupported = errors.New("httpapi: the printer's firmware cannot pair")
	ErrPrinterPairingRefused     = errors.New("httpapi: the printer refused the pairing request")
	ErrPrinterPairingUnreachable = errors.New("httpapi: the printer did not answer")
	ErrPrinterPairingAddress     = errors.New("httpapi: the printer address is not on the local network")
	ErrPrinterPairingConnect     = errors.New("httpapi: the printer cannot use this server address")
	ErrPrinterPairingLimited     = errors.New("httpapi: too many printer pairings are open")
)

// PrinterPairingLockedError is a printer that takes no pairing request for a
// while, and for how long.
type PrinterPairingLockedError struct {
	RetryAfter time.Duration
}

func (e *PrinterPairingLockedError) Error() string {
	return "httpapi: the printer takes no pairing request for " + e.RetryAfter.String()
}

// PrinterPairingDetailError is a refusal that carries the printer's own words,
// which name the field it refused. It wraps one of the sentinels above.
type PrinterPairingDetailError struct {
	Err    error
	Detail string
}

func (e *PrinterPairingDetailError) Error() string { return e.Err.Error() + ": " + e.Detail }

// Unwrap makes errors.Is reach the sentinel.
func (e *PrinterPairingDetailError) Unwrap() error { return e.Err }

// PrinterPairings sends a pairing to a printer on the local network and
// follows it until the printer has registered. The composition root
// implements it, because the printer's protocol is a connector's and the
// transport may not import one.
type PrinterPairings interface {
	// Start sends the pairing and answers once the printer shows the
	// question.
	Start(ctx context.Context, request PrinterPairingRequest) (PrinterPairing, error)
	// Get reads one pairing.
	Get(ctx context.Context, id string) (PrinterPairing, error)
	// Cancel withdraws one pairing, and takes its question off the printer's
	// screen while it is still there.
	Cancel(ctx context.Context, id string) (PrinterPairing, error)
}

// PrinterPairingConnect is this server's address as the printer stores it.
type PrinterPairingConnect struct {
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Host string `json:"host" doc:"The host name or IPv4 address the printer dials this server at. A printer stores at most 20 characters of it, or 35 for a name ending in prusa3d.com, so a longer name is refused by the printer." minLength:"1" maxLength:"35" example:"192.168.1.50"`
	Port int    `json:"port" doc:"The port the printer dials."                                                                                                                                                                                                    example:"8080"         minimum:"1" maximum:"65535"`
	TLS  bool   `json:"tls"  doc:"Whether the printer dials with TLS."`
}

// PrinterPairingRequest is what a person asks for.
type PrinterPairingRequest struct {
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Printer string                `json:"printer"        doc:"The printer's address on the local network: a host name or an IP address, with an optional port. The port is 80 when none is given." minLength:"1" maxLength:"255" example:"192.168.1.20"`
	Connect PrinterPairingConnect `json:"connect"        doc:"This server's address, which the printer stores when the person at it accepts."`
	Name    string                `json:"name,omitempty" doc:"A name the printer shows beside the question, at most 20 bytes."                                                                                   maxLength:"20"`
}

// PrinterPairing is one pairing, as the person who started it reads it.
type PrinterPairing struct {
	ID string `json:"id" doc:"The pairing's id on this server, which every other operation on it takes."`
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Code    string                `json:"code"    doc:"The code the printer shows. Show it to the person, who checks it matches the printer's screen before accepting." example:"K7QX-29FM"`
	Printer string                `json:"printer" doc:"The address the pairing was sent to, as host:port."`
	Connect PrinterPairingConnect `json:"connect" doc:"The server address the printer was given."`
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	State     string    `json:"state"      doc:"waiting while the question is on the printer's screen. approved once the person accepted, while the printer registers. admitted once the registration was approved. refused, expired and withdrawn end the pairing." enum:"waiting,approved,admitted,refused,expired,withdrawn"`
	StartedAt time.Time `json:"started_at" doc:"When the pairing was sent."`
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Until *time.Time `json:"until,omitempty" doc:"When the pairing stops waiting for the current step: for the person while waiting, for the printer's registration once approved. Absent once the pairing ended."`
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Registration *PairedRegistration `json:"registration,omitempty" doc:"The registration the pairing approved. Present once the state is admitted."`
}

// PairedRegistration is the registration a pairing approved.
type PairedRegistration struct {
	Code      string `json:"code"                doc:"The registration's code, as the registrations surface names it."`
	Connector string `json:"connector"           doc:"The connector instance that holds the registration."`
	Serial    string `json:"serial,omitempty"    doc:"The serial number the printer reported."`
	Model     string `json:"model,omitempty"     doc:"The model the printer reported, in its own spelling."`
	DeviceID  string `json:"device_id,omitempty" doc:"The device the approval bound the printer to. Open its page with this id."`
}

type startPrinterPairingInput struct {
	Body PrinterPairingRequest
}

type printerPairingOutput struct {
	Body PrinterPairing
}

// PrinterPairingRef is the pairing a path names. It is exported because Huma
// binds parameters by reflection and cannot write into an unexported embedded
// field.
type PrinterPairingRef struct {
	ID string `path:"pairing_id" doc:"The pairing's id, as starting it answered." maxLength:"64" minLength:"1" example:"4f2a9c1e7b3d4a6f8e0c2b5d7a9f1e3c"`
}

type printerPairingInput struct {
	PrinterPairingRef
}

var (
	printerPairingBusy = fault.Register(fault.DomainPrinter(), "PAIRING_BUSY", http.StatusConflict,
		fault.Document(
			"The printer is printing or shows another dialog, so it cannot ask about pairing",
			"Wait until the printer is idle and its screen shows no dialog, then try again.",
			fault.RetryAllowed,
			fault.ShowDetail,
		))

	printerPairingPending = fault.Register(fault.DomainPrinter(), "PAIRING_PENDING", http.StatusConflict,
		fault.Document(
			"The printer already shows a pairing question",
			"Answer or cancel the question on the printer, or wait a minute for it to close, then try again.",
			fault.RetryAllowed,
			fault.ShowDetail,
		))

	printerPairingLocked = fault.Register(fault.DomainPrinter(), "PAIRING_LOCKED", http.StatusTooManyRequests,
		fault.Document(
			"The printer takes no pairing request for a while",
			"Wait for the time Retry-After gives in seconds. A printer locks pairing for five minutes "+
				"after a cancel, a question nobody answered, or three requests in five minutes.",
			fault.RetryAllowed,
			fault.ShowDetail,
			RetryAfterParam,
		))

	printerPairingDisabled = fault.Register(fault.DomainPrinter(), "PAIRING_DISABLED", http.StatusTooManyRequests,
		fault.Document(
			"Pairing is off on the printer until it restarts",
			"A person cancelled two pairing questions in a row. Restart the printer to pair again, or "+
				"add it with a settings file instead.",
			fault.RetryNever,
			fault.ShowDetail,
		))

	printerPairingUnsupported = fault.Register(fault.DomainPrinter(), "PAIRING_UNSUPPORTED",
		http.StatusUnprocessableEntity, fault.Document(
			"The printer's firmware cannot pair",
			"Update the printer's firmware, or add the printer with a settings file on a USB drive.",
			fault.RetryNever,
			fault.ShowDetail,
		))

	printerPairingRefused = fault.Register(fault.DomainPrinter(), "PAIRING_REFUSED",
		http.StatusUnprocessableEntity, fault.Document(
			"The printer refused the pairing request",
			"The detail says which value the printer refused. A server address longer than the "+
				"printer stores is the usual cause: use the IP address instead of a long name.",
			fault.RetryNever,
			fault.ShowDetail,
		))

	printerPairingUnreachable = fault.Register(fault.DomainPrinter(), "PAIRING_UNREACHABLE", http.StatusBadGateway,
		fault.Document(
			"This server got no answer from the printer",
			"Check the printer's address on its network screen, and that the printer and this server "+
				"are on the same network.",
			fault.RetryAllowed,
			fault.ShowDetail,
		))

	printerPairingAddress = fault.Register(fault.DomainPrinter(), "PAIRING_ADDRESS_REFUSED",
		http.StatusUnprocessableEntity, fault.Document(
			"This server pairs only with a printer on the local network",
			"Type the address the printer shows on its network screen: a private address such as "+
				"192.168.1.20, or a name that resolves to one.",
			fault.RetryNever,
			fault.ShowDetail,
		))

	printerPairingConnect = fault.Register(fault.DomainPrinter(), "PAIRING_SERVER_ADDRESS",
		http.StatusUnprocessableEntity, fault.Document(
			"The printer cannot use the server address in the request",
			"Give an address the printer reaches this server at: not localhost, at most 35 characters, "+
				"and a port between 1 and 65535.",
			fault.RetryNever,
			fault.ShowDetail,
		))

	printerPairingLimited = fault.Register(fault.DomainPrinter(), "PAIRING_LIMITED", http.StatusConflict,
		fault.Document(
			"This server already has a pairing open with that printer, or too many open at once",
			"Cancel the open pairing or wait a minute for it to end, then try again.",
			fault.RetryAllowed,
			fault.ShowDetail,
		))
)

// printerPairingErrors are the refusals every operation on this surface
// answers with. Pairing a printer admits it, so it takes the permission that
// approving a registration takes.
var printerPairingErrors = []int{http.StatusUnauthorized, http.StatusForbidden}

func registerPrinterPairings(api huma.API, cfg Config) {
	registerStartPrinterPairing(api, cfg)
	registerGetPrinterPairing(api, cfg)
	registerCancelPrinterPairing(api, cfg)
}

func registerStartPrinterPairing(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "start-printer-pairing",
		Method:      http.MethodPost,
		Path:        printerPairingsPath,
		Summary:     "Ask a printer on the local network to pair",
		Description: "This server sends a pairing request to the printer's local API, and the printer " +
			"shows a question with the code from the answer. When the person at the printer accepts, " +
			"the printer stores the server address from the request and registers with this server. " +
			"That registration is approved with no second step, and the pairing's state becomes " +
			"admitted.\n" +
			"\n" +
			"This server sends the request, and not the browser, because the printer is on this " +
			"server's network. Only private, link-local and shared addresses are dialled. Firmware " +
			"without pairing answers PAIRING_UNSUPPORTED: add that printer with a settings file. It " +
			"takes admit_device on the driver resource kind.",
		Tags:          []string{"registrations"},
		DefaultStatus: http.StatusCreated,
		Errors: append([]int{
			http.StatusConflict, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusBadGateway,
		}, printerPairingErrors...),
	}, func(ctx context.Context, in *startPrinterPairingInput) (*printerPairingOutput, error) {
		if err := requireRegistrationAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		if cfg.PrinterPairings == nil {
			return nil, printerPairingUnsupported.New("this server pairs no printer")
		}

		started, err := cfg.PrinterPairings.Start(ctx, in.Body)
		if err != nil {
			return nil, printerPairingFault(err, "start a printer pairing", "")
		}

		return &printerPairingOutput{Body: started}, nil
	})
}

func registerGetPrinterPairing(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-printer-pairing",
		Method:      http.MethodGet,
		Path:        printerPairingPath,
		Summary:     "Read where one printer pairing stands",
		Description: "The state of the pairing and, once the printer's registration was approved, the " +
			"device it became. This server asks the printer about the question once a second, so " +
			"reading this more often learns nothing new. A pairing that ended stays readable for ten " +
			"minutes. It takes admit_device on the driver resource kind.",
		Tags:   []string{"registrations"},
		Errors: append([]int{http.StatusNotFound}, printerPairingErrors...),
	}, func(ctx context.Context, in *printerPairingInput) (*printerPairingOutput, error) {
		if err := requireRegistrationAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		if cfg.PrinterPairings == nil {
			return nil, unknownPrinterPairing(in.ID)
		}

		read, err := cfg.PrinterPairings.Get(ctx, in.ID)
		if err != nil {
			return nil, printerPairingFault(err, "read a printer pairing", in.ID)
		}

		return &printerPairingOutput{Body: read}, nil
	})
}

func registerCancelPrinterPairing(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "cancel-printer-pairing",
		Method:      http.MethodDelete,
		Path:        printerPairingPath,
		Summary:     "Withdraw one printer pairing",
		Description: "Takes the question off the printer's screen while it is still there. A printer " +
			"that already accepted keeps the server address it stored, and its registration waits " +
			"for a person on the registrations surface. A pairing that already ended is answered " +
			"as it ended. It takes admit_device on the driver resource kind.",
		Tags:   []string{"registrations"},
		Errors: append([]int{http.StatusNotFound}, printerPairingErrors...),
	}, func(ctx context.Context, in *printerPairingInput) (*printerPairingOutput, error) {
		if err := requireRegistrationAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		if cfg.PrinterPairings == nil {
			return nil, unknownPrinterPairing(in.ID)
		}

		cancelled, err := cfg.PrinterPairings.Cancel(ctx, in.ID)
		if err != nil {
			return nil, printerPairingFault(err, "withdraw a printer pairing", in.ID)
		}

		return &printerPairingOutput{Body: cancelled}, nil
	})
}

func unknownPrinterPairing(id string) error {
	return httpfault.ResourceNotFound(httpfault.Reference(printerPairingObject, id))
}

// printerPairingFault renders what the implementation reported.
func printerPairingFault(err error, doing, id string) error {
	detail := ""
	if said, ok := errors.AsType[*PrinterPairingDetailError](err); ok {
		detail = said.Detail
	}

	if locked, ok := errors.AsType[*PrinterPairingLockedError](err); ok {
		seconds := max(int64(math.Ceil(locked.RetryAfter.Seconds())), 1)
		refused := printerPairingLocked.New("the printer takes no pairing request for now").
			With(fault.IntParam(RetryAfterParam, seconds))

		return huma.ErrorWithHeaders(refused, http.Header{"Retry-After": []string{strconv.FormatInt(seconds, 10)}})
	}

	for _, known := range []struct {
		sentinel error
		def      *fault.Definition
		message  string
	}{
		{ErrPrinterPairingBusy, printerPairingBusy, "the printer is busy"},
		{ErrPrinterPairingPending, printerPairingPending, "the printer already shows a pairing question"},
		{ErrPrinterPairingOpen, printerPairingLimited, "a pairing with this printer is already open"},
		{ErrPrinterPairingLimited, printerPairingLimited, "too many printer pairings are open"},
		{ErrPrinterPairingDisabled, printerPairingDisabled, "pairing is off on the printer until it restarts"},
		{ErrPrinterPairingUnsupported, printerPairingUnsupported, "the printer's firmware cannot pair"},
		{ErrPrinterPairingRefused, printerPairingRefused, "the printer refused the request"},
		{ErrPrinterPairingUnreachable, printerPairingUnreachable, "the printer did not answer"},
		{ErrPrinterPairingAddress, printerPairingAddress, "the printer address is not on the local network"},
		{ErrPrinterPairingConnect, printerPairingConnect, "the printer cannot use this server address"},
	} {
		if !errors.Is(err, known.sentinel) {
			continue
		}

		message := known.message
		if detail != "" {
			message += ": " + detail
		}

		return known.def.New(message)
	}

	if errors.Is(err, ErrPrinterPairingUnknown) {
		return unknownPrinterPairing(id)
	}

	return serverFault(err, doing)
}
