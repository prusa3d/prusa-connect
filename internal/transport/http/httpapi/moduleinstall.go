package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// The addresses the marketplace page calls. A POST to the collection installs
// and a DELETE of one namespace under it removes. A module's own routes are
// served under the same subtree, so a module is reached and administered at
// one address.
//
// index, plan, removals, why, why-not and outdated are literal segments. A
// namespace is at least two DNS labels (sdk/extension.Namespace), so no module
// can claim one of them.
const (
	moduleIndexPath     = modulesPathPrefix + "/index"
	modulePlanPath      = modulesPathPrefix + "/plan"
	moduleRemovalPath   = modulesPathPrefix + "/removals/{" + namespaceParam + "}"
	moduleWhyPath       = modulesPathPrefix + "/why/{" + namespaceParam + "}"
	moduleWhyNotPath    = modulesPathPrefix + "/why-not/{" + namespaceParam + "}"
	moduleOutdatedPath  = modulesPathPrefix + "/outdated"
	moduleNamespacePath = modulesPathPrefix + "/{" + namespaceParam + "}"
)

// ModuleInstaller installs a module, takes one away, and says what there is to
// install. It is declared here because this is where it is consumed, and the
// composition root satisfies it with the same functions `module add` and
// `module remove` call, so the command and the API behave the same.
//
// No method takes an instance version. A module install is not a write to one
// declaration a caller read first. It stages a release, reads the manifest and
// writes a declaration that did not exist, so a fencing token has nothing to
// protect.
type ModuleInstaller interface {
	// Install stages the release, checks it and writes the declaration into the
	// machine-owned overlay. Nothing is written until the manifest has been read
	// and the release verified, so a refusal leaves the deployment as it was.
	Install(ctx context.Context, principal string, release ModuleRelease) (ModuleInstallation, error)

	// Remove stops the module's process, deletes what was installed for it, and
	// then takes its declaration out of the overlay. A module the operator
	// declared in their own file is refused: that file is theirs and this
	// server never writes it. A module another installed module requires is
	// refused. withUnused also removes the modules an install added for it
	// that nothing else requires.
	Remove(ctx context.Context, principal string, namespace string, withUnused bool) error

	// Removal says what a removal of a module would meet: the installed
	// modules that require it, which refuse it, and the modules an install
	// added for it that it would leave unused.
	Removal(ctx context.Context, principal string, namespace string) (ModuleRemoval, error)

	// Plan is the modules an install of one release adds, in the order the
	// install writes them.
	Plan(ctx context.Context, principal string, release ModuleReleaseRef) (ModulePlan, error)

	// Why, WhyNot and Outdated answer what composer why, why-not and
	// outdated answer about the installed modules.
	Why(ctx context.Context, principal string, namespace string) ([]ModuleReason, error)
	WhyNot(ctx context.Context, principal string, namespace, version string) ([]ModuleReason, error)
	Outdated(ctx context.Context, principal string) ([]ModuleOutdated, error)

	// Index is the modules an operator can install: the index file in the
	// modules directory, then the configured marketplace's catalogue. A
	// deployment pointing at neither answers an empty list.
	Index(ctx context.Context, principal string) (ModuleIndex, error)

	// Marketplace is the marketplace this deployment reads and whether a person
	// switched it off. ok is false when core.marketplace.url names none.
	Marketplace(ctx context.Context) (market ModuleMarketplace, ok bool, err error)

	// SetMarketplace stores the switches in the machine-owned overlay and
	// applies them at once: switched off, the index lists no marketplace row
	// and nothing is fetched from it. A nil community leaves the community
	// switch as it is.
	SetMarketplace(ctx context.Context, enabled bool, community *bool) (market ModuleMarketplace, ok bool, err error)

	// LeftDatabases is every database of a module's own that no declaration
	// grants, of a kind the caller administers.
	LeftDatabases(ctx context.Context, principal string) ([]ModuleDatabaseLeft, error)

	// PurgeDatabases deletes the databases left behind under one instance, or
	// with forget only forgets their records.
	PurgeDatabases(
		ctx context.Context, principal string, instance ref.Ref, forget bool,
	) ([]ModuleDatabasePurged, error)
}

// ModuleRelease is one install: which module, which release of it, and where
// the bytes are.
//
// The module path is separate from the release because the namespace every
// address of this module is built from is derived from the path and from
// nothing in the manifest.
type ModuleRelease struct {
	Module        string                `json:"module"                   doc:"The module's path, which is the repository it is built from. The namespace every address of this module is built from is derived from it."                                                                                                                                                                                                                            maxLength:"253"  minLength:"1"`
	Version       string                `json:"version"                  doc:"The release to accept, in the range grammar a module author already writes: 1.2.3 names one release, ^1.2.0 names a set, * takes whatever the manifest says. The manifest is held to it."                                                                                                                                                                             maxLength:"128"  minLength:"1"`
	From          string                `json:"from,omitempty"           doc:"Where the bytes are: an https URL of the release archive, which takes sha256, or a directory or .tar.gz on this server's own filesystem. Omitted, the release is the one the configured marketplace offers for this module and version, fetched from where it says and checked against its digest. Fetching a release from a repository by tag is not in this build." maxLength:"2048" minLength:"1"`
	Sha256        string                `json:"sha256,omitempty"         doc:"The SHA-256 of the .tar.gz, as 64 hex digits. Required when from is a URL: a fetched release whose bytes hash to anything else is refused before it is unpacked. Ignored for a path on this server, whose checksums.txt is verified instead."                                                                                                                         maxLength:"64"                 pattern:"^[0-9a-fA-F]{64}$"`
	Approve       bool                  `json:"approve,omitempty"        doc:"Whether to write the permissions and the events the manifest asks for and switch the instance on. Without it the module is installed, switched off, and the answer says what it asked for."`
	RecordsMiB    int                   `json:"records_mib,omitempty"    doc:"With approve, the room to grant the module's records in MiB, in place of what its manifest asks for. Zero grants what it asks for, or on an update keeps a room the operator set, and without approve a value is refused."                                                                                                                                                                                                       minimum:"0" maximum:"1024"`
	MemoryMiB     int                   `json:"memory_mib,omitempty"     doc:"With approve, the memory budget of the module's process in MiB, from 16 to 8192, in place of what its manifest recommends. Zero follows the recommendation, or on an update keeps a budget the operator set, and without approve a value is refused."                                                                                                                                                                            minimum:"0" maximum:"8192"`
	ApproveListen bool                  `json:"approve_listen,omitempty" doc:"With approve, also grant the ports the manifest lists under needs.listen. A port is reachable from the network, so it is approved apart from the other requests. Without it the module is installed without its ports, and requests lists each one."`
	Listen        map[string]ModulePort `json:"listen,omitempty"         doc:"With approve_listen, the port to grant under each name the manifest lists, in place of the port it asks for. The network stays the one the manifest names. A name the manifest does not list is refused."`
	// Database is where the module's own database goes. The operator chooses
	// it and the module does not.
	Database *ModuleDatabaseChoice `json:"database,omitempty" doc:"With approve, where the database of its own goes, for a manifest that asks for one under needs.database. Omitted on a first install, it is a SQLite file in the module's data directory. Omitted on an update, the database the module already has is kept. Refused without approve, and for a manifest that asks for no database."`
	// Settings is what `module add -settings` reads from a file. A module with
	// a required setting does not start without it, so an install that could
	// not carry it would answer failed every time.
	Settings ResourceConfig `json:"settings,omitempty" doc:"The module's settings, written into the same declaration: the keys the manifest declares under options, such as the address of the system it sends to. A key left out takes the manifest's default. A required key with no value stops the start, and the answer's state is failed with the key in reason. On a reinstall of the same module, leaving settings out keeps each stored key the new manifest still declares with the same type, and a credential sent as the marker a read answers keeps the stored credential, as put-resource does. A release under another module path is refused with MODULE_NAME_TAKEN. It is what module add -settings reads from a file."`
	// Expect is what the page showed the operator. The manifest is held to it,
	// so an approve never grants a request the operator did not read.
	Expect *ModuleRequests `json:"expect,omitempty" doc:"With approve, the requests the page showed the operator before they approved, usually copied from the index row. An install whose manifest asks for anything beyond them is refused with MODULE_REQUESTS_CHANGED and nothing is written. Omitted, only a marketplace install is held, to what the marketplace lists for the release."`
	// Dependencies is the plan the person approved.
	Dependencies []ModuleReleaseRef `json:"dependencies,omitempty" doc:"The modules plan-module-install answered for this release, which the person approved with it. The install adds exactly these, in the order the plan gives, with the same approve. A plan that resolves differently now is refused with MODULE_PLAN_CHANGED and nothing is written. Omitted, the install adds nothing and is refused with MODULE_PLAN_CHANGED when the release requires a module this server does not have."`
}

