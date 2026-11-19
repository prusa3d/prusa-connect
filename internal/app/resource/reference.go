package resource

import (
	"reflect"

	"github.com/prusa3d/prusa-connect/internal/app/config"
)

// EntryType is the struct that one instance block under `resources:` decodes
// into. cmd/generate-config-reference reads the key paths and their types from
// it, so the reference page lists the same keys the decoder accepts.
func EntryType() reflect.Type { return reflect.TypeFor[fileEntry]() }

// Keys has one row for each key of an instance block. The map key is the path
// an operator writes. It is a table and not struct tags for the same reason as
// config.Keys.
//
//nolint:gochecknoglobals // the inventory of documented instance keys, read by the generator.
var Keys = map[string]config.Key{
	"resources.<kind>.<name>.type": {
		Doc: "The implementation this instance runs. It cannot change after the instance is created: " +
			"remove the instance and add it again with the new type.",
		Required: "yes",
	},
	"resources.<kind>.<name>.enabled": {
		Doc:     "Run this instance. When false, the instance keeps its settings and its identity and does not run.",
		Default: "`true`",
	},
	"resources.<kind>.<name>.hidden_contributions": {
		Doc: "The places of the module's UI hidden from every viewer, as `<slot>/<id>`, such as " +
			"`file_action/plan` or `pages/<path>`. A hidden page still opens by link. " +
			"A dashboard card cannot be hidden.",
		Default: "none",
	},
	"resources.<kind>.<name>.required": {
		Doc: "Report the whole server as not ready while this instance is not running. It still starts and " +
			"retries the instance. A module instance in the overlay may not set it.",
		Default: "`false`",
	},
	"resources.<kind>.<name>.runtime": {
		Doc: "Where the implementation runs: builtin is inside this binary, service is a child process, " +
			"script is an expression in this file, asset is a module of files and no process.",
		Default: "`builtin`",
	},
	"resources.<kind>.<name>.script": {
		Doc:      "The expression a script instance runs over each event. Event sinks only.",
		Required: "when runtime is script",
	},
	"resources.<kind>.<name>.reads": {
		Doc: "The query verbs this instance answers, where two instances of one kind would both answer them.",
	},
	"resources.<kind>.<name>.config": {
		Doc: "The settings of the implementation. The core passes the block to the implementation and " +
			"does not read it.",
	},
	"resources.<kind>.<name>.service.path": {
		Doc:      "The directory that holds the executable of the child process.",
		Required: "when runtime is service",
	},
	"resources.<kind>.<name>.service.checksum": {
		Doc:      "The sha256 digest of that executable, checked again before every launch.",
		Required: "when runtime is service",
	},
	"resources.<kind>.<name>.service.contract_major": {
		Doc:      "The major version of the plugin protocol the child process uses.",
		Required: "when runtime is service. There is no default, and an instance without it is refused",
	},
	"resources.<kind>.<name>.service.events": {
		Doc: "The endpoint names this module may publish on. Each name maps to the event type the host " +
			"publishes for it. When absent, the module may publish on none.",
	},
	"resources.<kind>.<name>.service.permissions": {
		Doc: "The actions this module may ever take in its own name. The list is an upper limit " +
			"and gives no permission by itself. When absent, the module may take none.",
	},
	"resources.<kind>.<name>.service.checks": {
		Doc: "The actions this module may ask about for a person it serves, such as whether they may " +
			"operate a printer, without taking them itself. It gives no permission by itself.",
	},
	"resources.<kind>.<name>.service.check_absent_people": {
		Doc: "Let this module ask about a person it is not serving, as a queue does after a restart. " +
			"It gives no permission by itself.",
	},
	"resources.<kind>.<name>.service.list_people": {
		Doc: "Let this module list the people this server holds, with their names, as a planner does to " +
			"assign work. It gives no permission by itself.",
	},
	"resources.<kind>.<name>.service.calls": {
		Doc: "The namespaces of the other modules whose routes this module may call. A call in a " +
			"person's name is held to that person's permissions. When absent, it may call none.",
	},
	"resources.<kind>.<name>.service.public_routes": {
		Doc: "The routes anybody may call on this module without signing in, each a method and a path " +
			"such as POST /hooks/announce/. When absent, there are none.",
	},
	"resources.<kind>.<name>.service.staged_routes": {
		Doc: "The routes whose request body the server writes into the module's data directory, each with " +
			"a method, a path and the largest body in MiB, from 1 to 4096.",
	},
	"resources.<kind>.<name>.service.request_headers": {
		Doc: "The extra request headers the server passes to this module's routes, such as " +
			"Print-After-Upload. A credential header such as Cookie is refused. When absent, there are none.",
	},
	"resources.<kind>.<name>.service.records_mib": {
		Doc: "The room this module's records may take, in MiB, from 1 to 1024. When absent, the module " +
			"holds 8 MiB. Its files are in a storage and do not count.",
	},
	"resources.<kind>.<name>.service.memory_mib": {
		Doc: "The memory budget of this module's process, in MiB, from 16 to 8192. When absent, the " +
			"module's recommendation applies, or 128 MiB when it makes none.",
	},
	"resources.<kind>.<name>.service.on_over_memory": {
		Doc: "What the server does over the memory budget: stop kills the process and counts towards " +
			"switching the module off, warn keeps it running. When absent, stop.",
	},
	"resources.<kind>.<name>.service.switch_off.failures": {
		Doc: "How many failed starts or stops inside the window switch this module off, from 0 to 100. " +
			"0 never switches it off. When absent, core.modules_switch_off.failures applies.",
	},
	"resources.<kind>.<name>.service.switch_off.window": {
		Doc: "How far back failed starts and stops count towards switching this module off, a duration " +
			"from 1m to 24h. When absent, core.modules_switch_off.window applies.",
	},
	"resources.<kind>.<name>.service.listen": {
		Doc: "The ports this module may listen on, by the name in its manifest, as udp/<port> or " +
			"tcp/<port>. An operator may move a port here. When absent, there are none.",
	},
	"resources.<kind>.<name>.service.data": {
		Doc: "Give this module a writable directory, module-data/<instance> beside the modules directory. " +
			"An update and a removal keep it.",
	},
	"resources.<kind>.<name>.service.database.engine": {
		Doc: "Where this module's own database is: sqlite for database.sqlite in its data directory, " +
			"postgres for the database dsn names. When the block is absent, it has none.",
	},
	"resources.<kind>.<name>.service.database.dsn": {
		Doc: "With engine postgres, the postgres:// URL the module logs in with. A read never answers it.",
	},
	"resources.<kind>.<name>.service.database.provisioned": {
		Doc: "Whether this server made the database and its role on its own Postgres server. " +
			"A removal leaves both.",
	},
	"resources.<kind>.<name>.service.mount": {
		Doc: "The root path the server serves the module's routes under. When absent, the server serves " +
			"them under the namespace of the module.",
	},
	"resources.<kind>.<name>.service.assets.path": {
		Doc: "The directory the module's browser files are served from.",
	},
	"resources.<kind>.<name>.service.assets.checksum": {
		Doc: "The sha256 digest over those files, checked again before every start.",
	},
	"resources.<kind>.<name>.asset.path": {
		Doc:      "The directory the files of a module that runs no process are served from.",
		Required: "when runtime is asset",
	},
	"resources.<kind>.<name>.asset.checksum": {
		Doc:      "The sha256 digest over those files, checked again before every start.",
		Required: "when runtime is asset",
	},
	"resources.<kind>.<name>.module.version": {
		Doc: "The version from the manifest of the module. The install command writes it.",
	},
	"resources.<kind>.<name>.module.summary": {
		Doc: "The one-line description from the manifest of the module, shown in listings.",
	},
	"resources.<kind>.<name>.module.title": {
		Doc: "The name a person reads, from the manifest of the module. Listings show the manifest name " +
			"where it is empty.",
	},
	"resources.<kind>.<name>.module.requires": {
		Doc: "The manifest's require, each checked before a start: a capability, a core range, another " +
			"module with a range, or a request header the manifest marks required.",
	},
	"resources.<kind>.<name>.module.suggests": {
		Doc: "The modules this module works better with, each with the reason, from the manifest's suggest. " +
			"A suggestion installs nothing.",
	},
	"resources.<kind>.<name>.module.conflicts": {
		Doc: "The releases of other modules this module refuses to run beside, from the manifest's conflict.",
	},
	"resources.<kind>.<name>.module.constraint": {
		Doc: "The constraint the operator asked for when they installed the module, which the lock's " +
			"content hash covers.",
	},
	"resources.<kind>.<name>.module.path": {
		Doc: "The module path the release was installed under, which is what an index row names it by.",
	},
	"resources.<kind>.<name>.module.source": {
		Doc: "Where the release came from, an address or a directory. The lock file records it.",
	},
	"resources.<kind>.<name>.module.digest": {
		Doc: "The sha256 of the release archive, when the source stated one. The lock file records it.",
	},
	"resources.<kind>.<name>.module.pulled": {
		Doc: "True when an install added this module because another module requires it.",
	},
	"resources.<kind>.<name>.module.file_types": {
		Doc: "The file types the module reads for the server, from its manifest's file_analysis block. " +
			"The server asks the module about such a file first.",
	},
	"resources.<kind>.<name>.module.view_types": {
		Doc: "The file types the module's file views draw for, and * for a view of every file. " +
			"Two enabled modules never share one.",
	},
	"resources.<kind>.<name>.module.camera_types": {
		Doc: "The camera types the module offers, from its manifest's camera_types. A camera added as " +
			"<namespace>/<name> runs on the generic camera, and the module feeds it its pictures.",
	},
	"resources.<kind>.<name>.module.options": {
		Doc: "The settings the module declares, each with its type, its default and its description. " +
			"A key in the config block replaces the default of that key alone.",
	},
	"resources.<kind>.<name>.module.author": {
		Doc: "The publisher of the module, shown in listings.",
	},
	"resources.<kind>.<name>.module.channel": {
		Doc: "The marketplace channel the release came from, stable or beta. Empty reads as stable. " +
			"The module index never offers a beta as the update of a stable install.",
	},
	"resources.<kind>.<name>.module.asked.permissions": {
		Doc: "The permissions the manifest asked for when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.events": {
		Doc: "The events the manifest asked to publish when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.checks": {
		Doc: "The actions the manifest asked to ask about when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.check_absent_people": {
		Doc: "Whether the manifest asked to ask about absent people when the operator approved the " +
			"install. An approved update keeps it off when the operator switched it off.",
	},
	"resources.<kind>.<name>.module.asked.list_people": {
		Doc: "Whether the manifest asked to list the people when the operator approved the install. An " +
			"approved update keeps it off when the operator switched it off.",
	},
	"resources.<kind>.<name>.module.asked.calls": {
		Doc: "The modules the manifest asked to call when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.public_routes": {
		Doc: "The routes the manifest asked to serve without a caller when the operator approved the " +
			"install. An approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.staged_routes": {
		Doc: "The staged routes the manifest asked for when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.asked.request_headers": {
		Doc: "The request headers the manifest asked for when the operator approved the install. An " +
			"approved update leaves out each one the operator removed from the service block.",
	},
	"resources.<kind>.<name>.module.allowed_origins": {
		Doc: "The browser origins this module adds to the ones the API answers while it is switched " +
			"on, each an exact https origin.",
		See: "using-the-api-from-a-page/",
	},
	"resources.<kind>.<name>.module.asked.listen": {
		Doc: "The ports the manifest asked for when the operator approved the install. An approved " +
			"update keeps a port grant whose name and network are unchanged.",
	},
	"resources.<kind>.<name>.module.asked.data": {
		Doc: "Whether the manifest asked for a data directory when the operator approved the install.",
	},
	"resources.<kind>.<name>.module.asked.database": {
		Doc: "Whether the manifest asked for a database of its own when the operator approved the install.",
	},
	"resources.<kind>.<name>.module.asked.records_mib": {
		Doc: "The records room the manifest asked for when the operator approved the install. A " +
			"service records_mib that differs from it is the operator's, and an approved update keeps it.",
	},
	"resources.<kind>.<name>.module.asked.memory_mib": {
		Doc: "The memory budget the manifest recommended when the operator approved the install. It " +
			"applies while service memory_mib is absent.",
	},
}
