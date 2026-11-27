package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// The device paths. A device is addressed by the id the device plane minted for
// it, which is also the id the authorization graph and the event stream name it
// by.
const (
	// paragraphBreak separates two paragraphs of an operation description. The
	// descriptions are concatenated string constants, and a literal newline
	// inside one of those is a compile error.
	paragraphBreak = "\n\n"

	devicesPath        = "/api/v1/devices"
	devicePath         = "/api/v1/devices/{device_id}"
	deviceCommandsPath = "/api/v1/devices/{device_id}/commands"
	deviceLabelsPath   = "/api/v1/devices/{device_id}/labels"
	deviceTagsPath     = "/api/v1/devices/{device_id}/tags"
	deviceNamePath     = "/api/v1/devices/{device_id}/name"
	deviceJobsPath     = "/api/v1/devices/{device_id}/jobs"
	deviceJobHistory   = "/api/v1/devices/{device_id}/jobs/history"
)

// DeviceFleet is the connected printers as this surface reads and commands
// them. It is declared here because this is where it is consumed, and the
// device plane in internal/app satisfies it.
//
// Every method takes the principal the middleware resolved rather than an
// already-authorized caller. Which printers a principal may see is a question
// for the authorization graph, and it is answered inside the query that
// produces the rows: a listing that fetched and then filtered would leak
// through its own count.
type DeviceFleet interface {
	// List returns one page of the printers the caller may view telemetry on,
	// and how many there are over every page. A printer whose session has ended
	// is still on it, carrying the last state it reported and why it went.
	//
	// Every row carries the issues the host has found on that printer, and the
	// page carries the rollup over the caller's whole authorized population.
	// Both are read from what this process already holds, so a page that says
	// what is wrong with a fleet costs the statements the listing already cost
	// (ADR-20260827-xca rule 4).
	List(ctx context.Context, principal string, query DeviceQuery) (DevicePage, error)

	// Get returns one printer. A printer the caller may not see and one this
	// server has never been dialled by are the same refusal.
	Get(ctx context.Context, principal string, id string) (Device, error)

	// Capabilities returns the capability set one printer resolved to, with
	// every argument's descriptor. It takes the action Get takes and answers
	// the same refusal, because it is the same reading in more detail.
	Capabilities(ctx context.Context, principal string, id string) (DeviceCapabilitySet, error)

	// Jobs returns the print on one printer and the one before it. It takes the
	// action Get takes and answers the same refusal, because a job is a reading
	// about a machine rather than something a caller does to one.
	//
	// It reads the record store and not the session, so a printer whose session
	// has ended still answers: what it was printing is exactly the thing a
	// person wants to know about a machine that went away.
	Jobs(ctx context.Context, principal string, id string) (DeviceJobs, error)

	// Job returns one print by its id. It takes observe on the print's device,
	// and a print on a device the caller cannot observe is refused exactly as
	// an id nobody minted is.
	Job(ctx context.Context, principal string, jobID string) (DeviceJob, error)

	// Command sends one command and returns what the printer made of it. The
	// command is validated against the printer's resolved capability set before
	// the driver is reached, so a refusal here names the verb or
	// the argument rather than reporting that the printer said no.
	//
	// A printer whose session has ended is refused rather than queued. The
	// queue a command waits in belongs to the session, and that session is gone.
	Command(ctx context.Context, principal string, id string, cmd PrinterCommand) (PrinterResult, error)

	// Storages returns the storage devices the printer reported. It takes the
	// same action a telemetry read takes, because it reads what the printer
	// already said and puts nothing on the wire. refresh asks the printer
	// first and takes the action a command takes. The session is what holds
	// that answer, so a printer whose session has ended is refused.
	Storages(ctx context.Context, principal string, id string, refresh bool) (DeviceStorages, error)

	// Files returns one directory of the printer's own file tree. It takes the
	// action a command takes: answering it sends a frame to the machine, so a
	// caller who may only read telemetry cannot make this server talk to a
	// printer on their behalf. A printer whose session has ended is refused,
	// because there is no machine on the other end to ask. A path the printer
	// already listed is answered from memory and marked stale unless refresh
	// is set.
	Files(ctx context.Context, principal string, id string, path string, refresh bool) (DeviceListing, error)

	// Thumbnail returns the picture the printer reads out of one print file on
	// its own storage. It takes what Files takes, because it asks the printer.
	// size and modified are the version the caller's listing reported, and a
	// picture held for that version answers without asking again. A file with
	// no picture reports StorageNoThumbnail.
	Thumbnail(
		ctx context.Context, principal string, id string, path string, size int64, modified int64,
	) (StorageThumbnail, error)

	// Send puts a file one of this deployment's storage resources holds onto
	// the printer's own storage. It takes the action a command takes for a
	// stronger reason than the listing does: it writes to the machine.
	//
	// It returns when the printer reports the file arrived, so ctx bounds the
	// whole transfer, and it answers with the transfer as it ended.
	Send(ctx context.Context, principal string, id string, upload DeviceUpload) (DeviceTransfer, error)

	// Transfers is every file this server has sent this printer and is sending
	// it, newest first. It takes the action a telemetry read takes: it reports
	// what this server did and puts nothing on the wire.
	Transfers(ctx context.Context, principal string, id string) ([]DeviceTransfer, error)

	// RunningTransfers is every transfer still running on any printer the
	// caller may read, newest first. A printer the caller cannot read adds
	// nothing, and the answer carries no count that could say it was there.
	RunningTransfers(ctx context.Context, principal string) ([]RunningTransfer, error)

	// StopTransfer ends a transfer that is running. It takes the action a
	// command takes, because it changes what the machine is doing, and a
	// transfer that has already ended is reported as not found.
	StopTransfer(ctx context.Context, principal string, id string, transfer string) error

	// CreateFolder, DeleteFile and DeleteFolder change what is stored on the
	// printer. Each takes the action a command takes, because each writes to the
	// machine, and each is gated on its own capability: a printer that can make
	// a folder and not remove one is a real answer.
	CreateFolder(ctx context.Context, principal string, id string, path string) error
	DeleteFile(ctx context.Context, principal string, id string, path string) error
	DeleteFolder(ctx context.Context, principal string, id string, path string) error

	// SetTags replaces the whole tag list on one device and returns what it
	// now carries. It replaces rather than merges, so a client takes a tag off
	// by leaving it out.
	//
	// It takes administer on the device. A tag is an administrative fact about
	// a machine rather than something the machine did.
	SetTags(ctx context.Context, principal string, id string, tags []string) ([]string, error)

	// SetLabels writes the tags a set of key and value labels spells, and
	// returns the labels the device then carries. It takes what SetTags takes.
	SetLabels(ctx context.Context, principal string, id string, labels map[string]string) (map[string]string, error)

	// SetName writes what an operator calls one device and returns what it now
	// carries. An empty name takes the operator's name off, and the answer is
	// then the name this server composes from what the machine reported.
	//
	// It takes administer on the device, which is what SetTags takes: naming a
	// machine is an administrative fact about it rather than something the
	// machine did.
	SetName(ctx context.Context, principal string, id string, name string) (string, error)

	// Retire marks one device id as no longer naming a machine. The record, the
	// prints, the command log and every grant written on the id stay, and the
	// cameras that watch it are removed. The connector takes back the
	// credential it issued the machine, so the machine has to be added again,
	// and a machine admitted again is given a new id rather than reviving this
	// record. It takes administer on the device.
	Retire(ctx context.Context, principal string, id string) error

	// JobHistory returns one page of the prints that have ended on a device,
	// newest first, by cursor. It takes the action Jobs takes, because a print
	// is a reading about a machine.
	JobHistory(ctx context.Context, principal string, id string, query DeviceJobQuery) (DeviceJobPage, error)

	// ListJobs returns one page of the prints on every device the caller may
	// observe, running and ended, newest first, by cursor. The permission is
	// resolved inside the query, so a device the caller reaches nothing on adds
	// no row and a named one answers an empty page.
	ListJobs(ctx context.Context, principal string, query JobQuery) (JobPage, error)

	// Print relays a file from one of this deployment's storages onto a printer
	// and starts it, and prints a file the printer already holds when the
	// request names no storage. It takes operate on the printer and
	// read_storage on the storage kind, and it returns when the printer has
	// answered the start command, so ctx bounds the transfer and the command
	// together.
	Print(ctx context.Context, principal string, id string, request DevicePrint) (DevicePrintResult, error)

	// StartPrint is Print, answering as soon as the transfer has opened. The
	// host goes on relaying the file and sends the start command itself when
	// the last byte has arrived.
	//
	// It is what the module wire calls. A module cannot hold a call open for
	// the minutes a large file takes, and a queue that starts prints overnight
	// would otherwise spend a call slot per printer on the transfer.
	StartPrint(ctx context.Context, principal string, id string, request DevicePrint) (DevicePrintResult, error)
}

