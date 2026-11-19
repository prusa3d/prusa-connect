package httpapi

import (
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/transport/devtools"
)

// The instants the file listings and the transfer rows are sampled at. One file
// was written a day before the other, and the sample of a running transfer has
// a start and no end. They are one set of values so that two sample bodies a
// reader compares agree with each other.
var (
	exampleWritten      = time.Date(2025, time.August, 26, 9, 14, 5, 0, time.UTC)
	exampleWrittenEarly = exampleWritten.Add(-24 * time.Hour)
	exampleSent         = exampleWritten.Add(67 * time.Second)
	exampleStopped      = exampleWrittenEarly.Add(56 * time.Second)
)

// The media types an example is attached under. An example goes on the media
// type the operation actually answers with, because Scalar reads the example
// out of the content entry it is rendering.
const (
	jsonMedia   = "application/json"
	sseMedia    = "text/event-stream"
	plainMedia  = "text/plain"
	okStatus    = "200"
	createdCode = "201"
)

// operationExample is one operation's worked example: the body a caller sends
// and the body the server answers with.
//
// Both are Go values of the same types the handler returns, so they are
// marshalled by the encoder that writes the response and cannot carry a member
// this server does not publish. TestEveryExampleValidatesAgainstItsOwnSchema
// holds the other half, and TestAPublishedExampleIsWhatTheHandlerWrites drives
// the real router and compares the bytes.
type operationExample struct {
	// Request is the body a caller sends, or nil for an operation that takes
	// none.
	Request any
	// Response is the body the server answers with.
	Response any
	// Status is the response the example is attached to. Empty means 200.
	Status string
	// Media is the content type it goes under. Empty means application/json.
	Media string
	// RequestMedia is the content type the request goes under when it differs
	// from the answer's, as a file upload does. Empty means Media.
	RequestMedia string
	// Query is the query string the sample sends, without the question mark.
	// It is what the rendered curl line and client call carry, and what the
	// round trip test sends, so a listing that shows one page names the page.
	Query string
	// Headers are the request headers the sample sends beside the credential,
	// which is how a write that needs a precondition carries its If-Match.
	Headers map[string]string
}

// The extensions the examples write onto an operation, read by the reference
// renderer (cmd/generate-sdk-reference) and by the round trip test. A vendor
// extension is the one member OpenAPI lets a document add, and these values
// describe the sample and not the operation.
const (
	// sampleExcuseExtension carries the sentence from bodylessOperations for an
	// operation with no sample, so the page says why instead of showing a hole.
	sampleExcuseExtension = "x-sample-excuse"
	// sampleQueryExtension carries operationExample.Query.
	sampleQueryExtension = "x-sample-query"
	// sampleHeadersExtension carries operationExample.Headers.
	sampleHeadersExtension = "x-sample-headers"
)

// ifMatchVersion is the precondition every write sample sends: the ETag of
// the configuration overlay the read samples report, quoted as a header
// carries it.
const ifMatchVersion = `"v7"`

// attachExamples writes every example into the document, after every operation
// is registered and before the document is rendered.
//
// It refuses a server whose example does not fit the operation it names: one
// for a status the operation cannot answer, or a media type it does not serve.
// That is a table and a handler disagreeing about what the operation does, and
// it would publish an example under a schema nothing checks it against.
//
// An operation with no example at all is not refused here.
// TestEveryPublishedOperationCarriesAnExample fails instead, because a missing
// example is a documentation defect and must not stop a server from starting.
//
// An example naming an operation this build did not register is skipped, because
// the camera operations are published only when a media plane is wired and one
// table serves both builds.
func attachExamples(api huma.API) error {
	examples := operationExamples()

	for path, item := range api.OpenAPI().Paths {
		for method, op := range operationsOfPath(item) {
			if op == nil {
				continue
			}

			example, carried := examples[op.OperationID]
			excuse, excused := bodylessOperations[op.OperationID]

			if carried && excused {
				return fmt.Errorf("%s %s: %w: it is excused in bodylessOperations and carries an example",
					method, path, ErrOperationWithoutExample)
			}

			if excused {
				extend(op, sampleExcuseExtension, excuse)

				continue
			}

			if !carried {
				continue
			}

			if err := attachOne(op, example); err != nil {
				return fmt.Errorf("%s %s: %w", method, path, err)
			}
		}
	}

	return nil
}

