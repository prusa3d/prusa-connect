package moonraker_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"example.com/prusa/moonraker"
	"example.com/prusa/moonraker/moonrakersim"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// settle is how long a case waits for something the machine has to send. It is
// a loopback socket, so anything that has not happened by then is not going to.
const settle = 5 * time.Second

// TestAMachineWithNoPauseResumeIsNotOfferedPause is the honesty rule as an
// assertion. Klipper registers PAUSE, RESUME and CANCEL_PRINT from its
// pause_resume section, so a machine without the section cannot pause, and a
// dialect that offered the verb anyway would be offering something that fails
// at the machine.
func TestAMachineWithNoPauseResumeIsNotOfferedPause(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{Objects: without(moonrakersim.DefaultObjects(), "pause_resume")})

	caps, _ := connect(t, machine).Capabilities()

	require.False(t, caps.Has(moonraker.VerbPausePrint), "pause was offered by a machine that cannot pause")
	require.False(t, caps.Has(moonraker.VerbResumePrint), "resume was offered by a machine that cannot resume")
	require.False(t, caps.Has(moonraker.VerbStopPrint), "stop was offered by a machine that cannot stop")
	require.True(t, caps.Has(moonraker.VerbSendStateInfo), "reading the state needs no section and was withheld")
}

// TestAMachineWithNoHeatedBedIsNotOfferedABedTarget is the same rule on the
// other object, and it also covers the argument key: a verb that goes must take
// its argument with it, or the descriptor describes an argument of nothing.
func TestAMachineWithNoHeatedBedIsNotOfferedABedTarget(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{Objects: without(moonrakersim.DefaultObjects(), "heater_bed")})

	caps, _ := connect(t, machine).Capabilities()

	require.False(t, caps.Has(moonraker.VerbBedTarget), "a bed target was offered by a machine with no bed")
	require.NotContains(t, caps, moonraker.ArgBedTarget, "the bed target's argument outlived its verb")
	require.True(t, caps.Has(moonraker.VerbNozzleTarget), "the hotend went with the bed")
}

// TestTheHotendCeilingIsTheMachinesOwnMaxTemp reads the bound off the machine
// rather than off this driver. A Voron with a 350 hotend and an Ender with a
// 260 one are the same dialect and not the same descriptor.
func TestTheHotendCeilingIsTheMachinesOwnMaxTemp(t *testing.T) {
	t.Parallel()

	status := moonrakersim.DefaultStatus()
	status["configfile"] = map[string]any{"settings": map[string]any{
		"extruder":   map[string]any{"min_temp": 10.0, "max_temp": 260.0},
		"heater_bed": map[string]any{"min_temp": 0.0, "max_temp": 120.0},
	}}

	caps, _ := connect(t, startMachine(t, moonrakersim.Options{Status: status})).Capabilities()

	limit, bounded := caps.Limit(moonraker.ArgNozzleTarget)
	require.True(t, bounded, "the hotend target carries no upper bound at all")
	require.InDelta(t, 260.0, limit, 0, "the ceiling is not the machine's own max_temp")

	_, err := caps.Validate(connectorapi.Command{
		Verb: moonraker.VerbNozzleTarget,
		Args: map[string]string{"nozzle_temperature": "300"},
	})
	require.Error(t, err, "a target above the machine's max_temp was accepted")
}

// TestPausingAPrintReachesTheMachine is the command half, end to end: the verb
// becomes Moonraker's own call and the machine is really asked.
func TestPausingAPrintReachesTheMachine(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	for _, verb := range []connectorapi.Capability{
		moonraker.VerbPausePrint, moonraker.VerbResumePrint, moonraker.VerbStopPrint,
	} {
		result, err := session.Do(t.Context(), connectorapi.Command{Verb: verb})
		require.NoError(t, err, "%s did not reach the machine", verb)
		require.True(t, result.OK, "%s was not accepted", verb)
	}

	require.True(t, machine.Called("printer.print.pause"), "pause reached no call")
	require.True(t, machine.Called("printer.print.resume"), "resume reached no call")
	require.True(t, machine.Called("printer.print.cancel"), "stop reached no call")
}