// Device is one printer on the wire: what it said it is, what this server
// resolved it can do, and the state it last reported.
type Device struct {
	// Kind is what type of device this is, from the closed set the core owns.
	// It is `printer` on every record this build serves, and it is published so
	// that a client reading a fleet from a federated peer running a newer core
	// can tell one device type from another without a second request
	// (ADR-20260826-5yj rules 8, 9 and 13).
	//
	// It carries no enum. A peer may name a kind this build does not carry, and
	// a client that meets one shows it rather than refusing the record.
	Kind string `json:"kind" doc:"What type of device this is. It is printer on every record this server produces. A federated peer running a newer core may name a kind this build does not carry, so read it as an open value."`

	ID       string `json:"id"       doc:"The printer's identity, stable across reconnections and never reissued."`
	Name     string `json:"name"     doc:"The display name, which is the model and the serial until somebody renames it."`
	Vendor   string `json:"vendor"   doc:"Who made it."`
	Model    string `json:"model"    doc:"The model it reported in the handshake."`
	Firmware string `json:"firmware" doc:"The firmware version it reported. Read it, do not order by it: a firmware lineup that forks is not a totally ordered set."`
	Serial   string `json:"serial"   doc:"The serial number it reported. A printer that reported none is rendered under its unit_uid instead, because a blank field names no machine and the unit id is what such a printer is told apart by. Read unit_uid beside it to see which of the two this is."`
	Driver   string `json:"driver"   doc:"The connector instance it arrived through."`

	// UnitUID is what the printer calls itself on its connector's own wire, and
	// it is a different identifier from both id and serial.
	//
	// id is this server's, minted here and meaningless to anything else. serial
	// is the number on the sticker, which Buddy 6.5.7 and 6.8.1 do not send at
	// all. This is the one a printer volunteers on every connection, so it is
	// what a person matches a row against a machine by when there is no serial,
	// and what a fleet spanning two servers joins rows on.
	UnitUID string `json:"unit_uid" doc:"What the printer calls itself on its connector's wire, empty for a connector whose devices have no such identifier. It is not this server's id, which is minted here, and it is not the serial number, which some firmware does not send. A printer that reported no serial is rendered with this value as its serial as well."`

	// Connected is false for a printer that has been seen and is away. Its
	// state is the last it reported.
	Connected bool `json:"connected" doc:"Whether a session is open right now."`

	// Departed is set once the printer's session has ended, and absent while it
	// is held. It is what separates a printer that was here from an id nothing
	// ever answered to: the fields above are the last this server knew, and the
	// operations that have to reach the machine are refused until it dials in
	// again.
	Departed *DeviceDeparture `json:"departed,omitempty" doc:"Present once the printer's session has ended. The rest of this record is then the last reading this server took."`

	// Resident is set for a printer the server runs on. Such a printer cannot
	// be retired (ADR-20261103-szc).
	Resident bool `json:"resident,omitempty" doc:"True for a printer that runs this server. Retiring it is refused with DEVICE_RESIDENT, because it would connect again at once under a new id. Absent for every other printer."`

	// Retired is set once this id has stopped being its machine's, and absent
	// while it still is. It is a different fact from Departed: a departed
	// device is one whose session ended and whose next session revives it, and
	// a retired one is a record of a machine that is now a different device.
	Retired *DeviceRetirement `json:"retired,omitempty" doc:"Present once this id has stopped naming a machine, which happens when the printer behind it was registered again and this server minted a second id for it. The record stays whole: its history, its commands and the grants on it are still readable under this id, and none of them belong to the machine's new id. It is not a departure, and a retired record never comes back."`

	// Capabilities is the resolved, enabled set: the driver's baseline narrowed
	// by the profile and by the printer's own probe, never widened (decision
	// C1).
	Capabilities []string `json:"capabilities" doc:"The verbs this printer accepts, sorted."`

	// CapabilityEpoch moves on every arrival, so a client can tell a set it
	// cached from the one in force.
	CapabilityEpoch uint64 `json:"capability_epoch" doc:"Changes whenever the printer reconnects. A capability set cached under an older epoch describes a printer that is no longer there."`

	// Actions is what this caller may do to this printer, decoded from the
	// permission row the listing was already filtered by. It costs no second
	// query and it cannot disagree with the filter that admitted the row
	// (ADR-20260827-wgm rule 9), cut down to what the credential's scopes
	// allow (ADR-20261025-tpp rule 5).
	//
	// Capabilities is what the machine accepts and this is what the caller
	// may do. A verb in Capabilities and an action absent here is a control to
	// grey out; the reverse is a permission this printer has no way to use.
	Actions []string `json:"actions" doc:"What this caller may do to this printer, in the order the authorization vocabulary declares. It is narrowed by the credential's scope, where capabilities is what the machine accepts."`

	// Verbs is the same answer per verb, which Actions cannot give for the
	// dangerous verbs: no class reaches one, so a caller holding every class
	// may still be refused a flash. It is the list a grant listing renders
	// for one subject (SubjectHold.Verbs), read for the caller.
	Verbs []string `json:"verbs" doc:"Every verb of this printer's kind this caller may send, sorted. The credential's scope narrows it as it narrows actions. A dangerous verb such as core.command.flash is here only when a grant or ownership of the printer gives it to the caller. A verb in capabilities and missing here is a command the server refuses this caller."`

	// AuthorizationEpoch moves on every grant write, so a client can tell an
	// action set it cached from the one in force. It is CapabilityEpoch's
	// counterpart for the other half of what decides whether a control renders.
	AuthorizationEpoch uint64 `json:"authorization_epoch" doc:"Changes whenever any grant is written. An action set cached under an older epoch describes permissions that have since changed."`

	// ProfileMatched is whether a capability profile covered the reported
	// identity. False is a reduced set rather than a refusal.
	ProfileMatched bool `json:"profile_matched" doc:"Whether a capability profile covered this model and firmware. False means the driver baseline is in force."`

	State DeviceState `json:"state" doc:"The addressable state tree, keyed by path."`

	// Job is the print the machine last reported, read out of the state tree
	// above rather than from the record store. It is here so that a fleet page
	// says what every machine is printing without a request per row, and it
	// carries no start and no end for the reason DeviceJobSummary gives.
	Job *DeviceJobSummary `json:"job,omitempty" doc:"The print this machine last reported, absent on one that has never printed. It is the reading the state tree carries; read GET /api/v1/devices/{device_id}/jobs for the row, with the start and the end."`

	// Tools is the printer's tool tree, read out of the state tree above at
	// the paths sdk/connectorapi/tools.go names (ADR-20261023-gna). It is a reading of
	// the tree and not a second copy of it, so a printer that is away answers
	// with what it said last.
	Tools *DeviceTools `json:"tools,omitempty" doc:"The printer's tool tree: its heads and the feeder slots on them, keyed by ids such as 0 and 0.2. It is read out of the state tree, so a printer that is away answers with what it said last. Absent on a printer that has said nothing about its tools."`

	// BuildVolume is the space a print may fill, as the driver reports it for
	// the printer's model. The host keeps no table of its own.
	BuildVolume *DeviceBuildVolume `json:"build_volume,omitempty" doc:"The space a print may fill on this printer, as its driver reports it. Absent when the driver has no figures for the model."`

	// Link is what the connection has cost, which is the one thing about a
	// connected printer the state tree cannot say.
	Link DeviceLink `json:"link" doc:"What this server has made of the frames the printer sent."`

	// Tags are the plain words an operator put on this device, in the order the
	// operator gave them. Nothing in the authorization graph reads one, so a
	// tag narrows a listing a caller was already entitled to and never widens
	// one.
	//
	// This server always sends the member. The schema marks it optional so a
	// client that builds device records of its own keeps compiling.
	//
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Tags []string `json:"tags" required:"false" doc:"The operator's tags on this device, such as workshop or night shift, in the order the operator gave them. It is empty on a device nobody has tagged. Nothing in the authorization graph reads a tag, so a tag filters a listing and never decides who may see what."`

	// Labels is the same tags read as key and value labels, for a client that
	// has not moved to tags. It is removed in a later release.
	//
	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Labels map[string]string `json:"labels" doc:"Deprecated. The tags read as labels: a tag key: value is the label key with that value, and any other tag is a label with an empty value. When two tags have the same key, only the later one is shown. Read tags instead. This member is removed in a later release."`

	// Issues is what the host has found wrong with this printer, worst first
	// and empty when there is nothing. It is an index into the rest of this
	// record rather than a second copy of it: a client reads the reason, then
	// reads departed, link or state for the particulars (ADR-20260827-xca rule 3).
	//
	// The reasons are a closed vocabulary served at
	// /api/v1/vocabularies/device-issues, so a client branches on the token and
	// a translator translates the sentence there.
	Issues []DeviceIssue `json:"issues" doc:"What the host has found wrong with this printer, worst first. Empty when there is nothing."`
}

