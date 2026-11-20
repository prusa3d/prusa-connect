package xbuddy

import (
	"strconv"
	"sync"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The capability keys this driver can put on the wire. They are the vocabulary
// internal/printer/profile carries, and the wire form beside each one is the
// half that vocabulary does not record.
const (
	VerbSendInfo     = connectorapi.VerbSendInfo
	VerbSendFileInfo = connectorapi.VerbSendFileInfo
	VerbCreateFolder = connectorapi.VerbCreateFolder
	VerbDeleteFile   = connectorapi.VerbDeleteFile
	VerbDeleteFolder = connectorapi.VerbDeleteFolder

	// The transfer verbs are this dialect's own and stay here. The contract
	// above names one generic capability, connectorapi.VerbFileUpload, and this
	// is where it becomes a wire command: nothing above the driver learns that
	// a Prusa board calls it an inline download.
	VerbStartInlineDownload  = connectorapi.Capability("core.command.start_inline_download")
	VerbStartConnectDownload = connectorapi.Capability("core.command.start_connect_download")
	VerbSendTransferInfo     = connectorapi.Capability("core.command.send_transfer_info")
	VerbStopTransfer         = connectorapi.Capability("core.command.stop_transfer")
	VerbSendJobInfo          = connectorapi.Capability("core.command.send_job_info")
	VerbSendStateInfo        = connectorapi.Capability("core.command.send_state_info")
	VerbStartPrint           = connectorapi.Capability("core.command.start_print")
	VerbPausePrint           = connectorapi.Capability("core.command.pause_print")
	VerbResumePrint          = connectorapi.Capability("core.command.resume_print")
	VerbStopPrint            = connectorapi.Capability("core.command.stop_print")
	VerbSetPrinterReady      = connectorapi.VerbSetPrinterReady
	VerbCancelPrinterReady   = connectorapi.Capability("core.command.cancel_printer_ready")
	VerbSetIdle              = connectorapi.Capability("core.command.set_idle")
	VerbResetPrinter         = connectorapi.Capability("core.command.reset_printer")
	VerbSetToken             = connectorapi.Capability("core.command.set_token")
	VerbCancelObject         = connectorapi.Capability("core.command.cancel_object")
	VerbUncancelObject       = connectorapi.Capability("core.command.uncancel_object")
	VerbHome                 = connectorapi.Capability("core.command.home")
	VerbMeshBedLeveling      = connectorapi.Capability("core.command.mesh_bed_leveling")
	VerbReset                = connectorapi.Capability("core.command.reset")
	VerbDisableSteppers      = connectorapi.Capability("core.command.disable_steppers")
	VerbBeep                 = connectorapi.Capability("core.command.beep")
	VerbMove                 = connectorapi.Capability("core.command.move")
	VerbMoveE                = connectorapi.Capability("core.command.move_e")
	VerbMoveZ                = connectorapi.Capability("core.command.move_z")
	VerbSetSpeed             = connectorapi.Capability("core.command.set_speed")
	VerbSetFlow              = connectorapi.Capability("core.command.set_flow")
	VerbSetNozzleTemp        = connectorapi.Capability("core.command.set_nozzle_temperature")
	VerbSetBedTemp           = connectorapi.Capability("core.command.set_bed_temperature")
	VerbLoadFilament         = connectorapi.Capability("core.command.load_filament")
	VerbUnloadFilament       = connectorapi.Capability("core.command.unload_filament")
	VerbZPowerMove           = connectorapi.Capability("core.command.z_power_move")
	VerbSetChamberTemp       = connectorapi.Capability("core.command.set_chamber_target_temp")
	VerbSetChamberFan        = connectorapi.Capability("core.command.set_chamber_fan_target")
	VerbSetChamberLED        = connectorapi.Capability("core.command.set_chamber_led_intensity")
	VerbSetAddonPower        = connectorapi.Capability("core.command.set_addon_power")
	VerbSetToolDiameter      = connectorapi.Capability("core.command.set_tool_nozzle_diameter")
	VerbSetToolHardened      = connectorapi.Capability("core.command.set_tool_hardened")
	VerbSetToolHighFlow      = connectorapi.Capability("core.command.set_tool_high_flow")
	VerbSetEnclosure         = connectorapi.Capability("core.command.set_enclosure_enabled")
	VerbSetEnclosureFilter   = connectorapi.Capability("core.command.set_enclosure_printing_filtration")
	VerbSetEnclosurePost     = connectorapi.Capability("core.command.set_enclosure_postprint")
	VerbSetEnclosurePostFor  = connectorapi.Capability("core.command.set_enclosure_postprint_filtration_time")
	VerbSetHostname          = connectorapi.Capability("core.command.set_hostname")
	VerbDialogAction         = connectorapi.Capability("core.command.dialog_action")
	VerbUpgrade              = connectorapi.Capability("core.command.upgrade")
	VerbFlash                = connectorapi.Capability("core.command.flash")

	// The raw gcode pair (ADR-20260825-z65). They are two keys and not one verb with a
	// forced flag because a profile that withholds the out-of-band frame has to
	// be able to keep the queued one.
	VerbSendGCode  = connectorapi.Capability("core.command.send_gcode")
	VerbForceGCode = connectorapi.Capability("core.command.force_gcode")
)

// setValueCommand is the one J frame twelve of the setters above collapse into.
// Which setter a SET_VALUE frame meant is carried by the argument names alone,
// which is why no two of them may share an argument set and why every one of
// those arguments is required. TestNoTwoSetValueVerbsReachTheWireUnderOneSetOfNames
// and TestEverySetValueArgumentIsRequired hold both halves.
const setValueCommand = "SET_VALUE"

// The bounds legacy/connector/xbuddy/commands/validator.go enforces in Go. They
// are descriptor data here because the host validates every argument before Do
// is reached, so a bound that stays in a driver branch is a bound
// nothing above the driver can see.
const (
	maxTokenLength    = 20
	maxHostnameLength = 35
	maxUint16         = 65535
	// maxHashLength is the base64 of the sixteen bytes a transfer is keyed by,
	// as legacy/connector/xbuddy/commands/validator.go:17 bounds it.
	maxHashLength = 29
	// maxFilamentNameLength is the longest filament name the firmware reads.
	// It keeps a name in eight bytes with the terminating zero, and it reads a
	// longer name as no name.
	maxFilamentNameLength = 7
	// maxUint32 is what orig_size may reach: the transfer wire carries a length
	// and an id as uint32, so 4 GiB is not expressible.
	maxUint32    = 4294967295
	percentLimit = 100
	fanAutomatic = -1
	// firstTool and lastTool bound the tool argument of the nozzle commands.
	// It is a head id as /tools/<id> publishes it, counted from 0 (ADR-20261022-6df
	// rule 8 as amended). The most heads a model has is eight, on an INDX
	// (headCounts in tools.go), so the last head is 7. The shipping validator
	// counted from 1 to 5; limits_test.go records the difference.
	firstTool         = 0
	lastTool          = 7
	minFiltrationTime = 60
	maxFiltrationTime = 5400

	// The bounds above are the shipping validator's. The ones below are this
	// host's own, because the shipping validator holds none on a setpoint: an
	// operator set a nozzle target of 9999 and one of -4000 on a live machine
	// and both were accepted with a 200. They are the driver baseline, so they
	// are the floor a device of an unknown model is held to, and
	// internal/printer/profile narrows them per model where a narrower number
	// is known. Each is deliberately wide: a bound that is too tight refuses a
	// command somebody's machine can carry out, and this tier only has to stop
	// the values no machine can.

	// A hotend setpoint. Zero is "off", which is how a heater is turned off
	// over M104, and 300 C is the highest ceiling any model in the profile
	// file has.
	minNozzleTemp = 0
	maxNozzleTemp = 300
	// A heated bed setpoint over M140. Zero is off; 120 C is the highest bed
	// ceiling in the fleet.
	minBedTemp = 0
	maxBedTemp = 120
	// A chamber setpoint. Zero is off; 80 C is above every enclosed model's
	// ceiling and below the temperature at which the enclosure itself is the
	// thing being cooked.
	minChamberTemp = 0
	maxChamberTemp = 80
	// The speed and flow percentages of M220 and M221. The firmware reads them
	// as a percentage of normal, and both commands take a value from well
	// below normal to ten times it; below 20% the machine stops moving usefully
	// and above 999% the printer clamps anyway.
	minPercentFactor = 20
	maxPercentFactor = 999
	// A feedrate in mm/min. It is a speed and not a displacement, so it has no
	// negative half and a zero would ask for a move that never finishes.
	// maxUint16 is far above any machine's travel speed and is the width the
	// wire field has.
	minFeedrate = 1
	maxFeedrate = maxUint16

	// keysPerVerb is what the baseline map is sized against: a verb and about
	// one argument each.
	keysPerVerb = 2
)

// gcodeTemplate renders the payload of a G or F frame.
type gcodeTemplate func(args arguments) (string, error)

// verb is one capability, its wire form, and what it accepts.
type verb struct {
	// frame is the type letter this verb travels under.
	frame frameKind
	// command is the name a J frame carries, empty for a gcode frame.
	command string
	// gcode renders a gcode payload, nil for a J frame.
	gcode gcodeTemplate
	// dropped names arguments the descriptor declares and the wire form does
	// not carry. They are reported in Result.Ignored rather than discarded
	// silently, because a command carried out without one of its arguments is
	// not the command that was asked for.
	dropped []string
	// args are the arguments this verb accepts, keyed by the name a kwargs
	// object or a gcode template reads them under.
	args map[string]connectorapi.Value
	// toolKey marks a tool setter: the setter travels as the one kwargs key
	// "tools.<k>.<toolKey>" and not as its arguments. It is empty for every
	// other verb. See toolsetter.go.
	toolKey string
	// restarts marks a verb that reboots the printer. The firmware drops the
	// socket before it can answer, so a connection that ends after the frame
	// is on the wire is how a restart that worked looks.
	restarts bool
	// settlesOnWrite marks a verb the firmware carries out without answering.
	// M997 resets the board within milliseconds of reading it and the socket
	// is left half open, so nothing ends the wait before the printer dials
	// again, which a flash takes longer than any caller waits. The frame
	// leaving is the outcome.
	settlesOnWrite bool
	// stateChecked marks a verb the firmware carries out in any state, so this
	// driver checks its stateGuards row before the frame is written. See
	// Session.refuseInState.
	stateChecked bool
}

// wireTable is the whole of what this port can send, but for the one
// alternative chamberOffForm gives.
//
// It is one table rather than a capability list beside a rewrite list, because
// the two disagreeing is the defect earlier research records: 31 of the 57
// verbs never reach the socket under the name the vocabulary gives them, and a
// generator that reads the vocabulary alone produces a driver that sends a
// frame no firmware receives.
//
// The gcode payloads and the SET_VALUE membership are transcribed from
// legacy/connector/xbuddy/commands/builder.go, buildGCodeCommand and
// shouldConvertToSetValue. TestTheRewriteMatchesTheShippingBuilder holds them to
// it.
//
//nolint:gochecknoglobals // the driver's compiled vocabulary; it is read-only after init.
var wireTable = map[connectorapi.Capability]verb{
	// J frames, sent under the name the vocabulary gives them.
	VerbSendInfo:      {frame: frameJSON, command: "SEND_INFO"},
	VerbSendStateInfo: {frame: frameJSON, command: "SEND_STATE_INFO"},
	VerbSendJobInfo: {frame: frameJSON, command: "SEND_JOB_INFO", args: map[string]connectorapi.Value{
		"job_id": boundedInteger(0, maxUint16),
	}},
	VerbStartPrint: {frame: frameJSON, command: "START_PRINT", args: map[string]connectorapi.Value{
		"path":         text(true, 0),
		"tool_mapping": {Flag: true, Kind: connectorapi.KindOpaque},
	}},
	// The file verbs. Each carries the one required path the shipping command
	// table declares, and the path reaches the wire through the same rewrite
	// and the same traversal refusal START_PRINT already gets.
	VerbSendFileInfo: {frame: frameJSON, command: "SEND_FILE_INFO", args: map[string]connectorapi.Value{
		"path": text(true, 0),
	}},
	VerbCreateFolder: {frame: frameJSON, command: "CREATE_FOLDER", args: map[string]connectorapi.Value{
		"path": text(true, 0),
	}},
	VerbDeleteFile: {frame: frameJSON, command: "DELETE_FILE", args: map[string]connectorapi.Value{
		"path": text(true, 0),
	}},
	VerbDeleteFolder: {frame: frameJSON, command: "DELETE_FOLDER", args: map[string]connectorapi.Value{
		"path": text(true, 0),
	}},

	// The transfer verbs. The device pulls: these start the exchange and the
	// bytes leave as T frames when the device asks for them, which is
	// transfer.go. hash and team_id are what the device's block request names
	// the transfer by, so they are a correlation token and not a checksum.
	//
	// START_ENCRYPTED_DOWNLOAD is deliberately absent. It names a second,
	// encrypted channel this host does not serve, and a verb that starts a
	// transfer nothing can feed leaves the device retrying a block request for
	// ever. limits_test.go's notPorted records it with that reason.
	VerbStartInlineDownload: {
		frame: frameJSON, command: "START_INLINE_DOWNLOAD", args: transferArguments(),
	},
	VerbStartConnectDownload: {
		frame: frameJSON, command: "START_CONNECT_DOWNLOAD", args: transferArguments(),
	},
	VerbSendTransferInfo: {frame: frameJSON, command: "SEND_TRANSFER_INFO"},
	VerbStopTransfer:     {frame: frameJSON, command: "STOP_TRANSFER"},
	// The dialog reply. A dialog is raised by the machine and the printer waits
	// on it, so a deployment with no way to send this is a printer somebody has
	// to walk to. The button is unconstrained because the shipping validator
	// constrains it to being a string and names its examples in a comment: a
	// closed set here would be this port inventing one and subtracting whatever
	// the firmware also answers to.
	VerbDialogAction: {frame: frameJSON, command: "DIALOG_ACTION", args: map[string]connectorapi.Value{
		"dialog_id": boundedInteger(0, maxUint32),
		"button":    text(true, 0),
	}},

	// The upgrade pair. Both name a firmware image already on the printer's own
	// storage and neither carries it, so the path travels the same rewrite and
	// the same traversal refusal every other path-bearing verb gets. The shipping
	// validator omits both from its own traversal list, which limits_test records
	// with the value that shows it.
	//
	// Both leave as the G-code M997: the firmware has no FLASH or UPGRADE JSON
	// command, and Prusa Connect sends the two the same way. See flash.go.
	VerbUpgrade: {
		frame: frameGCode, gcode: gcodeFlash, settlesOnWrite: true, stateChecked: true,
		args: map[string]connectorapi.Value{"path": text(false, 0)},
	},
	VerbFlash: {
		frame: frameGCode, gcode: gcodeFlash, settlesOnWrite: true, stateChecked: true,
		args: map[string]connectorapi.Value{"path": text(false, 0)},
	},

	VerbPausePrint:         {frame: frameJSON, command: "PAUSE_PRINT"},
	VerbResumePrint:        {frame: frameJSON, command: "RESUME_PRINT"},
	VerbStopPrint:          {frame: frameJSON, command: "STOP_PRINT"},
	VerbSetPrinterReady:    {frame: frameJSON, command: "SET_PRINTER_READY"},
	VerbCancelPrinterReady: {frame: frameJSON, command: "CANCEL_PRINTER_READY"},
	VerbSetIdle:            {frame: frameJSON, command: "SET_IDLE"},
	VerbResetPrinter:       {frame: frameJSON, command: "RESET_PRINTER", restarts: true},
	VerbSetToken: {frame: frameJSON, command: "SET_TOKEN", args: map[string]connectorapi.Value{
		"token": text(true, maxTokenLength),
	}},
	VerbCancelObject: {frame: frameJSON, command: "CANCEL_OBJECT", args: map[string]connectorapi.Value{
		"id": boundedInteger(0, maxUint16),
	}},
	VerbUncancelObject: {frame: frameJSON, command: "UNCANCEL_OBJECT", args: map[string]connectorapi.Value{
		"id": boundedInteger(0, maxUint16),
	}},

	// G frames. Fifteen verbs the vocabulary names never travel as JSON, and
	// the upgrade pair above is two more.
	VerbHome: {frame: frameGCode, gcode: gcodeHome, args: map[string]connectorapi.Value{
		"axis": {Flag: true, Kind: connectorapi.KindString, Supported: homeAxes},
	}},
	// A full G28 first, the line HOME sends. On a tool changer the firmware
	// picks up a tool only while it homes Z (G28.cpp in Prusa-Firmware-Buddy).
	// G28 O, and the G28 O a bare G29 runs, skips homing on a printer that is
	// homed with every tool docked, and G29 then probes with no tool.
	VerbMeshBedLeveling: {frame: frameGCode, gcode: fixedGCode("G28\r\nG29")},
	VerbReset:           {frame: frameGCode, gcode: fixedGCode("M999 R"), restarts: true},
	VerbDisableSteppers: {frame: frameGCode, gcode: fixedGCode("M84")},
	// The firmware's own single beep (sound.cpp in Prusa-Firmware-Buddy). The
	// piezo cannot play 100 Hz, so a short M300 at that pitch is only a click.
	VerbBeep:           {frame: frameGCode, gcode: fixedGCode("M300 S950 P800")},
	VerbUnloadFilament: {frame: frameGCode, gcode: fixedGCode("M702 W2")},
	VerbLoadFilament: {frame: frameGCode, gcode: gcodeLoadFilament, args: map[string]connectorapi.Value{
		"filament": text(false, maxFilamentNameLength),
	}},
	VerbMove: {frame: frameGCode, gcode: gcodeMove, args: map[string]connectorapi.Value{
		"feedrate": in(mmPerMinute, boundedInteger(minFeedrate, maxFeedrate)),
		"x":        {Flag: true, Kind: connectorapi.KindNumber, Unit: millimeter},
		"y":        {Flag: true, Kind: connectorapi.KindNumber, Unit: millimeter},
	}},
	VerbMoveE: {frame: frameGCode, gcode: gcodeMoveE, args: map[string]connectorapi.Value{
		"feedrate": in(mmPerMinute, boundedInteger(minFeedrate, maxFeedrate)),
		"length":   {Flag: true, Kind: connectorapi.KindNumber, Required: true, Unit: millimeter},
	}},
	VerbMoveZ: {frame: frameGCode, gcode: gcodeMoveZ, args: map[string]connectorapi.Value{
		"feedrate": in(mmPerMinute, boundedInteger(minFeedrate, maxFeedrate)),
		"distance": {Flag: true, Kind: connectorapi.KindNumber, Required: true, Unit: millimeter},
	}},
	VerbZPowerMove: {frame: frameGCode, gcode: gcodeZPowerMove, args: map[string]connectorapi.Value{
		"distance": {Flag: true, Kind: connectorapi.KindNumber, Required: true, Unit: millimeter},
	}},
	VerbSetSpeed: {frame: frameGCode, gcode: gcodeSingle("M220 S%.0f", "speed"),
		args: map[string]connectorapi.Value{
			"speed": in(percent, boundedInteger(minPercentFactor, maxPercentFactor)),
		}},
	VerbSetFlow: {frame: frameGCode, gcode: gcodeSingle("M221 S%.0f", "flow"),
		args: map[string]connectorapi.Value{
			"flow": in(percent, boundedInteger(minPercentFactor, maxPercentFactor)),
		}},
	VerbSetNozzleTemp: {frame: frameGCode, gcode: gcodeNozzleTemp,
		args: map[string]connectorapi.Value{
			"nozzle_temperature": in(celsius, boundedInteger(minNozzleTemp, maxNozzleTemp)),
			"tool":               optional(boundedInteger(firstTool, lastTool)),
		}},
	VerbSetBedTemp: {frame: frameGCode, gcode: gcodeSingle("M140 S%.0f", "bed_temperature"),
		args: map[string]connectorapi.Value{
			"bed_temperature": in(celsius, boundedInteger(minBedTemp, maxBedTemp)),
		}},

	// Raw gcode. The payload carries no Supported set here, so the baseline is
	// unconstrained and a profile narrows it to a closed word list where a
	// deployment wants one (ADR-20260825-z65 rule 3). What no profile can express is in
	// gcodeRaw.
	VerbSendGCode: {frame: frameGCode, gcode: gcodeRaw, args: map[string]connectorapi.Value{
		"gcode": text(true, maxRawGCode),
	}},
	VerbForceGCode: {frame: frameForced, gcode: gcodeRaw, args: map[string]connectorapi.Value{
		"gcode": text(true, maxRawGCode),
	}},

	// SET_VALUE frames. Twelve setters share one command name, and the argument
	// names are the only thing that says which one was meant.
	VerbSetChamberTemp: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"chamber.target_temp": in(celsius, boundedInteger(minChamberTemp, maxChamberTemp)),
	}},
	VerbSetChamberFan: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"chamber.fan_pwm_target": in(percent, boundedInteger(fanAutomatic, percentLimit)),
	}},
	VerbSetChamberLED: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"chamber.led_intensity": in(percent, boundedInteger(0, percentLimit)),
	}},
	VerbSetAddonPower: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"addon_power": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
	}},
	VerbSetToolDiameter: {frame: frameJSON, command: setValueCommand, toolKey: "nozzle_diameter",
		args: map[string]connectorapi.Value{
			"tool":            boundedInteger(firstTool, lastTool),
			"nozzle_diameter": in(millimeter, nonNegative()),
		}},
	VerbSetToolHardened: {frame: frameJSON, command: setValueCommand, toolKey: "hardened",
		args: map[string]connectorapi.Value{
			"tool":     boundedInteger(firstTool, lastTool),
			"hardened": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
		}},
	VerbSetToolHighFlow: {frame: frameJSON, command: setValueCommand, toolKey: "high_flow",
		args: map[string]connectorapi.Value{
			"tool":      boundedInteger(firstTool, lastTool),
			"high_flow": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
		}},
	VerbSetEnclosure: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"enclosure_enabled": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
	}},
	VerbSetEnclosureFilter: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"enclosure_printing_filtration": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
	}},
	VerbSetEnclosurePost: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"enclosure_postprint": {Flag: true, Kind: connectorapi.KindBoolean, Required: true},
	}},
	VerbSetEnclosurePostFor: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"enclosure_postprint_filtration_time": in(second, boundedInteger(minFiltrationTime, maxFiltrationTime)),
	}},
	VerbSetHostname: {frame: frameJSON, command: setValueCommand, args: map[string]connectorapi.Value{
		"hostname": text(true, maxHostnameLength),
	}},
}