// TestACancelledPauseNeverReachesTheMachine is the half a refusal cannot show.
// The conformance suite only asks that a cancelled command is not reported as
// done, and a driver that sends it and then abandons the answer satisfies that
// while pausing the print anyway.
func TestACancelledPauseNeverReachesTheMachine(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := session.Do(ctx, connectorapi.Command{Verb: moonraker.VerbPausePrint})

	require.Error(t, err, "a cancelled pause was answered")
	require.False(t, result.OK)

	// Never and not False. The machine records a call on its own goroutine, so
	// reading the record the instant Do returns passes against a driver that
	// wrote the frame and then abandoned the answer.
	require.Never(t, func() bool {
		return machine.Called("printer.print.pause")
	}, 200*time.Millisecond, 10*time.Millisecond,
		"a caller that had already given up paused the print")
}

// TestAMachineThatRefusesAPauseSaysWhy holds the other outcome. A refusal is an
// answer and not a broken connection, so it comes back as a result that is not
// OK and carries the machine's own words.
func TestAMachineThatRefusesAPauseSaysWhy(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{
		Refused: map[string]string{"printer.print.pause": "Print Pause Failed"},
	})

	result, err := connect(t, machine).Do(t.Context(), connectorapi.Command{Verb: moonraker.VerbPausePrint})

	require.NoError(t, err, "a machine that said no was reported as a connection failure")
	require.False(t, result.OK, "a refused pause was reported as carried out")
	require.Equal(t, "Print Pause Failed", result.MachineReason)
}

// TestSettingATargetBecomesOneKlipperCommand pins the line the machine
// receives. SET_HEATER_TEMPERATURE returns as soon as the target is set, so a
// caller is not held for the minutes the heater takes.
func TestSettingATargetBecomesOneKlipperCommand(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	caps, _ := session.Capabilities()

	cmd, err := caps.Validate(connectorapi.Command{
		Verb: moonraker.VerbNozzleTarget,
		Args: map[string]string{"nozzle_temperature": "215"},
	})
	require.NoError(t, err)

	result, err := session.Do(t.Context(), cmd)
	require.NoError(t, err)
	require.True(t, result.OK)

	require.Equal(t, []string{"M104 S215"}, machine.Scripts(),
		"a target with no head named is the head printing now, which M104 sets")
}

// TestATargetForAHeadSetsThatExtruder maps the tool argument, counted from 0
// as the head ids are, onto Klipper's extruder names, and bounds it by the
// extruders the machine lists.
func TestATargetForAHeadSetsThatExtruder(t *testing.T) {
	t.Parallel()

	status := moonrakersim.DefaultStatus()
	status["extruder1"] = map[string]any{"temperature": 25.0, "target": 0.0}
	status["configfile"]["settings"].(map[string]any)["extruder1"] = map[string]any{
		"min_temp": 0.0, "max_temp": 300.0, "nozzle_diameter": 0.6,
	}

	machine := startMachine(t, moonrakersim.Options{
		Objects: append(moonrakersim.DefaultObjects(), "extruder1"),
		Status:  status,
	})
	session := connect(t, machine)

	caps, _ := session.Capabilities()

	for _, tool := range []string{"0", "1"} {
		cmd, err := caps.Validate(connectorapi.Command{
			Verb: moonraker.VerbNozzleTarget,
			Args: map[string]string{"nozzle_temperature": "215", "tool": tool},
		})
		require.NoError(t, err)

		result, err := session.Do(t.Context(), cmd)
		require.NoError(t, err)
		require.True(t, result.OK)
	}

	require.Equal(t, []string{
		"SET_HEATER_TEMPERATURE HEATER=extruder TARGET=215",
		"SET_HEATER_TEMPERATURE HEATER=extruder1 TARGET=215",
	}, machine.Scripts())

	_, err := caps.Validate(connectorapi.Command{
		Verb: moonraker.VerbNozzleTarget,
		Args: map[string]string{"nozzle_temperature": "215", "tool": "2"},
	})
	require.Error(t, err, "a head the machine does not list was accepted")
}

