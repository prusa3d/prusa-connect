package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/upgrade"
	"github.com/prusa3d/prusa-connect/sdk/event"
)

// TestEveryPublishedOperationCarriesAnExample reads the bytes a client
// generates from, not the table they were written in.
//
// It is the gate on what the reference was missing: forty operations, every one
// of them described in prose, and not one of them showing a body. An operation
// whose success answer carries no body is excused by name in
// bodylessOperations, so the exception is a line in a diff.
func TestEveryPublishedOperationCarriesAnExample(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	checked, excused := 0, 0

	for path, item := range document.Paths {
		for method, op := range item {
			if reason, bodyless := bodylessOperations[op.OperationID]; bodyless {
				require.NotEmptyf(t, reason, "%s %s is excused with no reason", method, path)

				excused++

				continue
			}

			checked++

			require.Truef(t, op.hasResponseExample(),
				"%s %s (%s) publishes no example on any answer it can give. A reader opening the "+
					"reference sees an operation they cannot try", method, path, op.OperationID)

			if op.RequestBody != nil {
				require.Truef(t, op.hasRequestExample(),
					"%s %s (%s) takes a body and shows none, so nothing tells a reader what to send",
					method, path, op.OperationID)
			}
		}
	}

	require.GreaterOrEqual(t, checked, 30, "this gate read no operations, or almost none")
	require.NotZero(t, excused, "bodylessOperations is unread, so the exception list is not being checked")
}

// TestNoContentAnswersDeclareNoBody keeps a 204 from promising a body. The
// server sends none, so a client that reads the declared JSON fails on a
// delete that worked.
func TestNoContentAnswersDeclareNoBody(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	checked := 0

	for path, item := range document.Paths {
		for method, op := range item {
			response, declared := op.Responses[strconv.Itoa(http.StatusNoContent)]
			if !declared {
				continue
			}

			checked++

			require.Emptyf(t, response.Content,
				"%s %s (%s) answers 204 and declares a body, which a client then fails to parse",
				method, path, op.OperationID)
		}
	}

	require.NotZero(t, checked, "this gate read no 204 answer")
}

// TestEveryExampleValidatesAgainstItsOwnSchema is the half a table of Go values
// cannot hold on its own: a value of the right type can still carry an enum
// member the schema does not list, break a length bound, or omit a member the
// schema requires.
//
// It runs the same validator the server runs on a request body, over the
// document's own schema, so an example that would be refused as a request or
// that no handler could produce as a response fails here.
func TestEveryExampleValidatesAgainstItsOwnSchema(t *testing.T) {
	t.Parallel()

	server := exampleServer(t)
	registry := server.api.OpenAPI().Components.Schemas

	checked := 0

	for path, item := range server.api.OpenAPI().Paths {
		for method, op := range operationsOfPath(item) {
			if op == nil {
				continue
			}

			for status, response := range op.Responses {
				content, carried := response.Content[jsonMedia]
				if !carried || content.Example == nil {
					continue
				}

				checked++
				requireValid(t, registry, content, huma.ModeReadFromServer,
					method+" "+path+" response "+status)
			}

			if op.RequestBody == nil {
				continue
			}

			content, carried := op.RequestBody.Content[jsonMedia]
			if !carried || content.Example == nil {
				continue
			}

			checked++
			requireValid(t, registry, content, huma.ModeWriteToServer, method+" "+path+" request")
		}
	}

	require.GreaterOrEqual(t, checked, 30, "this gate validated no examples")
}

// TestAVocabularyExampleEntryIsOneTheServerSends is the same property for the
// operations whose answer is too long to publish whole. Every entry in the
// example has to appear, byte for byte, in what the operation actually answers,
// so an invented error code or a capability key nothing declares fails here.
func TestAVocabularyExampleEntryIsOneTheServerSends(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))
	handler := newTestServer(t).Handler()

	checked := 0

	for path, operation := range map[string]string{
		vocabulariesPath:            "list-vocabularies",
		vocabularyErrorCodesPath:    "get-error-code-vocabulary",
		vocabularyEventTypesPath:    "get-event-type-vocabulary",
		vocabularyResourceKindsPath: "get-resource-kind-vocabulary",
		vocabularyCapabilitiesPath:  "get-capability-vocabulary",
		vocabularyAuthorizationPath: "get-authorization-vocabulary",
		vocabularyDeviceIssuesPath:  "get-device-issue-vocabulary",
	} {
		rec := call(t, handler, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		served := rec.Body.String()

		for _, entry := range entriesOf(t, document.exampleFor(t, operation, okStatus)) {
			checked++

			require.Containsf(t, served, entry,
				"the example for %s carries an entry %s never sends:\n  %s", operation, path, entry)
		}
	}

	require.GreaterOrEqual(t, checked, 6, "this gate compared no vocabulary entries")
}

