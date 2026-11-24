package xbuddy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildVolumePatchesNameTheModelsVolume(t *testing.T) {
	t.Parallel()

	got := written(buildVolumePatches("coreone"))
	require.Equal(t, map[string]string{
		"/build_volume/x": "250", "/build_volume/y": "220", "/build_volume/z": "270",
	}, got, "the model is matched without case")

	require.Equal(t, map[string]string{
		"/build_volume/x": "250", "/build_volume/y": "205.5", "/build_volume/z": "275",
	}, written(buildVolumePatches("CoreOne_INDX")), "an INDX variant has a bed of its own")

	require.Empty(t, buildVolumePatches("MK2.5"), "a model with no figures reports nothing")
}

// The firmware names its model in User-Agent-Printer as
// include/common/printer_model_data.hpp spells it, which for the INDX variants
// is not the spelling of the tables. Each must still find its row.
func TestTheModelSpellingsFirmwareSendsFindTheirFigures(t *testing.T) {
	t.Parallel()

	require.Equal(t, map[string]string{
		"/build_volume/x": "250", "/build_volume/y": "205.5", "/build_volume/z": "275",
	}, written(buildVolumePatches("COREONEINDX")), "the CORE One INDX as its firmware spells it")

	require.Equal(t, map[string]string{
		"/build_volume/x": "301", "/build_volume/y": "270", "/build_volume/z": "330",
	}, written(buildVolumePatches("COREONEL-INDX")), "the CORE One L INDX as its firmware spells it")

	for reported, want := range map[string]int{
		"COREONEINDX":   8,
		"COREONEL-INDX": 8,
		"CoreOne_INDX":  8,
		"XL":            5,
		"MK4S":          1,
		"MK3.9":         1,
	} {
		heads, known := headsOf(reported)
		require.True(t, known, "%s is a model the driver has figures for", reported)
		require.Equal(t, want, heads, "the heads of %s", reported)
	}

	_, known := headsOf("XLP")
	require.False(t, known, "a model with no row stays unknown: folding is not a prefix match")
}