// extend writes one vendor extension onto the operation.
func extend(op *huma.Operation, key string, value any) {
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}

	op.Extensions[key] = value
}

// attachOne writes one example onto the operation's request body and onto the
// response it is for.
func attachOne(op *huma.Operation, example operationExample) error {
	media := example.Media
	if media == "" {
		media = jsonMedia
	}

	status := example.Status
	if status == "" {
		status = okStatus
	}

	if example.Query != "" {
		extend(op, sampleQueryExtension, example.Query)
	}

	if len(example.Headers) > 0 {
		extend(op, sampleHeadersExtension, example.Headers)
	}

	if example.Request != nil {
		requestMedia := example.RequestMedia
		if requestMedia == "" {
			requestMedia = media
		}

		content, err := requestContent(op, requestMedia)
		if err != nil {
			return err
		}

		content.Example = example.Request
	}

	response, published := op.Responses[status]
	if !published {
		return fmt.Errorf("%w: the example is for %s and the operation does not answer it",
			ErrOperationWithoutExample, status)
	}

	content, published := response.Content[media]
	if !published {
		return fmt.Errorf("%w: the example is %s and the %s answer is not",
			ErrOperationWithoutExample, media, status)
	}

	content.Example = example.Response

	return nil
}

func requestContent(op *huma.Operation, media string) (*huma.MediaType, error) {
	if op.RequestBody == nil {
		return nil, fmt.Errorf("%w: the example sends a body and the operation takes none",
			ErrOperationWithoutExample)
	}

	content, published := op.RequestBody.Content[media]
	if !published {
		return nil, fmt.Errorf("%w: the example sends %s and the operation does not accept it",
			ErrOperationWithoutExample, media)
	}

	return content, nil
}