// TestTelemetryReachesTheStateTree is the reading half. What Klipper pushes
// arrives on the state channel under the paths every other dialect here writes,
// with the two conversions this dialect owes: the state word and the fraction
// Klipper counts progress on.
func TestTelemetryReachesTheStateTree(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	machine.Push(map[string]map[string]any{
		"print_stats":    {"state": "printing", "filename": "benchy.gcode", "print_duration": 64.0},
		"display_status": {"progress": 0.21},
		"extruder":       {"temperature": 215.4, "target": 220.0},
		"toolhead":       {"position": []any{10.5, 20.25, 0.4, 1.0}},
		"gcode_move":     {"speed_factor": 1.0},
	})

	tree := collect(t, session, map[string]string{
		"/state":               `"PRINTING"`,
		"/job/name":            `"benchy.gcode"`,
		"/job/time_printing":   `64`,
		"/job/progress":        `21`,
		"/tools/0/nozzle_temp": `215.4`,
		"/axis/x":              `10.5`,
		"/axis/z":              `0.4`,
		"/speed":               `100`,
	})

	require.Equal(t, `220`, tree["/tools/0/nozzle_target_temp"], "the hotend target never arrived")
}

// TestTheSensorsOfTheMachineAreDescribed reads a machine with a heated
// chamber and an enclosure sensor: each is a reading beside the bed.
func TestTheSensorsOfTheMachineAreDescribed(t *testing.T) {
	t.Parallel()

	status := moonrakersim.DefaultStatus()
	status["heater_generic chamber"] = map[string]any{"temperature": 35.5, "target": 40.0}
	status["temperature_sensor enclosure"] = map[string]any{"temperature": 30.25}

	machine := startMachine(t, moonrakersim.Options{
		Objects: append(moonrakersim.DefaultObjects(), "heater_generic chamber", "temperature_sensor enclosure"),
		Status:  status,
	})
	session := connect(t, machine)

	collect(t, session, map[string]string{
		"/readings/bed/role":             `"bed"`,
		"/readings/chamber/role":         `"chamber"`,
		"/readings/chamber/target_path":  `"/temperature/chamber_target"`,
		"/readings/enclosure/value_path": `"/temperature/enclosure"`,
		"/readings/enclosure/name":       `"enclosure"`,
		"/temperature/chamber":           `35.5`,
		"/temperature/chamber_target":    `40`,
		"/temperature/enclosure":         `30.25`,
	})
}

// TestTheVolumeIsTheTravelOfEachAxis reads the build volume out of the
// stepper sections of the machine's configuration.
func TestTheVolumeIsTheTravelOfEachAxis(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	collect(t, session, map[string]string{
		"/build_volume/x": `300`,
		"/build_volume/y": `305`,
		"/build_volume/z": `280`,
	})
}

// TestEveryExtruderIsATool reads a machine with two extruders: each is a head
// counted from 0, with the nozzle its configuration names, the temperatures
// it reports, and whether it is the active one.
func TestEveryExtruderIsATool(t *testing.T) {
	t.Parallel()

	status := moonrakersim.DefaultStatus()
	status["extruder1"] = map[string]any{"temperature": 180.5, "target": 190.0}
	status["toolhead"]["extruder"] = "extruder1"
	status["configfile"]["settings"].(map[string]any)["extruder1"] = map[string]any{
		"min_temp": 0.0, "max_temp": 300.0, "nozzle_diameter": 0.6,
	}

	machine := startMachine(t, moonrakersim.Options{
		Objects: append(moonrakersim.DefaultObjects(), "extruder1"),
		Status:  status,
	})
	session := connect(t, machine)

	collect(t, session, map[string]string{
		"/tool_count":                       `2`,
		"/tools/0/enabled":                  `true`,
		"/tools/1/type":                     `"FFF_PRINT_HEAD"`,
		"/tools/0/features/nozzle_diameter": `0.4`,
		"/tools/1/features/nozzle_diameter": `0.6`,
		"/tools/1/nozzle_temp":              `180.5`,
		"/tools/1/nozzle_target_temp":       `190`,
		"/tools/0/active":                   `false`,
		"/tools/1/active":                   `true`,
		"/tools/0/nozzle_target_temp":       `0`,
	})

	// A toolchange moves the active member. Head 0's heater is off, which its
	// target of 0 says.
	machine.Push(map[string]map[string]any{"toolhead": {"extruder": "extruder"}})

	tree := collect(t, session, map[string]string{
		"/tools/0/active": `true`,
		"/tools/1/active": `false`,
	})

	for _, flat := range []string{"/active_tool", "/temperature/nozzle", "/temperature/nozzle_target", "/material"} {
		require.NotContains(t, tree, flat, "a path outside the tool tree was written")
	}
}

