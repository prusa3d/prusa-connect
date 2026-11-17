package xbuddy

import (
	"encoding/json"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The chamber's air temperature and the target it is held to. They sit beside
// the bed's, because the chamber is the third heater a person sets.
const (
	chamberTempPath   = "/temperature/chamber"
	chamberTargetPath = "/temperature/chamber_target"
)

// reportChamber writes what one telemetry frame says about the air inside the
// printer.
//
// Two blocks carry it (Prusa-Firmware-Buddy, src/connect/render.cpp). A CORE
// One and a CORE One L send a chamber block in every full frame, with the
// reading and the target. An XL sends an enclosure block while an enclosure is
// fitted and switched on. It has a reading and no target, because the XL does
// not heat its enclosure.
//
// The firmware writes 0 for a reading it does not have
// (src/connect/marlin_printer.cpp). Air inside a printer at 0 C is no reading,
// so the node goes, as a flow of 0 does. A target of 0 is the firmware's word
// for no target and stays 0, as the bed's does.
//
// A frame with neither block says nothing of the chamber. Reduced frames carry
// neither, so the reading the last full frame wrote stands.
func (s *Session) reportChamber(fields map[string]json.RawMessage) {
	if raw, present := fields["chamber"]; present {
		block := chamberBlock(raw)

		if temp, said := block["temp"]; said {
			s.report(connectorapi.StatePatch{Path: chamberTempPath, Value: readingOrNull(temp)})
		}

		if target, said := block["target_temp"]; said {
			s.report(connectorapi.StatePatch{Path: chamberTargetPath, Value: target})
		}

		return
	}

	if raw, present := fields["enclosure"]; present {
		if temp, said := chamberBlock(raw)["temp"]; said {
			s.report(connectorapi.StatePatch{Path: chamberTempPath, Value: readingOrNull(temp)})
		}
	}
}

// chamberBlock reads a block of numbers. Only numbers are kept, so a value of
// another type is a key the block did not carry.
func chamberBlock(raw json.RawMessage) map[string]json.RawMessage {
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil
	}

	for key, value := range block {
		var number float64
		if json.Unmarshal(value, &number) != nil {
			delete(block, key)
		}
	}

	return block
}

// readingOrNull passes a temperature on as the printer encoded it, and a 0 as
// null.
func readingOrNull(value json.RawMessage) json.RawMessage {
	var number float64
	if json.Unmarshal(value, &number) == nil && number == 0 {
		return nullValue
	}

	return value
}
