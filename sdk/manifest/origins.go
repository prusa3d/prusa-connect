package manifest

import (
	"net/url"
	"strings"
)

// originScheme is the one scheme a module may bring an origin of. A page
// served over plain http can be rewritten by anybody on the network path, so
// an origin a release brings to every operator is held to https. An operator
// who wants an http origin names it in core.api.allowed_origins themselves.
const originScheme = "https"

// IsAllowedOrigin reports whether origin is one a module may bring: an
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
