package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The discovery paths: the printers this server hears on the local network,
// and whether it announces itself there.
const (
	nearbyPath     = "/api/v1/discovery/nearby"
	visibilityPath = "/api/v1/discovery/visibility"
)

// discoveryKind is the resource kind the discovery routes are decided on. The
// printers nearby are the ones a pairing starts from, and a pairing is written
// on the link, so the same grants answer both. Every org viewer holds
// view_resource on it, and every org administrator holds configure_resource.
const discoveryKind = ref.KindLink

// Discovery is what this server hears and announces on the local network. It
// is declared here because this is where it is consumed. The composition root
// satisfies it, because it holds the announcer, the listener and the
// machine-owned file together.
type Discovery interface {
	// Nearby is every printer this server hears, sorted by name.
	Nearby(ctx context.Context) []NearbyPrinter
	// Visibility is whether this server is hidden, and what its announcer is
	// doing.
	Visibility(ctx context.Context) (Visibility, error)
	// SetHidden stores the hidden setting and applies it at once. A hidden
	// server withdraws every record it announces with a goodbye.
	SetHidden(ctx context.Context, hidden bool) (Visibility, error)
}

// NearbyPrinter is one printer or server this one hears on the local network.
// It is a name and an address. Being on the list grants it nothing.
type NearbyPrinter struct {
	Name     string   `json:"name"                doc:"The name the other printer announces, which is its title."`
	Address  string   `json:"address"             doc:"The address the other printer's API answers on."`
	Port     int      `json:"port"                doc:"The port of the other printer's API."`
	TLS      bool     `json:"tls"                 doc:"True when that port expects https."`
	Services []string `json:"services"            doc:"The mDNS service names it answers under, sorted."`
	URL      string   `json:"url"                 doc:"The address of the other printer's own web page."`
	DeviceID string   `json:"device_id,omitempty" doc:"The id of the printer on this server that connects from this address, or last connected from it and is away now. It is absent for a printer that has not connected here since this server started, and for one the caller may not observe."`
}

// Visibility is the hidden setting and what the announcer does about it.
type Visibility struct {
	Hidden       bool     `json:"hidden"              doc:"True when this server announces nothing on the local network. It still serves its page and its API at its address."`
	Announcement string   `json:"announcement"        doc:"What the announcer is doing: announcing, hidden, off when core.discovery.enabled is false, or failed when this machine would not open the mDNS socket, refuses every packet the server sends, or other machines hold every name it may take."`
	Name         string   `json:"name,omitempty"      doc:"The name a slicer shows for this server, which is core.frontend.title, with the suffix a rename added when another machine on the network holds it. The name, host, port, services and addresses are absent when core.discovery.enabled is false, because nothing is announced."`
	Host         string   `json:"host,omitempty"      doc:"The host name this server's records name under the .local domain. With core.discovery.host_records system it is the name the machine's own mDNS responder holds."`
	Port         int      `json:"port,omitempty"      doc:"The API port the announcement points at. It is absent until the server has bound it."`
	Services     []string `json:"services,omitempty"  doc:"The mDNS service names this server announces under, one record for each."`
	Addresses    []string `json:"addresses,omitempty" doc:"This machine's addresses, read from its interfaces at the time of the request, which the announcement carries. With core.discovery.host_records system the machine's own mDNS responder announces them. A loopback address is left out."`
	Bridged      bool     `json:"bridged"             doc:"True inside a container whose one network is a bridge, where the announcement reaches no machine on the real network."`
}

type nearbyOutput struct {
	Body struct {
		Items []NearbyPrinter `json:"items" doc:"Every printer this server hears, sorted by name. The list never holds more than 256."`
	}
}

type visibilityOutput struct {
	Body Visibility
}

type visibilityBody struct {
	Hidden bool `json:"hidden" doc:"True hides this server from the local network, and false shows it again."`
}

type setVisibilityInput struct {
	Body visibilityBody
}

// discoveryErrors is what a discovery route may be answered with besides its
// body.
var discoveryErrors = []int{http.StatusUnauthorized, http.StatusForbidden}

// registerDiscovery mounts the discovery operations. They are published
// whether or not a listener is wired, because the document is a contract.
func registerDiscovery(api huma.API, cfg Config) {
	registerNearby(api, cfg)
	registerGetVisibility(api, cfg)
	registerSetVisibility(api, cfg)
}

func registerNearby(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "list-nearby-printers",
		Method:      http.MethodGet,
		Path:        nearbyPath,
		Summary:     "The printers this server hears on the local network",
		Description: "Every printer or server that announces itself over mDNS on the same network, " +
			"with the address of its own web page. A printer on this list is a name and an address, " +
			"and being on it grants nothing. The list holds at most 256 entries, and an entry leaves " +
			"it when its record expires or its printer sends a goodbye. An entry whose address is " +
			"the one a printer connected to this server comes from names that printer in device_id, " +
			"when the caller holds observe on it. A printer that is away is named by the address it " +
			"last came from, until a connected printer comes from that address. It takes view_resource on the link resource " +
			"kind, which every org viewer holds.",
		Tags:   []string{"discovery"},
		Errors: discoveryErrors,
	}, func(ctx context.Context, _ *struct{}) (*nearbyOutput, error) {
		if err := requireDiscoveryView(ctx, cfg.Grants, "the printers nearby"); err != nil {
			return nil, err
		}

		out := &nearbyOutput{}
		out.Body.Items = []NearbyPrinter{}

		if cfg.Discovery != nil {
			out.Body.Items = append(out.Body.Items, cfg.Discovery.Nearby(ctx)...)
		}

		if err := withholdUnobserved(ctx, cfg.Grants, out.Body.Items); err != nil {
			return nil, err
		}

		return out, nil
	})
}