// ModuleReleaseRef names one release of one module.
type ModuleReleaseRef struct {
	Module  string `json:"module"  doc:"The module's path." maxLength:"253" minLength:"1" example:"github.com/acme/threemf"`
	Version string `json:"version" doc:"The release."       maxLength:"64"  minLength:"1" example:"1.4.0"`
	// Sha256 pins the bytes the person approved, so an index that points the
	// same version at other bytes after the plan was read is refused.
	Sha256 string `json:"sha256,omitempty" doc:"The digest the plan answered for this release. The install is refused with MODULE_PLAN_CHANGED when the release it would fetch now has another digest. Omitted only for a release the plan answered without one." maxLength:"64" pattern:"^[0-9a-fA-F]{64}$"`
	// Expect is what the page showed about this module's requests. A module
	// the plan adds is held to it, as the module the person picked is held to
	// its own expect.
	Expect *ModuleRequests `json:"expect,omitempty" doc:"With approve, the requests the plan showed for this module. Its manifest is held to them. An approved install whose plan carries a module without them is refused with MODULE_REQUESTS_CHANGED and nothing is written."`
	// ApproveListen grants this module's ports, which a person approves apart
	// from its other requests.
	ApproveListen bool `json:"approve_listen,omitempty" doc:"With approve, also grant the ports this module lists under needs.listen. Without it the module is installed without its ports."`
}

// ModulePlanned is one module a plan adds.
type ModulePlanned struct {
	ModuleOffer

	RequiredBy []string `json:"required_by" doc:"Who requires this module and with which range, each as module range."`
}

// ModulePlan is the modules an install adds for one release.
type ModulePlan struct {
	Dependencies []ModulePlanned `json:"dependencies" doc:"The modules this server does not have that the release requires, with the releases the resolver chose and every request each one makes, in the order the install writes them. Empty when nothing is missing."`
	// Conflicts is what would refuse the install with
	// MODULE_FILE_TYPE_CONFLICT.
	Conflicts []ModuleFileTypeConflict `json:"conflicts" doc:"The enabled modules that read or draw files of a type the release or a module it adds claims too. While any is listed, an approved install is refused with MODULE_FILE_TYPE_CONFLICT. Empty when none does."`
}

// ModuleFileTypeConflict is two modules that would claim one file type.
type ModuleFileTypeConflict struct {
	Module string   `json:"module" doc:"The namespace of the module the plan installs."                                                        example:"com.github.aux.acme.threemf"`
	With   string   `json:"with"   doc:"The namespace of the enabled module it conflicts with."                                                example:"com.github.aux.other.threemf"`
	Use    string   `json:"use"    doc:"What both do with the files: file_analysis reads them for the server, file_view draws what they hold."                                        enum:"file_analysis,file_view"`
	Types  []string `json:"types"  doc:"The file types both claim, such as .3mf."                                                                                                                                    nullable:"false"`
}

// ModuleReason is one answer of why and why-not.
type ModuleReason struct {
	By         string `json:"by"                   doc:"The namespace of the module that says it, or the operator." example:"com.github.aux.acme.planner"`
	Constraint string `json:"constraint,omitempty" doc:"The constraint it puts, when the reason is one."            example:"^1.2"`
	Text       string `json:"text"                 doc:"The reason, as an operator reads it."                       example:"com.github.aux.acme.planner 1.0.0 requires com.github.aux.acme.threemf ^1.2"`
}

// ModuleOutdated is one installed module against the index.
type ModuleOutdated struct {
	Namespace string `json:"namespace"         doc:"The module's namespace."                                                                           example:"com.github.aux.acme.threemf"`
	Path      string `json:"path,omitempty"    doc:"The module path it was installed under."                                                           example:"github.com/acme/threemf"`
	Installed string `json:"installed"         doc:"The release installed."                                                                            example:"1.2.0"`
	Allowed   string `json:"allowed,omitempty" doc:"The newest release every installed module, the operator's constraint and the stabilities allow."   example:"1.4.0"`
	Latest    string `json:"latest,omitempty"  doc:"The newest release the stabilities allow."                                                         example:"2.0.0"`
	Status    string `json:"status"            doc:"up-to-date, update-possible when allowed is newer than installed, or blocked when only latest is."                                       enum:"up-to-date,update-possible,blocked"`
}