// homeAxes is what HOME accepts.
//
// legacy/connector/xbuddy/commands/validator.go checks the argument character by
// character against XYZ in either case, which accepts "ZYX" and "XX" as well.
// A descriptor declares a closed set and cannot declare a character class, so
// this is narrower than the shipping check by exactly the orderings and repeats
// nobody sends.
//
//nolint:gochecknoglobals // descriptor data, read-only after init.
var homeAxes = []string{"X", "Y", "Z", "XY", "XZ", "YZ", "XYZ", "x", "y", "z", "xy", "xz", "yz", "xyz"}

// sentNotInvokable are the wire forms this driver sends for itself and keeps out
// of the published baseline, with the reason for each.
//
// A published key is invokable through POST /api/v1/devices/{device_id}/commands
// (ADR-20260826-ezr rules 3 and 7), and the exchange these two start cannot be fed from
// there: the block requests the device answers with are matched against the
// transfer Send is holding, so a caller-started download leaves a device asking
// for ranges nothing can serve. That is the same reason
// START_ENCRYPTED_DOWNLOAD has no wire form at all. STOP_TRANSFER and
// SEND_TRANSFER_INFO stay invokable: both are answerable on their own, and
// stopping a transfer is a thing an operator at a machine genuinely wants.
//
//nolint:gochecknoglobals // descriptor data, read-only after init.
var sentNotInvokable = map[connectorapi.Capability]string{
	VerbStartInlineDownload: "the start of the transfer exchange in transfer.go. A caller reaches it " +
		"through Transfers.Send, which is what holds the file the device then asks for",
	VerbStartConnectDownload: "the same exchange under this dialect's other name for it, and the same " +
		"reason: nothing feeds a transfer no Send is holding",
}