// operationExamples is one worked example per operation. See operationExample
// for what holds each one to what the server actually sends.
//
//nolint:funlen // one entry per operation, and splitting it would hide the set the gate reads.
func operationExamples() map[string]operationExample {
	examples := map[string]operationExample{
		"get-health": {Response: map[string]any{
			"status": "ok", "version": "0.1.0", "instance_id": "8f14e45f-ceea-4e7a-9c2b-3d1f6a5b7c90",
		}},

		"get-principal": {
			Response: map[string]any{"principal": "user:alice", "display_name": "Alice Novak", "username": "alice"},
		},

		"get-diagnostics": {Response: map[string]any{"extensions": []string{"Prusa printer logs"}}},

		"get-ui-settings": {Response: AppSettings{
			Enabled:          true,
			Title:            "Prusa Connect",
			DefaultLanguage:  "en",
			Theme:            "system",
			PrinterAdmission: "approval",
			APIReference:     referenceExample(),
			Dashboards:       dashboardsCarried,
			Diagnostics:      diagnosticsCarried,
			Modules:          true,
			Upgrades:         true,
		}},

		"get-readiness": {Response: ReadinessReport{
			App:       AppServed,
			Discovery: "announcing",
			Ready:     true,
			Serving:   2,
			Declared:  2,
			Blocked:   []string{},
			Instances: []ReadinessInstance{
				{Resource: "storage:local", State: "ready", Required: true, Serving: true},
				{Resource: "driver:xbuddy", State: "ready", Required: true, Serving: true},
			},
		}},

		"get-connector-health": {Response: map[string]any{"items": []ConnectorRollup{{
			Connector: "driver:xbuddy",
			Status:    "degraded",
			Counts: map[string]int{
				"connected": 11, "connecting": 0, "disconnected": 1, "error": 0, "unknown": 0,
			},
			Degraded: []ConnectorDevice{{
				ID:       "b7e40d29-5c13-4f86-9a2b-0e6d8f34a1c5",
				Name:     "Workshop MK4",
				State:    "disconnected",
				Reason:   "SessionEnded",
				Since:    &exampleTime,
				LastSeen: &exampleTime,
				Attempts: 4,
			}},
			Total: 12,
		}}}},

		"get-metrics": {
			Media:    plainMedia,
			Response: exampleExposition,
		},

		// The page is asked for explicitly, because the default belongs to the
		// device plane and the answer names the limit it applied.
		"list-devices": {Query: "limit=50", Response: DevicePage{
			Items:   []Device{examplePrinter(), exampleStoppedPrinter()},
			Total:   12,
			Limit:   50,
			Offset:  0,
			Issues:  exampleIssueSummary(),
			Filters: exampleFilters(),
			Sorts:   exampleSorts(),
			Sort:    "urgency",
			Order:   "asc",
		}},

		"get-device": {Response: examplePrinter()},

		"get-device-capabilities": {Response: exampleDeviceCapabilities()},

		"get-device-jobs": {Response: exampleDeviceJobs()},

		// The print a link names. It is the current print of the jobs example.
		"get-job": {Response: *exampleDeviceJobs().Current},

		// The first page is asked for with a size, because the answer names the
		// limit it applied and the cursor that follows it.
		"list-device-jobs": {Query: "limit=50", Response: exampleDeviceJobHistory()},

		// The whole fleet's prints, running and ended, one page.
		"list-jobs": {Query: "limit=50", Response: exampleJobPage()},

		"set-device-name": {
			Request:  map[string]any{"name": exampleDeviceName},
			Response: map[string]any{"name": exampleDeviceName},
		},

		"check-access": {
			Request: accessCheckBody{
				ObjectType: "device", ObjectID: examplePrinterID, Action: "operate",
			},
			Response: AccessDecision{
				Allowed: true,
				Actions: []string{"observe", "operate"},
				Epoch:   exampleAuthorizationEpoch,
			},
		},

		"send-device-command": {
			Request: PrinterCommand{
				Verb: "core.command.set_nozzle_temperature",
				Args: map[string]string{"nozzle_temperature": "215"},
				Key:  "01K4C9YF2PS9V4YQF1N5KGH8ZB",
			},
			Response: PrinterResult{OK: true, CommandID: "01K4C9YF3T0XN9WQ8CJ4A2M7RD"},
		},

		"send-printer-gcode": {
			Request: PrinterGCodeRequest{Lines: []string{"M104 S215", "M140 S60", "M105"}},
			Response: PrinterGCodeOutcome{Completed: 3, Steps: []PrinterGCodeStep{
				{GCode: "M104 S215", Status: stepAccepted, Answer: "ok"},
				{GCode: "M140 S60", Status: stepAccepted, Answer: "ok"},
				{GCode: "M105", Status: stepAccepted, Answer: "ok T:215.0 /215.0 B:60.0 /60.0"},
			}},
		},

		"run-printer-gcode-preset": {Response: PrinterGCodeOutcome{
			Completed: 2,
			Stopped:   true,
			Steps: []PrinterGCodeStep{
				{GCode: "M104 S215", Status: stepAccepted, Answer: "ok"},
				{GCode: "M140 S60", Status: stepAccepted, Answer: "ok"},
				{
					GCode: "G28", Status: stepRefused,
					Detail: "the printer did not accept it", MachineReason: "Device is busy",
				},
			},
		}},

		"list-device-storages": {Response: DeviceStorages{Reported: true, Items: []DeviceStorage{
			{Mountpoint: "/usb", Name: "usb", Type: "USB", FreeSpace: int64Ptr(7_314_915_328)},
			{Mountpoint: "/internal", Name: "internal", Type: "INTERNAL", FreeSpace: nil, ReadOnly: true},
		}}},

		"list-device-files": {Query: "path=usb/models", Response: DeviceListing{
			Directory: DeviceFile{
				Path: "usb/models", Name: "models", Folder: true, Modified: &exampleWritten,
			},
			Items: []DeviceFile{
				{
					Path: "usb/models/bracket.bgcode", Name: "BRACK~1.BGC",
					DisplayName: "bracket.bgcode", Size: 4_182_016, Modified: &exampleWritten,
				},
				{
					Path: "usb/models/spares", Name: "SPARES",
					Folder: true, Modified: &exampleWrittenEarly,
				},
			},
		}},

		"send-device-file": {
			Request: DeviceUpload{
				Storage: "local", Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
			},
			Response: map[string]any{
				"path": "usb/bracket.bgcode",
				"transfer": DeviceTransfer{
					TransferID: exampleTransferID, Storage: exampleStorageName,
					Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
					BytesTotal: 4_182_016, BytesSent: 4_182_016, State: "done",
					StartedAt: exampleWritten, EndedAt: &exampleSent,
				},
			},
		},

		// The two-click print: the file was uploaded to a storage a moment ago,
		// and this one call relays it and starts it.
		"print-device-file": {
			Request: DevicePrint{
				Storage: "local", Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
			},
			Response: map[string]any{
				"path": "usb/bracket.bgcode",
				"transfer": DeviceTransfer{
					TransferID: exampleTransferID, Storage: exampleStorageName,
					Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
					BytesTotal: 4_182_016, BytesSent: 4_182_016, State: "done",
					StartedAt: exampleWritten, EndedAt: &exampleSent,
				},
				"command": PrinterResult{OK: true, CommandID: "01K4C9YF3T0XN9WQ8CJ4A2M7RD"},
			},
		},

		"list-device-transfers": {Response: map[string]any{"items": []DeviceTransfer{
			{
				TransferID: exampleTransferID, Storage: exampleStorageName,
				Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
				BytesTotal: 4_182_016, BytesSent: 1_048_576, State: "sending",
				StartedAt: exampleWritten,
			},
			{
				TransferID: "01M1GRYPQ4B8V0J3XN2C7WD5ZK", Storage: "local",
				Source: "models/spares/clip.bgcode", Path: "usb/clip.bgcode",
				BytesTotal: 218_112, BytesSent: 91_004, State: "interrupted",
				Reason: "server restarted", StartedAt: exampleWrittenEarly, EndedAt: &exampleStopped,
			},
		}}},

		"list-running-transfers": {Response: map[string]any{"items": []RunningTransfer{
			{
				DeviceTransfer: DeviceTransfer{
					TransferID: exampleTransferID, Storage: exampleStorageName,
					Source: "models/bracket.bgcode", Path: "usb/bracket.bgcode",
					BytesTotal: 4_182_016, BytesSent: 1_048_576, State: "sending",
					StartedAt: exampleWritten,
				},
				DeviceID: examplePrinterID, PrinterName: exampleDeviceName,
				Mine: false, StartedBy: exampleSubject, CanStop: true,
			},
		}}},

		"create-device-folder": {
			Request:  map[string]any{"path": "usb/models"},
			Response: map[string]any{"path": "usb/models"},
		},

		"list-storages": {Response: map[string]any{"items": []Storage{
			{
				Name: "local", DisplayName: "Local disk", Browse: true, Write: true,
				AllowedExtensions: []string{".gcode", ".bgcode", ".stl", ".3mf"},
			},
			{Name: "staging", DisplayName: "Staging area", Browse: true, Write: false, AllowedExtensions: []string{}},
		}}},

		// Two modules, each with a namespace derived from its own module path.
		// A reader compares the byte totals against the 8 MiB one namespace
		// may hold.
		"list-state-namespaces": {Response: map[string]any{"items": []StateNamespace{
			{
				Namespace: exampleNamespace, Instance: "sink:planner",
				Keys: 412, Bytes: 1_048_576,
			},
			{
				Namespace: "com.github.aux.acme.connect", Instance: "sink:connect",
				Keys: 3, Bytes: 512,
			},
		}}},

		// One module's settings, as its own pane reads and writes them. The
		// keys are the person's own: another person signed in to the same
		// deployment lists their own and never these.
		"list-module-state": {Response: map[string]any{
			"items": []string{"theme", exampleStateKey},
		}},

		"get-module-state": {Response: exampleStateRecord()},

		"put-module-state": {
			Request:  moduleStateBody{Value: exampleStateValue()},
			Response: exampleStateRecord(),
		},

		// A module's document, rebased: the one path it describes is under
		// the mount the namespace produces.
		"get-module-openapi": {Response: map[string]any{
			"openapi": "3.1.0",
			"info":    map[string]any{"title": "planner", "version": "1.0.0"},
			"paths": map[string]any{
				"/api/v1/modules/com.github.aux.acme.planner/spools": map[string]any{
					"get": map[string]any{
						"operationId": "list-spools",
						"summary":     "The spools on the shelf",
						"description": "Every spool the module knows about.",
						"responses":   map[string]any{"200": map[string]any{"description": "The listing."}},
					},
				},
			},
		}},

		"list-storage-files": {Query: "path=models", Response: map[string]any{"items": []StorageEntry{
			{
				Name: "bracket.bgcode", Path: "models/bracket.bgcode",
				Size: 4_182_016, Modified: exampleWritten,
				SHA256: "9f1a1f0e5a4dbe4c3d33d1fbbc6a8b0e2e0f0f6a2d5c1e7b8a90c3d4e5f60718",
			},
			{Name: "spares", Path: "models/spares", Folder: true, Modified: exampleWrittenEarly},
		}}},

		"stat-storage-entry": {Query: "path=models/bracket.bgcode", Response: StorageEntry{
			Name: "bracket.bgcode", Path: "models/bracket.bgcode",
			Size: 4_182_016, Modified: exampleWritten,
			SHA256: "9f1a1f0e5a4dbe4c3d33d1fbbc6a8b0e2e0f0f6a2d5c1e7b8a90c3d4e5f60718",
		}},

		"create-storage-folder": {
			Request:  map[string]any{"path": "models/spares"},
			Response: map[string]any{"path": "models/spares"},
		},

		"move-storage-entry": {
			Request:  map[string]any{"from": "models/bracket.bgcode", "to": "spares/bracket.bgcode"},
			Response: map[string]any{"path": "spares/bracket.bgcode"},
		},

		"list-gcode-presets": {Response: map[string]any{
			"items": []GCodePreset{exampleGCodePreset()},
		}},

		"get-gcode-preset": {Response: exampleGCodePreset()},

		"put-gcode-preset": {
			Headers:  map[string]string{"If-Match": ifMatchVersion},
			Request:  gcodePresetBody{GCodePresetSpec: exampleGCodePreset().GCodePresetSpec},
			Response: exampleGCodePreset(),
		},

		"list-resources": {Response: map[string]any{
			"items": []ResourceInstance{exampleResource(), exampleAssetResource()},
		}},

		"get-resource": {Response: exampleResource()},

		"get-module-memory": {Response: ModuleMemoryPool{
			PoolBytes: new(int64(1 << 30)), CommittedBytes: 384 << 20, Source: "detected",
		}},

		"get-resource-memory": {Response: ResourceMemoryHistory{
			Items: []MemoryPoint{
				{At: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC), Value: new(int64(96 << 20))},
				{At: time.Date(2026, 10, 30, 9, 0, 2, 0, time.UTC)},
			},
			Stops: []ResourceStop{},
		}},

		"get-resource-log": {Response: ResourceLog{Lines: []ResourceLogLine{
			{At: time.Date(2026, 10, 30, 9, 0, 1, 0, time.UTC), Text: "listening on the module socket"},
		}}},

		"put-resource": {
			Headers:  map[string]string{"If-Match": ifMatchVersion},
			Request:  resourceSpecBody{ResourceSpec: exampleResource().ResourceSpec},
			Response: exampleResource(),
		},

		"list-grants": {Response: map[string]any{"items": []Grant{
			{Object: "device:" + examplePrinterID, Relation: "manager", Subject: exampleSubject},
			{Object: "device:" + examplePrinterID, Relation: "viewer", Subject: "group:workshop#member"},
		}}},

		"write-grant": {
			Status:  createdCode,
			Request: grantBody{Relation: "operator", Subject: "user:bob"},
			Response: Grant{
				Object: "device:" + examplePrinterID, Relation: "operator", Subject: "user:bob",
			},
		},

		"list-device-verb-grants": {Response: map[string]any{"items": []VerbGrant{
			{
				Device:  "device:" + examplePrinterID,
				Verb:    "core.command.send_gcode",
				Subject: "user:alice",
			},
		}}},

		"write-device-verb-grant": {
			Status: createdCode,
			Request: verbGrantBody{
				Verb: "core.command.send_gcode", Subject: "user:alice",
			},
			Response: VerbGrant{
				Device:  "device:" + examplePrinterID,
				Verb:    "core.command.send_gcode",
				Subject: "user:alice",
			},
		},

		"accept-declaration-change": {
			Status: createdCode,
			Request: acceptanceBody{
				Module: "github.com/acme/prusa-adapter-shelf",
				Verb:   "acme.io.filament-shelf.command.calibrate_scale",
				From:   "configure",
				To:     "operate",
			},
			Response: map[string]any{
				"kind":   exampleDeviceKind,
				"module": "github.com/acme/prusa-adapter-shelf",
				"verb":   "acme.io.filament-shelf.command.calibrate_scale",
				"to":     "operate",
			},
		},

		"list-declaration-acceptances": {Response: map[string]any{
			"items": []AcceptedChange{{
				Kind:     exampleDeviceKind,
				Module:   "github.com/acme/prusa-adapter-shelf",
				Verb:     "acme.io.filament-shelf.command.calibrate_scale",
				To:       "operate",
				Class:    "configure",
				Markings: "",
			}},
		}},

		// The verbs are what observe and operate reach on a printer, read from
		// the same table the handler reads, so the list is complete.
		"list-subject-grants": {Response: map[string]any{
			"subject": exampleSubject,
			"devices": []SubjectHold{{
				Device:  examplePrinterID,
				Kind:    "printer",
				Actions: []string{"observe", "operate"},
				Verbs: authz.VerbsReached(authz.DeviceKindPrinter,
					authz.MaskOf([]authz.Action{authz.ActionObserve, authz.ActionOperate}), nil),
			}},
		}},

		"list-subject-direct-grants": {Response: map[string]any{
			"subject": exampleSubject,
			"items": []Grant{
				{Object: "device:" + examplePrinterID, Relation: "operator", Subject: exampleSubject},
			},
		}},

		"list-cameras": {Response: map[string]any{"items": []Camera{{
			Name:          exampleCameraName,
			Type:          "generic",
			DeviceID:      examplePrinterID,
			Formats:       []string{"image/jpeg", "multipart/x-mixed-replace"},
			Serving:       true,
			Snapshot:      true,
			Stream:        true,
			MinIntervalMS: exampleCameraIntervalMS,
		}}}},

		"list-camera-settings": {Response: map[string]any{"items": []CameraSetting{exampleCameraSetting()}}},

		"put-camera": {
			Headers:  map[string]string{"If-Match": `"1"`},
			Request:  exampleCameraSetting().CameraSpec,
			Response: exampleCameraSetting(),
		},

		"list-camera-types": {Response: map[string]any{"items": []CameraType{
			{Type: "generic"},
			{
				Type:   "com.github.aux.prusa3d.prusa-connect-buddy3d-camera/buddy3d",
				Title:  "Buddy3D camera",
				Module: "com.github.aux.prusa3d.prusa-connect-buddy3d-camera",
			},
		}}},

		"stream-events": {Media: sseMedia, Response: exampleEventStream()},

		"list-history-facts":        {Response: exampleFactPage()},
		"list-device-history-facts": {Response: exampleFactPage()},

		"list-device-commands": {Response: exampleCommandPage()},
		"get-command":          {Response: exampleCommand()},

		"list-history-samples":        {Response: exampleSamplePage()},
		"list-device-history-samples": {Response: exampleSamplePage()},

		"watch-camera": {
			Media:    sdpMediaType,
			Status:   "201",
			Request:  exampleOffer,
			Response: exampleAnswer,
		},

		"get-developer-connectors": {Response: devtools.Inventory{
			Connectors: []devtools.Connector{{
				Name:      exampleDriver,
				Qualified: "core/" + exampleDriver,
				Namespace: "core",
				Version:   "1.0.0",
				Contract:  "1.0.0",
				Stability: "stable",
				Summary: "The dialect Prusa xBuddy and Buddy boards speak, over the frozen " +
					"registration and websocket wire.",
				Direction: "inbound",
				Status:    devtools.StatusMounted,
				Routes:    []string{"/p/register", "/p/ws"},
				Baseline:  99,
			}},
			Routes: []devtools.Route{
				{
					Pattern: "/p/register",
					Reason: "the driver:xbuddy connector: a path this dialect's firmware has compiled " +
						"in, frozen before this API existed and not updatable on our schedule",
				},
				{
					Pattern: "/p/ws",
					Reason: "the driver:xbuddy connector: a path this dialect's firmware has compiled " +
						"in, frozen before this API existed and not updatable on our schedule",
				},
			},
		}},

		"get-developer-device-commands": {Response: map[string]any{
			"items": []devtools.Command{{
				At:        exampleTime,
				DeviceID:  examplePrinterID,
				Verb:      "core.command.set_nozzle_temperature",
				Args:      map[string]string{"nozzle_temperature": "215"},
				ElapsedMS: 41,
				OK:        true,
			}},
			"dropped": 0,
		}},
	}

	maps.Copy(examples, vocabularyExamples())
	maps.Copy(examples, setupExamples())
	maps.Copy(examples, accountExamples())
	maps.Copy(examples, credentialExamples())
	maps.Copy(examples, appPairingExamples())
	maps.Copy(examples, pairingExamples())
	maps.Copy(examples, connectorEndpointExamples())
	maps.Copy(examples, linkExamples())
	maps.Copy(examples, discoveryExamples())
	maps.Copy(examples, admissionExamples())
	maps.Copy(examples, storageObjectExamples())
	maps.Copy(examples, moduleInstallExamples())
	maps.Copy(examples, dashboardExamples())
	maps.Copy(examples, maintenanceExamples())

	return examples
}

