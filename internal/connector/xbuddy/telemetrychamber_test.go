package xbuddy

import "testing"

// A CORE One's chamber block lands at the chamber's two paths. The frame is
// the firmware's own shape (Prusa-Firmware-Buddy, src/connect/render.cpp): the
// reading with one decimal, the target whole, and the fans and the light
// beside them.
func TestAChamberBlockLandsAtTheChamberPaths(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"PRINTING","temp_bed":100.0,"chamber":{"temp":38.4,"target_temp":45,`+
		`"fan_1_rpm":3200,"fan_2_rpm":3100,"fan_pwm_target":-1,"led_intensity":100}}`)
	printer.awaitDialogTree(t, map[string]string{
		chamberTempPath: "38.4", chamberTargetPath: "45",
		"/chamber/fan_pwm_target": "-1", "/chamber/fan_1_rpm": "3200", "/chamber/fan_2_rpm": "3100",
		"/chamber/led_intensity": "100",
	})

	// A reduced frame has no chamber block, and the reading stands.
	printer.text(t, `{"state":"PRINTING","progress":12}`)
	printer.text(t, `{"state":"PRINTING","temp_bed":99.0}`)
	printer.awaitDialogTree(t, map[string]string{bedPath: "99.0"})

	if held := printer.dialogValue(chamberTempPath); held != "38.4" {
		t.Errorf("%s reads %s after frames with no chamber block, want 38.4", chamberTempPath, held)
	}
}

// A target of 0 is no target, and stays 0 as the bed's does. A reading of 0 is
// the firmware's stand-in for no reading, so the node goes.
func TestAChamberZeroIsNoTargetAndNoReading(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","chamber":{"temp":24.0,"target_temp":40}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "24.0", chamberTargetPath: "40"})

	printer.text(t, `{"state":"IDLE","chamber":{"temp":0.0,"target_temp":0}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "null", chamberTargetPath: "0"})
}

// An XL with its enclosure on reports the enclosure's air at the chamber's
// reading, and no target, because it does not heat the enclosure.
func TestAnEnclosureBlockIsTheChamberReading(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","enclosure":{"temp":31,"fan_rpm":1500,"time_in_use":3600}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "31"})

	if got := printer.dialogPatchCount(chamberTargetPath); got != 0 {
		t.Errorf("an enclosure block wrote %d patches at %s, and it carries no target", got, chamberTargetPath)
	}
}

// A block that is not an object, or a value that is not a number, is skipped
// and leaves the session serving.
func TestAMalformedChamberBlockIsSkipped(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","chamber":"hot"}`)
	printer.text(t, `{"state":"IDLE","chamber":{"temp":"warm","target_temp":35}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTargetPath: "35"})

	if got := printer.dialogPatchCount(chamberTempPath); got != 0 {
		t.Errorf("%d patches arrived at %s from blocks with no numeric reading", got, chamberTempPath)
	}
}

// An XL whose enclosure is switched off stops sending the block. The next full
// frame, the one with temp_bed, has none, and the reading goes. A reduced frame
// in between leaves it standing.
func TestAnEnclosureSwitchedOffTakesItsReadingAway(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","temp_bed":25.0,"enclosure":{"temp":31,"fan_rpm":1500,"time_in_use":3600}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "31"})

	printer.text(t, `{"state":"IDLE","progress":0}`)
	printer.text(t, `{"state":"IDLE","temp_bed":24.0}`)
	printer.awaitDialogTree(t, map[string]string{bedPath: "24.0", chamberTempPath: "null"})
}

// A machine with no chamber gets no chamber node from its full frames.
func TestAFullFrameWithNoChamberWritesNothingThere(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","temp_bed":25.0}`)
	printer.text(t, `{"state":"IDLE","temp_bed":26.0}`)
	printer.awaitDialogTree(t, map[string]string{bedPath: "26.0"})

	if got := printer.dialogPatchCount(chamberTempPath); got != 0 {
		t.Errorf("%d patches arrived at %s from a machine with no chamber", got, chamberTempPath)
	}
}

// A null in the block is no value, so it writes nothing.
func TestANullInTheChamberBlockIsSkipped(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.text(t, `{"state":"IDLE","chamber":{"temp":24.0,"target_temp":40}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "24.0", chamberTargetPath: "40"})

	printer.text(t, `{"state":"IDLE","chamber":{"temp":25.0,"target_temp":null}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "25.0"})

	if held := printer.dialogValue(chamberTargetPath); held != "40" {
		t.Errorf("%s reads %s after a null target, want 40", chamberTargetPath, held)
	}
}
