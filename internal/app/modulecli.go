package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/moduledeps"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// The commands that install a module, list what is installed and take one away.
//
// A module with a process half and a browser half is one repository, one
// module.yaml, one release and one declaration. These three commands are the
// whole of what an operator types to get there, and none of them edits the
// operator's own configuration file: the declaration goes into the
// machine-owned overlay, which is the file this server writes and they do not.

// ModuleCommand is the word that selects the module commands.
const ModuleCommand = "module"

// The two verbs beside list.
const (
	addVerb    = "add"
	removeVerb = "remove"
)

// versionSeparator divides the module path from the release in the one
// argument `module add` takes. It is Go's own spelling, which is what an author
// already writes for the same pair.
const versionSeparator = "@"

// checkAbsentPeople is the manifest key, and the request line, of the
// permission to ask about a person the module is not serving.
const checkAbsentPeople = "check_absent_people"

// Module runs the module commands: add, list, remove and purge, and install,
// update, why, why-not and outdated, which follow Composer's.
func Module(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: %s %s <module path>%s<constraint> [-from <release>]|%s|"+
			"%s <module or kind:name> [-with-unused]|%s <module or kind:name> -yes|%s|"+
			"%s [<module>...] [-with-dependencies]|%s <module>|%s <module> <version>|%s",
			ErrUsage, ModuleCommand, addVerb, versionSeparator, listVerb, removeVerb, purgeVerb, installVerb,
			updateVerb, whyVerb, whyNotVerb, outdatedVerb)
	}

	flags := flag.NewFlagSet(ModuleCommand+" "+args[0], flag.ContinueOnError)
	flags.SetOutput(out)

	var (
		configPath = flags.String("config", defaultConfigPath, "the operator's configuration file")
		from       = flags.String("from", "",
			"the release to install: a directory, or a .tar.gz of the same tree")
		approve = flags.Bool("approve", false,
			"write the permissions and the events the manifest asks for, grant the module what "+
				"those permissions name, and switch the instance on")
		settings = flags.String("settings", "",
			"a JSON file with the settings of the module, written into the same declaration")
		recordsMiB = flags.Int("records-mib", 0,
			"with -approve, the room to grant the module's records in MiB, in place of what it asks for")
		memoryMiB = flags.Int("memory-mib", 0,
			"with -approve, the memory budget of the module's process in MiB, in place of what it recommends")
		approveListen = flags.Bool("approve-listen", false,
			"with -approve, also grant the ports the manifest lists under needs.listen")
		listen = flags.String("listen", "",
			"with -approve-listen, name=port pairs separated by commas, to grant another port under a name")
		database = flags.String("database", "",
			"with -approve, where the module's own database goes: sqlite, postgres or provision")
		databaseDSN = flags.String("database-dsn", "",
			"with -database postgres, the postgres:// URL of the module's database")
		yes = flags.Bool("yes", false,
			"with purge, delete the databases the module left behind. Without it purge only says what it would do")
		forget = flags.Bool("forget", false,
			"with purge, forget the record of each database and leave the database itself alone")
		withUnused = flags.Bool("with-unused", false,
			"with remove, also remove the modules an install added for it that nothing else requires")
		withDependencies = flags.Bool("with-dependencies", false,
			"with update, also update the modules an install added for the modules named")
	)

	operands, err := parseAround(flags, args[1:])
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}

	core, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	if err = core.Validate(); err != nil {
		return err
	}

	switch args[0] {
	case addVerb:
		moved, listenErr := parseListenFlag(*listen)
		if listenErr != nil {
			return listenErr
		}

		return addModule(ctx, out, core, *configPath, moduleRequest{
			source: firstOf(operands), from: *from, approve: *approve, settings: *settings,
			recordsMiB: *recordsMiB, memoryMiB: *memoryMiB, approveListen: *approveListen, listen: moved,
			database: databaseFlag(*database, *databaseDSN),
		})
	case listVerb:
		return listModules(out, core, *configPath)
	case removeVerb:
		return removeModule(ctx, out, core, *configPath, firstOf(operands), *withUnused)
	case purgeVerb:
		return purgeModule(ctx, out, core, *configPath, firstOf(operands), *yes, *forget)
	case installVerb:
		return installFromLock(ctx, out, core, *configPath, *approve)
	case updateVerb:
		return updateModules(ctx, out, core, *configPath, operands, *withDependencies, *approve)
	case whyVerb:
		return whyModule(out, core, *configPath, firstOf(operands))
	case whyNotVerb:
		version := ""
		if len(operands) > 1 {
			version = operands[1]
		}

		return whyNotModule(ctx, out, core, *configPath, firstOf(operands), version)
	case outdatedVerb:
		return outdatedModules(ctx, out, core, *configPath)
	default:
		return fmt.Errorf("%w: %s %s is not a command. It is %s, %s, %s, %s, %s, %s, %s, %s or %s",
			ErrUsage, ModuleCommand, args[0], addVerb, listVerb, removeVerb, purgeVerb, installVerb, updateVerb,
			whyVerb, whyNotVerb, outdatedVerb)
	}
}