// DeviceTools is the tool tree of one printer, in the shape of the public
// Prusa architecture's printer configuration. A member the printer did not
// report is left out, and never sent as a zero.
type DeviceTools struct {
	ToolCount    *int                   `json:"tool_count,omitempty"     doc:"How many heads the printer has room for, enabled or not. Absent when the printer did not say."`
	Tools        map[string]*DeviceTool `json:"tools"                    doc:"Every tool the printer reported, by id. A head is 0, 1 and on, and slot k of the feeder on tool X is X.k. A tool the printer has switched off is null."`
	GCodeToolMap map[string]string      `json:"gcode_tool_map,omitempty" doc:"The tool id each G-code tool number Tn selects, when the printer reports it. A G-code tool number it leaves out takes the default: Tn is slot n of the one head when the printer has one head and that head carries a feeder, and head n on any other printer, including one with a feeder on more than one head. Prusa Connect reports it, and the architecture has no such member."`
}

// DeviceTool is one tool: a head, or a slot of a feeder. Its members carry
// the names and the nesting of the public Prusa architecture's tool
// configuration and telemetry, so a client that reads the architecture reads
// this. A member the printer did not report is left out, and never sent as a
// zero.
type DeviceTool struct {
	Type     string              `json:"type,omitempty"     doc:"FFF_PRINT_HEAD or FFF_FEEDER_SLOT."`
	Active   bool                `json:"active"             doc:"Whether the tool is selected, as the architecture's telemetry marks it. A head is active while it is picked, and the one head of a printer that is not a toolchanger always is. A slot is active while it is loaded into its head, even when that head is not picked."`
	Features *DeviceToolFeatures `json:"features,omitempty" doc:"What the nozzle of a head is. A slot prints through the nozzle of the head above it."`
	// Feeder and MaterialPackageInstance each have three readings, which is
	// why they are Nullable and not pointers: left out, null and a value.
	Feeder                  Nullable[DeviceFeeder]                  `json:"feeder,omitzero"                    doc:"The feeder on the tool, such as a multi-material unit. Null is a tool known to have no feeder, and an absent member is a printer that did not say."`
	MaterialPackageInstance Nullable[DeviceMaterialPackageInstance] `json:"material_package_instance,omitzero" doc:"The material loaded in the tool. Null is a tool with nothing loaded, and an absent member is a printer that did not say. A tool with a feeder carries none; its slots do."`
	NozzleTemp              *float64                                `json:"nozzle_temp,omitempty"              doc:"The nozzle temperature of a head, in degrees Celsius."`
	NozzleTargetTemp        Nullable[float64]                       `json:"nozzle_target_temp,omitzero"        doc:"The temperature a head is heating to, in degrees Celsius. Null while the heater is off, and absent when the printer did not say."`
	PrintFanRPM             *float64                                `json:"print_fan_rpm,omitempty"            doc:"The speed of a head's print fan, in revolutions per minute."`
	HeatbreakFanRPM         *float64                                `json:"heatbreak_fan_rpm,omitempty"        doc:"The speed of a head's heatbreak fan, in revolutions per minute."`
	ConsumedWeight          *float64                                `json:"consumed_weight,omitempty"          doc:"How much of the material loaded in the tool is used, in grams."`
	OdometerID              *int64                                  `json:"odometer_id,omitempty"              doc:"A number that changes, without counting up, each time the odometers of the tool are reset. A reset can come at a restart, at the end of a print, at a filament change and at other times. A client that sees it change counts again from zero."`
	OdometerE               *float64                                `json:"odometer_e,omitempty"               doc:"How much filament the tool has extruded since its odometers were last reset, in millimetres."`
	OdometerWeight          *float64                                `json:"odometer_weight,omitempty"          doc:"The weight of that filament, in grams. Absent when the printer cannot turn length into weight."`
}

// DeviceToolFeatures is the nozzle of one head.
type DeviceToolFeatures struct {
	NozzleDiameterMM *float64 `json:"nozzle_diameter,omitempty"  doc:"The nozzle diameter, in millimetres."`
	NozzleHighFlow   *bool    `json:"nozzle_high_flow,omitempty" doc:"Whether the nozzle is a high-flow one."`
	NozzleHardened   *bool    `json:"nozzle_hardened,omitempty"  doc:"Whether the nozzle is hardened against abrasive filament."`
	NozzleHighTemp   *bool    `json:"nozzle_high_temp,omitempty" doc:"Whether the nozzle is made for high temperatures."`
}

