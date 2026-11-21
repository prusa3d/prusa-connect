package manifest

import (
	"fmt"
	"net/url"
	"strings"
)

// MaxAllowedOrigins is how many origins one manifest may name under
// allowed_origins.
const MaxAllowedOrigins = 8

// originScheme is the one scheme a manifest may ask for. A page served over
// plain http can be rewritten by anybody on the network path, so an origin a
// release asks every operator for is held to https. An operator who wants an
// http origin names it in core.api.allowed_origins themselves.
const originScheme = "https"

// readAllowedOrigins reads the browser origins under allowed_origins. Each is
// an exact https origin in the form a browser writes its Origin header in.
func readAllowedOrigins(tree map[string]any) ([]string, error) {
	origins, err := textList(tree, "", "allowed_origins")
	if err != nil {
		return nil, err
	}

	if err = refuseRepeat("allowed_origins", origins); err != nil {
		return nil, err
	}

	if len(origins) > MaxAllowedOrigins {
		return nil, fmt.Errorf("%w: allowed_origins names %d origins, and the limit is %d",
			ErrInvalid, len(origins), MaxAllowedOrigins)
	}

	for _, origin := range origins {
		if !IsAllowedOrigin(origin) {
			return nil, fmt.Errorf("%w: allowed_origins names %q, expected an exact https origin in lower case "+
				"with no path and no default port, such as https://slicer.example", ErrInvalid, origin)
		}
	}

	return origins, nil
}

// IsAllowedOrigin reports whether origin is one a manifest may ask for: an
// https scheme and a host in lower case, a port only when it is not 443, and
// nothing after the host. A browser writes its Origin header in that one
// form, and the host compares the two byte for byte.
func IsAllowedOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != originScheme || parsed.Hostname() == "" {
		return false
	}

	if strings.ToLower(origin) != origin || strings.Contains(parsed.Host, "*") {
		return false
	}

	return parsed.Scheme+"://"+parsed.Host == origin && parsed.Port() != "443"
}