// ModuleUnused is one module a removal would leave unused.
type ModuleUnused struct {
	Instance string `json:"instance"          doc:"The instance, as kind:name."                 example:"sink:threemf"`
	Module   string `json:"module"            doc:"Its namespace."                              example:"com.github.acme.threemf"`
	Version  string `json:"version,omitempty" doc:"Its release."                                example:"1.4.0"`
	Title    string `json:"title,omitempty"   doc:"The name a person reads, from its manifest."`
}

// ModuleRemoval is what a removal of a module would meet.
type ModuleRemoval struct {
	RequiredBy []ModuleDependant `json:"required_by" doc:"The installed modules that require this one. While any is listed, the removal is refused with MODULE_REQUIRED_BY."`
	Unused     []ModuleUnused    `json:"unused"      doc:"The modules an install added for this one that nothing else requires once it is gone. uninstall-module removes them with with_unused, and leaves them installed without it."`
}

// ModuleRequests is what a module asks the operator for: the permissions it
// takes as itself, the events it publishes, whether it may ask about a person
// it is not serving, the routes it answers without a caller, the routes whose
// body it reads from a file, the request headers it reads, the room for its
// records, its ports and its directory.
type ModuleRequests struct {
	Permissions       []string              `json:"permissions,omitempty"         doc:"The actions the module takes as itself."`
	Checks            []string              `json:"checks,omitempty"              doc:"The actions the module asks about for a person it serves, without taking them as itself."`
	Events            map[string]string     `json:"events,omitempty"              doc:"Each endpoint the module publishes on, against the event name after its namespace."`
	CheckAbsentPeople bool                  `json:"check_absent_people,omitempty" doc:"Whether the module may ask about a person it is not serving."`
	ListPeople        bool                  `json:"list_people,omitempty"         doc:"Whether the module may list the people this server holds, with their names."`
	Calls             []string              `json:"calls,omitempty"               doc:"The namespaces of the other modules whose routes the module calls."`
	PublicRoutes      []string              `json:"public_routes,omitempty"       doc:"The routes the module answers without a signed-in caller, each a method and a path under its mount, such as POST /hooks/announce/."`
	StagedRoutes      []ModuleStagedRoute   `json:"staged_routes,omitempty"       doc:"The routes whose request body the server writes into the module's data directory, each with the largest body the module asks for."`
	RequestHeaders    []ModuleRequestHeader `json:"request_headers,omitempty"     doc:"The request headers the server passes to the module's routes beside the ones every module gets, each with the reason the module gives."`
	RecordsMiB        int                   `json:"records_mib,omitempty"         doc:"The room the module asks for its records, in MiB. Zero is the default room."                                                                                                    minimum:"0" maximum:"1024"`
	MemoryMiB         int                   `json:"memory_mib,omitempty"          doc:"The memory budget the module recommends for its process, in MiB. Zero is no recommendation, and the server default of 128 MiB applies."                                         minimum:"0" maximum:"8192"`
	Listen            []ModuleListen        `json:"listen,omitempty"              doc:"The ports the module listens on for a protocol of its own. They are approved with approve_listen."`
	Data              bool                  `json:"data,omitempty"                doc:"Whether the module asks for a writable directory of its own."`
	Database          bool                  `json:"database,omitempty"            doc:"Whether the module asks for a database of its own. The operator chooses where it goes with database on the install."`
	AllowedOrigins    []string              `json:"allowed_origins,omitempty"     doc:"The browser origins the module asks this server to answer, each an exact https origin. A page served from one may call the API from a browser while the module is switched on."                            maxItems:"8" nullable:"false"`
}

// ModuleDatabaseChoice is where an operator puts a module's own database.
type ModuleDatabaseChoice struct {
	Mode string `json:"mode"          doc:"sqlite is a file in the module's data directory. postgres is the database dsn names, which the operator made. provision has this server make a database and a role of its own for the module on this server's own Postgres server, and needs persistence.type postgres." enum:"sqlite,postgres,provision"`
	DSN  string `json:"dsn,omitempty" doc:"With mode postgres, the postgres:// URL of the module's database, with the role and the password the module logs in with. It is stored in this server's own configuration and never answered by a read. Refused with any other mode."                                                                     maxLength:"2048"`
}

// ModulePort is the port an operator grants under one name, in place of the
// port the manifest asks for.
type ModulePort int

// Schema bounds the port the way the manifest's own port is bounded. A struct
// tag on the map field would bound the map, and huma reads no tag for the
// values of a map.
func (ModulePort) Schema(huma.Registry) *huma.Schema {
	first, last := 1.0, 65535.0

	return &huma.Schema{Type: huma.TypeInteger, Format: "int64", Minimum: &first, Maximum: &last}
}

// ModuleStagedRoute is one route whose request body the server writes into a
// module's data directory, with the largest body it takes there.
type ModuleStagedRoute struct {
	Method string `json:"method"  doc:"The HTTP method of the route."                                                                           example:"PUT"`
	Path   string `json:"path"    doc:"The path of the route under the module's mount, where a segment written {name} matches any one segment." example:"/files/{name}"`
	MaxMiB int    `json:"max_mib" doc:"The largest request body on the route, in MiB. A larger body is refused with BODY_TOO_LARGE."            example:"1024"          minimum:"1" maximum:"4096"`
}

// ModuleRequestHeader is one request header a module asks the server to pass
// to its routes (ADR-20261118-nnm).
type ModuleRequestHeader struct {
	Name   string `json:"name"   doc:"The name of the header."                        example:"Print-After-Upload"                                      maxLength:"64"`
	Reason string `json:"reason" doc:"Why the module reads it, as its manifest says." example:"PrusaSlicer 2 says whether to print the file it uploads" maxLength:"200"`
	// Required says the module cannot work without the header.
	Required bool `json:"required,omitempty" doc:"Whether the module cannot work without the header. An install that would not grant it is refused, and the instance does not start without it."`
}

// ModuleListen is one port a module asks to listen on.
type ModuleListen struct {
	Name    string `json:"name"    doc:"The name the manifest gives the port. An install moves a port by this name."   maxLength:"32"`
	Network string `json:"network" doc:"The transport the port is on."                                                                enum:"udp,tcp"`
	Port    int    `json:"port"    doc:"The port the manifest asks for."                                                                             minimum:"1" maximum:"65535"`
	Why     string `json:"why"     doc:"The sentence the manifest gives for the port, for the person who approves it."`
}

