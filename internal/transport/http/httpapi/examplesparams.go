package httpapi

// The values a path parameter's example carries, one per thing the API names.
//
// A parameter's example is a struct tag, and a tag is a literal: it cannot name
// a Go constant. So the value is written twice, once in the tag beside the
// field and once here, and TestEveryPathParameterCarriesAnExample holds the two
// together. The fixtures use these constants, so the id in a rendered sample's
// path is the id in the response beside it.
const (
	// exampleAccountID is the colleague's account, the one an operator would
	// delete or reset. Deleting the owner's own account is not the sample to
	// show.
	exampleAccountID = "u4k2p9z1m7x3c8v5b0nq"
	// exampleCameraName is the camera on the example printer.
	exampleCameraName = "bench-top"
	// exampleCameraIntervalMS is the cadence that camera declares. It is the
	// mjpeg driver's shipped default, ten seconds, so the sample shows the
	// number an operator meets rather than a fast one.
	exampleCameraIntervalMS = 10_000
	// exampleCommandID is the invocation the command examples record.
	exampleCommandID = "01K3Q7W0YQ4S2C4H2S8W9YQ7V1"
	// exampleJobID is the print the example printer is running, as the jobs
	// read reports it and as a link to the print carries it.
	exampleJobID = "0199e3a4-5bb5-7aac-8a2c-9c3982e6373f"
	// exampleDeviceKind is the qualified kind the acceptance examples are
	// written against. It carries a slash, which a caller escapes in the path.
	exampleDeviceKind = "acme.io/filament-shelf"
	// exampleEndpointName is the connector endpoint the endpoint examples
	// declare.
	exampleEndpointName = "bench-printer"
	// exampleLinkName is the link the link examples publish, and exampleNodeID
	// is the one node under it.
	exampleLinkName = "parent"
	exampleNodeID   = "6f1c0a2d5b8e4711"
	// exampleNamespace is the module namespace the state and OpenAPI examples
	// describe.
	exampleNamespace = "com.github.aux.acme.planner"
	// examplePairingProvider and examplePairedSubject are the pairing the
	// pairing examples write and delete: an identity provider's name, and the
	// subject as that provider's token spells it.
	examplePairingProvider = "prusa-account"
	examplePairedSubject   = "8f1e1b2c-0000-4000-8000-abcdefabcdef"
	// exampleStateKey is the record the module settings examples read and
	// write, under the namespace above.
	exampleStateKey = "units"
	// examplePresetName is the preset the preset examples publish.
	examplePresetName = "preheat-pla"
	// exampleRegistrationCode is the code the pending registration displays.
	exampleRegistrationCode = "K7PQ2XR4MN"
	// exampleResourceKind and exampleResourceName are the one resource instance
	// the resource examples publish.
	exampleResourceKind = "storage"
	exampleResourceName = "local"
	// exampleStorageName is the storage the file examples browse. It is the
	// resource above, under the name it is declared by.
	exampleStorageName = exampleResourceName
	// exampleSubject is the principal every grant example is about.
	exampleSubject = "user:alice"
	// exampleTransferID is the transfer the file examples follow.
	exampleTransferID = "01M1GRZD931RAKEXTQ7V15GC1K"
	// exampleDeclaredModel is a model a driver declared rather than one the
	// core carries a profile for, which is the only kind with a drawing.
	exampleDeclaredModel = "Bench_One"
)

// parameterExamples is the value every path parameter's example has to be, by
// parameter name. A name that means different things on different routes lists
// every value it may carry.
//
// It is what makes the examples one story: a reader following device_id from
// the listing into a command sees the same printer. A path parameter whose
// name is not here fails the gate, so a new route records its value here.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var parameterExamples = map[string][]string{
	"account":     {exampleAccountID},
	"camera":      {exampleCameraName},
	"code":        {exampleRegistrationCode},
	"command_id":  {exampleCommandID},
	"credential":  {exampleCredentialID},
	"dashboard":   {exampleDashboardID},
	"device_id":   {examplePrinterID},
	"id":          {exampleNodeID},
	"job_id":      {exampleJobID},
	"key":         {exampleStateKey},
	"model":       {exampleDeclaredModel},
	"kind":        {exampleResourceKind, exampleDeviceKind, exampleLeftKind},
	"name":        {exampleResourceName, examplePresetName, exampleEndpointName, exampleLinkName, exampleLeftName},
	"namespace":   {exampleNamespace},
	"object_id":   {examplePrinterID},
	"object_type": {"device"},
	"pairing_id":  {examplePrinterPairingID},
	"preset":      {examplePresetName},
	"provider":    {examplePairingProvider},
	"storage":     {exampleStorageName},
	"subject":     {exampleSubject, examplePairedSubject},
	"transfer_id": {exampleTransferID},
	"user_code":   {exampleUserCode},
}
