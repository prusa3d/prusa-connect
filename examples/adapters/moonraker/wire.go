package moonraker

import "encoding/json"

// jsonrpcVersion is the version every frame on this wire carries.
const jsonrpcVersion = "2.0"

// The JSON-RPC methods this dialect calls. Moonraker serves the same names over
// the websocket and over HTTP, so these are the API and not a framing of it.
const (
	methodPrinterInfo          = "printer.info"
	methodObjectsList          = "printer.objects.list"
	methodObjectsQuery         = "printer.objects.query"
	methodObjectsSubscribe     = "printer.objects.subscribe"
	methodPrintStart           = "printer.print.start"
	methodPrintPause           = "printer.print.pause"
	methodPrintResume          = "printer.print.resume"
	methodPrintCancel          = "printer.print.cancel"
	methodGcodeScript          = "printer.gcode.script"
	methodFilesRoots           = "server.files.roots"
	methodFilesGetDirectory    = "server.files.get_directory"
	methodFilesMetadata        = "server.files.metadata"
	methodFilesPostDirectory   = "server.files.post_directory"
	methodFilesDeleteFile      = "server.files.delete_file"
	methodFilesDeleteDirectory = "server.files.delete_directory"
)

// notifyStatusUpdate is the notification a subscription arrives as. Its params
// are the changed status and the Klipper event time, in that order.
const notifyStatusUpdate = "notify_status_update"

// The Klipper objects this dialect reads. Each one is a section in the
// machine's own configuration, so a machine that has not got the section does
// not report the object either.
const (
	objectPrintStats    = "print_stats"
	objectDisplayStatus = "display_status"
	objectExtruder      = "extruder"
	objectHeaterBed     = "heater_bed"
	objectToolhead      = "toolhead"
	objectGcodeMove     = "gcode_move"
	objectPauseResume   = "pause_resume"
	objectConfigFile    = "configfile"
	objectVirtualSDCard = "virtual_sdcard"
)

// subscribed is every object whose readings become state patches, in the order
// a subscription names them.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var subscribed = []string{
	objectPrintStats, objectDisplayStatus, objectExtruder,
	objectHeaterBed, objectToolhead, objectGcodeMove,
}

// rpcRequest is one call on its way to the machine.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      uint64 `json:"id"`
}

// rpcFrame is anything the machine sends. A frame carrying an id answers a
// call; one carrying a method and no id is a notification nobody asked for.
type rpcFrame struct {
	ID     *uint64         `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Params json.RawMessage `json:"params"`
}

// rpcError is the machine's own refusal, passed on as it spelled it.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// printerInfo is what printer.info answers. The fields this dialect does not
// read are left out rather than carried, because a field nothing uses is a
// field nobody keeps true.
type printerInfo struct {
	State           string `json:"state"`
	StateMessage    string `json:"state_message"`
	Hostname        string `json:"hostname"`
	SoftwareVersion string `json:"software_version"`
}

// objectList is what printer.objects.list answers: every Klipper object this
// machine has.
type objectList struct {
	Objects []string `json:"objects"`
}

// statusReply is what a query and a subscription both answer: one reading of
// every object that was asked for.
type statusReply struct {
	EventTime float64                    `json:"eventtime"`
	Status    map[string]json.RawMessage `json:"status"`
}

// configDump is the machine's own configuration as Klipper parsed it, keyed by
// section. Only the two heater sections are read, so the rest stays raw.
type configDump struct {
	Settings map[string]json.RawMessage `json:"settings"`
}

// heaterConfig is the pair of bounds a Klipper heater section declares. Both
// are pointers so that a section leaving one out is told apart from one
// declaring zero.
type heaterConfig struct {
	MinTemp *float64 `json:"min_temp"`
	MaxTemp *float64 `json:"max_temp"`
}

// fileRoot is one entry returned by server.files.roots.
type fileRoot struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Permissions string `json:"permissions"`
}

// diskUsage is the storage space summary returned by server.files.get_directory.
type diskUsage struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Free  int64 `json:"free"`
}

// moonrakerDir is one subdirectory returned by server.files.get_directory.
type moonrakerDir struct {
	Dirname     string  `json:"dirname"`
	Modified    float64 `json:"modified"`
	Size        int64   `json:"size"`
	Permissions string  `json:"permissions"`
}

// moonrakerThumbnail is one preview image entry returned by Moonraker.
type moonrakerThumbnail struct {
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Size         int64  `json:"size"`
	RelativePath string `json:"relative_path"`
}

// moonrakerFile is one file entry returned by server.files.get_directory.
type moonrakerFile struct {
	Filename    string               `json:"filename"`
	Modified    float64              `json:"modified"`
	Size        int64                `json:"size"`
	Permissions string               `json:"permissions"`
	Thumbnails  []moonrakerThumbnail `json:"thumbnails"`
}

// directoryReply is what server.files.get_directory answers.
type directoryReply struct {
	Dirs      []moonrakerDir  `json:"dirs"`
	Files     []moonrakerFile `json:"files"`
	DiskUsage *diskUsage      `json:"disk_usage"`
	RootInfo  *fileRoot       `json:"root_info"`
}

// fileMetadataReply is what server.files.metadata answers for one file.
type fileMetadataReply struct {
	Size        int64                `json:"size"`
	Modified    float64              `json:"modified"`
	Permissions string               `json:"permissions"`
	Thumbnails  []moonrakerThumbnail `json:"thumbnails"`
}

// objectsParam is the params both query and subscribe take. A nil value asks
// for every field of that object.
func objectsParam(objects []string) map[string]any {
	asked := make(map[string]any, len(objects))
	for _, object := range objects {
		asked[object] = nil
	}

	return map[string]any{"objects": asked}
}
