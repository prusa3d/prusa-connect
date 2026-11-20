package webrtc

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every keyframe on the live socket carries the parameter sets, not only the
// first. A browser that fell behind skips to a keyframe, and a decoder
// configures itself from that keyframe alone.
func TestEveryKeyframeOnTheLiveSocketCarriesTheParameterSets(t *testing.T) {
	t.Parallel()

	stream := annexB(
		unit(headerSPS, 6), unit(headerPPS, 4), unit(headerIDR, 90),
		unit(headerNonIDR, 40),
		unit(headerIDR, 90),
		unit(headerNonIDR, 40),
	)

	units := NewAccessUnits(io.NopCloser(bytes.NewReader(stream)), discard())

	var keys []bool

	for {
		picture, key, err := units.Next()
		if err == io.EOF { //nolint:errorlint // Next returns io.EOF itself.
			break
		}

		require.NoError(t, err)

		keys = append(keys, key)

		if key {
			require.True(t, carries(picture, unitSPS), "a keyframe arrived with no SPS")
			require.True(t, carries(picture, unitPPS), "a keyframe arrived with no PPS")
		} else {
			require.False(t, carries(picture, unitSPS), "a picture that is not a keyframe carried an SPS")
		}
	}

	require.Equal(t, []bool{true, false, true, false}, keys)
	require.NoError(t, units.Close())
}