// DeviceFeeder is the feeder on one tool, such as a multi-material unit.
type DeviceFeeder struct {
	SlotCount            int    `json:"slot_count"                        doc:"How many slots the feeder has, enabled or not. Its slots are the child ids 0 to slot_count - 1."`
	Model                string `json:"model,omitempty"                   doc:"The feeder's model, such as MMU3."`
	BaseModel            string `json:"base_model,omitempty"              doc:"The family of the feeder's model, such as MMU3."`
	Type                 string `json:"type,omitempty"                    doc:"The kind of feeder. The architecture allows only MMU now."`
	MMUProgressCode      *int64 `json:"mmu_progress_code,omitempty"       doc:"The progress code the multi-material unit reports: the step of its work it is at, 0 when it is done. The codes are the multi-material unit firmware's."`
	MMUCommandInProgress string `json:"mmu_command_in_progress,omitempty" doc:"The command the multi-material unit runs now, as one character, such as L for a load, U for an unload and T for a toolchange. Absent while it runs none."`
}

// DeviceMaterialPackageInstance is the material loaded in one tool, nested as
// the architecture nests it: the instance of a package, the package's
// material, and its abbreviation. The printer reports only the abbreviation.
type DeviceMaterialPackageInstance struct {
	Package DeviceMaterialPackage `json:"package" doc:"The package the loaded material came in."`
}

// DeviceMaterialPackage is the package of a material package instance.
type DeviceMaterialPackage struct {
	Material DeviceMaterial `json:"material" doc:"The material in the package."`
}

// DeviceMaterial is the material of a package.
type DeviceMaterial struct {
	Abbreviation string `json:"abbreviation" doc:"The short name of the material, such as PLA."`
}

// DeviceBuildVolume is the space a print may fill on one printer.
type DeviceBuildVolume struct {
	XMM float64 `json:"x_mm" doc:"The width of the area a print may cover, in millimetres."`
	YMM float64 `json:"y_mm" doc:"The depth of the area a print may cover, in millimetres."`
	ZMM float64 `json:"z_mm" doc:"The highest a print may reach, in millimetres."`
}

// DeviceIssue is one thing the host has found wrong with a printer.
//
// It carries no sentence. The sentence, the longer description and what a
// person does about it belong to the vocabulary at
// /api/v1/vocabularies/device-issues, which a client reads once and caches: an
// English sentence repeated on four hundred records is bytes nobody can
// translate (ADR-20260825-bhe rule 10, ADR-20260827-xca rule 5).
type DeviceIssue struct {
	Reason string `json:"reason" doc:"The closed token to branch on. Read the device-issues vocabulary for what it means and what to do about it."`

	// Severity is a property of the reason and never of one printer, so it is
	// the same on every record carrying that reason. It is repeated here so a
	// client can colour a badge without having fetched the vocabulary yet.
	Severity string `json:"severity" enum:"info,warning,error" doc:"How loudly to report it. It is fixed by the reason, and it is repeated here so a badge renders before the vocabulary is loaded."`

	// Detail is what the machine itself said, passed through as it spelled it.
	// It is the printer's vocabulary rather than this API's, so it is shown to
	// a person and never branched on, exactly as machine_reason is.
	Detail string `json:"detail,omitempty" doc:"What the machine itself said, in its own words, when the reason has such a word. Show it to a person rather than branching on it."`
}

// DeviceIssueSummary is what is wrong across every non-retired printer this
// caller may see, whatever page was asked for and whatever filter was applied.
//
// It is over the whole authorized population on purpose. A badge that counted
// only the current page would change when somebody turned to page two, and a
// badge that counted only the filtered rows would read zero exactly when the
// filter was doing its job. The count discloses nothing the listing does not:
// it is computed over the rows the same authorized walk produced.
type DeviceIssueSummary struct {
	// Affected is printers, not issues. One printer carrying three reasons
	// counts once here and three times across Counts.
	Affected int `json:"affected" doc:"How many non-retired printers this caller may see carry at least one issue, over every page and whatever filter was applied."`

	// Counts holds a key for every reason this build knows, including the ones
	// at zero, so a client reading a key it expects does not have to tell zero
	// from absent.
	// The keys are the device-issue vocabulary's, and nothing here enumerates
	// them: a federated peer running a newer core may count a reason this build
	// has never heard of, and a client that meets one shows it rather than
	// refusing the page.
	Counts map[string]int `json:"counts" doc:"How many printers carry each reason, keyed by the reason token. Every reason this build knows is a key, including the ones at zero."`

	// BySeverity is the strip's chips: a printer carrying an error and a
	// warning counts under both.
	BySeverity map[string]int `json:"by_severity" doc:"How many printers carry an issue of each severity, over every page and whatever filter was applied. error, warning and info are always keys, including at zero. A printer carrying issues of two severities counts under both."`
}

// DeviceFilterValue is one value a listing may be narrowed by.
type DeviceFilterValue struct {
	Value string `json:"value" doc:"One value the caller may filter by, spelled as the first device carrying it spells it."`
	Count int    `json:"count" doc:"How many printers choosing this value would list, with every other filter applied and this filter's own selection ignored. For tag the selection stays applied, because tags narrow by AND: the printers carrying every chosen tag and this one."`
}

// DeviceFilter is one way the listing may be narrowed, as this caller's fleet
// offers it.
type DeviceFilter struct {
	Param     string              `json:"param"                enum:"search,state,model,severity,tag" doc:"The query parameter this filter is sent as."`
	Kind      string              `json:"kind"                 enum:"multi,text"                      doc:"multi takes a comma-separated list of values; text takes one string."`
	Values    []DeviceFilterValue `json:"values,omitempty"                                            doc:"For multi: the values present among the printers this caller may see, and every value the request selected."`
	MaxLength int                 `json:"max_length,omitempty"                                        doc:"For text: the longest value the server accepts."`
}

// DeviceSort is one field the listing may be ordered by.
type DeviceSort struct {
	Field        string `json:"field"         enum:"urgency,name,model,state,last_seen,id" doc:"The value the listing's sort parameter takes to order by this field."`
	DefaultOrder string `json:"default_order" enum:"asc,desc"                              doc:"The direction a person reading this field expects first."`
}

// DeviceLink is how well this server and one printer are understanding each
// other: telemetry the host did not read in time, and frames the driver had no
// reading of.
//
// It is here because a printer running firmware this build does not fully
// understand connects, stays connected and reports nothing, and every other
// field on this struct reads exactly as it does for a printer that is idle. The
// rate is what separates them: sixty refused frames on a printer that has been
// up for a week is nothing, and sixty in the last minute is a dialect this
// build does not know.
//
// A driver that counts neither reports Counted false, and every figure below is
// zero. That is not a healthy link, it is no measurement.
type DeviceLink struct {
	Counted bool `json:"counted" doc:"Whether the driver behind this printer counts these at all. False means no measurement, not a clean link."`

	RefusedFrames          uint64  `json:"refused_frames"            doc:"Frames the printer sent that the driver had no reading of and dropped, since this session began. A frame the driver reads and declines to serve is not counted."`
	RefusedFramesPerMinute float64 `json:"refused_frames_per_minute" doc:"The refusal rate over the trailing window this reading was measured over. Anything above zero on a connected printer is firmware speaking something this build does not."`

	DroppedTelemetry          uint64  `json:"dropped_telemetry"            doc:"Telemetry patches discarded because this server was not reading them fast enough, since this session began."`
	DroppedTelemetryPerMinute float64 `json:"dropped_telemetry_per_minute" doc:"The drop rate over the trailing window this reading was measured over."`

	MeasuredOverSeconds float64 `json:"measured_over_seconds" doc:"How long the trailing window behind both rates covers. It is reported so that a rate is never read without the span it came from."`
}