// TestEveryPathParameterCarriesAnExample is the gate ADR-20260910-he3 rule 9 names. A
// rendered sample fills the braces of a path and the values of a query from
// these examples, so a parameter without one leaves a reader with
// GET /api/v1/devices/{device_id} and nothing to put in it.
//
// A path parameter's example is also held to parameterExamples, so the id a
// sample's path names is the id its response is about.
func TestEveryPathParameterCarriesAnExample(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	checked := 0

	for path, item := range document.Paths {
		for method, op := range item {
			for _, param := range op.Parameters {
				if param.In != "path" && param.In != "query" {
					continue
				}

				checked++

				example, carried := param.example()
				require.Truef(t, carried,
					"%s %s: the %s parameter %s carries no example. Add an example tag to its field",
					method, path, param.In, param.Name)

				if param.In != "path" {
					continue
				}

				allowed, listed := parameterExamples[param.Name]
				require.Truef(t, listed,
					"%s %s: the path parameter %s is not in parameterExamples, so nothing holds its "+
						"example to the fixtures", method, path, param.Name)

				var value string

				require.NoErrorf(t, json.Unmarshal(example, &value),
					"%s %s: the example of %s is not a string", method, path, param.Name)
				require.Containsf(t, allowed, value,
					"%s %s: the example of %s is %q and the fixtures use %v",
					method, path, param.Name, value, allowed)
			}
		}
	}

	require.GreaterOrEqual(t, checked, 100, "this gate read no parameters, or almost none")
}

// TestEveryParameterExampleSatisfiesItsOwnSchema runs the validator the server
// runs on a request over each parameter's example, so an example that breaks
// the parameter's own pattern, format, enum or bound fails here rather than in
// a reader's terminal.
func TestEveryParameterExampleSatisfiesItsOwnSchema(t *testing.T) {
	t.Parallel()

	server := exampleServer(t)
	registry := server.api.OpenAPI().Components.Schemas

	checked := 0

	for path, item := range server.api.OpenAPI().Paths {
		for method, op := range operationsOfPath(item) {
			if op == nil {
				continue
			}

			for _, param := range op.Parameters {
				if param.In != "path" && param.In != "query" {
					continue
				}

				require.NotEmptyf(t, param.Schema.Examples, "%s %s: %s carries no example", method, path, param.Name)

				checked++
				requireValidValue(t, registry, param.Schema, param.Schema.Examples[0], huma.ModeWriteToServer,
					method+" "+path+" parameter "+param.Name)
			}
		}
	}

	require.GreaterOrEqual(t, checked, 100, "this gate validated no parameter examples")
}

// TestEveryTagAnOperationCarriesIsDescribed pins the sidebar. A tag used and
// never declared is a group heading with no explanation, in an order OpenAPI
// 3.1 leaves to the tool.
func TestEveryTagAnOperationCarriesIsDescribed(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	described := map[string]bool{}

	for _, tag := range document.Tags {
		require.NotEmptyf(t, tag.Description, "the tag %s is declared with no description", tag.Name)

		_, twice := described[tag.Name]
		require.Falsef(t, twice, "the tag %s is declared twice", tag.Name)

		described[tag.Name] = false
	}

	require.NotEmpty(t, document.Tags, "the document declares no tags, so this gate read nothing")

	for path, item := range document.Paths {
		for method, op := range item {
			require.NotEmptyf(t, op.Tags, "%s %s carries no tag, so it lands outside every group",
				method, path)

			for _, name := range op.Tags {
				_, declared := described[name]
				require.Truef(t, declared,
					"%s %s carries the tag %s and nothing describes it. Add it to apiTags in "+
						"reference.go", method, path, name)

				described[name] = true
			}
		}
	}

	for name, used := range described {
		require.Truef(t, used, "the tag %s is described and no operation carries it", name)
	}
}

