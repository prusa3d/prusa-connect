package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

const allowedOriginsPath = "/api/v1/allowed-origins"

// MaxSettingsOrigins is how many origins the settings list may hold.
const MaxSettingsOrigins = 32

// The places an allowed origin comes from.
const (
	// OriginFromFile is core.api.allowed_origins in the operator's file.
	OriginFromFile = "file"
	// OriginFromSettings is the list a person keeps through this API.
	OriginFromSettings = "settings"
	// OriginFromExtension is an extension that is switched on, whose install
	// an operator approved.
	OriginFromExtension = "extension"
)

// BrowserOrigins is the browser origins this server answers, from every place
// one comes from. The composition root satisfies it, because it holds the
// operator's file, the machine-owned file and the installed modules together.
type BrowserOrigins interface {
	// Any reports whether any origin is allowed. Allows answers one request,
	// so both read a snapshot and take no lock.
	Any() bool
	// Allows reports whether a page served from origin may read an answer.
	Allows(origin string) bool
	// AllowedOrigins is every allowed origin with where it comes from, and
	// the version of the settings list.
	AllowedOrigins(ctx context.Context) (AllowedOriginList, error)
	// SetAllowedOrigins stores the settings list and applies it at once, when
	// the precondition holds for the version of the list it replaces.
	SetAllowedOrigins(ctx context.Context, origins []string, pre Precondition) (AllowedOriginList, error)
}

// AllowedOriginList is every allowed origin, and the version of the settings
// list a write quotes in If-Match.
type AllowedOriginList struct {
	Items   []AllowedOrigin
	Version string
}

// AllowedOrigin is one browser origin this server answers, and where it comes
// from. An origin two places name is listed once for each.
type AllowedOrigin struct {
	Origin    string `json:"origin"              doc:"The origin, as a browser writes it in its Origin header: a scheme and a host, with a port only when it is not the scheme's default."`
	Source    string `json:"source"              doc:"Where the origin comes from: file for core.api.allowed_origins in the configuration file, settings for the list this API stores, and extension for an extension that is switched on. Only a settings origin is changed through this API." enum:"file,settings,extension"`
	Extension string `json:"extension,omitempty" doc:"The namespace of the extension the origin comes from. Absent unless the source is extension. The origin stays allowed while that extension is switched on, and leaves when it is switched off or removed."`
	Title     string `json:"title,omitempty"     doc:"The name a person reads for that extension. Absent unless the source is extension."`
}

type allowedOriginsOutput struct {
	ETag string `header:"ETag" doc:"The version of the settings list, to quote in If-Match on the next write."`
	Body struct {
		Items   []AllowedOrigin `json:"items"   doc:"Every browser origin this server answers, sorted by source and then by origin."`
		Version string          `json:"version" doc:"The version of the settings list, to quote in If-Match on the next write. Empty from a server that stores no list."`
	}
}

type setAllowedOriginsInput struct {
	IfMatch string `header:"If-Match" doc:"The version of the settings list last read, quotes included. A write whose version is not the stored list's is refused, so a change another person made in the meantime is not written over."`
	Body    struct {
		Origins []string `json:"origins" doc:"The whole settings list, which replaces the stored one. Each entry is an exact origin such as https://slicer.example: a lowercase scheme and host, a port only when it is not the scheme's default, and nothing after the host. A wildcard is refused. An empty list stores none." maxItems:"32" nullable:"false"`
	}
}

func registerAllowedOrigins(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "list-allowed-origins",
		Method:      http.MethodGet,
		Path:        allowedOriginsPath,
		Summary:     "The browser origins that may call this API",
		Description: "Every origin a page may call this API from, other than this server's own, with " +
			"where each comes from: core.api.allowed_origins in the configuration file, the list stored " +
			"through set-allowed-origins, and each extension that is switched on and was granted an " +
			"origin. It takes view_resource on the link resource kind, which every org viewer holds.",
		Tags:   []string{"system"},
		Errors: discoveryErrors,
	}, func(ctx context.Context, _ *struct{}) (*allowedOriginsOutput, error) {
		if err := requireDiscoveryView(ctx, cfg.Grants); err != nil {
			return nil, err
		}

		if cfg.Origins == nil {
			return allowedOriginsOf(AllowedOriginList{})
		}

		list, err := cfg.Origins.AllowedOrigins(ctx)
		if err != nil {
			return nil, err
		}

		return allowedOriginsOf(list)
	})

	register(api, huma.Operation{
		OperationID: "set-allowed-origins",
		Method:      http.MethodPut,
		Path:        allowedOriginsPath,
		Summary:     "Replace the stored list of browser origins that may call this API",
		Description: "Stores the list in the machine-owned file and applies it at once, with no " +
			"restart. The server answers a page from a stored origin as it answers one from " +
			"core.api.allowed_origins: it echoes the origin and lets the browser send the credential. " +
			"The list is added to the configuration file's and to the origins of the extensions that " +
			"are switched on, and it changes neither. An origin from one of those is not removed here: " +
			"edit the file, or switch the extension off. It takes the version last read in If-Match, " +
			"so a change another person stored in the meantime is refused and not written over. It " +
			"takes the admin role on the organization, because an allowed origin acts with the rights " +
			"of whoever opens its page.",
		Tags: []string{"system"},
		Errors: append(discoveryErrors, http.StatusNotFound, http.StatusPreconditionFailed,
			http.StatusUnprocessableEntity, http.StatusPreconditionRequired),
	}, func(ctx context.Context, in *setAllowedOriginsInput) (*allowedOriginsOutput, error) {
		if err := requireOrgAdmin(ctx, cfg, "the allowed browser origins"); err != nil {
			return nil, err
		}

		if cfg.Origins == nil {
			return nil, httpfault.ResourceNotFound(AllowedOriginsReference)
		}

		pre, err := precondition(in.IfMatch, "", false)
		if err != nil {
			return nil, err
		}

		list, err := cfg.Origins.SetAllowedOrigins(ctx, in.Body.Origins, pre)
		if err != nil {
			return nil, err
		}

		return allowedOriginsOf(list)
	})
}

// AllowedOriginsReference names the settings list in a fault.
var AllowedOriginsReference = httpfault.Reference("system", "allowed-origins")

// allowedOriginsOf is the answer to a read or a write of the list.
func allowedOriginsOf(list AllowedOriginList) (*allowedOriginsOutput, error) {
	out := &allowedOriginsOutput{}
	out.Body.Items = append([]AllowedOrigin{}, list.Items...)
	out.Body.Version = list.Version

	if list.Version != "" {
		etag, err := validatedETag(list.Version)
		if err != nil {
			return nil, err
		}

		out.ETag = etag
	}

	return out, nil
}