// DeviceDeparture is a printer whose session has ended: why it went, and when.
//
// The reason is this server's own vocabulary and every value it takes is listed
// here, so a client branches on it. What a driver cannot tell apart is not
// invented: a machine that was switched off, a cable that was pulled and a
// firmware that stopped answering all end one session, and all three are
// session_ended.
type DeviceDeparture struct {
	Reason string `json:"reason" enum:"session_ended,server_stopping" doc:"Why the session ended. session_ended is the connector releasing the printer, which covers every way a machine stops answering. server_stopping is this server shutting down, so the printer is not known to have gone anywhere."`

	At time.Time `json:"at" doc:"When the session ended, as this server saw it."`
}

// DeviceRetirement is a device id that has stopped naming a machine: why, and
// when.
//
// A printer that changes owner is registered again, and the approval mints a
// second id and retires this one. Nothing is merged and nothing
// is deleted, so the previous owner's history, commands, grants and audit stay
// readable under this id and reach the machine's new id not at all.
//
// The reason is an open value. This build writes one, and a client that meets
// another shows it rather than refusing the record: a device removed from a
// site and one whose id an operator replaced by hand are both retirements with
// a reason of their own, and a boolean could not tell them apart.
type DeviceRetirement struct {
	Reason string `json:"reason" doc:"Why this id stopped naming a machine. ownership_changed is a second registration for the same printer, which is how a machine that changed hands is separated from its previous owner's records. Read it as an open value: a newer core, or a federated peer, may name a reason this build does not carry."`

	At time.Time `json:"at" doc:"When this id was retired."`
}

// DeviceState is the addressable state tree flattened to the paths the driver
// reports. Each value is passed through exactly as the printer encoded it: a
// server that re-encodes a number changes it.
type DeviceState map[string]json.RawMessage

// Schema describes the state as the open object it is. The paths a printer
// reports are the driver's, so nothing here can enumerate them.
func (DeviceState) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: huma.TypeObject,
		Description: "The printer's state tree, keyed by path such as /tools/0/nozzle_temp. Values are " +
			"passed through as the printer encoded them, and a path the printer stopped reporting is absent.",
		AdditionalProperties: true,
	}
}

// PrinterCommand is one request for a printer to do something.
type PrinterCommand struct {
	Verb string            `json:"verb"           doc:"The capability key to invoke. It must be one the printer's capability set lists."   maxLength:"63" minLength:"1"`
	Args map[string]string `json:"args,omitempty" doc:"Argument values as text, checked against the capability descriptor before sending."`

	// Alone in its own alignment group: gofmt aligns struct tags per block, and
	// this tag is 460 characters, so grouping it with the fields above pads
	// their tags out to its width to line the columns up.
	Key string `json:"key,omitempty" doc:"The caller's idempotency key, scoped to this printer. While a command sent under this key is still running, a second request carrying it is not sent to the printer again: it is answered with the first one's command_id and duplicate true. An empty key is not deduplicated. It is not remembered across a restart of this server, and it is not a queue: a command that must survive an outage is desired state, reconciled when the printer reconnects." maxLength:"128"`
}

// PrinterResult is what the printer made of a command.
//
// Ignored and Substituted exist so that a printer never silently disagrees with
// its caller: a command carried out differently says which parameters changed
// and to what. Both are empty in the normal case.
//
// A body with ok false is the printer's refusal and is answered with 200: the
// request was carried out, and what it produced was a no. Every printer refusal
// takes this shape, whether or not the firmware said why. A refusal this server
// decided, such as a verb the printer does not have or an argument the verb does
// not accept, never reaches it: that is a problem document with a registered
// code, and MachineReason is not one of those codes.
//
// FaultCode is how a client branches on a refusal without reading the machine's
// vocabulary. It is the registered code of the refusal itself, and it is on
// every one of them, so the two questions divide: the code says what class of
// answer this is, and MachineReason carries the printer's own word where the
// printer sent one.
type PrinterResult struct {
	OK bool `json:"ok" doc:"Whether the printer accepted the command."`

	// CommandID is this invocation's id, minted by this server. It is what the
	// three durable frames of the command's lifecycle are joined by, so a
	// caller reads back what happened to a command it sent from the event
	// stream or from the stored past under this one value.
	CommandID string `json:"command_id,omitempty" doc:"This invocation's id, minted by the server. The lifecycle events of the command carry it, so it is what a caller reads its record back under."`

	// Duplicate says this response is the first invocation's id rather than a
	// second dispatch. The command was not sent to the printer again.
	Duplicate bool `json:"duplicate,omitempty" doc:"True when a command under this idempotency key was already running for this printer. The printer was not sent the command a second time, and command_id names the invocation that is running."`

	// MachineReason is the printer's own word for a refusal, and it is here
	// because "not accepted" is not always the end of the exchange: a machine
	// that says the file is already there is answered by printing that file.
	//
	// It is the machine's vocabulary and not this API's. Nothing here defines
	// the set of words, a firmware may spell one this build has never seen, and
	// two vendors may spell the same condition differently, so it is shown to a
	// person rather than branched on. The identifier a client branches on is the
	// error code on a problem document, which a refused command does not have.
	// FaultCode is the registered code of the refusal, and it is here rather
	// than only on a problem document because a printer's refusal is not a
	// problem document: it is a 200 whose body says no.
	//
	// It is what a client branches on. Unlike MachineReason it is this API's
	// vocabulary and it is published: the code is in the error reference with
	// its retry hint and what to do about it. A printer that refused and sent
	// no word of its own carries the code and no machine reason at all, which
	// is why the code and not the reason is the field to read.
	FaultCode string `json:"fault_code,omitempty" doc:"The registered code of the refusal, absent on a command the printer accepted. It is this API's vocabulary rather than the machine's, so it is what a client branches on: read it with the error reference, which publishes each code's retry hint. core.printer/COMMAND_REFUSED is a refusal the printer gave a reason for, carried in machine_reason; core.printer/COMMAND_REFUSED_WITHOUT_REASON is one it gave no word for at all, and carries no machine reason."`

	MachineReason string `json:"machine_reason,omitempty" doc:"Why the printer refused, in the printer's own words, passed through as it spelled them. It is the machine's vocabulary rather than this API's: no list of values is published, a firmware may send one this server has never seen, and it is meant to be shown to a person rather than branched on. Absent on a command the printer accepted, and absent for a printer whose dialect has no such word: a printer that refused and said nothing carries fault_code and no reason. To branch on the refusal, read fault_code beside it."`

	Ignored     []string          `json:"ignored,omitempty"     doc:"Parameters the printer dropped."`
	Substituted map[string]string `json:"substituted,omitempty" doc:"Parameters the printer replaced, and with what."`

	// Answer is what the machine said back, in its own words, and it is empty
	// for the machines that say nothing. Shipping Buddy firmware is one of
	// those: it acknowledges a command and sends no reply text, so this field
	// is absent on every real printer served today and present on the
	// simulator, where a G-code line is answered the way a serial port would
	// answer it.
	//
	// It is the machine's vocabulary, like MachineReason. Show it to a person
	// and branch on nothing in it.
	Answer string `json:"answer,omitempty" doc:"What the printer said back, in its own words, and absent for a printer that answers a command without saying anything. Shipping Buddy firmware answers nothing, so this is empty on a real machine and carries the serial reply on the simulator: ok for most lines, the temperatures for M105, the position for M114. It is the machine's vocabulary rather than this API's, so show it to a person and branch on fault_code instead."`
}

// Page is one window of a list, as a caller asked for it. A zero Limit is the
// server's default page size and a zero Offset is the first page, so the zero
// value is the request a client that does not page makes.
type Page struct {
	Limit  int
	Offset int
}

