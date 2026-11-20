package simulator

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The chamber of an enclosed machine, as its telemetry reports it.
//
// A CORE One and a CORE One L send a chamber block in every full telemetry
// frame (Prusa-Firmware-Buddy, src/connect/render.cpp). The profiles carry the
// chamber setter for exactly those models, so the setter is what marks a
// machine with a chamber here, and no model is named.

const (
	// chamberSetter is the profile's name for the chamber target setter. The
	// driver sends a target as SET_VALUE, and a target of 0 as M141 S0.
	chamberSetter = "SET_CHAMBER_TARGET_TEMP"
	// printingChamberTemp is the air a print with no chamber target warms the
	// enclosure to. Like every reading here it is fixed.
	printingChamberTemp = 32.0
	// chamberFanAutomatic is the firmware's fan_pwm_target while the fans run
	// on their own (src/connect/printer.hpp).
	chamberFanAutomatic = -1
	// chamberLightFull is the side strip at full brightness, in percent.
	chamberLightFull = 100
	// chamberFanRPM is what each fan reports while it runs.
	chamberFanRPM = 3000
	// enclosureFanRPM is what an XL enclosure's filtration fan reports.
	enclosureFanRPM = 1500
	// enclosureFilterSeconds is how long the filter has run. It is fixed, as
	// every reading here is.
	enclosureFilterSeconds = 3600
)

// chamber is what a machine with one holds of it. A target of 0 is the
// firmware's word for no target.
type chamber struct {
	target int
	fan    int
	light  int
}

// newChamber is the chamber of a machine just switched on: no target, the fans
// automatic and the light full.
func newChamber() chamber {
	return chamber{fan: chamberFanAutomatic, light: chamberLightFull}
}

// hasChamber reports whether the machine is one of the enclosed models.
func (p *Printer) hasChamber() bool {
	_, declared := p.model.Command(chamberSetter)

	return declared
}

// setChamber applies one of the three chamber setters the way the firmware
// does (src/connect/planner.cpp). The caller holds the lock.
//
// The firmware cuts a target above the model's ceiling down to it without a
// word (src/feature/chamber/chamber.cpp). From 7.0.0 it also drops a target
// while the fans are set by hand, because only fans on auto act on one, and
// still answers that it finished (M141_M191.cpp). It takes a negative fan
// target as automatic and holds the rest to 0 to 100
// (src/common/pwm_utils.hpp). It keeps the light on a 0 to 255 scale, so the
// percentage that comes back is rounded down.
func (p *Printer) setChamber(command string, kwargs map[string]any) {
	number := func(name string) (int, bool) {
		value, ok := kwargs[name].(float64)

		return int(value), ok
	}

	switch command {
	case chamberSetter:
		// A CORE One on firmware 8.0.0 was seen to ignore a SET_VALUE
		// target of 0 and still answer that it finished, while M141 S0
		// turned the target off. Its source treats 0 like any other target,
		// so this copies one printer's behaviour, not a firmware rule.
		if target, ok := number("chamber.target_temp"); ok && target != 0 {
			p.setChamberTarget(target)
		}
	case "SET_CHAMBER_FAN_TARGET":
		if fan, ok := number("chamber.fan_pwm_target"); ok {
			p.chamber.fan = chamberFanAutomatic
			if fan >= 0 {
				p.chamber.fan = min(fan, chamberLightFull)
			}
		}
	case "SET_CHAMBER_LED_INTENSITY":
		if light, ok := number("chamber.led_intensity"); ok {
			p.chamber.light = light * pwmTop / chamberLightFull * chamberLightFull / pwmTop
		}
	}
}

// setChamberTarget holds a target the way M141 does (M141_M191.cpp): only
// fans on auto take one, and the ceiling cuts a higher one down. The caller
// holds the lock.
func (p *Printer) setChamberTarget(target int) {
	if p.chamber.fan == chamberFanAutomatic {
		p.chamber.target = min(max(target, 0), p.chamberCeiling())
	}
}