// baseline is the Set the wire table describes: everything this driver's code can
// put on the wire, whether a caller can ask for it or not. It is built once: a
// driver's Baseline is pure and is called on every arrival.
//
//nolint:gochecknoglobals // one immutable value, built once.
var baseline = sync.OnceValue(func() connectorapi.Set {
	set := make(connectorapi.Set, len(wireTable)*keysPerVerb)

	// The generic file capabilities this driver serves. They have no wireTable
	// row because none of them is one frame: the upload is the whole exchange in
	// transfer.go. A host asks the resolved set whether a printer can be sent a
	// file, and this is what answers.
	set[connectorapi.VerbFileUpload] = connectorapi.Value{Flag: true}

	for key, declared := range wireTable {
		set[key] = connectorapi.Value{Flag: true}

		for name, value := range declared.args {
			set[connectorapi.ArgumentKey(key, name)] = value
		}
	}

	return set
})

// published is what this driver declares to a host: the baseline less the verbs
// only the driver itself sends. It is what a profile is resolved against and
// what a caller's command is validated against, so a key that is not here cannot
// be asked for from outside.
//
//nolint:gochecknoglobals // one immutable value, built once.
var published = sync.OnceValue(func() connectorapi.Set {
	set := baseline().Clone()

	for key := range sentNotInvokable {
		delete(set, key)

		for name := range wireTable[key].args {
			delete(set, connectorapi.ArgumentKey(key, name))
		}
	}

	return set
})