// DevicePage is one page of the fleet.
//
// Total is over every page, so a client can size a pager without walking the
// fleet, and it counts only what this caller may see: the permission is
// resolved inside the query that produces both, so the count discloses nothing
// the listing does not.
//
// Limit and Offset are what the server applied, which is not always what was
// asked for: a limit above the cap is answered with the cap rather than
// refused, so a fleet-wide export pages instead of allocating.
type DevicePage struct {
	Items  []Device `json:"items"`
	Total  int      `json:"total"  doc:"How many printers the request matched over every page. With with_issues set, it counts only the printers carrying an issue."`
	Limit  int      `json:"limit"  doc:"The page size the server applied, which is the cap when a larger one was asked for."`
	Offset int      `json:"offset" doc:"The offset the server applied."`

	// Issues is over the caller's whole authorized population rather than over
	// Items, so one request gives a client both the page it asked for and the
	// badge it shows beside every page.
	Issues DeviceIssueSummary `json:"issues" doc:"What is wrong across every non-retired printer this caller may see, whatever page or filter was asked for."`

	Filters []DeviceFilter `json:"filters" doc:"How this listing may be narrowed, with the values present in this caller's fleet."`
	Sorts   []DeviceSort   `json:"sorts"   doc:"The fields this listing may be ordered by."`
	Sort    string         `json:"sort"    doc:"The field the server ordered this page by."`
	Order   string         `json:"order"   doc:"The direction the server ordered this page in."`
}

// DeviceQuery is one request for a page of the fleet: the window, and whether
// to narrow it to the printers that need attention.
//
// The filter and the window are separate values because they answer different
// questions. The window says how much of an answer to send, and the filter says
// which printers the answer is about.
type DeviceQuery struct {
	Page

	// WithIssues narrows the page to printers carrying at least one issue. It
	// changes Items and Total and never DeviceIssueSummary, which is what lets
	// a client show "3 of 400 need attention" from the filtered request.
	WithIssues bool

	// Tags narrows the page to devices carrying every one of these tags. Case,
	// joiners and emoji selectors are ignored in the match. It narrows Items and Total the way WithIssues does. It can
	// only ever narrow, because the permission is resolved in the query that
	// produced the rows and no tag reaches that query.
	Tags []string

	// Labels is the same filter spelled as key or key=value clauses, for a
	// client that has not moved to tags.
	Labels []string

	// States narrows the page to devices reporting one of these machine states.
	// The values are the machine's own words, and the comparison ignores case.
	// A device that is not connected matches OFFLINE instead of its last state.
	States []string

	// Models narrows the page to devices of one of these models, again in the
	// machine's own spelling and again ignoring case.
	Models []string

	// Search narrows the page to devices whose name, model, serial or unit id
	// contains this text. It is what a person types when they are looking for
	// one machine.
	Search string

	// Retired says what to do with the devices an operator retired: exclude,
	// include or only. An empty value is exclude.
	Retired string

	// Severities narrows the page to devices carrying an issue of one of these
	// severities.
	Severities []string

	// Sort names the field to order by, and Order the direction. An empty Sort
	// orders by urgency, worst first.
	Sort  string
	Order string
}

// listDevicesInput is the page a caller asked for. Both are optional, and a
// client that sends neither gets the first page at the server's default size,
// so a client written against an unpaged listing keeps working.
type listDevicesInput struct {
	Limit int `query:"limit" minimum:"0" doc:"How many printers to return. Omitted or zero is the server's default page size. A value above the server's cap is answered with the cap, and the answer says which limit was applied." example:"50"`

	Offset int `query:"offset" minimum:"0" doc:"How many printers to skip. The order is stable across pages, so paging by offset walks the fleet exactly once." example:"0"`

	WithIssues bool `query:"with_issues" doc:"Return only the printers carrying at least one issue. It narrows items and total; the issues rollup stays over every printer this caller may see, so a client filtering to the problems still knows how large the fleet behind them is." example:"true"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Tag []string `query:"tag" maxItems:"16" doc:"Return only the devices carrying every one of these tags, comma separated. The match is on the whole tag and ignores case, joiners and emoji selectors, so petg only finds PETG only. It narrows items and total and never the issues rollup, and it can only narrow: a tag decides no permission." example:"workshop,night shift"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Labels []string `query:"labels" maxItems:"16" doc:"Deprecated. The tag filter spelled as labels, each key or key=value, comma separated. key=value finds the tag key: value, and a bare key finds the tag key or any tag that starts with key and a colon. A term that is not a valid label key or value is refused. Use tag instead. This parameter is removed in a later release." example:"site=brno"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	State []string `query:"state" maxItems:"16" doc:"Return only the devices reporting one of these machine states, comma separated. The values are the machine's own words, from the vocabulary the device kind declares, and the match ignores case. A device that is not connected matches OFFLINE and not the last state it reported. A device reporting none of them is left out." example:"PRINTING,PAUSED"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Model []string `query:"model" maxItems:"16" doc:"Return only the devices of one of these models, comma separated. The values are the machine's own spelling, as get-device-model-vocabulary lists them, and the match ignores case." example:"MK4S,CoreOne"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Search string `query:"search" maxLength:"96" doc:"Return only the devices whose name, model, serial or unit id contains this text, ignoring case, and the devices whose model the catalogue shows under a name that contains it. It is a substring match, so the last digits of a serial read off the sticker are enough. The device id is not searched: it is minted here and means nothing to a person typing." example:"brno"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Retired string `query:"retired" enum:"exclude,include,only" doc:"What to do with the devices an operator retired. exclude is the default and leaves them out, because a retired device is a record of a machine that is no longer here. include puts them back, which is how the records of a machine that changed hands are read. only answers with those alone." example:"exclude"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Severity []string `query:"severity" maxItems:"3" doc:"Return only the devices carrying an issue of one of these severities, comma separated: error, warning, info. It narrows items and total and never the issues rollup." example:"error,warning"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Sort string `query:"sort" enum:"urgency,name,model,state,last_seen,id" doc:"Order the answer by this field. Omitted is urgency: errors first, then printers waiting for a person, then silent ones, then the rest; ties are broken by device id. The order is applied before the page is cut, so paging through a sorted fleet walks it once." example:"name"`

	//nolint:lll // the huma tag is one field's whole contract and does not wrap.
	Order string `query:"order" enum:"asc,desc" doc:"Which way to read the sort field. It is asc unless you say otherwise, and it applies to the sort field, or to urgency when sort is omitted." example:"asc"`
}

type listDevicesOutput struct {
	Body DevicePage
}

// DeviceRef is the device a path names. It is exported because Huma binds
// parameters by reflection and cannot write into an unexported embedded field.
type DeviceRef struct {
	DeviceID string `path:"device_id" doc:"The device's identity, as the listing reports it." maxLength:"63" example:"9f2a4c18-6d3b-4e07-b5a1-8c0f2d7e4b93"`
}

type getDeviceInput struct {
	DeviceRef
}

type deviceOutput struct {
	Body Device
}

type sendCommandInput struct {
	DeviceRef

	Body PrinterCommand
}

type sendCommandOutput struct {
	Body PrinterResult
}

// deviceErrors are the refusals every operation on this surface can answer
// with. There is no separate "you may not" and "it is not there": a printer the
// caller holds nothing on is reported as absent, in the same bytes.
var deviceErrors = []int{
	http.StatusUnauthorized,
	http.StatusForbidden,
	http.StatusNotFound,
}