// TestEveryOperationSaysWhatCredentialItNeeds reads the requirement a client
// would resolve: the operation's own when it declares one, and the document's
// root when it does not. Either way the scheme it names has to be one the
// document defines, because a client cannot send a credential under a name
// nothing describes.
func TestEveryOperationSaysWhatCredentialItNeeds(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	require.NotEmpty(t, document.Security, "the root requirement is what 29 of 40 operations inherit")

	checked, public := 0, 0

	for path, item := range document.Paths {
		for method, op := range item {
			checked++

			requirements := document.Security
			if op.Security != nil {
				requirements = *op.Security
			}

			if len(requirements) == 0 {
				public++

				require.Containsf(t, publicOperations, strings.ToUpper(method)+" "+path,
					"%s %s publishes no requirement and is not in publicOperations", method, path)

				continue
			}

			for _, requirement := range requirements {
				for scheme := range requirement {
					require.Containsf(t, document.Components.SecuritySchemes, scheme,
						"%s %s requires %q and components.securitySchemes does not define it",
						method, path, scheme)
				}
			}
		}
	}

	require.GreaterOrEqual(t, checked, 30, "this gate read no operations")
	require.Equal(t, len(publicOperations), public)
}

// TestTheLandingPageAndTheBaseUrlArePublished covers the two members the
// reference renders before a reader has opened an operation: the prose on the
// landing page, and the server a request is sent to.
func TestTheLandingPageAndTheBaseUrlArePublished(t *testing.T) {
	t.Parallel()

	document := servedDocument(t, exampleServer(t))

	require.NotEmpty(t, document.Info.Description,
		"Scalar renders info.description as the landing page, so a document without one opens blank")
	require.Contains(t, document.Info.Description, "## ",
		"the landing prose is headed, because Scalar builds sidebar entries from its headings")

	require.NotEmpty(t, document.Servers, "with no server, Scalar selects none and sends a request with no host")
	require.Equal(t, "/", document.Servers[0].URL,
		"the first server has to resolve against the origin serving the document, which is what makes "+
			"one document right for a local run and for a self-hosted deployment")
}

// exampleServer is a server with every operation registered, including the
// camera surface, which is published only when a media plane is wired. The
// example gates read all of them.
func exampleServer(t *testing.T) *Server {
	t.Helper()

	server, err := New(exampleConfig(noPrinters{}, seededStore(), seededPresets()))
	require.NoError(t, err)

	return server
}

func exampleConfig(fleet DeviceFleet, resources ResourceStore, presets GCodePresetStore) Config {
	return Config{
		Version:     "test",
		Principals:  fixedPrincipal("user:alice"),
		Resources:   resources,
		Presets:     presets,
		Endpoints:   seededEndpoints(),
		Grants:      &fakeGrantStore{},
		Access:      exampleAccessReview(),
		Sessions:    newFakeSessions(),
		Accounts:    &fakeAccounts{},
		Readiness:   &fakeReadiness{report: readyReport()},
		Diagnostics: exampleBundle{},
		Devices:     fleet,
		Cameras:     exampleCameras{},

		CameraSettings: &stubCameraSettings{},
		Stores:         noStorages{},
		Events:         noEvents{},
		Maintenance:    exampleMaintenance{},
	}
}

type exampleMaintenance struct{}

func (exampleMaintenance) ServerVersion(context.Context) (ServerVersion, error) {
	return maintenanceExamples()["get-server-version"].Response.(ServerVersion), nil
}

func (exampleMaintenance) UpgradeStatus(context.Context) (upgrade.Journal, error) {
	return exampleUpgradeJournal(), nil
}

func (exampleMaintenance) UpgradeHistory(context.Context) ([]upgrade.Journal, error) {
	return []upgrade.Journal{}, nil
}

func (exampleMaintenance) Releases(context.Context) ([]upgrade.Release, error) {
	return maintenanceExamples()["list-server-upgrade-releases"].Response.([]upgrade.Release), nil
}

func (exampleMaintenance) Stage(context.Context, MaintenanceStage) (upgrade.Journal, error) {
	return exampleUpgradeJournal(), nil
}

func (exampleMaintenance) StageBundle(context.Context, io.Reader, string) (upgrade.Journal, error) {
	return exampleUpgradeJournal(), nil
}

func (exampleMaintenance) Prepare(context.Context) (upgrade.Journal, error) {
	return exampleUpgradeJournal(), nil
}

func (exampleMaintenance) Apply(context.Context, MaintenanceApply) (upgrade.Journal, error) {
	return exampleUpgradeJournal(), nil
}