// TestOneExtruderIsHeadZero holds ADR-20261025-pze: a machine with one extruder
// reports its nozzle under head 0, and nowhere else.
func TestOneExtruderIsHeadZero(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	machine.Push(map[string]map[string]any{"extruder": {"temperature": 201.5, "target": 210.0}})

	tree := collect(t, session, map[string]string{
		"/tools/0/nozzle_temp":        `201.5`,
		"/tools/0/nozzle_target_temp": `210`,
		"/tools/0/active":             `true`,
	})

	require.NotContains(t, tree, "/temperature/nozzle", "the reading was written outside the tree")
	require.NotContains(t, tree, "/temperature/nozzle_target", "the target was written outside the tree")
}

// TestAMachineThatHangsUpClosesTheStateChannel is the signal the host retires a
// printer on and the dial loop redials on. A dialect that leaves the channel
// open holds a printer that is not there.
func TestAMachineThatHangsUpClosesTheStateChannel(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	patches := connect(t, machine).State()

	machine.HangUp()

	deadline := time.NewTimer(settle)
	defer deadline.Stop()

	for {
		select {
		case _, open := <-patches:
			if !open {
				return
			}
		case <-deadline.C:
			t.Fatal("the machine hung up and the state channel stayed open")
		}
	}
}

// TestAnUnreachableAddressIsNotASession is what the dial loop climbs its ladder
// on. Nothing is listening, so Connect answers with an error rather than a
// session nothing is behind.
func TestAnUnreachableAddressIsNotASession(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	address := machine.Address()
	require.NoError(t, machine.Close())

	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()

	driver := moonraker.NewDriver(time.Second)

	session, err := driver.Connect(ctx, connectorapi.Endpoint{Address: address}, driver.Baseline())

	require.Error(t, err, "an address nothing answers on produced a session")
	require.Nil(t, session)
}

// TestAMachineThatAnswersNothingDoesNotHoldTheDialLoop is a design requirement from
// the dialect's side. One endpoint is one goroutine, and a machine that accepts
// a connection and then says nothing would hold that goroutine for the life of
// the process if the call were not bounded.
func TestAMachineThatAnswersNothingDoesNotHoldTheDialLoop(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	driver := moonraker.NewDriver(200 * time.Millisecond)

	began := time.Now()
	session, err := driver.Connect(
		t.Context(), connectorapi.Endpoint{Address: listener.Addr().String()}, driver.Baseline())

	require.Error(t, err, "a machine that said nothing produced a session")
	require.Nil(t, session)
	require.Less(t, time.Since(began), settle, "a silent machine held the dial for longer than the bound")
}

// TestTheOperatorNamesWhatKlipperDoesNot reads the endpoint options. Klipper
// reports no vendor, model or serial, so an operator who wrote them is the only
// source there is and this dialect must not overwrite them.
func TestTheOperatorNamesWhatKlipperDoesNot(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{Hostname: "voron-bench"})

	driver := moonraker.NewDriver(0)

	named, err := driver.Connect(t.Context(), connectorapi.Endpoint{
		Address: machine.Address(),
		Options: map[string]string{"vendor": "Rat Rig", "model": "V-Core 4", "serial": "RR-0007"},
	}, driver.Baseline())
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, named.Close()) })

	require.Equal(t, "Rat Rig", named.Identity().Vendor)
	require.Equal(t, "V-Core 4", named.Identity().Model)
	require.Equal(t, "RR-0007", named.Identity().Serial)

	bare := connect(t, machine)
	require.Equal(t, "Klipper", bare.Identity().Vendor, "a machine that named no maker reports none")
	require.Equal(t, "voron-bench", bare.Identity().Model, "the hostname is what a machine with no model is called")
}

