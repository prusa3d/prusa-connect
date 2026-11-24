package xbuddy

import (
	"encoding/json"
	"strconv"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// buildVolume is the space a print may fill on one model, in millimetres.
type buildVolume struct {
	x, y, z float64
}

// buildVolumes is the printable volume of each model this driver reaches,
// keyed by the model's name in the capability profiles. A printer reports its
// model in its own spelling, such as COREONEINDX for CoreOne_INDX, and
// sameModel is what matches the two.
//
// The figures are the ones PrusaSlicer slices against: bed_shape and
// max_print_height of the printer profiles in its PrusaResearch.ini vendor
// bundle, which is public. A file sliced for the model fits inside them by
// construction. The iX and the INDX variants of the CORE One and CORE One L
// have no public slicer profile, so their figures are X_BED_SIZE, Y_BED_SIZE
// and Z_SIZE of the public firmware: include/marlin/Configuration_iX.h, and
// the HAS_INDX() branch of Configuration_COREONE.h and
// Configuration_COREONEL.h. An INDX bed differs from the plain model's, and
// the firmware keeps a strip of it for the nozzle cleaner and the tool dock,
// so those rows are an upper bound rather than the area a slicer would use.
// A model with no row reports no volume.
//
//nolint:gochecknoglobals,mnd // a fixed table, transcribed and read-only.
var buildVolumes = map[string]buildVolume{
	"MINI":     {x: 180, y: 180, z: 180},
	"MK3.5":    {x: 250, y: 210, z: 210},
	"MK3.5S":   {x: 250, y: 210, z: 210},
	"MK3.9":    {x: 250, y: 210, z: 220},
	"MK3.9S":   {x: 250, y: 210, z: 220},
	"MK4":      {x: 250, y: 210, z: 220},
	"MK4S":     {x: 250, y: 210, z: 220},
	"XL":       {x: 360, y: 360, z: 360},
	"CoreOne":  {x: 250, y: 220, z: 270},
	"CoreOneL": {x: 300, y: 300, z: 330},
	"IX":       {x: 260, y: 260, z: 175},

	"CoreOne_INDX":  {x: 250, y: 205.5, z: 275},
	"CoreOneL_INDX": {x: 301, y: 270, z: 330},
}

// buildVolumeOf is the volume of a model, matched by sameModel, and false for
// a model this driver has no figures for.
func buildVolumeOf(model string) (buildVolume, bool) {
	for name, volume := range buildVolumes {
		if sameModel(name, model) {
			return volume, true
		}
	}

	return buildVolume{}, false
}

// sameModel reports whether a printer's spelling of its model names a row of
// this driver's tables. It folds both the way the host folds a model, case and
// separators dropped, because firmware sends COREONEINDX and COREONEL-INDX in
// User-Agent-Printer (include/common/printer_model_data.hpp) where the tables
// say CoreOne_INDX and CoreOneL_INDX. A comparison that only ignored case
// left every INDX with no build volume and no head count.
func sameModel(name, reported string) bool {
	return connectorapi.FoldModel(name) == connectorapi.FoldModel(reported)
}

// buildVolumePatches is what one model reports about its volume.
func buildVolumePatches(model string) []connectorapi.StatePatch {
	volume, known := buildVolumeOf(model)
	if !known {
		return nil
	}

	number := func(value float64) json.RawMessage {
		return json.RawMessage(strconv.FormatFloat(value, 'f', -1, 64))
	}

	return []connectorapi.StatePatch{
		{Path: connectorapi.BuildVolumeXPath, Value: number(volume.x)},
		{Path: connectorapi.BuildVolumeYPath, Value: number(volume.y)},
		{Path: connectorapi.BuildVolumeZPath, Value: number(volume.z)},
	}
}