func (exampleMaintenance) Cancel(context.Context) error { return nil }

func (exampleMaintenance) Backups(context.Context) ([]upgrade.BackupManifest, error) {
	return []upgrade.BackupManifest{exampleBackupManifest()}, nil
}

func (exampleMaintenance) CreateBackup(context.Context, string, bool) (upgrade.BackupManifest, error) {
	return exampleBackupManifest(), nil
}

func (exampleMaintenance) RestoreBackup(context.Context, string) error { return nil }

func (exampleMaintenance) DeleteBackup(context.Context, string) error { return nil }

func (exampleMaintenance) Restart(context.Context, MaintenanceRestart) error { return nil }

// exampleAccessReview holds exactly the answer the check-access example
// publishes, for exactly the printer every other example is about. The example
// and the fleet listing then describe one caller looking at one printer, rather
// than two unrelated readings that happen to sit in the same document.
func exampleAccessReview() *fakeAccessReview {
	return &fakeAccessReview{
		epoch: exampleAuthorizationEpoch,
		held: map[string][]authz.Action{
			"device:" + examplePrinterID: {
				authz.ActionObserve,
				authz.ActionOperate,
			},
		},
	}
}

// exampleCameras publishes the camera surface so the example gates cover the
// five operations a build with no media plane leaves out.
type exampleCameras struct{}

// exampleBundle is a wired support bundle, so get-ui-settings reports one
// wherever the build carries the route.
type exampleBundle struct{}

func (exampleBundle) Served() bool { return true }

func (exampleBundle) Extensions() []string { return []string{"Prusa printer logs"} }

func (exampleBundle) Bundle(context.Context, string, DiagnosticsChoice, []byte) ([]byte, error) {
	return []byte("PK"), nil
}

func (exampleCameras) Cameras(context.Context, string) ([]Camera, error) { return nil, nil }

func (exampleCameras) Snapshot(context.Context, string, string) (CameraSnapshot, error) {
	return CameraSnapshot{}, nil
}

func (exampleCameras) Stream(context.Context, string, string) (CameraStream, error) {
	return CameraStream{}, nil
}

func (exampleCameras) Watch(context.Context, string, string, string) (CameraAnswer, error) {
	return CameraAnswer{}, nil
}

// Live refuses, because a published example is a request a plain HTTP client
// sends, and the live view is a socket only a websocket client opens.
func (exampleCameras) Live(_ context.Context, _ string, name string) (CameraLive, error) {
	return nil, CameraNotFound(name)
}

func (exampleCameras) Push(context.Context, string, string, CameraFrame) error { return nil }

func (exampleCameras) CameraTypes(context.Context, string) ([]CameraType, error) { return nil, nil }

// exampleResourceStore answers the resource operations with the instance the
// examples publish, and refuses everything else.
type exampleResourceStore struct{ ResourceStore }

func (exampleResourceStore) List(context.Context, string, ref.Kind) (ResourceListing, error) {
	return ResourceListing{
		Items:   []ResourceInstance{exampleResource(), exampleAssetResource()},
		Version: "v7",
	}, nil
}

func (exampleResourceStore) Get(_ context.Context, _ string, _ ref.Ref) (ResourceInstance, error) {
	return exampleResource(), nil
}

// examplePresetStore answers the preset operations with the preset the examples
// publish.
type examplePresetStore struct{ GCodePresetStore }

func (examplePresetStore) ListPresets(context.Context, string) (GCodePresetListing, error) {
	return GCodePresetListing{Items: []GCodePreset{exampleGCodePreset()}, Version: "v7"}, nil
}

func (examplePresetStore) GetPreset(context.Context, string, string) (GCodePreset, error) {
	return exampleGCodePreset(), nil
}

// servedDocument decodes the document off the wire, so every gate above reads
// what a client reads rather than the object it was rendered from.
func servedDocument(t *testing.T, server *Server) publishedDocument {
	t.Helper()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, specJSONPath, nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var document publishedDocument

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &document))

	return document
}

// publishedDocument is the document as a client decodes it. It carries only the
// members the example and tag gates read.
type publishedDocument struct {
	Info struct {
		Description string `json:"description"`
	} `json:"info"`
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Tags []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tags"`
	Security   []map[string][]string `json:"security"`
	Components struct {
		SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
	} `json:"components"`
	Paths map[string]publishedPath `json:"paths"`
}

