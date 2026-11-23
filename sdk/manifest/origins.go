package manifest

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
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
			return nil, fmt.Errorf("%w: allowed_origins names %q, expected an exact https origin as a browser "+
				"writes it: lower case, an ASCII host, no path and no default port, such as https://slicer.example",
				ErrInvalid, origin)
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

	return parsed.Scheme+"://"+parsed.Host == origin && parsed.Port() != "443" && browserHost(parsed)
}

// browserHost reports whether the host and the port are in the form a browser
// writes them in: an ASCII host (a browser sends a name in punycode), an IP
// address in its shortest form, and a port as a plain number from 1 to 65535.
// The core holds core.api.allowed_origins to the same form.
func browserHost(parsed *url.URL) bool {
	if strings.HasSuffix(parsed.Host, ":") {
		return false
	}

	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return false
		}
	}

	host := parsed.Hostname()
	for i := range len(host) {
		if host[i] >= utf8.RuneSelf {
			return false
		}
	}

	// A browser reads a host whose last label is a number as an IPv4 address,
	// so 0x7f.1 reaches it as 127.0.0.1.
	last := host[strings.LastIndexByte(host, '.')+1:]
	if strings.HasPrefix(parsed.Host, "[") || strings.HasPrefix(last, "0x") ||
		(last != "" && strings.Trim(last, "0123456789") == "") {
		addr, err := netip.ParseAddr(host)

		return err == nil && addr.Zone() == "" && addr.String() == host
	}

	return true
}