// commandErrors adds what this server answers when it decides not to send the
// command. 400 is the descriptor's: an argument the verb does not accept, one it
// needs, or one outside the range it declares. 409 is a verb the resolved set
// does not carry, a printer this server mirrors for reading only, and a printer
// whose session has ended.
//
// A command the printer refused is not here. It was sent, the machine answered,
// and the answer is a 200 carrying PrinterResult with ok false and the machine's
// own reason. Reporting that as an error status would say this server declined
// to act, which is the opposite of what happened.
var commandErrors = append([]int{http.StatusBadRequest, http.StatusConflict}, deviceErrors...)

// registerDevices publishes the printers this server has been dialled by: the
// listing, one printer, and the one way to make a printer do something.
//
// Nothing here creates a printer. A printer is not configuration: it registers
// itself over a connector's own wire and is admitted by the host,
// which is why there is no POST on the collection.
func registerDevices(api huma.API, fleet DeviceFleet) {
	registerListDevices(api, fleet)
	registerGetDevice(api, fleet)
	registerDeviceCapabilities(api, fleet)
	registerDeviceJobs(api, fleet)
	registerDeviceJobHistory(api, fleet)
	registerJob(api, fleet)
	registerListJobs(api, fleet)
	registerSendCommand(api, fleet)
	registerDeviceFiles(api, fleet)
	registerSetDeviceTags(api, fleet)
	registerSetDeviceLabels(api, fleet)
	registerSetDeviceName(api, fleet)
	registerRetireDevice(api, fleet)
}

// setNameInput is the whole name, sent as one member so that a client cannot
// send a second one this server ignores and read the answer as if it had been
// taken.
type setNameInput struct {
	DeviceRef

	Body struct {
		//nolint:lll // the huma tag is one field's whole contract and does not wrap.
		Name string `json:"name" maxLength:"96" doc:"What to call this device. It is trimmed, it carries no control character, and it is at most 96 characters. An empty name takes the operator's name off, and the device reads under the model and the serial it reported again."`
	}
}

type setNameOutput struct {
	Body struct {
		//nolint:lll // the huma tag is one field's whole contract and does not wrap.
		Name string `json:"name" doc:"What the device is called now. It is the name that was sent, or the name this server composes from the reported model and serial when the request cleared it."`
	}
}

func registerSetDeviceName(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "set-device-name",
		Method:      http.MethodPut,
		Path:        deviceNamePath,
		Summary:     "Name a device",
		Description: "Writes what an operator calls this device. It is the name every listing and " +
			"every device read then shows, instead of the model and the serial this server composes " +
			"one from." + paragraphBreak +
			"A name is not a tag. A device carries many tags, and nothing in a list of tags says " +
			"which of them a screen puts at the top of a card. The name is the one value that " +
			"answers what the machine is called." +
			paragraphBreak +
			"The name survives the machine reconnecting, because it is kept on the device record " +
			"and written by this operation alone. Sending an empty name takes it off, and the " +
			"device reads under the model and the serial it reported again. The answer is what the " +
			"device is called after the write, so a client renders it without a second read." +
			paragraphBreak +
			"It takes administer on the device, which is what replacing the tags takes: naming a " +
			"machine is an administrative fact about it rather than something the machine did. A " +
			"rename is published on the event stream as com.prusa.connect.device.record.changed " +
			"with name among its changed fields.",
		Tags:   []string{"devices"},
		Errors: append([]int{http.StatusBadRequest}, deviceErrors...),
	}, func(ctx context.Context, in *setNameInput) (*setNameOutput, error) {
		held, err := fleet.SetName(ctx, principalFrom(ctx), in.DeviceID, in.Body.Name)
		if err != nil {
			return nil, err
		}

		out := &setNameOutput{}
		out.Body.Name = held

		return out, nil
	})
}

type retireDeviceInput struct {
	DeviceRef
}

type retireDeviceOutput struct {
	Status int
}

func registerRetireDevice(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "retire-device",
		Method:      http.MethodDelete,
		Path:        devicePath,
		Summary:     "Retire a device this deployment no longer has",
		Description: "Marks the device id as no longer naming a machine, for an operator who has " +
			"taken the printer away. It leaves the fleet listing, because that listing is what is " +
			"here now, and it is read back with `?retired=include` or `?retired=only`." +
			paragraphBreak +
			"Nothing is deleted. The record, the prints, the command log and every grant written on " +
			"the id stay exactly where they are, so last month's prints are still readable under " +
			"the id they were filed on, and `GET /api/v1/devices/{device_id}` still answers. That " +
			"is the whole difference between retiring and deleting. The cameras that watch the " +
			"printer are the exception: the first retirement removes them, so none keeps running " +
			"with no printer page to be seen on." + paragraphBreak +
			"The printer loses its access. The connector takes back the token and the approved " +
			"fingerprint it issued the printer, so the printer is refused with 401 when it dials " +
			"again. To come back it has to be added again: Add Printer to Connect on the printer, " +
			"and an approval where a person approves new printers. A connector that admits every " +
			"printer checks no credential, so there the printer joins again by itself." +
			paragraphBreak +
			"The session is closed where one is open, and the identity this server bound to the " +
			"machine is released, so a printer admitted again is given a new device id rather than " +
			"reviving this record. Nothing restores a retired record, and adding the printer again " +
			"never gives back the old credential. A retirement carries a reason, and this one is " +
			"`removed_by_operator`; `ownership_changed` is the other, written when a machine that " +
			"changed hands is registered again." + paragraphBreak +
			"Retiring twice is the same answer as retiring once, and the second call takes the " +
			"credential back again in case the first one failed partway. It takes administer on " +
			"the device, which the manager role confers." + paragraphBreak +
			"A printer that runs this server has `resident: true` on its record, and its retirement " +
			"is refused with DEVICE_RESIDENT. Its driver would connect again at once, and the " +
			"machine would come back under a new id.",
		Tags:   []string{"devices"},
		Errors: append([]int{http.StatusConflict}, deviceErrors...),
	}, func(ctx context.Context, in *retireDeviceInput) (*retireDeviceOutput, error) {
		if err := fleet.Retire(ctx, principalFrom(ctx), in.DeviceID); err != nil {
			return nil, err
		}

		return &retireDeviceOutput{Status: http.StatusNoContent}, nil
	})
}

// setTagsInput is the whole list, sent as one member so that a client cannot
// send a second one this server ignores and read the answer as if it had been
// taken.
type setTagsInput struct {
	DeviceRef

	Body struct {
		//nolint:lll // the huma tag is one field's whole contract and does not wrap.
		Tags []string `json:"tags" maxItems:"32" doc:"The whole tag list this device is to carry, in the order to show it. A tag left out is removed, so this replaces rather than merges. Each tag is trimmed, keeps its case, and carries no comma. A new tag is at most 64 characters, and a tag the device already carries is kept at any length when its text is exactly the same. Two tags that differ only in case, joiners or emoji selectors are the same tag."`
	}
}

type setTagsOutput struct {
	Body struct {
		Tags []string `json:"tags" doc:"What the device carries now, as it was cleaned."`
	}
}