func int64Ptr(v int64) *int64 { return new(v) }

// ErrOperationWithoutExample reports an operation the example table does not
// cover, or an example that does not fit the operation it names.
var ErrOperationWithoutExample = errors.New("httpapi: an operation is published with no example")

// exampleStateValue is the document one module's settings pane keeps: the units
// the person chose. It is an object rather than a string, because that is what
// a pane holding more than one choice stores.
func exampleStateValue() StateValue {
	return StateValue(`{"length":"mm","weight":"g"}`)
}

// exampleStateRecord is that document under the key it is stored at, which is
// what a read answers and what a write answers back.
func exampleStateRecord() ModuleStateRecord {
	return ModuleStateRecord{Key: exampleStateKey, Value: exampleStateValue()}
}

// referenceExample is the API reference path the get-ui-settings example
// shows. It is the path this build serves, so a build that leaves the page out
// publishes an example without it, the same as its handler answers.
func referenceExample() string {
	page, _ := ReferencePage()
	return page
}

// exampleCameraSetting is one camera on the example printer, whose password
// reads back masked.
func exampleCameraSetting() CameraSetting {
	return CameraSetting{
		Name: exampleCameraName,
		CameraSpec: CameraSpec{Type: "rtsp", Config: ResourceConfig(
			`{"printer":"` + examplePrinterID + `","url":"rtsp://camera.local/stream","password":"xxxxx"}`)},
		Version: "1",
	}
}
