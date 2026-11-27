package gen

// The tables that say what this module serves.
//
// A binding names a core operation or a core schema and says which of its
// parts the module can answer. Everything else about the schema, the names,
// the types and every sentence, is read out of the core's own OpenAPI
// document, so nothing here restates what the document already says.
//
// The tables are also the gate. The generator walks the document and refuses
// to write a schema when it finds an operation or a property that neither
// table mentions. A core that grows an operation, or grows a property on a
// schema this module serves, therefore fails the build here rather than
// quietly serving a surface that has fallen behind.

// The wire methods the resolvers call. They are sdk/deviceapi's, and they are
// written as text because the generator only names them in the schema.
const (
	methodList    = "device.list"
	methodGet     = "device.get"
	methodJobs    = "device.jobs"
	methodFiles   = "device.files"
	methodCommand = "device.command"
	methodApprove = "device.approve"
)

// The two root type names.
const (
	rootQuery    = "Query"
	rootMutation = "Mutation"
)

// The reasons an operation is not served. Each is one sentence, and the
// generator writes none of them into the schema: they are here so a person
// reading this table learns why a field they expected is missing.
const (
	// reasonControlPlane covers identity, grants, resources and modules.
	reasonControlPlane = "it is the control plane. A module must not be a second door onto " +
		"the graph that decides what the module may do, nor onto the switch that turns it off"
	// reasonBytes covers everything that moves file content.
	reasonBytes = "it moves file bytes, and no file content crosses the module wire. " +
		"A file reaches a device through the core's own transfer route"
	// reasonStream covers everything that answers over time rather than at once.
	reasonStream = "it answers over time, and one GraphQL answer is one whole document"
	// reasonNoWire covers everything the fleet wire has no method for.
	reasonNoWire = "the module wire carries no method that reads it"
)

// unboundGroups is every core operation this module does not serve, with the
// reason. The generator refuses a document carrying an operation that is
// neither bound below nor listed here.
//
//nolint:gochecknoglobals // a closed table the generator reads; a const cannot hold a slice.
var unboundGroups = []struct {
	reason     string
	operations []string
}{
	{
		reasonControlPlane,
		[]string{
			"accept-declaration-change",
			"apply-server-upgrade",
			"approve-app-pairing",
			"approve-link-node",
			"begin-login",
			"cancel-server-upgrade",
			"check-access",
			"check-setup-oidc",
			"check-setup-storage",
			"collect-app-pairing",
			"complete-login",
			"create-account",
			"create-server-backup",
			"create-credential",
			"create-first-account",
			"create-pairing",
			"create-setup-certificates",
			"delete-account",
			"delete-camera",
			"delete-connector-endpoint",
			"delete-module-state",
			"delete-pairing",
			"delete-resource",
			"delete-server-backup",
			"deny-app-pairing",
			"disconnect-beta-portal",
			"get-app-pairing",
			"get-auth-info",
			"get-beta-portal-pairing-status",
			"get-diagnostics",
			"get-discovery-visibility",
			"get-link",
			"get-module-marketplace",
			"get-module-memory",
			"get-module-openapi",
			"get-module-removal",
			"get-module-why",
			"get-module-why-not",
			"get-module-state",
			"get-principal",
			"get-resource",
			"get-resource-log",
			"get-resource-memory",
			"get-server-upgrade",
			"get-server-version",
			"get-setup",
			"get-setup-configuration",
			"get-time-server",
			"get-ui-settings",
			"install-module",
			"list-accounts",
			"list-allowed-origins",
			"list-connector-endpoints",
			"list-credentials",
			"list-declaration-acceptances",
			"list-device-verb-grants",
			"list-grants",
			"list-links",
			"list-module-databases",
			"list-module-index",
			"list-module-outdated",
			"list-module-state",
			"list-pairings",
			"list-resources",
			"list-server-backups",
			"list-server-upgrade-history",
			"list-server-upgrade-releases",
			"list-state-namespaces",
			"list-subject-direct-grants",
			"list-subject-grants",
			"logout",
			"open-page-session",
			"password-login",
			"plan-module-install",
			"prepare-server-upgrade",
			"put-connector-endpoint",
			"put-module-state",
			"purge-module-database",
			"put-camera",
			"put-resource",
			"refresh-beta-portal-pairing",
			"record-setup",
			"request-app-pairing",
			"reject-link-node",
			"restart-resource",
			"restart-server",
			"restore-server-backup",
			"revoke-credential",
			"revoke-link-node",
			"set-account-password",
			"set-account-role",
			"set-allowed-origins",
			"set-discovery-visibility",
			"set-module-marketplace",
			"set-time-server",
			"start-beta-portal-pairing",
			"stage-server-upgrade",
			"uninstall-module",
			"withdraw-device-verb-grant",
			"withdraw-grant",
			"write-device-verb-grant",
			"write-grant",
		},
	},
	{reasonBytes, []string{
		"create-diagnostics-bundle", "delete-device-file", "delete-storage-entry",
		"download-storage-object",
		"get-camera-snapshot", "get-device-file-thumbnail", "get-device-model-artwork",
		"get-printer-authority", "get-storage-thumbnail", "post-storage-object",
		"put-camera-snapshot",
		"run-printer-gcode-preset", "send-device-file", "send-printer-gcode",
		"stage-server-upgrade-bundle", "upload-storage-object",
	}},
	{reasonStream, []string{
		"get-developer-view", "get-metrics", "live-camera", "stream-camera", "stream-events",
		"watch-camera",
	}},
	{
		reasonNoWire,
		[]string{
			"create-dashboard",
			"create-device-folder",
			"create-storage-folder",
			"delete-dashboard",
			"delete-device-folder",
			"delete-gcode-preset",
			"delete-storage-entries",
			"get-dashboard",
			"list-dashboards",
			"replace-dashboard",
			"set-default-dashboard",
			"get-authorization-vocabulary",
			"get-capability-vocabulary",
			"get-command",
			"get-connector-health",
			"get-developer-connectors",
			"get-developer-device-commands",
			"get-device-capabilities",
			"get-device-issue-vocabulary",
			"get-device-model-vocabulary",
			"get-device-state-vocabulary",
			"get-error-code-vocabulary",
			"get-event-stream-stats",
			"get-event-type-vocabulary",
			"get-extension-vocabulary",
			"get-gcode-preset",
			"get-health",
			"get-job",
			"get-readiness",
			"get-resource-kind-vocabulary",
			"list-camera-settings",
			"list-camera-types",
			"list-cameras",
			"list-device-commands",
			"list-device-history-facts",
			"list-device-history-samples",
			"list-device-jobs",
			"list-device-transfers",
			"list-gcode-presets",
			"list-history-facts",
			"list-history-samples",
			"list-jobs",
			"list-nearby-printers",
			"list-pending-registrations",
			"list-running-transfers",
			"list-storage-files",
			"list-storages",
			"list-vocabularies",
			"move-storage-entry",
			"print-device-file",
			"put-gcode-preset",
			"reject-registration",
			"retire-device",
			"set-device-labels",
			"set-device-name",
			"set-device-tags",
			"stat-storage-entry",
			"stop-device-transfer",
		},
	},
}