func registerSetDeviceTags(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "set-device-tags",
		Method:      http.MethodPut,
		Path:        deviceTagsPath,
		Summary:     "Replace the tags on a device",
		Description: "Writes the whole tag list, so a tag absent from the body is taken off. A tag " +
			"is a plain word or short phrase a person puts on a printer, such as `workshop`, " +
			"`PETG only` or `night shift`. It has no key and no value." + paragraphBreak +
			"Each tag is trimmed, and a run of spaces inside it becomes one space. The case is " +
			"kept as sent. A new tag is at most 64 characters, and a device carries at most 32 " +
			"tags. A comma, a control character and a character that shows nothing are refused, " +
			"and a tag has at least one visible character. A joiner is accepted between two " +
			"visible characters, and an emoji or text selector right after one, so emoji and " +
			"scripts such as Persian work. An accent must sit on a visible character. " +
			"Two tags that differ only in case, joiners or selectors are the same tag, so the " +
			"second one is refused. Each broken rule has its own error code, named in the error " +
			"reference." + paragraphBreak +
			"A tag converted from a label can be up to 382 characters long. A write keeps a long " +
			"tag when the device already carries exactly the same text, so a client can send the " +
			"whole list back with one tag added." + paragraphBreak +
			"Nothing in the authorization graph reads a tag. A tag narrows the device listing " +
			"through its `tag` parameter and never decides who may see what." + paragraphBreak +
			"It takes administer on the device, which the manager role confers and an operator " +
			"does not hold. A tag is an administrative fact about a machine rather than something " +
			"the machine did. " +
			"The answer is the cleaned list, so a client renders it without a second read.",
		Tags:   []string{"devices"},
		Errors: append([]int{http.StatusBadRequest}, deviceErrors...),
	}, func(ctx context.Context, in *setTagsInput) (*setTagsOutput, error) {
		held, err := fleet.SetTags(ctx, principalFrom(ctx), in.DeviceID, in.Body.Tags)
		if err != nil {
			return nil, err
		}

		out := &setTagsOutput{}
		out.Body.Tags = held

		return out, nil
	})
}

// setLabelsInput replaces the whole set. The map is the body rather than a
// member of one, so a client cannot send a member this server ignores and read
// the answer as if it had been taken.
type setLabelsInput struct {
	DeviceRef

	Body struct {
		//nolint:lll // the huma tag is one field's whole contract and does not wrap.
		Labels map[string]string `json:"labels" doc:"The whole label set this device is to carry. Each label becomes one tag: the key alone when the value is empty, and key: value otherwise. The tags are held to the tag rules."`
	}
}

type setLabelsOutput struct {
	Body struct {
		Labels map[string]string `json:"labels" doc:"The tags the device carries now, read as labels."`
	}
}

func registerSetDeviceLabels(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "set-device-labels",
		Method:      http.MethodPut,
		Path:        deviceLabelsPath,
		Summary:     "Replace the tags on a device, spelled as labels (deprecated)",
		Description: "Deprecated. Use `PUT /api/v1/devices/{device_id}/tags`. This route is removed " +
			"in a later release." + paragraphBreak +
			"A device carries tags and no labels. This route writes the tags a label set spells, " +
			"so a client written against labels keeps working. A label with an empty value becomes " +
			"the tag named by its key, and a label with a value becomes the tag `key: value`. The " +
			"whole tag list is replaced, and the tags are held to the tag rules." + paragraphBreak +
			"Reading tags as labels loses tags, because a label set holds one value per key. When " +
			"two tags have the same key, the later one in the list wins. The tags `site: brno` and " +
			"`site: prague` become the one label `site: prague`, and the tags `site: brno` and " +
			"`site` become the label `site` with an empty value. A client that reads the labels and " +
			"writes them back through this route removes the tags it did not read. This route " +
			"also writes the tags in the order of their keys, so it loses the order a person " +
			"gave. Use the tags route to keep every tag." + paragraphBreak +
			"It takes administer on the device, which is what replacing the tags takes.",
		Tags:   []string{"devices"},
		Errors: append([]int{http.StatusBadRequest}, deviceErrors...),
	}, func(ctx context.Context, in *setLabelsInput) (*setLabelsOutput, error) {
		held, err := fleet.SetLabels(ctx, principalFrom(ctx), in.DeviceID, in.Body.Labels)
		if err != nil {
			return nil, err
		}

		out := &setLabelsOutput{}
		out.Body.Labels = held

		return out, nil
	})
}

func registerListDevices(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "list-devices",
		Method:      http.MethodGet,
		Path:        devicesPath,
		Summary:     "The devices this caller may see",
		Description: "Every printer that has dialled in and that the caller holds observe on. " +
			"A printer appears here when it is admitted, not when it is configured: nothing in the " +
			"configuration file names one. A printer whose session has ended stays on the list, " +
			"carrying the last state it reported and a `departed` member saying why it went, so a " +
			"machine that was here reads differently from an id that never answered. The answer is " +
			"one page: it carries the total over every page and the limit and offset that were " +
			"applied, so a fleet larger than one page is walked with the offset parameter rather " +
			"than truncated.\n\n" +
			"Every printer carries an `issues` array saying what this server has found wrong with " +
			"it, worst first, and the page carries the rollup of those issues over every printer " +
			"the caller may see. So one request is enough to load a fleet page and say which " +
			"machines need somebody. `with_issues=true` narrows the page to those machines. The " +
			"reasons are a closed set published at `/api/v1/vocabularies/device-issues`, which " +
			"carries the sentence and what to do about each one.",
		Tags:   []string{"devices"},
		Errors: deviceErrors,
	}, func(ctx context.Context, in *listDevicesInput) (*listDevicesOutput, error) {
		page, err := fleet.List(ctx, principalFrom(ctx), DeviceQuery{
			Page:       Page{Limit: in.Limit, Offset: in.Offset},
			WithIssues: in.WithIssues,
			Tags:       in.Tag,
			Labels:     in.Labels,
			States:     in.State,
			Models:     in.Model,
			Search:     in.Search,
			Retired:    in.Retired,
			Severities: in.Severity,
			Sort:       in.Sort,
			Order:      in.Order,
		})
		if err != nil {
			return nil, err
		}

		return &listDevicesOutput{Body: page}, nil
	})
}

func registerGetDevice(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "get-device",
		Method:      http.MethodGet,
		Path:        devicePath,
		Summary:     "One device",
		Description: "What the printer said it is, the capability set this server resolved for it, and " +
			"the state it last reported. A printer whose session has ended is answered here too, with " +
			"a `departed` member saying why it went; an id this server has never been dialled by is a " +
			"404.",
		Tags:   []string{"devices"},
		Errors: deviceErrors,
	}, func(ctx context.Context, in *getDeviceInput) (*deviceOutput, error) {
		found, err := fleet.Get(ctx, principalFrom(ctx), in.DeviceID)
		if err != nil {
			return nil, err
		}

		return &deviceOutput{Body: found}, nil
	})
}

func registerSendCommand(api huma.API, fleet DeviceFleet) {
	register(api, huma.Operation{
		OperationID: "send-device-command",
		Method:      http.MethodPost,
		Path:        deviceCommandsPath,
		Summary:     "Send a command to a device",
		Description: "Validates the command against the capability set this server resolved for the " +
			"device and, only if it passes, sends it. A verb the device does not have and an argument " +
			"the verb does not accept are both refused here, so the driver is never reached with " +
			"something nothing checked. It takes operate on the device.",
		Tags:   []string{"devices"},
		Errors: commandErrors,
	}, func(ctx context.Context, in *sendCommandInput) (*sendCommandOutput, error) {
		result, err := fleet.Command(ctx, principalFrom(ctx), in.DeviceID, in.Body)
		if err != nil {
			return nil, err
		}

		return &sendCommandOutput{Body: result}, nil
	})
}

// TransformSchema says in the document what the tools map holds: a tool, or
// null for a tool the printer has switched off. The generator cannot tell
// that from the Go type.
func (DeviceTools) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if tools, ok := s.Properties["tools"]; ok {
		if ref, isSchema := tools.AdditionalProperties.(*huma.Schema); isSchema {
			tools.AdditionalProperties = &huma.Schema{AnyOf: []*huma.Schema{ref, {Type: "null"}}}
		}
	}

	return s
}
