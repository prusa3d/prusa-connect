package xbuddy_test

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/internal/printer/capability"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The live test this exists for: an operator set a nozzle target of 9999 and one
// of -4000, and both were accepted with a 200 and put on the wire. The values a
// printer cannot carry out are refused by the descriptor now, which is the
// refusal the HTTP surface renders as a 400.
func TestASetpointOutsideItsRangeIsRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		verb    connectorapi.Capability
		arg     string
		value   string
		refused bool
	}{
		{name: "a nozzle target no hotend reaches", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "9999", refused: true},
		{name: "a nozzle target below off", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "-4000", refused: true},
		{name: "a nozzle target one over the ceiling", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "301", refused: true},
		{name: "a nozzle target somebody prints at", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "215"},
		{name: "the nozzle target that turns the heater off", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "0"},
		{name: "the highest nozzle target in the fleet", verb: connectorapi.Capability(
			"core.command.set_nozzle_temperature"), arg: "nozzle_temperature", value: "300"},
		{name: "a bed target no printer reaches", verb: connectorapi.Capability(
			"core.command.set_bed_temperature"), arg: "bed_temperature", value: "500", refused: true},
		{name: "a bed target below off", verb: connectorapi.Capability(
			"core.command.set_bed_temperature"), arg: "bed_temperature", value: "-1", refused: true},
		{name: "a bed target somebody prints on", verb: connectorapi.Capability(
			"core.command.set_bed_temperature"), arg: "bed_temperature", value: "60"},
		{name: "a chamber target no enclosure reaches", verb: connectorapi.Capability(
			"core.command.set_chamber_target_temp"), arg: "chamber.target_temp", value: "200", refused: true},
		{name: "a chamber target an enclosure holds", verb: connectorapi.Capability(
			"core.command.set_chamber_target_temp"), arg: "chamber.target_temp", value: "40"},
		{name: "a speed factor of nothing", verb: connectorapi.Capability(
			"core.command.set_speed"), arg: "speed", value: "0", refused: true},
		{name: "a speed factor past what the firmware clamps to", verb: connectorapi.Capability(
			"core.command.set_speed"), arg: "speed", value: "1000", refused: true},
		{name: "a speed factor somebody prints at", verb: connectorapi.Capability(
			"core.command.set_speed"), arg: "speed", value: "100"},
		{name: "a flow factor of nothing", verb: connectorapi.Capability(
			"core.command.set_flow"), arg: "flow", value: "0", refused: true},
		{name: "a flow factor somebody prints at", verb: connectorapi.Capability(
			"core.command.set_flow"), arg: "flow", value: "95"},
	}

	baseline := driverBaseline()

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !baseline.Has(testCase.verb) {
				t.Fatalf("this driver does not send %s, so the case measures nothing", testCase.verb)
			}

			_, err := baseline.Validate(connectorapi.Command{
				Verb: testCase.verb,
				Args: map[string]string{testCase.arg: testCase.value},
			})

			refused := err != nil
			if refused != testCase.refused {
				t.Fatalf("%s = %s: refused = %v, want %v (%v)",
					testCase.verb, testCase.value, refused, testCase.refused, err)
			}

			// A refusal has to be the one the HTTP surface renders as a 400,
			// and not a missing argument or an unknown verb.
			if refused && !fault.HasCode(err, connectorapi.ArgumentOutOfRange.Code()) {
				t.Fatalf("%s = %s was refused as %v, want an out-of-range fault",
					testCase.verb, testCase.value, err)
			}
		})
	}
}

// The bound a printer is actually held to is the one the merge produces, so a
// setpoint is measured through the resolver for a real model as well as against
// the baseline: a profile that dropped the bound would leave the baseline's,
// and one that widened it would be ignored, and neither can be seen from the
// descriptor alone.
func TestAResolvedPrinterIsHeldToItsModelsSetpointRange(t *testing.T) {
	t.Parallel()

	profiles, err := profile.Builtin()
	if err != nil {
		t.Fatalf("loading the built-in profiles: %v", err)
	}

	resolver := capability.NewResolver(profiles, slog.New(slog.DiscardHandler))
	nozzle := connectorapi.Capability("core.command.set_nozzle_temperature")

	for _, model := range profiles.Models() {
		resolution := resolver.Resolve(t.Context(), driverBaseline(), identityOf(model), nil, 1)
		if !resolution.ProfileMatched {
			t.Fatalf("%s matched no profile", model)
		}

		for value, want := range map[string]bool{"9999": true, "-4000": true, "215": false} {
			_, validateErr := resolution.Capabilities.Validate(connectorapi.Command{
				Verb: nozzle,
				Args: map[string]string{"nozzle_temperature": value},
			})

			if refused := validateErr != nil; refused != want {
				t.Errorf("%s nozzle target %s: refused = %v, want %v (%v)",
					model, value, refused, want, validateErr)
			}
		}
	}
}