// firstOf is the first word that was not a flag, and empty when there was
// none.
func firstOf(operands []string) string {
	if len(operands) == 0 {
		return ""
	}

	return operands[0]
}

// addModule installs one module and writes its declaration.
//
// Nothing is written until the manifest has been read and the release verified,
// so a refusal leaves the deployment exactly as it was. A release's signature
// is not verified. A release is not fetched from a tag on a repository: it
// arrives as the path `-from` names, or from the index.
func addModule(
	ctx context.Context, out io.Writer, core config.Core, configPath string, request moduleRequest,
) error {
	if err := modulesRefused(core); err != nil {
		return fmt.Errorf("%w. To install modules, set it to true in %s, or delete the line, "+
			"and restart a running server", err, configPath)
	}

	// The graph is opened only when this install has grants to write or old ones
	// to take back, which is why it is a function and not a handle. A command
	// that opened the database anyway would create and migrate it on its way to
	// writing nothing.
	installer := commandInstaller(core, configPath)

	// With no -from, the index names the release: the newest one the
	// constraint after the @ and every installed module allow, as composer
	// require takes it.
	if strings.TrimSpace(request.from) == "" {
		done, err := installer.fromIndex(ctx, &request)
		if err != nil {
			return err
		}

		defer done()
	}

	operation := moduleInstall{
		request: request,
		grants:  func() (*moduleAuthority, func(), error) { return openModuleGrants(ctx, core) },
	}

	// The modules the release requires and this server lacks come from the
	// same index the API reads, and are installed with the same approve.
	// The command prints the plan it installed.
	if request.address == "" {
		request.address = request.from
	}

	operation.request = request
	operation.dependencies = installer.dependenciesFor(nil, false, operation)

	held, err := installDeclared(ctx, core, configPath, operation)
	if err != nil {
		// A grant that failed after the declaration was written leaves a
		// declared module. The lines that did land are printed before the error,
		// so nothing that happened is hidden.
		if len(held.granted) > 0 || len(held.withdrawn) > 0 {
			report(out, held)
		}

		return err
	}

	report(out, held)

	if err = recordLock(core, configPath); err != nil {
		return err
	}

	return runningCoreNotice(ctx, out, core, configPath, len(held.granted) > 0,
		"The same install through the API, POST /api/v1/modules, takes effect at once.")
}