// objectBinding is one of the core's schemas served as a GraphQL object type.
// The type keeps the schema's name and every member keeps the property's name,
// in the snake_case the REST surface uses.
type objectBinding struct {
	// Schema is the component schema in the core document, and the GraphQL
	// type name.
	Schema string
	// Served is the properties the module can fill, in the order the schema
	// declares them.
	Served []string
	// Absent is the properties the module cannot fill, each with the reason.
	// A property in neither list fails the generator.
	Absent map[string]string
}

// The reasons a property is absent from a type. A member that would always
// answer null is worse than a member that is not there: a client cannot tell
// the two apart, and the first one looks like a fault in the fleet.
const (
	absentNotOnWire              = "the fleet wire does not carry it"
	absentListingSelfDescription = "the printers listing's self-description; a GraphQL client pages and filters by its own query"
	absentHostDecides            = "it is what the host decided for one HTTP caller, and the wire answers no equivalent"
	absentObjectsNotPorted       = "the objects a print can skip are read from the REST jobs route; this module does not map them yet"
	absentOutcomeNotPorted       = "how a print ended is read from the REST jobs route; this module does not map it yet"
)

// objects is every schema this module serves.
//
//nolint:gochecknoglobals // a closed table the generator reads.
var objects = []objectBinding{
	{
		Schema: "Device",
		Served: []string{
			"kind", "id", "name", "vendor", "model", "firmware", "serial", "driver",
			"connected", "capabilities", "capability_epoch", "state", "tags", "labels",
		},
		Absent: map[string]string{
			"actions":             absentHostDecides,
			"authorization_epoch": absentHostDecides,
			"build_volume":        "a reading of the state tree, which the state field serves whole",
			"departed":            absentNotOnWire,
			"issues":              absentNotOnWire,
			"job":                 "the jobs field answers it, from device.jobs",
			"link":                absentNotOnWire,
			"profile_matched":     absentNotOnWire,
			"resident":            absentNotOnWire,
			"retired":             absentNotOnWire,
			"tools":               "a reading of the state tree, which the state field serves whole",
			"unit_uid":            absentNotOnWire,
			"verbs":               absentHostDecides,
		},
	},
	{
		Schema: "DevicePage",
		Served: []string{"items", "total", "limit", "offset"},
		Absent: map[string]string{
			"filters": absentListingSelfDescription,
			"issues":  absentNotOnWire,
			"order":   absentListingSelfDescription,
			"sort":    absentListingSelfDescription,
			"sorts":   absentListingSelfDescription,
		},
	},
	{Schema: "DeviceJobs", Served: []string{"current", "last"}},
	{
		Schema: "DeviceJob",
		Served: []string{
			"id", "device_job_id", "path", "display_name", "state", "progress", "time_remaining_ms",
			"started", "ended",
		},
		Absent: map[string]string{
			"device_id":    absentNotOnWire,
			"object_count": absentObjectsNotPorted,
			"objects":      absentObjectsNotPorted,
			"outcome":      absentOutcomeNotPorted,
		},
	},
	{
		Schema: "DeviceFile",
		Served: []string{
			"path", "name", "display_name", "folder", "size", "modified", "read_only", "estimated_seconds",
			"printer_model", "layer_height_mm", "material",
		},
	},
	{
		Schema: "DeviceStorages",
		Served: []string{"items"},
		Absent: map[string]string{"reported": absentNotOnWire},
	},
	{
		Schema: "DeviceStorage",
		Served: []string{"mountpoint", "name", "type", "free_space", "read_only"},
	},
	{
		Schema: "DeviceListing",
		Served: []string{"directory", "items"},
		Absent: map[string]string{
			"stale":         absentNotOnWire,
			"reading":       absentNotOnWire,
			"reading_since": absentNotOnWire,
			"fault":         absentNotOnWire,
		},
	},
	{
		Schema: "Registration",
		Served: []string{"code", "connector", "fingerprint", "serial", "model", "firmware", "approved"},
		Absent: map[string]string{"device_id": absentNotOnWire},
	},
	{
		Schema: "PrinterResult",
		Served: []string{"ok", "machine_reason", "ignored", "substituted"},
		Absent: map[string]string{
			"answer":     absentNotOnWire,
			"command_id": absentNotOnWire,
			"duplicate":  "the wire carries no idempotency key, so no command is ever a repeat of one",
			"fault_code": "a refusal leaves through the GraphQL errors list and carries the code there",
		},
	},
}

