package config

import (
	"fmt"
	"net/netip"
)

// maxPort is the highest UDP port number.
const maxPort = 65535

// WebRTC is how a browser reaches the relay that plays a camera's H.264
// stream. The zero value is the shipped behaviour: one random UDP port per
// viewer, and only the addresses bound on this machine are offered.
//
// A server in a container or behind NAT is reached at an address it does not
// hold, so a browser is given candidates it cannot dial and the stream never
// starts. The two keys let an operator publish one port and name the address
// a browser reaches it at.
type WebRTC struct {
	// UDPPort is the one UDP port every viewer is served on. Zero takes a
	// random port for each viewer, which a container cannot publish.
	UDPPort int `yaml:"udp_port"`
	// Addresses are IP addresses a browser reaches this server at, offered
	// beside the addresses bound on this machine. They are added and never
	// replace them, so a browser on the same network keeps its direct path.
	// They need UDPPort.
	Addresses []string `yaml:"addresses"`
}

// validate refuses a port out of range and an entry that is not one IP
// address. A host name is refused because the relay offers addresses only, and
// a name resolved once at startup goes stale without a sign.
func (w WebRTC) validate() error {
	if w.UDPPort < 0 || w.UDPPort > maxPort {
		return fmt.Errorf("%w: core.webrtc.udp_port is %d, expected 0 for a random port or 1 to %d",
			ErrInvalid, w.UDPPort, maxPort)
	}

	if len(w.Addresses) > 0 && w.UDPPort == 0 {
		return fmt.Errorf("%w: core.webrtc.addresses is set and core.webrtc.udp_port is not; "+
			"an advertised address is served only on one fixed port", ErrInvalid)
	}

	for _, address := range w.Addresses {
		parsed, err := netip.ParseAddr(address)
		if err != nil || parsed.Zone() != "" {
			return fmt.Errorf("%w: core.webrtc.addresses has %q, expected one IP address such as 192.0.2.10",
				ErrInvalid, address)
		}

		if parsed.IsUnspecified() {
			return fmt.Errorf("%w: core.webrtc.addresses has %q, which no browser can dial",
				ErrInvalid, address)
		}
	}

	return nil
}