// ModuleEgress is one connection a release makes outside Prusa Connect, as its
// manifest declares it under egress. Exactly one of host and option is set.
type ModuleEgress struct {
	Host   string `json:"host,omitempty"   doc:"The host it connects to: an exact name, *.domain one level deep, or an IP address. Absent when option names the host."`
	Option string `json:"option,omitempty" doc:"The setting of the module whose value the operator sets to the host. Absent when host names it."`
	Ports  []int  `json:"ports"            doc:"The ports it connects to."`
	From   string `json:"from"             doc:"Where the connection starts: the module's process on the server, or its page in the person's browser."                 enum:"process,ui"`
	Why    string `json:"why"              doc:"Why the module connects there, in the author's words."`
	Sends  string `json:"sends"            doc:"What leaves the server or the browser on that connection, in the author's words."`
}

// ModuleInstallation is what one install produced: the declaration that was
// written, and what the module asked for and was not given.
type ModuleInstallation struct {
	Instance  string `json:"instance"  doc:"The declared instance, as kind:name. It is what the resources listing and every grant name it by."`
	Type      string `json:"type"      doc:"The registered type the declaration names, which is the namespace and the manifest's name joined."`
	Namespace string `json:"namespace" doc:"The namespace its routes, its bundle and its records are served under."`
	Kind      string `json:"kind"      doc:"The resource kind it is declared under, as the manifest names it."`
	Version   string `json:"version"   doc:"The version the manifest declares."`
	Runtime   string `json:"runtime"   doc:"How the type is reached: service for a module that runs a process, asset for one that is only a bundle."`
	Root      string `json:"root"      doc:"Where the files landed on this server."`
	Enabled   bool   `json:"enabled"   doc:"Whether the instance was switched on. Without approve it is false, and put-resource switches it on once the requests below have been read."`
	Verified  int    `json:"verified"  doc:"How many files were checked against the release's checksum file. Zero is a release that carries none."`
	// Running and Restart describe this server and not the release. A caller
	// reads Running to learn whether the module is up, and a page says
	// "restart to finish" only when Restart is true.
	Running bool `json:"running" doc:"Whether the module is serving now. It is true only when state is ready or degraded. An install made without approve leaves the instance switched off, so it is false until put-resource switches it on."`
	// State is read after the start has had a moment to finish, because a
	// module that refuses its settings fails a moment after the declaration is
	// written, and an answer read before that would call it running.
	State   string `json:"state"            enum:"disabled,pending,starting,ready,degraded,retrying,failed" doc:"The state the module reached, in the words list-resources uses. The install waits a few seconds for the start to finish. ready and degraded are serving. failed and retrying did not start, and reason says why. starting is a start still in progress when the wait ended: read list-resources for the rest. pending is an instance waiting for something it requires, or for the restart that restart asks for. disabled is an install made without approve."`
	Reason  string `json:"reason,omitempty"                                                                 doc:"Why the module did not start, when state is failed or retrying. It is the message list-resources carries on the instance's failed condition, such as a required setting nobody wrote or settings the module refused. A settings change written with put-resource starts the instance again."`
	Restart bool   `json:"restart"                                                                          doc:"Whether this server has to be restarted before the module takes effect. It is false for an install this server reconciled, which is every install it accepts while it is serving. When it is true the declaration is written and the next start reads it."`
	// Approved is what an approved install gave the module, and Withheld is
	// what it asked for and could not be given, in the same words as Requests.
	Approved []string `json:"approved,omitempty" doc:"What an install made with approve gave the module, one per line in the words requests uses: each permission a grant was written for, check_absent_people when the module may ask about a person it is not serving, list_people when it may list the people this server holds, the room granted its records as records <n> MiB, each event, the data directory, the database of its own, and each port granted with approve_listen. Empty without approve."`
	Withheld []string `json:"withheld,omitempty" doc:"What an install made with approve could not give the module: each permission no grant was written for, one per line. That is a permission the organization cannot confer on a module. The module cannot take it as itself, and may still ask access.check whether a person holds it. Write that grant on a printer by hand, or leave the module without it. Empty without approve."`
	// Requests is what the manifest asked for and was not given, one line each,
	// and empty for an install that approved them. Without the list an operator
	// would find the module switched off and see no reason for it.
	Requests []string `json:"requests,omitempty" doc:"What the manifest asks for and has not been given: a permission, more room for its records, an event, a data directory, a database of its own or a port, one per line. Empty for an install made with approve, except a port, which stays here until an install is made with approve_listen. Read them, then install again with approve."`
	// Added is the part of Approved that is new on an update. The rest of an
	// update keeps what the operator decided about the release it replaced.
	// Dependencies is the modules this install added because the release
	// requires them.
	Dependencies []string `json:"dependencies,omitempty" doc:"The modules this install added because the release requires them, as module@version, in the order they were written."`
	Added        []string `json:"added,omitempty"        doc:"On an update made with approve, what this release asks for that the release it replaced did not, and that was granted, in the words requests uses. A permission or an event the operator took away from the module stays away, and a records room the operator set is kept. Empty for a new install."`
	// The facts about a database of the module's own. The module reports the
	// first two when it starts.
	SchemaVersion int      `json:"schema_version,omitempty" doc:"For a module with a database of its own that is running, the schema version its database holds after this start migrated it. Absent for a module that is not running or reports none."`
	Migrations    []string `json:"migrations,omitempty"     doc:"The migrations this start applied to the module's own database, in order, by the name the module's ledger records. They ran in one transaction. Empty when there was nothing to apply."`
	DatabaseLeft  string   `json:"database_left,omitempty"  doc:"On an update that moved the module's own database, or granted none, where the old database is, without its credential. It is kept with its data and listed by list-module-databases until purge-module-database deletes it."`
}