// moduleInstall is one install as both callers make it: what to install, and
// the check the resource kind has to pass before anything is written.
//
// mayDeclare is nil for the command. An operator running it already has the
// configuration file open and holds every grant there is, so asking the graph
// would be asking them for permission to do what they are already doing. The
// API passes the graph's answer for the kind the manifest names, which is the
// first moment that kind is known.
type moduleInstall struct {
	request    moduleRequest
	mayDeclare func(ctx context.Context, kind ref.Kind) error
	// grants opens the authorization graph the module's own grants are written
	// in, and answers what to call when the install is over. It is called only
	// when there is a grant to write or an old one to take back.
	//
	// Nil is a caller with no graph, and the install then writes the declaration
	// and no grant.
	grants func() (*moduleAuthority, func(), error)
	// keepPrevious keeps the release an update replaces until the caller
	// has seen the new one start, so it can roll the update back. The API
	// sets it, because its server starts the module at once. The command
	// writes for the next start and keeps nothing.
	keepPrevious bool
	// replacing stops the running instance of a module that is already
	// installed, after every check has passed and before its files are
	// replaced. A running process holds its own executable, and the host
	// verifies the old digest on its next launch, so the new release must not
	// land under it. Nil is the command: no process of this one is running the
	// module.
	replacing func(instance ref.Ref)
	// dependencies adds the modules the release requires that this server
	// does not have, before the release's own declaration is written. Nil
	// adds nothing, and an install whose release requires a missing module is
	// then refused by the resolver's check.
	dependencies dependencyInstall
	// approved is what a person approved of each module a plan adds, by module
	// path, when they approved the plan over the API. Nil is the command,
	// where the operator reads each manifest.
	approved map[string]approvedRelease
	// candidateCore is the signed core version an offline transaction will
	// activate with this release. Empty checks the running core.
	candidateCore string
}

// stopper is the stop a removal of a module this install added uses, and nil
// for an install with no process to stop.
func (i moduleInstall) stopper() func(ref.Ref) bool {
	if i.replacing == nil {
		return nil
	}

	return func(instance ref.Ref) bool {
		i.replacing(instance)

		return true
	}
}

// installDeclared is what `module add` and the install operation both do: stage
// the release, check it, and write the one declaration that carries both
// halves.
//
// Nothing is written until the manifest has been read and the release verified,
// so a refusal leaves the deployment exactly as it was. A release's signature
// is not verified. A release is not fetched from a tag on a repository: it
// arrives as the path or the URL `from` names.
func installDeclared(
	ctx context.Context, core config.Core, configPath string, install moduleInstall,
) (moduleReport, error) {
	request := install.request

	modulePath, version, err := moduleSource(request)
	if err != nil {
		return moduleReport{}, err
	}

	namespace, err := extension.NamespaceOf(modulePath)
	if err != nil {
		return moduleReport{}, fmt.Errorf("%s names no namespace: %w", modulePath, err)
	}

	dir, done, err := stageRelease(request.from)
	if err != nil {
		return moduleReport{}, err
	}

	defer done()

	held, err := readRelease(dir)
	if err != nil {
		return moduleReport{}, err
	}

	if err = releaseMatches(request.source, version, held.manifest); err != nil {
		return moduleReport{}, err
	}

	runningCore := coreVersion()
	if install.candidateCore != "" {
		runningCore = install.candidateCore
	}
	if err = coreMatchesVersion(held.manifest, runningCore); err != nil {
		return moduleReport{}, err
	}

	if err = requirementsMeetableVersion(held.manifest, runningCore); err != nil {
		return moduleReport{}, err
	}

	// What the manifest asks for before anything is decided about it. It is
	// held to what the operator was shown, and recorded in the declaration.
	asked, err := shownAsks(request, held.manifest, string(namespace))
	if err != nil {
		return moduleReport{}, err
	}

	checked, err := verifyChecksums(dir)
	if err != nil {
		return moduleReport{}, err
	}

	instance, err := ref.New(ref.Kind(held.manifest.Kind), held.manifest.Name)
	if err != nil {
		return moduleReport{}, fmt.Errorf("the release's %s: %w", manifestName, err)
	}

	if err = installableUnder(instance.Kind(), held.manifest); err != nil {
		return moduleReport{}, err
	}

	// After the manifest names the kind and before anything is written, so a
	// caller who may not administer that kind's configuration changes nothing.
	if install.mayDeclare != nil {
		if err = install.mayDeclare(ctx, instance.Kind()); err != nil {
			return moduleReport{}, err
		}
	}

	// The modules the release requires and this server lacks go first, so
	// the release's own declaration meets every range it names. A failure
	// further on takes them away again.
	pulled, undo := []moduledeps.Step(nil), func(context.Context) {}

	if install.dependencies != nil {
		if pulled, undo, err = install.dependencies(ctx, modulePath, string(namespace), held.manifest); err != nil {
			return moduleReport{}, err
		}
	}

	made, err := installResolved(ctx, core, configPath, install, resolvedInstall{
		held: held, namespace: string(namespace), instance: instance, asked: asked, checked: checked,
		modulePath: modulePath, constraint: constraintFor(request, version),
	})
	if err != nil {
		undo(ctx)

		return made, err
	}

	for _, one := range pulled {
		made.dependencies = append(made.dependencies, one.Release.Module+"@"+one.Release.Version)
	}

	return made, nil
}