// A bound row may only narrow. The profile tier is intersected with the
// baseline, so a row wider than the baseline is ignored rather than granted,
// and a row that is ignored is a number somebody wrote and nothing enforces.
// This is the gate on the data, and it lives here because
// internal/printer/profile may not import a connector (ADR-20260824-kkr rule 4).
func TestNoProfileBoundIsWiderThanTheDriverBaseline(t *testing.T) {
	t.Parallel()

	profiles, err := profile.Builtin()
	if err != nil {
		t.Fatalf("loading the built-in profiles: %v", err)
	}

	if len(profiles.Bounds) == 0 {
		t.Fatal("the built-in registry carries no argument bounds, so this gate watches nothing")
	}

	baseline := driverBaseline()
	checked := 0

	for _, bound := range profiles.Bounds {
		declared, present := baseline[bound.Argument]
		if !present {
			// A bound on an argument this driver does not send is a bound for
			// another dialect, and the baseline here says nothing about it.
			continue
		}

		checked++

		if declared.HasMin && bound.Min < declared.Min {
			t.Errorf("%s is bounded from %v in %s and from %v in the driver baseline, which is wider",
				bound.Argument, bound.Min, boundsFile, declared.Min)
		}

		if declared.HasLimit && bound.Max > declared.Limit {
			t.Errorf("%s is bounded to %v in %s and to %v in the driver baseline, which is wider",
				bound.Argument, bound.Max, boundsFile, declared.Limit)
		}

		if !declared.HasMin || !declared.HasLimit {
			t.Errorf("%s carries a per-model range and no baseline bound, so a device of an unknown "+
				"model is offered more than a recognised one", bound.Argument)
		}
	}

	if checked == 0 {
		t.Error("no bound row names an argument this driver sends, so this gate compared nothing")
	}
}

// boundsFile is what a failure above points a reader at.
const boundsFile = "internal/printer/profile/bounds.json"

// unboundedOnPurpose are the numeric arguments of a setter or a move that carry
// no range, with the reason. A displacement is not a setpoint: it has a negative
// half, because a move goes both ways, and no ceiling this repository can
// defend, because the travel of the largest machine is not recorded anywhere
// here. Guessing one refuses a move somebody's printer can make.
var unboundedOnPurpose = map[string]string{
	"core.command.move#x":                                   "an absolute position on an axis, which is negative on a machine whose origin is not its corner",
	"core.command.move#y":                                   "an absolute position, as for x",
	"core.command.move_e#length":                            "a length of filament, negative to retract",
	"core.command.move_z#distance":                          "a distance along Z, negative to go down",
	"core.command.z_power_move#distance":                    "a distance along Z, as for move_z",
	"core.command.set_tool_nozzle_diameter#nozzle_diameter": "bounded below by refuseExcludedValues, and no ceiling is published for the largest nozzle a tool takes",
}

// Every numeric setpoint this driver sends is bounded, and every numeric
// argument that is not is named above with its reason. A setter added without a
// range is the live fault this file exists for, repeated.
func TestEverySetpointArgumentIsBounded(t *testing.T) {
	t.Parallel()

	baseline := driverBaseline()
	unbounded := map[string]bool{}

	for _, key := range baseline.Keys() {
		if _, isArgument := key.Argument(); !isArgument {
			continue
		}

		value := baseline[key]
		if !value.Flag || !value.Kind.IsNumeric() || len(value.Supported) > 0 {
			continue
		}

		verb := key.Verb().String()
		if !strings.HasPrefix(verb, "core.command.set_") && !strings.HasPrefix(verb, "core.command.move") &&
			!strings.HasPrefix(verb, "core.command.z_power_move") {
			continue
		}

		if value.HasMin && value.HasLimit {
			continue
		}

		unbounded[key.String()] = true

		if unboundedOnPurpose[key.String()] == "" {
			t.Errorf("%s takes %s with no range, so any number a caller sends reaches the printer. "+
				"Bound it, or name it in unboundedOnPurpose with the reason", key.Verb(), key.String())
		}
	}

	for key := range unboundedOnPurpose {
		if !unbounded[key] {
			t.Errorf("%s is recorded as deliberately unbounded and it is bounded, so the record is stale", key)
		}
	}
}

// A chamber target is held to the ceiling the model's firmware has: 55 C on a
// CORE One and 60 C on a CORE One L, each with its INDX build. The firmware
// cuts a higher target down without a word, so the host refuses it rather than
// report a target the chamber will not keep.
func TestAChamberTargetIsHeldToTheModelsCeiling(t *testing.T) {
	t.Parallel()

	profiles, err := profile.Builtin()
	if err != nil {
		t.Fatalf("loading the built-in profiles: %v", err)
	}

	resolver := capability.NewResolver(profiles, slog.New(slog.DiscardHandler))
	chamber := connectorapi.Capability("core.command.set_chamber_target_temp")

	for model, ceiling := range map[string]int{
		"CoreOne": 55, "CoreOne_INDX": 55, "CoreOneL": 60, "CoreOneL_INDX": 60,
	} {
		resolution := resolver.Resolve(t.Context(), driverBaseline(), identityOf(model), nil, 1)

		for value, refused := range map[int]bool{0: false, ceiling: false, ceiling + 1: true, -1: true} {
			_, validateErr := resolution.Capabilities.Validate(connectorapi.Command{
				Verb: chamber,
				Args: map[string]string{"chamber.target_temp": strconv.Itoa(value)},
			})
			if (validateErr != nil) != refused {
				t.Errorf("%s: chamber target %d refused = %v, want %v (%v)",
					model, value, validateErr != nil, refused, validateErr)
			}
		}
	}
}