// ModuleOffer is one row of the index: a module that can be installed, and
// whether this deployment already has it.
type ModuleOffer struct {
	Module      string `json:"module"                doc:"The module's path, which is what an install names."`
	Name        string `json:"name"                  doc:"What the module calls itself, for a card heading."`
	Description string `json:"description,omitempty" doc:"One sentence saying what it does."`
	Version     string `json:"version"               doc:"The release the index offers."`
	Source      string `json:"source"                doc:"Where that release's bytes are, which an install passes back as from."`
	Author      string `json:"author,omitempty"      doc:"Who answers for the module, where the index names one. It is the manifest's codeowner, copied into the index row."`
	Kind        string `json:"kind,omitempty"        doc:"The resource kind it is declared under, so a page can say what it plugs into."`
	Installed   bool   `json:"installed"             doc:"Whether this deployment already declares this module. The instance it declares is on the resources listing, with the version it is running."`
	Instance    string `json:"instance,omitempty"    doc:"The instance this deployment declares for it, as kind:name, and absent for a module that is not installed."`
	Namespace   string `json:"namespace,omitempty"   doc:"The namespace derived from the module's path, which uninstall-module takes. Absent for a row whose path names none."`
	// The fields below are what a marketplace row adds. A row from the local
	// index carries origin and, once installed, installed_version.
	Origin           string   `json:"origin,omitempty"            doc:"Where the row comes from: local for the index file in the modules directory, marketplace for the catalogue core.marketplace.url names."                                                     enum:"local,marketplace"`
	InstalledVersion string   `json:"installed_version,omitempty" doc:"The version this deployment has installed, from its declaration. Absent for a module that is not installed. A row whose version is newer is an update."`
	Title            string   `json:"title,omitempty"             doc:"The name a person reads, where the row has one beside the manifest's short name."`
	Sha256           string   `json:"sha256,omitempty"            doc:"The SHA-256 of the release archive, which an install from this row is checked against."`
	Trust            string   `json:"trust,omitempty"             doc:"How far the marketplace vouches for the module: community, verified or official."`
	PageURL          string   `json:"page_url,omitempty"          doc:"The module's page on the marketplace, for a person who wants to read more before installing."`
	Permissions      []string `json:"permissions,omitempty"       doc:"The permissions the release's manifest asks for, which an install with approve grants."`
	Platforms        []string `json:"platforms,omitempty"         doc:"The platforms the release ships a binary for, as goos_goarch. A marketplace row is only offered when this server's own platform is among them."`
	Channel          string   `json:"channel,omitempty"           doc:"The marketplace channel of the release on offer. A marketplace row offers the newest stable release, and is never a beta offered as the update of a stable install. Absent on a local row." enum:"stable,beta"`
	// The requests below complete Permissions, so a page shows exactly what an
	// install with approve grants and sends the same values back as expect.
	Checks            []string              `json:"checks,omitempty"              doc:"The actions the release's manifest asks to ask about for a person, without taking them as itself."`
	Events            map[string]string     `json:"events,omitempty"              doc:"The events the release's manifest publishes: each endpoint against the event name after the namespace."`
	CheckAbsentPeople bool                  `json:"check_absent_people,omitempty" doc:"Whether the release's manifest asks to ask about a person the module is not serving."`
	ListPeople        bool                  `json:"list_people,omitempty"         doc:"Whether the release's manifest asks to list the people this server holds, with their names."`
	Calls             []string              `json:"calls,omitempty"               doc:"The namespaces of the other modules whose routes the release's manifest asks to call."`
	PublicRoutes      []string              `json:"public_routes,omitempty"       doc:"The routes the release's manifest asks to answer without a signed-in caller, each a method and a path under its mount."`
	StagedRoutes      []ModuleStagedRoute   `json:"staged_routes,omitempty"       doc:"The routes whose request body the release's manifest asks the server to write into its data directory, each with the largest body it asks for."`
	RequestHeaders    []ModuleRequestHeader `json:"request_headers,omitempty"     doc:"The request headers the release's manifest asks the server to pass to its routes, each with the reason it gives."`
	RecordsMiB        int                   `json:"records_mib,omitempty"         doc:"The room the release's manifest asks for its records, in MiB. Absent is the default room."`
	MemoryMiB         int                   `json:"memory_mib,omitempty"          doc:"The memory budget the release's manifest recommends for its process, in MiB. Absent is no recommendation, and the server default of 128 MiB applies."`
	Process           bool                  `json:"process"                       doc:"Whether the release runs a process of its own. False for a UI-only module, which has no memory budget."`
	Listen            []ModuleListen        `json:"listen,omitempty"              doc:"The ports the release's manifest asks to listen on. An install grants them only with approve_listen."`
	Data              bool                  `json:"data,omitempty"                doc:"Whether the release's manifest asks for a writable directory of its own."`
	Database          bool                  `json:"database,omitempty"            doc:"Whether the release's manifest asks for a database of its own."`
	AllowedOrigins    []string              `json:"allowed_origins,omitempty"     doc:"The browser origins the release's manifest asks this server to answer, each an exact https origin. An install with approve lets a page served from one call the API from a browser while the module is switched on." maxItems:"8" nullable:"false"`
	// The modules the release depends on.
	Requires  []ResourceRequirement `json:"requires,omitempty"  doc:"The release's require: the core constraint, the capabilities, and the modules it cannot run without, each with a constraint in Composer's grammar. An install adds the modules this server does not have; read the plan first."`
	Suggests  []ResourceRequirement `json:"suggests,omitempty"  doc:"The modules the release works better with, each with the reason. A suggestion installs nothing."`
	Conflicts []ResourceRequirement `json:"conflicts,omitempty" doc:"The releases of other modules the release refuses to run beside, each with the constraint."`
	// FileTypes and ViewTypes are the file types the release reads for the
	// server and draws a file view for.
	FileTypes     []string `json:"file_types,omitempty" doc:"The file types the release reads for the server, from its manifest's file_analysis block."                                                                                                                                                                                                                                                                       maxItems:"16" nullable:"false"`
	ViewTypes     []string `json:"view_types,omitempty" doc:"The file types the release's file-view entries draw for, and * for an entry that draws for every file."                                                                                                                                                                                                                                                          maxItems:"16" nullable:"false"`
	RequestsKnown bool     `json:"requests_known"       doc:"Whether permissions, events, check_absent_people, list_people and records_mib are every request of the release. True for a marketplace row, and for a local row whose index entry states them or whose source is a release directory on this server. When false, install without approve first, read requests in the answer, then approve with those as expect."`
	// What a person reads before installing: the licence and where the release
	// connects outside Prusa Connect.
	License     string         `json:"license,omitempty"      doc:"The licence the release is published under, as its manifest names it. Absent when the release names none."`
	Egress      []ModuleEgress `json:"egress,omitempty"       doc:"Every connection the release makes outside Prusa Connect, as its manifest declares them. Read it with egress_known."`
	EgressKnown bool           `json:"egress_known,omitempty" doc:"Whether the release declares its connections outside Prusa Connect. True with an empty egress means it connects nowhere outside; false or absent means a release that says nothing about them."`
}

