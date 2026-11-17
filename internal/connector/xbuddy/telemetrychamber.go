package xbuddy

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"

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
//nolint:gochecknoglobals // a lookup table, read-only after init.
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

// reportChamberSetter writes the value a chamber setter the printer finished
// left behind.
//
// The firmware leaves the chamber out of what decides that a telemetry frame is
// a full one (src/connect/printer.cpp), and only a full frame carries the
// chamber block. A new target or light shows up in telemetry when something
// else moves, or after five minutes at the latest. So the value is written
// here, the way the firmware will report it.
func (s *Session) reportChamberSetter(cmd connectorapi.Command) {
	var (
		path  string
		arg   string
		value func(int) int
	)

	switch cmd.Verb {
	case VerbSetChamberTemp:
		path, arg, value = chamberTargetPath, "chamber.target_temp", func(v int) int { return v }
	case VerbSetChamberFan:
		path, arg, value = chamberPaths["fan_pwm_target"], "chamber.fan_pwm_target", reportedFanTarget
	case VerbSetChamberLED:
		path, arg, value = chamberPaths["led_intensity"], "chamber.led_intensity", reportedLight
	default:
		return
	}

	sent, err := strconv.Atoi(cmd.Args[arg])
	if err != nil {
		return
	}

	s.report(connectorapi.StatePatch{Path: path, Value: json.RawMessage(strconv.Itoa(value(sent)))})
}

// reportedFanTarget is the fan target the firmware reports after it was sent
// percent. A negative one hands the fans back to their own control, and the
// rest is held to 0 to 100 and goes through the 0 to 255 PWM scale both ways
// (src/common/pwm_utils.hpp).
func reportedFanTarget(percent int) int {
	if percent < 0 {
		return fanAutomatic
	}

	pwm := math.Round(float64(min(percent, percentLimit)) * pwmScale / percentLimit)

	return int(math.Round(pwm * percentLimit / pwmScale))
}

// reportedLight is the light the firmware reports after it was sent percent.
// It stores percent * 255 / 100 and reports stored * 100 / 255, both in whole
// numbers, so 50 comes back as 49 (src/connect/planner.cpp,
// src/connect/marlin_printer.cpp).
func reportedLight(percent int) int {
	return percent * pwmScale / percentLimit * percentLimit / pwmScale
}

// pwmScale is the top of the firmware's PWM and brightness scale.
const pwmScale = 255