// noteChamberTarget applies each M141 line of a G-code payload on a machine
// with a chamber. The driver turns a chamber target off with M141 S0. A line
// with more than one S is skipped, as the firmware rejects it.
func (p *Printer) noteChamberTarget(payload []byte) {
	if !p.hasChamber() {
		return
	}

	for line := range strings.Lines(strings.ToUpper(string(payload))) {
		words := strings.Fields(line)
		if len(words) == 0 || words[0] != "M141" {
			continue
		}

		var targets []float64

		for _, word := range words[1:] {
			value, isS := strings.CutPrefix(word, "S")
			if v, err := strconv.ParseFloat(value, 64); isS && err == nil {
				targets = append(targets, v)
			}
		}

		if len(targets) != 1 || targets[0] < 0 {
			continue
		}

		p.mu.Lock()
		p.setChamberTarget(int(targets[0]))
		p.mu.Unlock()
	}
}

// pwmTop is the top of the firmware's 0 to 255 PWM and brightness scale.
const pwmTop = 255

// chamberCeiling is the highest target the model's chamber takes. The profile
// records the firmware's ceiling as the bound of the setter's argument.
func (p *Printer) chamberCeiling() int {
	command, declared := p.model.Command(chamberSetter)
	if !declared {
		return 0
	}

	if value, ok := command.Arguments["chamber.target_temp"]; ok && value.HasLimit {
		return int(value.Limit)
	}

	return math.MaxInt
}

// chamberFields is the chamber block of one frame, in the firmware's keys.
//
// The chamber has fans and no heater, so a target is a ceiling the fans hold
// the air under, and only while they run on their own: a fan target set by
// hand turns that off (src/feature/xbuddy_extension). A print warms the air
// and the room is the floor. The reading is fixed rather than climbing, for
// the reason every reading here is.
func (p *Printer) chamberFields(printing bool) map[string]any {
	temp := ambientTemp
	if printing {
		temp = printingChamberTemp
	}

	if target := float64(p.chamber.target); target > 0 && p.chamber.fan == chamberFanAutomatic && temp > target {
		temp = max(target, ambientTemp)
	}

	rpm := chamberFanRPM
	if p.chamber.fan == 0 {
		rpm = 0
	}

	return map[string]any{
		"temp":           temp,
		"target_temp":    p.chamber.target,
		"fan_1_rpm":      rpm,
		"fan_2_rpm":      rpm,
		"fan_pwm_target": p.chamber.fan,
		"led_intensity":  p.chamber.light,
	}
}

// enclosureFields is the enclosure block of an XL with one, in the firmware's
// keys (src/connect/render.cpp). The firmware writes the temperature as a
// whole number, and the time the filter has run in seconds.
func (p *Printer) enclosureFields(printing bool) map[string]any {
	temp := int(ambientTemp)
	if printing {
		temp = int(printingChamberTemp)
	}

	return map[string]any{
		"temp":        temp,
		"fan_rpm":     enclosureFanRPM,
		"time_in_use": enclosureFilterSeconds,
	}
}

// sendChanged puts a block into the frame when it differs from the one this
// connection last carried. The caller holds the lock.
//
// The firmware sends the chamber and enclosure blocks in full telemetry only:
// the first frame of a connection, a frame after something in the fingerprint
// moved, and one every five minutes (src/connect/planner.cpp). From 7.0.0 the
// fingerprint holds the chamber's target, fan target and light beside its fan
// speeds (src/connect/printer.cpp), so a setter shows within seconds. A
// reduced frame leaves the blocks out. Sending a block when it changed and on
// a new connection is that cadence without the clock.
func (p *Printer) sendChanged(fields map[string]any, key string, block map[string]any) {
	if p.blocksSession != p.sessions || p.blocksSent == nil {
		p.blocksSent = map[string]string{}
		p.blocksSession = p.sessions
	}

	encoded := fmt.Sprint(block)
	if p.blocksSent[key] == encoded {
		return
	}

	p.blocksSent[key] = encoded
	fields[key] = block
}