// ModuleMarketplace is the marketplace the index read, and how that read went.
type ModuleMarketplace struct {
	URL       string `json:"url"               doc:"The marketplace core.marketplace.url names."`
	Enabled   bool   `json:"enabled"           doc:"Whether this server reads it. False when a person switched it off, and then the index lists none of its rows and nothing is fetched from it."`
	Community bool   `json:"community"         doc:"Whether an organization administrator chose to list the community extensions, the rows whose trust is verified or community. False until one does, and the web app then lists only the official rows and the installed ones. The index still answers every row, and an install that names a hidden one is not refused."`
	BrowseURL string `json:"browse_url"        doc:"Where a person browses its Connect modules in a browser."`
	Reachable bool   `json:"reachable"         doc:"Whether the marketplace answered the last read. False when it has not answered yet or the last refresh failed; the local rows are still listed, and so is a stale copy."`
	ReadAt    string `json:"read_at,omitempty" doc:"When the catalogue the rows come from was read, in RFC 3339. Absent when it has never been read."                                                                                                                                                                                                                       format:"date-time"`
	Problem   string `json:"problem,omitempty" doc:"Why the last read failed, in this server's words. A stale copy of the catalogue may still be listed."`
	Stale     bool   `json:"stale"             doc:"Whether the marketplace rows are a copy kept after the last read failed. reachable is then false and problem says why."`
}

// ModuleIndex is what the index answers: the rows, and the marketplace they
// came from when one is configured.
type ModuleIndex struct {
	Items       []ModuleOffer
	Marketplace *ModuleMarketplace
}

type installModuleInput struct {
	Body ModuleRelease
}

type installModuleOutput struct {
	Status int
	Body   ModuleInstallation
}

type uninstallModuleInput struct {
	Namespace  string `path:"namespace" doc:"The module's namespace, as the resources listing reports it."                                                                                                     maxLength:"253" example:"com.github.aux.acme.planner"`
	WithUnused bool   `                 doc:"Also remove the modules an install added for this one that nothing else requires once it is gone. get-module-removal lists them. Without it they stay installed."                 example:"false"                       query:"with_unused"`
}

type uninstallModuleOutput struct {
	Status int
}

type moduleIndexOutput struct {
	Body struct {
		Items       []ModuleOffer      `json:"items"                 doc:"The modules this deployment can install: the local index in the order it lists them, then the marketplace's rows for modules the local index does not name. Empty when the operator has pointed this server at neither."`
		Marketplace *ModuleMarketplace `json:"marketplace,omitempty" doc:"The marketplace the rows were read from, and whether it answered. Absent when core.marketplace.url is empty."`
	}
}

// moduleInstallErrors are what an install answers besides its body. 400 is a
// release that does not match what was asked for, a manifest that declares
// nothing to install, or a source this build cannot read. 404 is a namespace
// nothing declares. 409 is a module the operator declared in their own file,
// which this server never writes, an uninstall whose files would not go, a
// name another module holds, a release asking for more than was shown, or an
// install while the operator has switched modules off.
// 501 is a build that serves the paths and installs nothing. 502 is a
// marketplace that did not answer, or a release that could not be downloaded.
var moduleInstallErrors = []int{
	http.StatusBadRequest,
	http.StatusUnauthorized,
	http.StatusForbidden,
	http.StatusNotFound,
	http.StatusConflict,
	http.StatusNotImplemented,
	http.StatusBadGateway,
}

// registerModuleInstall publishes installing a module without shell access.
//
// The operations are registered whatever a deployment wired, because the
// document is a contract. A build with no installer publishes the same paths
// and answers each with the refusal that says so.
func registerModuleInstall(api huma.API, cfg Config) {
	registerInstallModule(api, cfg)
	registerModulePlan(api, cfg)
	registerModuleRemoval(api, cfg)
	registerModuleWhy(api, cfg)
	registerModuleWhyNot(api, cfg)
	registerModuleOutdated(api, cfg)
	registerUninstallModule(api, cfg)
	registerModuleIndex(api, cfg)
	registerModuleMarketplace(api, cfg)
	registerModuleDatabases(api, cfg)
}

func registerInstallModule(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "install-module",
		Method:      http.MethodPost,
		Path:        modulesPathPrefix,
		Summary:     "Install a module",
		Description: "Stages a release, reads its manifest, holds the version to the range that was " +
			"asked for, checks this core is one the module runs on, verifies the digests and writes the " +
			"declaration into the machine-owned overlay. It is the `module add` command over HTTP, " +
			"through the same functions and with the same refusals, so a page can install without " +
			"shell access.\n\n" +
			"Nothing is written until every check has passed, so a refusal leaves the deployment " +
			"exactly as it was. Without `approve` the module is installed and switched off, and the " +
			"answer lists the permissions and the events the manifest asks for: read them, then " +
			"install again with `approve` or switch the instance on with put-resource. A module the " +
			"operator declared in their own configuration file is refused, because this server never " +
			"writes that file.\n\n" +
			"The install takes effect at once. This server reads the declaration it has just written " +
			"and converges on it: a module that was switched on starts here, its routes and its bundle " +
			"are served, and list-resources carries the new instance with its `ui` object, so a page " +
			"that reads the listing again draws the module's sidebar entry without anybody restarting " +
			"anything. The answer waits a few seconds for the module to start. `state` says what it " +
			"reached: `ready` when it is serving, `failed` with the cause in `reason` when it did not " +
			"start, and `starting` when the start was still going when the wait ended. A module whose " +
			"settings are missing or refused is `failed`, and the install itself still succeeded: write " +
			"the settings with put-resource, which starts the module again. `running` is true only when the " +
			"module is serving, and `restart` is true only when this server still has to be restarted " +
			"to pick the declaration up.\n\n" +
			"With no `from`, the release is the one the configured marketplace offers for `module` at " +
			"exactly `version`, and its digest is the marketplace's. Every fetched release is hashed " +
			"while it is written and refused with MODULE_DIGEST_MISMATCH when it differs from the " +
			"stated digest, before anything is unpacked. A `from` URL takes `sha256`. A download is " +
			"https, and so is every redirect it follows. It never connects to this machine or the " +
			"local network, except to the port of a marketplace configured on loopback for local " +
			"development. A download that fails answers MODULE_DOWNLOAD_FAILED, with the same detail " +
			"whatever the address answered.\n\n" +
			"With `approve`, the manifest is held to what the operator was shown: what the " +
			"marketplace lists for the release, and `expect` when the request carries it. A manifest " +
			"asking for anything beyond either is refused with MODULE_REQUESTS_CHANGED. A release " +
			"whose name another module already holds is refused with MODULE_NAME_TAKEN.\n\n" +
			"While the operator's core.modules_enabled is false, every install and every update is refused " +
			"with MODULES_DISABLED, and in safe mode SAFE_MODE, before anything is read or downloaded.\n\n" +
			"With `approve`, the install writes the module's grants into the authorization graph, the " +
			"same grants `module add -approve` writes. A module that asks for observe gets the viewer " +
			"role on the organization, so it reads every printer. The grants are written as the caller, " +
			"so the caller needs the admin role on the organization, and an install the graph would " +
			"refuse writes nothing. The same role is needed whenever `approve` records what the module " +
			"may do as itself, even a permission no grant answers, `check_absent_people` and " +
			"`list_people`. " +
			"`approved` lists what was written and `withheld` what was not.\n\n" +
			"A reinstall takes back each grant the old declaration justified and the new manifest no " +
			"longer asks for, and it is refused before anything is written when the caller could not " +
			"take one back. The graph does not record who wrote a grant, so a grant an administrator " +
			"wrote by hand that is the same as one of those is taken back too. Installs and " +
			"removals of modules run one at a time.\n\n" +
			"Installing over an installed module is the update: the running process is stopped " +
			"before its files are replaced. With `approve`, an update keeps what the operator " +
			"decided: a permission or an event they took away stays away, a records room they set is " +
			"kept unless `records_mib` names another, and `added` lists what the release asks for " +
			"that the one before did not.\n\n" +
			"An update is kept only when the new release starts. The release it replaces is kept " +
			"aside until then, and the answer waits up to a minute, because a module may migrate a " +
			"database of its own before it reports ready. A new release that fails to start, such as " +
			"one whose migration failed or one older than its database allows, is taken back: the old " +
			"files, declaration and grants return, the old release runs again, and the answer is " +
			"MODULE_UPDATE_ROLLED_BACK with the cause in reason. A migration that fails is undone whole, " +
			"so the database is as it was. `schema_version` and `migrations` say what a start that " +
			"succeeded did to the module's own database.\n\n" +
			"It takes configure_resource on the resource kind the manifest declares, which is the " +
			"permission to administer that part of this server's configuration.",
		Tags:          []string{"resources"},
		DefaultStatus: http.StatusCreated,
		Errors:        moduleInstallErrors,
		// A release is downloaded inside this request, so it takes the longer
		// budget, and the generated client leaves its own deadline off it.
		Metadata: streams(),
	}, func(ctx context.Context, in *installModuleInput) (*installModuleOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		installed, err := cfg.ModuleInstall.Install(ctx, principalFrom(ctx), in.Body)
		if err != nil {
			return nil, err
		}

		return &installModuleOutput{Status: http.StatusCreated, Body: installed}, nil
	})
}