// keeps reports whether an install keeps the release it replaces until the
// new one has started: an update, made by a caller that asked for it.
func (i moduleInstall) keeps(declared resource.Declaration) bool {
	return i.keepPrevious && !declared.Ref.IsZero()
}

// stopReplaced stops the running instance of a module declared at another
// version, before its files are replaced. The same version again writes the
// same declaration, which the store plans as nothing, so an instance stopped
// for it would stay down.
func (i moduleInstall) stopReplaced(declared resource.Declaration, version string, instance ref.Ref) {
	if i.replacing == nil || declared.Ref.IsZero() {
		return
	}

	if declared.Spec.Module == nil || declared.Spec.Module.Version != version {
		i.replacing(instance)
	}
}

// moduleSource reads the one argument `module add` takes, and the flag that
// says where the bytes are.
//
// The argument is the module path, because the namespace of every address of
// this module is derived from the path. Nothing the author wrote in the
// manifest takes part in it. A directory has no module path, so the two are
// separate words.
func moduleSource(request moduleRequest) (string, string, error) {
	modulePath, version, found := strings.Cut(request.source, versionSeparator)
	if !found || strings.TrimSpace(modulePath) == "" || strings.TrimSpace(version) == "" {
		return "", "", fmt.Errorf("%w: %s %s <module path>%s<version> -from <release>",
			ErrUsage, ModuleCommand, addVerb, versionSeparator)
	}

	if strings.TrimSpace(request.from) == "" {
		return "", "", fmt.Errorf("%w: -from names the release to install, as a directory or a %s, or "+
			"the index names one", ErrUsage, archiveSuffix)
	}

	return strings.TrimSpace(modulePath), strings.TrimSpace(version), nil
}

// releaseMatches holds the release in front of the operator to what they asked
// for. The version after the @ is a range in the grammar a module author
// already writes, so `@1.2.3` names one release, `@^1.2.0` names a set, and
// `@*` takes whatever the release says it is.
func releaseMatches(source, wanted string, declared manifest.Manifest) error {
	asked, err := manifest.ParseConstraint(wanted)
	if err != nil {
		return fmt.Errorf("%s: the version after %s: %w", source, versionSeparator, err)
	}

	if !asked.HoldsText(declared.Version) {
		return fmt.Errorf("%s was asked for and the release's %s says version %s, which it does not hold",
			source, manifestName, declared.Version)
	}

	return nil
}

// coreMatchesVersion holds a core version to the versions the module says it
// runs on. A module installed on a core it was not built against fails on its
// first call, for a reason nobody can read off the wire.
func coreMatchesVersion(declared manifest.Manifest, running string) error {
	if strings.TrimSpace(declared.Core) == "" {
		return nil
	}

	if _, err := manifest.ParseConstraint(declared.Core); err != nil {
		return fmt.Errorf("the release's %s: require.core: %w", manifestName, err)
	}

	// A build that reports no version skips the check. A checkout replaced by
	// path is the case: Go has no version for source taken from a directory,
	// and every install would otherwise be refused on a development build.
	in, readable := install.CoreHolds(declared.Core, running)
	if !readable || in {
		return nil
	}

	return fmt.Errorf("the module runs on core %s, and this core is %s", declared.Core, running)
}