// publishedPath is one path item as a client decodes it, keyed by method.
//
// A path item may carry members that are not operations: a summary, a
// description and the parameters every operation on it shares. The file
// family under a module's mount is published that way and carries no
// operation at all, so a decode that took every member for an operation would
// fail on it. Only the methods are kept, which is what every gate here reads.
type publishedPath map[string]publishedOperation

// publishedMethods are the members of a path item that are operations. Huma
// writes no others.
var publishedMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace,
}

func (p *publishedPath) UnmarshalJSON(data []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return fmt.Errorf("decode the path item: %w", err)
	}

	*p = publishedPath{}

	for name, raw := range members {
		if !slices.Contains(publishedMethods, strings.ToUpper(name)) {
			continue
		}

		var op publishedOperation
		if err := json.Unmarshal(raw, &op); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}

		(*p)[name] = op
	}

	return nil
}

type publishedOperation struct {
	OperationID   string                 `json:"operationId"`
	Tags          []string               `json:"tags"`
	Security      *[]map[string][]string `json:"security"`
	SampleQuery   string                 `json:"x-sample-query"`
	SampleHeaders map[string]string      `json:"x-sample-headers"`
	Parameters    []publishedParameter   `json:"parameters"`
	RequestBody   *publishedBody         `json:"requestBody"`
	Responses     map[string]struct {
		Content map[string]publishedMedia `json:"content"`
	} `json:"responses"`
}

// publishedParameter is one parameter as a client decodes it. Huma writes a
// field's example tag into the schema's examples list, and a parameter object
// may also carry one of its own, so the gate reads both places.
type publishedParameter struct {
	Name    string          `json:"name"`
	In      string          `json:"in"`
	Example json.RawMessage `json:"example"`
	Schema  struct {
		Examples []json.RawMessage `json:"examples"`
	} `json:"schema"`
}

// example is the value a renderer would fill the parameter with, and whether
// there is one.
func (p publishedParameter) example() (json.RawMessage, bool) {
	candidates := append([]json.RawMessage{p.Example}, p.Schema.Examples...)

	for _, candidate := range candidates {
		if len(candidate) > 0 {
			return candidate, true
		}
	}

	return nil, false
}

type publishedBody struct {
	Content map[string]publishedMedia `json:"content"`
}

type publishedMedia struct {
	Example json.RawMessage `json:"example"`
}

func (op publishedOperation) hasResponseExample() bool {
	for _, response := range op.Responses {
		for _, media := range response.Content {
			if len(media.Example) > 0 {
				return true
			}
		}
	}

	return false
}

func (op publishedOperation) hasRequestExample() bool {
	for _, media := range op.RequestBody.Content {
		if len(media.Example) > 0 {
			return true
		}
	}

	return false
}

// exampleFor is the example one operation publishes on one status, as the JSON
// text a client would read.
func (d publishedDocument) exampleFor(t *testing.T, operationID, status string) string {
	t.Helper()

	for _, item := range d.Paths {
		for _, op := range item {
			if op.OperationID != operationID {
				continue
			}

			media, carried := op.Responses[status].Content[jsonMedia]
			require.Truef(t, carried, "%s answers %s with no JSON body", operationID, status)
			require.NotEmptyf(t, media.Example, "%s publishes no example on %s", operationID, status)

			return string(media.Example)
		}
	}

	t.Fatalf("the document publishes no operation called %s", operationID)

	return ""
}

// entriesOf is every entry of every list in a vocabulary example, as the JSON
// text each one was published as. It is what the served answer has to contain.
func entriesOf(t *testing.T, example string) []string {
	t.Helper()

	var body map[string]json.RawMessage

	require.NoError(t, json.Unmarshal([]byte(example), &body))

	var entries []string

	for _, value := range body {
		var list []json.RawMessage
		if json.Unmarshal(value, &list) != nil {
			continue
		}

		for _, item := range list {
			entries = append(entries, string(item))
		}
	}

	return entries
}

// requireValid runs the document's own schema over one example, in the
// direction that example travels.
func requireValid(t *testing.T, registry huma.Registry, content *huma.MediaType, mode huma.ValidateMode, where string) {
	t.Helper()

	requireValidValue(t, registry, content.Schema, content.Example, mode, where)
}