func registerUninstallModule(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "uninstall-module",
		Method:      http.MethodDelete,
		Path:        moduleNamespacePath,
		Summary:     "Remove an installed module",
		Description: "Stops the process this core is running for the module, deletes the files that " +
			"were installed for it, and then takes its declaration out of the machine-owned overlay. " +
			"The files go before the declaration. A delete that fails then leaves the module " +
			"installed and the same request finishes the job, instead of a deployment with no " +
			"declaration and files nobody will collect.\n\n" +
			"A delete can fail for a moment after the process stops, because the system has not let " +
			"go of the module's own executable yet. The server waits for that and retries; what " +
			"outlasts the wait is answered as a conflict naming the directory that is still there, " +
			"with the module still installed.\n\n" +
			"A module the operator declared in their own configuration file is refused: remove the " +
			"block there instead. A namespace nothing declares is a 404. It takes configure_resource " +
			"on the resource kind the module is declared under.\n\n" +
			"What stays: the records the module kept in the record store, under its namespace, and its " +
			"data directory, module-data/<name> beside the modules directory, both of which the same module " +
			"reads again when it is installed again; a database of the module's own, which " +
			"list-module-databases lists until purge-module-database deletes it; and every file in a storage, " +
			"which belongs to the storage and not to the module. What goes: the module's files under the " +
			"modules directory, its declaration with its settings, and the grants its install wrote.\n\n" +
			"The module's grants are taken back as the principal the server's configuration names, " +
			"core.principal, and not as the caller, because taking a grant away gives nobody anything. " +
			"An install writes them as the caller.\n\n" +
			"The removal takes effect at once. The instance leaves list-resources and the readiness " +
			"report in this same request, so a page that reads either again no longer draws the " +
			"module.\n\n" +
			"A module another installed module requires is refused with MODULE_REQUIRED_BY, whose " +
			"requires names each such module and its range. Remove those modules first. With " +
			"`with_unused`, the modules an install added for this one that nothing else requires any " +
			"more are removed in the same request; get-module-removal lists them first.",
		Tags:          []string{"resources"},
		DefaultStatus: http.StatusNoContent,
		Errors:        moduleInstallErrors,
	}, func(ctx context.Context, in *uninstallModuleInput) (*uninstallModuleOutput, error) {
		if extension.Namespace(in.Namespace).Validate() != nil {
			return nil, httpfault.RouteNotFound(http.MethodDelete, modulesPathPrefix+"/"+in.Namespace)
		}

		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		if err := cfg.ModuleInstall.Remove(ctx, principalFrom(ctx), in.Namespace, in.WithUnused); err != nil {
			return nil, err
		}

		return &uninstallModuleOutput{Status: http.StatusNoContent}, nil
	})
}

func registerModuleIndex(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "list-module-index",
		Method:      http.MethodGet,
		Path:        moduleIndexPath,
		Summary:     "The modules this deployment can install",
		Description: "One row per module an operator can install: what it is called, what it does, the " +
			"release on offer, where that release's bytes are, and whether this deployment already has " +
			"it. `source` is what install-module takes as `from`, so a card installs what it shows. " +
			"When `requests_known` is true, `permissions`, `events`, `check_absent_people`, " +
			"`list_people` and `records_mib` are every request of the release, and a page sends them back as `expect`; " +
			"when it is false, a page installs without `approve` first and reads the answer's " +
			"`requests`.\n\n" +
			"The rows come from the index file in the modules directory and then from the marketplace " +
			"core.marketplace.url names, for this build's version and platform. The marketplace is read " +
			"in the background of a request and kept for a few minutes, so a page that opens this list " +
			"does not wait on it each time; a marketplace that does not answer leaves its last copy, or " +
			"no rows, and the local rows are still served. `marketplace` says which. A deployment " +
			"pointing at neither answers an empty list, which is the honest answer for a server with " +
			"nothing to offer rather than an error a page has to handle. It takes configure_resource on " +
			"a resource kind, because reading what can be installed is part of administering what is " +
			"installed. While the operator's core.modules_enabled is false it answers MODULES_DISABLED " +
			"and reads nothing. In safe mode it answers SAFE_MODE.",
		Tags: []string{"resources"},
		Errors: []int{
			http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict,
			http.StatusNotImplemented, http.StatusInternalServerError,
		},
	}, func(ctx context.Context, _ *struct{}) (*moduleIndexOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		index, err := cfg.ModuleInstall.Index(ctx, principalFrom(ctx))
		if err != nil {
			return nil, err
		}

		out := &moduleIndexOutput{}
		out.Body.Items = index.Items
		out.Body.Marketplace = index.Marketplace

		if out.Body.Items == nil {
			out.Body.Items = []ModuleOffer{}
		}

		return out, nil
	})
}