// boundedInteger returns a required whole-number argument with both bounds,
// which is the shape every range the shipping validator enforces takes: Min is
// the value's own lower bound and Limit is the bound on its magnitude. Every
// bounded argument in this dialect is required, so there is nothing to say.
func boundedInteger(lowest, highest float64) connectorapi.Value {
	return connectorapi.Value{
		Flag: true, Kind: connectorapi.KindInteger, Required: true,
		Min: lowest, HasMin: true, Limit: highest, HasLimit: true,
	}
}

// The units this dialect's arguments count in, under short names so a row of
// the wire table stays on one line.
const (
	celsius     = connectorapi.UnitCelsius
	percent     = connectorapi.UnitPercent
	millimeter  = connectorapi.UnitMillimeter
	mmPerMinute = connectorapi.UnitMillimeterPerMinute
	second      = connectorapi.UnitSecond
)

// in is value counted in unit.
func in(unit connectorapi.Unit, value connectorapi.Value) connectorapi.Value {
	value.Unit = unit

	return value
}

// nonNegative is a required fractional number that may not go below zero.
func nonNegative() connectorapi.Value {
	return connectorapi.Value{Flag: true, Kind: connectorapi.KindNumber, Required: true, Min: 0, HasMin: true}
}

// optional is value with nothing required of the caller.
func optional(value connectorapi.Value) connectorapi.Value {
	value.Required = false

	return value
}