// requireValidValue runs one schema over one example, as the value a client
// would decode rather than the Go value it was written as.
func requireValidValue(
	t *testing.T, registry huma.Registry, schema *huma.Schema, example any, mode huma.ValidateMode, where string,
) {
	t.Helper()

	encoded, err := json.Marshal(example)
	require.NoErrorf(t, err, "%s: the example cannot be encoded", where)

	var value any

	require.NoError(t, json.Unmarshal(encoded, &value))

	result := &huma.ValidateResult{}
	huma.Validate(registry, schema, huma.NewPathBuffer(nil, 0), mode, value, result)

	require.Emptyf(t, result.Errors, "%s: the published example does not satisfy the published schema", where)
}

// TestTheEventStreamExampleIsAParseableStream is not a gate on the frames'
// content, which no fixture can produce, but on the shape a reader parses: the
// retry frame first, then a telemetry frame with no id, then two state frames
// with one each.
func TestTheEventStreamExampleIsAParseableStream(t *testing.T) {
	t.Parallel()

	const newline = "\n"

	blank := newline + newline

	frames := strings.Split(strings.TrimSuffix(exampleEventStream(), blank), blank)
	require.Len(t, frames, 4,
		"the example shows a retry frame, a telemetry frame and two state frames")

	require.Equal(t, "retry: "+strconv.FormatInt(defaultRetry.Milliseconds(), 10), frames[0],
		"the first frame sets the reconnection delay this deployment hands a reader")
	require.NotContains(t, frames[1], "id: ",
		"telemetry holds no place to resume from, so an id on it is a position a reader cannot use")
	require.Contains(t, frames[2], "id: ", "a durable event carries the place to resume from")
	require.Contains(t, frames[3], "id: ", "a durable event carries the place to resume from")

	for _, frame := range frames[1:] {
		lines := strings.Split(frame, newline)

		data, found := strings.CutPrefix(lines[len(lines)-1], "data: ")
		require.True(t, found, "every event frame carries a data member")
		require.True(t, json.Valid([]byte(data)), "the data member is a CloudEvents envelope")
	}
}

// TestTheEventStreamExampleCarriesWhatTheHubPublishes holds the frames'
// content against the two places that build a real one: the hub writes the
// subject from the origin, and internal/app builds the payload. A client
// generator reads these examples, so a member spelled some other way here is a
// client written to a shape nothing sends.
func TestTheEventStreamExampleCarriesWhatTheHubPublishes(t *testing.T) {
	t.Parallel()

	const newline = "\n"

	blank := newline + newline

	origin, err := event.NewOrigin(event.KindDevice, examplePrinterID)
	require.NoError(t, err)

	frames := strings.Split(strings.TrimSuffix(exampleEventStream(), blank), blank)
	require.Len(t, frames, 4)

	envelopes := make([]event.Envelope, 0, 3)

	for _, frame := range frames[1:] {
		lines := strings.Split(frame, newline)

		data, found := strings.CutPrefix(lines[len(lines)-1], "data: ")
		require.True(t, found)

		var envelope event.Envelope
		require.NoError(t, json.Unmarshal([]byte(data), &envelope))

		envelopes = append(envelopes, envelope)
	}

	for _, envelope := range envelopes {
		require.Equal(t, origin.String(), envelope.Subject,
			"the hub writes the subject from the origin, so it carries the kind")
	}

	var sample struct {
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}

	require.NoError(t, json.Unmarshal(envelopes[0].Data, &sample))
	require.NotEmpty(t, sample.Path, "a telemetry payload is one path and the value it now reads")
	require.NotEmpty(t, sample.Value)

	var presence map[string]any

	require.NoError(t, json.Unmarshal(envelopes[1].Data, &presence))
	require.Contains(t, presence, "device_id", "a device frame names the machine device_id")
	require.Contains(t, presence, "connected",
		"presence reports whether the device is reachable under connected")

	// The print frame is built from the payload type internal/app publishes, so
	// decoding it back into that type is what holds the example to the code.
	var settled event.JobSettled

	require.NoError(t, json.Unmarshal(envelopes[2].Data, &settled))
	require.NotEmpty(t, settled.JobID, "a print frame names the print it is about")
	require.Equal(t, event.JobFinished, settled.Outcome,
		"the outcome is one of the four sdk/event declares")
	require.Equal(t, settled.JobID, envelopes[2].CausationID,
		"the two frames of one print are joined by the job id")
	require.False(t, settled.Ended.Before(settled.Started))
}
