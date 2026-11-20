package httpapi

// bodylessOperations are the operations whose successful answer carries nothing
// a worked example could show, each with the reason. The exceptions to the
// example gate are in this one list, so adding one is a line a reviewer sees
// in a diff.
//
// gosec G101 matches the -login keys. The values are prose describing an empty
// answer.
//
//nolint:gochecknoglobals,gosec // a lookup table, read-only after init. G101 matches
var bodylessOperations = map[string]string{
	"begin-login":                "303 to the issuer. The body is empty and the answer is the Location header",
	"complete-login":             "303 back to the application, with the session in a Set-Cookie header",
	"logout":                     "204. The answer is the expired cookie, not a body",
	"delete-resource":            "204. A delete that answered a body would be describing something that is gone",
	"delete-camera":              "204. A delete that answered a body would be describing something that is gone",
	"delete-gcode-preset":        "204, for the same reason the resource delete is",
	"delete-dashboard":           "204, for the same reason the resource delete is",
	"delete-connector-endpoint":  "204, for the same reason the resource delete is",
	"withdraw-grant":             "204, for the same reason the resource delete is",
	"withdraw-device-verb-grant": "204, for the same reason withdrawing a tuple is",
	"restart-resource":           "204. The instance comes back as the configuration in force declares it, and the answer says only that it was restarted",
	"get-printer-authority":      "one DER certificate. A worked example of binary bytes is the file, and it does not belong in a contract document",
	"get-camera-snapshot":        "one JPEG or PNG frame. A worked example of an image is the image, and it does not belong in a contract document",
	"put-camera-snapshot":        "204, and the body it takes is one JPEG. A worked example of an image is the image, and it does not belong in a contract document",
	"stream-camera":              "a multipart stream of those frames, for as long as the viewer holds it",
	"live-camera":                "a websocket of binary pictures, which no example in a document can show",
	"get-developer-view":         "one self-contained HTML page. It is read in a browser rather than parsed",
	"delete-device-file":         "204, for the same reason the resource delete is",
	"delete-device-folder":       "204, for the same reason the resource delete is",
	"delete-storage-entry":       "204, for the same reason the resource delete is",
	"delete-storage-entries":     "204, for the same reason the resource delete is",
	"download-storage-object":    "the file's own bytes. A worked example of a file is the file, and it does not belong in a contract document",
	"create-diagnostics-bundle":  "one zip archive, for the same reason a stored file is",
	"get-storage-thumbnail":      "one PNG or JPEG picture, for the same reason the camera's still image is",
	"get-device-file-thumbnail":  "one PNG or JPEG picture, for the same reason the camera's still image is",
	"get-device-model-artwork":   "one SVG, PNG or WebP drawing, for the same reason the camera's still image is",
	"stop-device-transfer":       "204. Stopping a transfer is the whole answer, and the transfer's own record says how it ended",
	"delete-account":             "204, for the same reason the resource delete is",
	"reject-registration":        "204, for the same reason the resource delete is",
	"set-account-password":       "204. A password change that answered a body would be describing the password",
	"set-account-role":           "204. The role is what the caller sent, and the account listing is where a page reads it back",
	"revoke-credential":          "204, for the same reason the resource delete is",
	"approve-app-pairing":        "204. The credential goes to the app on its next poll, and the person's listing is where it shows",
	"deny-app-pairing":           "204. The app is told on its next poll",
	"delete-pairing":             "204, for the same reason the resource delete is",
	"delete-module-state":        "204, for the same reason the resource delete is",
	"uninstall-module":           "204, for the same reason the resource delete is",
	"retire-device":              "204. Retiring keeps the record, so the device's own read still answers and is where a client looks afterwards. The printer loses its access and has to be added again",
	"cancel-server-upgrade":      "204. The staged transaction and its temporary files are gone",
	"restore-server-backup":      "202. Restoration is the operation and a fresh listing describes the retained backup",
	"delete-server-backup":       "204, for the same reason the resource delete is",
	"disconnect-beta-portal":     "204. Disconnecting returns no representation of the removed authorization",
}
