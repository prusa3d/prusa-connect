package xbuddy

import (
	"encoding/json"
	"slices"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The chamber's air temperature and the target it is held to. They sit beside
// the bed's, because the chamber is the third heater a person sets.
const (
	chamberTempPath   = "/temperature/chamber"
	chamberTargetPath = "/temperature/chamber_target"
)

// chamberPaths is where the rest of a chamber block lands. Each key keeps the
// firmware's name, which is also the argument its setter takes. A fan target
// of -1 means the fans run on their own, and the light is a percentage.
//
//nolint:gochecknoglobals,gosec // a read-only lookup table. G101 takes the "pw" of "fan_pwm_target" for a password.
var chamberPaths = map[string]string{
	"fan_pwm_target": "/chamber/fan_pwm_target",
	"fan_1_rpm":      "/chamber/fan_1_rpm",
	"fan_2_rpm":      "/chamber/fan_2_rpm",
	"led_intensity":  "/chamber/led_intensity",
}

// reportChamber writes what one telemetry frame says about the air inside the
// printer, its fans and its light.
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
// Reduced frames carry neither block, so the reading the last full frame wrote
// stands. A full frame, the one that carries temp_bed, with neither block
// means the enclosure was switched off, and the reading goes.
func (s *Session) reportChamber(fields map[string]json.RawMessage) {
	if raw, present := fields["chamber"]; present {
		block := chamberBlock(raw)

		if temp, said := block["temp"]; said {
			s.reportChamberReading(readingOrNull(temp))
		}

		if target, said := block["target_temp"]; said {
			s.report(connectorapi.StatePatch{Path: chamberTargetPath, Value: target})
		}

		keys := make([]string, 0, len(chamberPaths))
		for key := range chamberPaths {
			keys = append(keys, key)
		}

		slices.Sort(keys)

		for _, key := range keys {
			if value, said := block[key]; said {
				s.report(connectorapi.StatePatch{Path: chamberPaths[key], Value: value})
			}
		}

		return
	}

	if raw, present := fields["enclosure"]; present {
		if temp, said := chamberBlock(raw)["temp"]; said {
			s.reportChamberReading(readingOrNull(temp))
		}

		return
	}

	if _, full := fields["temp_bed"]; full && s.chamberReported {
		s.reportChamberReading(nullValue)
	}
}

// reportChamberReading writes the chamber's reading and remembers whether one
// stands, so that a full frame with no block removes only a reading this
// session wrote. A printer with no chamber never gets the node.
func (s *Session) reportChamberReading(value json.RawMessage) {
	s.chamberReported = string(value) != string(nullValue)
	s.report(connectorapi.StatePatch{Path: chamberTempPath, Value: value})
}

// chamberBlock reads a block of numbers. Only numbers are kept, so a value of
// another type, null included, is a key the block did not carry.
func chamberBlock(raw json.RawMessage) map[string]json.RawMessage {
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil
	}

	for key, value := range block {
		// A JSON null unmarshals into a float64 without an error, and it is
		// no number either.
		var number float64
		if string(value) == "null" || json.Unmarshal(value, &number) != nil {
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