// transferArguments is the argument set the two download verbs share. They are
// the same four arguments in the shipping command table, which is what makes one
// the other's alias.
func transferArguments() map[string]connectorapi.Value {
	return map[string]connectorapi.Value{
		"path":      text(true, 0),
		"hash":      text(true, maxHashLength),
		"team_id":   {Flag: true, Kind: connectorapi.KindInteger, Required: true, Min: 0, HasMin: true},
		"orig_size": in(connectorapi.UnitByte, boundedInteger(0, maxUint32)),
	}
}

// text returns a string argument, bounded in bytes when limit is not zero.
func text(required bool, limit float64) connectorapi.Value {
	return connectorapi.Value{
		Flag: true, Kind: connectorapi.KindString, Required: required,
		Limit: limit, HasLimit: limit > 0,
	}
}

// chamberOffForm is the G-code form a chamber target of 0 can take, and
// whether this command is one. A CORE One on 8.0.0, fans on auto, ignored a
// SET_VALUE of 0 that M141 S0 as G-code obeyed. Its source treats 0 like any
// other target, so this is an observation on one printer, not a firmware rule.
//
// A G frame holds the queue, and the firmware refuses every command, Stop
// included, until the line has run. So the session sends this form only when
// the printer is quiescent. See Session.quiescent. It carries only the table's
// arguments, so it must need none of the table's flags.
func chamberOffForm(capability connectorapi.Capability, form verb, values map[string]string) (verb, bool) {
	if capability != VerbSetChamberTemp {
		return verb{}, false
	}

	target, err := strconv.ParseFloat(values["chamber.target_temp"], 64)

	return verb{frame: frameGCode, gcode: fixedGCode("M141 S0"), args: form.args}, err == nil && target == 0
}