// requireModuleAdmin refuses a caller who may configure no resource kind that a
// module can be declared under, and a build that wires no installer.
//
// It is the first of two checks, and it runs before anything is fetched or
// unpacked. The kind a release is declared under is in the manifest, which
// can only be read once the bytes are here, so the installer asks the graph
// again about that one kind before it writes anything. Both checks are
// configure_resource. This one limits who can make this server fetch, and the
// installer's decides what they may declare.
func requireModuleAdmin(ctx context.Context, cfg Config) error {
	if cfg.ModuleInstall == nil {
		return httpfault.InstallNotConfigured()
	}

	if cfg.Grants == nil {
		return httpfault.Unexpected("ask the authorization graph who may install a module")
	}

	who, err := caller(ctx)
	if err != nil {
		return err
	}

	for _, kind := range ref.SupervisedKinds() {
		object, objectErr := authz.NewObject(authz.TypeResourceKind, kind.String())
		if objectErr != nil {
			return httpfault.Unexpected("name the " + kind.String() + " resource kind")
		}

		allowed, canErr := cfg.Grants.Can(ctx, who, authz.ActionConfigureResource, object)
		if canErr != nil {
			return httpfault.Unexpected("ask the authorization graph who may install a module")
		}

		if allowed {
			return nil
		}
	}

	return requireKindAdmin(ctx, cfg.Grants, ref.KindAsset, "the modules on this server")
}

type moduleRemovalInput struct {
	Namespace string `path:"namespace" doc:"The module's namespace, as the resources listing reports it." maxLength:"253" example:"com.github.aux.acme.planner"`
}

type moduleRemovalOutput struct {
	Body ModuleRemoval
}

func registerModuleRemoval(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-module-removal",
		Method:      http.MethodGet,
		Path:        moduleRemovalPath,
		Summary:     "What a removal of a module would meet",
		Description: "Answers the installed modules that require this one, which refuse its removal, and the " +
			"modules an install added for it that nothing else would require once it is gone. A page " +
			"reads it before it asks the person, so it can say who still needs the module, or offer to " +
			"remove the unused modules with it. Nothing is written. It takes configure_resource on the " +
			"kind the module is declared under.",
		Tags:   []string{"resources"},
		Errors: moduleInstallErrors,
	}, func(ctx context.Context, in *moduleRemovalInput) (*moduleRemovalOutput, error) {
		if extension.Namespace(in.Namespace).Validate() != nil {
			return nil, httpfault.RouteNotFound(http.MethodGet, modulesPathPrefix+"/removals/"+in.Namespace)
		}

		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		removal, err := cfg.ModuleInstall.Removal(ctx, principalFrom(ctx), in.Namespace)
		if err != nil {
			return nil, err
		}

		return &moduleRemovalOutput{Body: removal}, nil
	})
}

type moduleReasonsOutput struct {
	Body struct {
		Items []ModuleReason `json:"items" doc:"The reasons, empty when there is none."`
	}
}

func registerModuleWhy(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-module-why",
		Method:      http.MethodGet,
		Path:        moduleWhyPath,
		Summary:     "What keeps a module installed",
		Description: "Answers what composer why answers: the operator, when they installed the module " +
			"themselves, and every installed module that requires it, with its constraint. Nothing is " +
			"written. It takes configure_resource on a resource kind, as the module index does.",
		Tags:   []string{"resources"},
		Errors: moduleInstallErrors,
	}, func(ctx context.Context, in *moduleRemovalInput) (*moduleReasonsOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		reasons, err := cfg.ModuleInstall.Why(ctx, principalFrom(ctx), in.Namespace)
		if err != nil {
			return nil, err
		}

		out := &moduleReasonsOutput{}
		out.Body.Items = append([]ModuleReason{}, reasons...)

		return out, nil
	})
}

type moduleWhyNotInput struct {
	Namespace string `path:"namespace" doc:"The module's namespace."  maxLength:"253" example:"com.github.aux.acme.planner"`
	Version   string `                 doc:"The release asked about." maxLength:"64"  example:"2.0.0"                       query:"version" required:"true"`
}

func registerModuleWhyNot(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-module-why-not",
		Method:      http.MethodGet,
		Path:        moduleWhyNotPath,
		Summary:     "What refuses one release of a module",
		Description: "Answers what composer why-not answers: every installed module whose require does not " +
			"hold the release or whose conflict does, and the release's own core constraint, stability, " +
			"require and conflict when the index offers it. An empty list is a release nothing refuses. " +
			"Nothing is written. It takes configure_resource on a resource kind.",
		Tags:   []string{"resources"},
		Errors: moduleInstallErrors,
	}, func(ctx context.Context, in *moduleWhyNotInput) (*moduleReasonsOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		reasons, err := cfg.ModuleInstall.WhyNot(ctx, principalFrom(ctx), in.Namespace, in.Version)
		if err != nil {
			return nil, err
		}

		out := &moduleReasonsOutput{}
		out.Body.Items = append([]ModuleReason{}, reasons...)

		return out, nil
	})
}

type moduleOutdatedOutput struct {
	Body struct {
		Items []ModuleOutdated `json:"items" doc:"Each installed module against the index."`
	}
}

func registerModuleOutdated(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "list-module-outdated",
		Method:      http.MethodGet,
		Path:        moduleOutdatedPath,
		Summary:     "The installed modules against the index",
		Description: "Answers what composer outdated answers: for each installed module the release " +
			"installed, the newest one every constraint and the stabilities allow, and the newest one the " +
			"stabilities allow. Nothing is written. It takes configure_resource on a resource kind.",
		Tags:   []string{"resources"},
		Errors: moduleInstallErrors,
	}, func(ctx context.Context, _ *struct{}) (*moduleOutdatedOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		rows, err := cfg.ModuleInstall.Outdated(ctx, principalFrom(ctx))
		if err != nil {
			return nil, err
		}

		out := &moduleOutdatedOutput{}
		out.Body.Items = append([]ModuleOutdated{}, rows...)

		return out, nil
	})
}