// connect dials one fake machine with the driver's whole baseline, which is
// what the dial loop hands an outbound dialect.
func connect(t *testing.T, machine *moonrakersim.Host) connectorapi.Session {
	t.Helper()

	driver := moonraker.NewDriver(0)

	session, err := driver.Connect(
		t.Context(), connectorapi.Endpoint{Address: machine.Address()}, driver.Baseline())
	if err != nil {
		t.Fatalf("connect to the fake machine: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Errorf("close the session: %v", closeErr)
		}
	})

	return session
}

// collect reads patches until every path wanted has arrived with the value
// wanted, and returns the tree it built. A path that never arrives fails with
// what did, which is the diagnosis somebody reading a failure needs.
func collect(t *testing.T, session connectorapi.Session, wanted map[string]string) map[string]string {
	t.Helper()

	tree := map[string]string{}
	patches := session.State()

	deadline := time.NewTimer(settle)
	defer deadline.Stop()

	for {
		if satisfied(tree, wanted) {
			return tree
		}

		select {
		case patch, open := <-patches:
			if !open {
				t.Fatalf("the state channel closed with %v held and %v wanted", tree, wanted)
			}

			tree[patch.Path] = string(patch.Value)
		case <-deadline.C:
			t.Fatalf("the machine reported %v, and %v was wanted", tree, wanted)
		}
	}
}

func satisfied(tree, wanted map[string]string) bool {
	for path, value := range wanted {
		if tree[path] != value {
			return false
		}
	}

	return true
}

// without is one object list with a section taken out, which is how a case
// builds a machine that is missing something.
func without(objects []string, dropped string) []string {
	out := make([]string, 0, len(objects))

	for _, object := range objects {
		if object != dropped {
			out = append(out, object)
		}
	}

	return out
}

// The session tells the host what the machine has, so the host lists that and
// not this driver's whole baseline.
//
// Narrowing its own session is not enough. The host resolves the set the API
// lists and the set a command is validated against, and without this answer it
// resolves the driver's baseline against a profile and offers a bed verb for a
// machine with no bed.
func TestTheHostIsToldWhatTheMachineHas(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{Objects: without(moonrakersim.DefaultObjects(), "heater_bed")})

	session := connect(t, machine)

	reporter, reports := session.(connectorapi.ReportsProbe)
	require.True(t, reports, "the host has no way to learn what this machine has")

	probe := reporter.Probe()
	require.NotNil(t, probe, "the machine answered for itself and the session reports nothing")

	require.False(t, probe.Has(moonraker.VerbBedTarget), "a machine with no bed reported a bed target")
	require.NotContains(t, probe, moonraker.ArgBedTarget, "the bed target's argument outlived its verb")
	require.True(t, probe.Has(moonraker.VerbNozzleTarget), "the hotend went with the bed")

	// A tier may only subtract. A key the driver's own code does not have
	// cannot be added by the machine saying it has one.
	for key := range probe {
		require.Contains(t, moonraker.NewDriver(0).Baseline(), key,
			"the probe carries a key this driver's baseline does not")
	}
}

// TestTheDialectDeclaresTheKlipperRow is the row the core does not carry
// (ADR-20261016-qrp). A page reads the machine's name and its drawing from what this
// driver declares, so a build that composes it serves them.
func TestTheDialectDeclaresTheKlipperRow(t *testing.T) {
	t.Parallel()

	types, err := connectorapi.NewRegistry()
	require.NoError(t, err)

	registration, err := types.Lookup(moonraker.Type)
	require.NoError(t, err)

	declared := registration.Dialect()
	require.NoError(t, connectorapi.ValidateDialect(declared))
	require.Len(t, declared.Models, 1)

	klipper := declared.Models[0]
	require.Equal(t, "Klipper", klipper.Model)
	require.Equal(t, connectorapi.ArtworkSVG, klipper.Artwork.MediaType)
	require.NotEmpty(t, klipper.Artwork.Bytes, "the drawing was not embedded")
}