// fieldBinding is one core operation served as a root field.
type fieldBinding struct {
	// Root is Query for a read and Mutation for anything that changes state.
	Root string
	// Name is the field, which is the operationId with its hyphens turned
	// into underscores unless Rename says otherwise.
	Name string
	// Operation is the operationId in the core document.
	Operation string
	// Method is the wire method the resolver calls.
	Method string
	// Params are the operation's own parameters this field takes, by name.
	Params []string
	// ParamsAbsent are the operation's parameters the wire cannot narrow by,
	// each with the reason.
	ParamsAbsent map[string]string
	// Body is the request body schema whose properties become arguments, and
	// empty for an operation with no body.
	Body string
	// BodyServed and BodyAbsent split that schema's properties the way an
	// objectBinding splits a response schema's.
	BodyServed []string
	BodyAbsent map[string]string
	// Description replaces the operation's description, for a field whose
	// wire method answers differently from the HTTP route. Empty keeps it.
	Description string
}

// The reasons an operation's parameter is not an argument.
const (
	absentParamNoWire = "the wire's listing does not narrow by it"
	absentParamOrder  = "the wire answers one page in the core's own order, and reordering a page " +
		"in the module would reorder that page alone"
)

// fields is every operation this module serves.
//
//nolint:gochecknoglobals // a closed table the generator reads.
var fields = []fieldBinding{
	{
		Root: rootQuery, Name: "devices", Operation: "list-devices", Method: methodList,
		Params: []string{"limit", "offset", "tag", "labels"},
		ParamsAbsent: map[string]string{
			"with_issues": absentParamNoWire,
			"state":       absentParamNoWire,
			"model":       absentParamNoWire,
			"search":      absentParamNoWire,
			"severity":    absentParamNoWire,
			"retired":     absentParamNoWire,
			"sort":        absentParamOrder,
			"order":       absentParamOrder,
		},
	},
	{
		Root: rootQuery, Name: "device", Operation: "get-device", Method: methodGet,
		Params: []string{"device_id"},
	},
	{
		Root: rootQuery, Name: "device_jobs", Operation: "get-device-jobs", Method: methodJobs,
		Params: []string{"device_id"},
	},
	{
		Root: rootQuery, Name: "device_files", Operation: "list-device-files", Method: methodFiles,
		Params:       []string{"device_id", "path"},
		ParamsAbsent: map[string]string{"refresh": absentParamNoWire},
		Description: "Asks the printer what is at a path and waits for its answer. An MK4S takes " +
			"12 to 15 s to list a USB root of about 300 files, so give the query a timeout to match. " +
			"An empty path is refused: device_storages answers what is mounted. A path carrying a .. " +
			"segment is refused and never reaches the printer. It takes operate on the printer.",
	},
	{
		Root: rootQuery, Name: "device_storages", Operation: "list-device-storages", Method: methodFiles,
		Params:       []string{"device_id"},
		ParamsAbsent: map[string]string{"refresh": absentParamNoWire},
		Description: "What the printer said its storages are the last time it was asked, answered " +
			"at once. It does not ask the printer again. Every answer that changes the list is " +
			"published as com.prusa.connect.device.storages.changed. It takes observe on the printer.",
	},
	{
		Root: rootMutation, Name: "send_device_command", Operation: "send-device-command",
		Method: methodCommand, Params: []string{"device_id"},
		Body: "PrinterCommand", BodyServed: []string{"verb", "args"},
		BodyAbsent: map[string]string{
			"key": "the wire carries no idempotency key, so a repeated command is a second command",
		},
	},
	{
		Root: rootMutation, Name: "approve_registration", Operation: "approve-registration",
		Method: methodApprove, Params: []string{"code"},
	},
}
