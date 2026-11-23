package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

const timeServerPath = "/api/v1/time-server"

// TimeServer is the SNTP server this process can run on the local network. It
// is satisfied by the composition root, which holds the server and the
// machine-owned file together.
type TimeServer interface {
	// TimeServer is the switch and what the server is doing.
	TimeServer(ctx context.Context) TimeServerState
	// SetTimeServer stores the switch and applies it at once.
	SetTimeServer(ctx context.Context, enabled bool) (TimeServerState, error)
}

// TimeServerState is the switch and what the server does about it.
type TimeServerState struct {
	Enabled bool   `json:"enabled"          doc:"True when this server answers the time on UDP port 123."`
	State   string `json:"state"            doc:"What the time server is doing: serving, off, or failed when this machine would not open UDP port 123."`
	Reason  string `json:"reason,omitempty" doc:"Why the time server failed, as the operating system said it. Absent unless the state is failed."`
	Cause   string `json:"cause,omitempty"  doc:"What the failure was, when it is known: permission when this process may not open a port below 1024, or in_use when another program holds the port. Absent otherwise." enum:"permission,in_use"`
	Port    int    `json:"port"             doc:"The UDP port a device asks for the time on."`
}

type timeServerOutput struct {
	Body TimeServerState
}

type setTimeServerInput struct {
	Body struct {
		Enabled bool `json:"enabled" doc:"True starts the time server, and false stops it."`
	}
}

func registerTimeServer(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-time-server",
		Method:      http.MethodGet,
		Path:        timeServerPath,
		Summary:     "Whether this server answers the time on the local network",
		Description: "Reports the time server switch and what the time server is doing. It takes " +
			"view_resource on the link resource kind, which every org viewer holds.",
		Tags:   []string{"discovery"},
		Errors: discoveryErrors,
	}, func(ctx context.Context, _ *struct{}) (*timeServerOutput, error) {
		if err := requireDiscoveryView(ctx, cfg.Grants, "the time server"); err != nil {
			return nil, err
		}

		if cfg.TimeServer == nil {
			return &timeServerOutput{Body: TimeServerState{State: timeServerOff, Port: timeServerPort}}, nil
		}

		return &timeServerOutput{Body: cfg.TimeServer.TimeServer(ctx)}, nil
	})

	register(api, huma.Operation{
		OperationID: "set-time-server",
		Method:      http.MethodPut,
		Path:        timeServerPath,
		Summary:     "Start or stop answering the time on the local network",
		Description: "Stores the time server switch in the machine-owned file and applies it at once, " +
			"with no restart. While on, this server answers SNTP requests on UDP port 123 with this " +
			"machine's clock, for devices on a network with no internet. A machine that will not open " +
			"the port leaves the switch on and reports the state failed with the reason. The stored " +
			"value wins over core.time_server.enabled in the configuration file. It takes " +
			"configure_resource on the link resource kind, which every org administrator holds.",
		Tags:   []string{"discovery"},
		Errors: append(discoveryErrors, http.StatusNotFound),
	}, func(ctx context.Context, in *setTimeServerInput) (*timeServerOutput, error) {
		if err := requireKindAdmin(ctx, cfg.Grants, discoveryKind, "the local network settings"); err != nil {
			return nil, err
		}

		if cfg.TimeServer == nil {
			return nil, httpfault.ResourceNotFound(httpfault.Reference("discovery", "time-server"))
		}

		state, err := cfg.TimeServer.SetTimeServer(ctx, in.Body.Enabled)
		if err != nil {
			return nil, httpfault.Unexpected("store the time server switch").Wrap(err)
		}

		return &timeServerOutput{Body: state}, nil
	})
}

// What a server with no time server reports: off, on the port every NTP
// client asks on.
const (
	timeServerOff  = "off"
	timeServerPort = 123
)
