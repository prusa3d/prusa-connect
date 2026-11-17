package simulator

// The chamber of an enclosed machine, as its telemetry reports it.
//
// A CORE One and a CORE One L send a chamber block in every full telemetry
// frame (Prusa-Firmware-Buddy, src/connect/render.cpp). The profiles carry the
// chamber setter for exactly those models, so the setter is what marks a
// machine with a chamber here, and no model is named.

const (
	// chamberSetter is the wire verb that sets the chamber's target.
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

// setChamber applies one of the three chamber setters. The caller holds the
// lock, and the arguments have passed the model's checks.
func (p *Printer) setChamber(command string, kwargs map[string]any) {
	number := func(name string) (int, bool) {
		value, ok := kwargs[name].(float64)

		return int(value), ok
	}

	switch command {
	case chamberSetter:
		if target, ok := number("chamber.target_temp"); ok {
			p.chamber.target = target
		}
	case "SET_CHAMBER_FAN_TARGET":
		if fan, ok := number("chamber.fan_pwm_target"); ok {
			p.chamber.fan = fan
		}
	case "SET_CHAMBER_LED_INTENSITY":
		if light, ok := number("chamber.led_intensity"); ok {
			p.chamber.light = light
		}
	}
}

// chamberFields is the chamber block of one frame, in the firmware's keys.
//
// The air reads the target once one is set, and the warmth of a print or the
// room otherwise. The reading is fixed rather than climbing, for the reason
// every reading here is.
func (p *Printer) chamberFields(printing bool) map[string]any {
	temp := ambientTemp
	if printing {
		temp = printingChamberTemp
	}

	if p.chamber.target > 0 {
		temp = float64(p.chamber.target)
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