func registerGetVisibility(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-discovery-visibility",
		Method:      http.MethodGet,
		Path:        visibilityPath,
		Summary:     "Whether this server is hidden from the local network",
		Description: "Reports the hidden setting and what the announcer is doing. It takes " +
			"view_resource on the link resource kind, which every org viewer holds.",
		Tags:   []string{"discovery"},
		Errors: discoveryErrors,
	}, func(ctx context.Context, _ *struct{}) (*visibilityOutput, error) {
		if err := requireDiscoveryView(ctx, cfg.Grants, "the printers nearby"); err != nil {
			return nil, err
		}

		if cfg.Discovery == nil {
			return &visibilityOutput{Body: Visibility{Announcement: announcementOff}}, nil
		}

		visibility, err := cfg.Discovery.Visibility(ctx)
		if err != nil {
			return nil, httpfault.Unexpected("read the hidden setting").Wrap(err)
		}

		return &visibilityOutput{Body: visibility}, nil
	})
}

func registerSetVisibility(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "set-discovery-visibility",
		Method:      http.MethodPut,
		Path:        visibilityPath,
		Summary:     "Hide this server from the local network, or show it again",
		Description: "Stores the hidden setting in the machine-owned file and applies it at once, " +
			"with no restart. A hidden server announces nothing and sends a goodbye for every " +
			"record it announced, so the other printers drop it from their lists. It still serves " +
			"its page and its API at its address. The stored value wins over " +
			"core.discovery.hidden in the configuration file. It takes configure_resource on the " +
			"link resource kind, which every org administrator holds.",
		Tags:   []string{"discovery"},
		Errors: append(discoveryErrors, http.StatusNotFound),
	}, func(ctx context.Context, in *setVisibilityInput) (*visibilityOutput, error) {
		if err := requireKindAdmin(ctx, cfg.Grants, discoveryKind, "the local network settings"); err != nil {
			return nil, err
		}

		if cfg.Discovery == nil {
			return nil, httpfault.ResourceNotFound(httpfault.Reference("discovery", "visibility"))
		}

		visibility, err := cfg.Discovery.SetHidden(ctx, in.Body.Hidden)
		if err != nil {
			return nil, httpfault.Unexpected("store the hidden setting").Wrap(err)
		}

		return &visibilityOutput{Body: visibility}, nil
	})
}

// announcementOff is what a server with no announcer reports.
const announcementOff = "off"

// requireDiscoveryView refuses a caller who may not see what the network
// settings show. They are decided on the link resource kind, and what names
// the setting in the refusal.
func requireDiscoveryView(ctx context.Context, grants GrantStore, what string) error {
	who, object, err := kindDecision(ctx, discoveryKind)
	if err != nil {
		return err
	}

	allowed, err := grants.Can(ctx, who, authz.ActionViewResource, object)
	if err != nil {
		return httpfault.Unexpected("ask the authorization graph who may see " + what)
	}

	if !allowed {
		return fault.New(authz.CodeAccessDenied(), "this caller may not see "+what).With(
			fault.StringParam("object", object.String()),
			fault.StringParam("action", authz.ActionViewResource.String()))
	}

	return nil
}

// withholdUnobserved clears the device id of every entry whose printer the
// caller may not observe. The entry itself stays, because the caller may see
// every printer on the network. Only the link to a printer of this server is
// withheld. A printer the caller reaches nothing on is never named, as on
// every other route.
func withholdUnobserved(ctx context.Context, grants GrantStore, items []NearbyPrinter) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}

	for i := range items {
		if items[i].DeviceID == "" {
			continue
		}

		object, named := authz.NewObject(authz.TypeDevice, items[i].DeviceID)
		if named != nil {
			items[i].DeviceID = ""

			continue
		}

		allowed, asked := grants.Can(ctx, who, authz.ActionObserve, object)
		if asked != nil {
			return httpfault.Unexpected("ask the authorization graph who may observe a printer nearby")
		}

		if !allowed {
			items[i].DeviceID = ""
		}
	}

	return nil
}

// seesDiscovery reports whether a caller holds view_resource on the link kind,
// the grant the discovery routes take to read. A caller with no identity, and
// a question the graph cannot answer, read as no.
func seesDiscovery(ctx context.Context, grants GrantStore, principal string) bool {
	if principal == "" || grants == nil {
		return false
	}

	who, err := authz.ParseSubject(principal)
	if err != nil {
		return false
	}

	object, err := authz.NewObject(authz.TypeResourceKind, discoveryKind.String())
	if err != nil {
		return false
	}

	allowed, err := grants.Can(ctx, who, authz.ActionViewResource, object)

	return err == nil && allowed
}