// requirementsMeetableVersion refuses a module this build could never run: one
// asking for a wire capability the server does not serve, or for a core
// version this server is not.
//
// It is the same question the supervision plane asks before every start, asked
// once here so an operator reads it while they are installing instead of off a
// failed instance afterwards. A requirement on a contract is the resolver's,
// which reads every installed module (installResolves).
func requirementsMeetableVersion(declared manifest.Manifest, running string) error {
	served := resourcehost.Capabilities()

	for _, required := range declared.Requires {
		if required.Capability != "" && !slices.Contains(served, required.Capability) {
			return fmt.Errorf("%w: the module requires the host capability %s, and this server serves %s",
				ErrUsage, required.Capability, strings.Join(served, ", "))
		}

		if required.Core != "" {
			if err := coreMatchesVersion(manifest.Manifest{Core: required.Core}, running); err != nil {
				return err
			}
		}
	}

	return nil
}

// coreVersion is this build's own version. The supervision plane reads the
// same answer when it holds a running module to the core range it declared.
func coreVersion() string { return install.CoreVersion() }

// errNothingToInstall reports a manifest that declares neither half, which is
// a release with nothing in it for the host to run or serve.
var errNothingToInstall = errors.New(
	"the manifest declares no process and no ui block, so there is nothing to install")

// installableUnder holds the kind the manifest names to the halves the module
// actually ships. A module that runs a process is declared under a kind that
// offers the service runtime, and one that is only a bundle is an asset.
func installableUnder(kind ref.Kind, declared manifest.Manifest) error {
	offered := ref.KindRuntimes(kind)

	if declared.ContractMajor > 0 {
		if !slices.Contains(offered, install.RuntimeService) {
			return fmt.Errorf("the manifest says kind %s and contract_major %d, and a module that runs a "+
				"process is declared under a kind that offers %s",
				kind, declared.ContractMajor, install.RuntimeService)
		}

		return nil
	}

	if !slices.Contains(offered, install.RuntimeAsset) {
		return fmt.Errorf("the manifest says kind %s and no contract_major, so this module runs no "+
			"process and is declared under kind %s", kind, ref.KindAsset)
	}

	if declared.UI == nil {
		return errNothingToInstall
	}

	return nil
}

// recordedOptions is the module's settings list as the declaration records it.
func recordedOptions(declared []manifest.Option) []install.Option {
	if len(declared) == 0 {
		return nil
	}

	out := make([]install.Option, 0, len(declared))
	for _, option := range declared {
		out = append(out, install.Option{
			Key:      option.Key,
			Type:     option.Type,
			Required: option.Required,
			Default:  option.Default,
			Summary:  option.Summary,
		})
	}

	return out
}

