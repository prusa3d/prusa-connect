package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
)

// A deployment with no webrtc block keeps the relay as it shipped: a random
// port for each viewer and no address but the ones bound on the machine.
func TestADeploymentWithNoWebRTCBlockTakesARandomPortAndAdvertisesNothing(t *testing.T) {
	t.Parallel()

	core, err := config.Load(write(t, "core:\n  principal: someone\n"))
	require.NoError(t, err)

	require.Zero(t, core.WebRTC.UDPPort)
	require.Empty(t, core.WebRTC.Addresses)
	require.NoError(t, core.Validate())
}

// The loader refuses every key it does not read, so the block it does read is
// proven read.
func TestTheStrictLoaderReadsBothWebRTCKeys(t *testing.T) {
	t.Parallel()

	core, err := config.Load(write(t, `core:
  webrtc:
    udp_port: 8189
    addresses:
      - 192.0.2.10
      - 2001:db8::10
`))
	require.NoError(t, err)

	require.Equal(t, 8189, core.WebRTC.UDPPort)
	require.Equal(t, []string{"192.0.2.10", "2001:db8::10"}, core.WebRTC.Addresses)
	require.NoError(t, core.Validate())
}

// A value the relay cannot use stops the server at startup, with the key it
// is written under, rather than leaving every camera view to fail later.
func TestAWebRTCValueTheRelayCannotUseIsRefusedAtStartup(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		webrtc config.WebRTC
		key    string
	}{
		"a negative port":   {config.WebRTC{UDPPort: -1}, "core.webrtc.udp_port"},
		"a port past 65535": {config.WebRTC{UDPPort: 65536}, "core.webrtc.udp_port"},
		"a host name": {
			config.WebRTC{UDPPort: 8189, Addresses: []string{"printers.example"}},
			"core.webrtc.addresses",
		},
		"an address with a port": {
			config.WebRTC{UDPPort: 8189, Addresses: []string{"192.0.2.10:8189"}},
			"core.webrtc.addresses",
		},
		"an address no browser dials": {
			config.WebRTC{UDPPort: 8189, Addresses: []string{"0.0.0.0"}},
			"core.webrtc.addresses",
		},
		"an address with a zone": {
			config.WebRTC{UDPPort: 8189, Addresses: []string{"fe80::1%eth0"}},
			"core.webrtc.addresses",
		},
		"an address with no fixed port": {
			config.WebRTC{Addresses: []string{"192.0.2.10"}},
			"core.webrtc.udp_port",
		},
		"an empty address": {config.WebRTC{UDPPort: 8189, Addresses: []string{""}}, "core.webrtc.addresses"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := config.Load(write(t, "core:\n  principal: someone\n"))
			require.NoError(t, err)

			core.WebRTC = tc.webrtc

			err = core.Validate()
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), tc.key)
		})
	}
}