// writeDeclaration installs the files and writes the one declaration that
// carries both halves.
func writeDeclaration(
	core config.Core, held release, namespace string, instance ref.Ref, decided declaredAs,
) (resource.Declaration, installed, error) {
	landed, err := installRelease(held, core.Modules, decided.keep)
	if err != nil {
		return resource.Declaration{}, installed{}, err
	}

	approve := decided.approve
	entry := install.Entry{
		Type:    namespace + extension.QualifiedSeparator + held.manifest.Name,
		Enabled: &approve,
		Config:  decided.settings,
		// What the manifest said about itself, kept in the declaration because
		// the staged release is thrown away and a module that runs a process
		// leaves no manifest on disk. A marketplace card reads it back off the
		// resources listing.
		Module: &install.ModuleEntry{
			Version: held.manifest.Version,
			Title:   held.manifest.Title,
			Summary: held.manifest.Summary,
			Author:  held.manifest.Codeowner,
			// What the module cannot run without, recorded for the same
			// reason: the host holds the running module to it at every start,
			// and there is no manifest on disk to read it from. The modules it
			// suggests are recorded beside it, because a suggested module
			// that is installed is held to its range.
			Requires:  recordedRequirements(held.manifest),
			Suggests:  recordedSuggests(held.manifest.Suggests),
			Conflicts: recordedConflicts(held.manifest.Conflicts),
			// The file types the module reads for the core, recorded for
			// the same reason: the host asks the running module, with no
			// manifest on disk.
			FileTypes: fileTypesOf(held.manifest.FileAnalysis),
			ViewTypes: viewTypesOf(held.manifest),
			// The camera types it offers, recorded for the same reason: a
			// camera of one is declared long after the release is gone
			// (ADR-20261106-km2).
			CameraTypes: cameraTypesOf(held.manifest.CameraTypes),
			// The settings the module reads, with the default of each. They
			// are the module's own default configuration: nothing is written
			// into the file here, and an operator overrides a key by writing
			// that key under the instance's config block.
			Options: recordedOptions(held.manifest.Options),
		},
	}

	bundle := &install.AssetEntry{Path: filepath.ToSlash(landed.bundleDir), Checksum: landed.bundleDigest}
	if landed.bundleDir == "" {
		bundle = nil
	}

	if held.binary == "" {
		entry.Runtime = install.RuntimeAsset.String()
		entry.Asset = bundle
	} else {
		major := held.manifest.ContractMajor
		entry.Runtime = install.RuntimeService.String()
		entry.Service = &install.ServiceEntry{
			Path:          filepath.ToSlash(landed.binaryDir),
			Checksum:      landed.binaryDigest,
			ContractMajor: &major,
			Assets:        bundle,
			MemoryMiB:     decided.memory.mib,
			OnOverMemory:  decided.memory.onOverMemory,
			SwitchOff:     decided.memory.switchOff.Entry(),
		}

		// These keys are requests. They are written only after the operator
		// has read them and said yes with -approve.
		if approve {
			entry.Service.Events = held.manifest.Events
			entry.Service.Permissions = held.manifest.Permissions
			entry.Service.Checks = held.manifest.Checks
			entry.Service.CheckAbsentPeople = held.manifest.CheckAbsentPeople
			entry.Service.ListPeople = held.manifest.ListPeople
			entry.Service.Calls = held.manifest.Calls
			entry.Service.PublicRoutes = routeLines(held.manifest.Routes.Public)
			entry.Service.StagedRoutes = install.StagedRoutesOf(held.manifest.Routes.Staged)
			entry.Service.RequestHeaders = install.RequestHeaderNames(
				install.RequestHeadersOf(held.manifest.Routes.Headers))
			entry.Service.RecordsMiB = held.manifest.RecordsMiB
			entry.Service.Listen = declaredListen(held.manifest.Needs.Listen)
			entry.Service.Data = held.manifest.Needs.Data
			entry.Service.Database = decided.database.Entry()
		}
	}

	// The ask the operator approved, kept so the next approved install can
	// tell a request they took away from one the release adds.
	if approve {
		asked := decided.asked
		entry.Module.Asked = &asked
		entry.Module.AllowedOrigins = held.manifest.AllowedOrigins
	}

	entry.Module.Channel = decided.channel
	entry.Module.Path, entry.Module.Source, entry.Module.Digest = decided.path, decided.address, decided.digest
	entry.Module.Pulled, entry.Module.Constraint = decided.pulled, decided.constraint

	written, err := resource.InstallModule(core.Overlay, instance, entry, decided.left)
	if err != nil {
		return resource.Declaration{}, installed{}, err
	}

	return written, landed, nil
}

// moduleNamed resolves what an operator typed: a module path, the type it
// produced, the instance's own name, or kind:name, the spelling the error for
// an ambiguous name lists.
func moduleNamed(rows []resource.Declaration, named string) (resource.Declaration, error) {
	namespace := ""
	if derived, err := extension.NamespaceOf(named); err == nil {
		namespace = string(derived)
	}

	var matched []resource.Declaration

	for _, row := range rows {
		held, _ := row.ModuleNamespace()
		if row.Ref.ID() == named || row.Ref.String() == named || row.Spec.Type == named ||
			(namespace != "" && held == namespace) {
			matched = append(matched, row)
		}
	}

	switch len(matched) {
	case 0:
		return resource.Declaration{}, fmt.Errorf("no module is declared under %q", named)
	case 1:
		return matched[0], nil
	default:
		names := make([]string, 0, len(matched))
		for _, row := range matched {
			names = append(names, row.Ref.String())
		}

		return resource.Declaration{}, fmt.Errorf("%q names %d declared modules: %s. Name one of them",
			named, len(matched), strings.Join(names, ", "))
	}
}
