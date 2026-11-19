// The modules this server can install, and the installs and removals a person
// starts from the Extensions page (ADR-20261005-64j), read through the SDK.
//
// A class, because the page and each of its cards share it (svelte-5.md). It
// lives in the application, beside Registrations, because the screen is the
// application's own.
//
// The index is one list the server owns, so it is replaced whole on every read.
// One action runs at a time: an install downloads a release and converges the
// server, and a second one started meanwhile would race it for the same
// overlay. The marketplace switch is written to the same overlay, so it waits
// for an action too, and an action waits for it.
//
// An install grants exactly what the person was shown. A row that knows every
// request of its release sends them back as `expect`. A row that does not is
// installed switched off first, and the requests the answer names are shown
// and approved in a second request (`pending`).
import { ApiError, type components, type PrusaConnect } from '@prusa/connect';

import type { ResourceInstance } from '@prusa/connect-ui/data';

import {
  type DatabaseChoice,
  mergeRequests,
  type ModuleRequests,
  requestsOfLines,
  requestsOfOffer,
} from '../extensions/requests';
import type { ModuleMemoryPool } from '../extensions/memory';
import { isNewer } from '../semver';

export type ModuleOffer = components['schemas']['ModuleOffer'];
export type ModuleMarketplace = components['schemas']['ModuleMarketplace'];
export type ModuleInstallation = components['schemas']['ModuleInstallation'];
export type ModulePlanned = components['schemas']['ModulePlanned'];
export type ModuleFileTypeConflict =
  components['schemas']['ModuleFileTypeConflict'];
export type ModuleRemoval = components['schemas']['ModuleRemoval'];
export type ModuleReason = components['schemas']['ModuleReason'];
export type ModuleOutdated = components['schemas']['ModuleOutdated'];

/**
 * The modules an install of one release adds, as the server planned them
 * (ADR-20261027-e6k rule 14). `refused` carries the refusal as it was thrown:
 * a conflict or a module no index offers.
 */
export type InstallPlan =
  | { readonly state: 'loading' }
  | {
      readonly state: 'ready';
      readonly dependencies: readonly ModulePlanned[];
      /** Enabled extensions that read or draw files of a type the install claims too (ADR-20261027-wvj). */
      readonly conflicts: readonly ModuleFileTypeConflict[];
    }
  | { readonly state: 'refused'; readonly error: unknown };

/**
 * What a removal of one module meets: the installed modules that require it,
 * and the modules an install added for it that it leaves unused.
 */
export type RemovalCheck =
  | { readonly state: 'loading' }
  | { readonly state: 'ready'; readonly removal: ModuleRemoval }
  | { readonly state: 'refused'; readonly error: unknown };

type ResourceKind = Parameters<PrusaConnect['putResource']>[0]['path']['kind'];

/** What keeps one module installed, as the server answers `why`. */
export type WhyCheck =
  | { readonly state: 'loading' }
  | { readonly state: 'ready'; readonly reasons: readonly ModuleReason[] }
  | { readonly state: 'refused'; readonly error: unknown };

/** Each installed module against the index, as the server answers `outdated`. */
export type OutdatedCheck =
  | { readonly state: 'loading' }
  | { readonly state: 'ready'; readonly rows: readonly ModuleOutdated[] }
  | { readonly state: 'refused'; readonly error: unknown };

/** What a card says about one row. */
export type OfferState = 'available' | 'installed' | 'update';

/** What this store needs of the SDK, so a test hands in a fake (svelte-data.md). */
export type ModuleCatalogueClient = Pick<
  PrusaConnect,
  | 'checkAccess'
  | 'getModuleMemory'
  | 'getModuleRemoval'
  | 'getModuleWhy'
  | 'installModule'
  | 'listModuleIndex'
  | 'listModuleOutdated'
  | 'planModuleInstall'
  | 'putResource'
  | 'restartResource'
  | 'setModuleMarketplace'
  | 'uninstallModule'
>;

/** What a removal names: the module and the namespace it was installed under. */
export interface RemovedModule {
  readonly module: string;
  readonly namespace?: string | undefined;
}

/**
 * An install waiting for the person to approve what the release asks for.
 * `unread`: the row did not know the requests, so the release was installed
 * switched off to read them. `changed`: the release asked for more than the
 * person was shown, and nothing was installed.
 */
export interface PendingApproval {
  readonly cause: 'unread' | 'changed';
  readonly offer: ModuleOffer;
  readonly version: string;
  /** Everything the second request approves, sent as `expect`. */
  readonly requests: ModuleRequests;
  /** For `changed`: each request beyond what was shown, in the server's words. */
  readonly found: readonly string[];
  /** The namespace the server's lines name events under. */
  readonly namespace: string | undefined;
}

/** What an approved install answered, kept until the person has read it. */
export interface InstallOutcome {
  readonly offer: ModuleOffer;
  /** True when it replaced a release that was installed. */
  readonly update: boolean;
  readonly answer: ModuleInstallation;
}

/**
 * The resource kinds a module can be declared under. The server lets a
 * person install, update and remove modules when they may configure any of
 * them, and each module only under its own kind.
 */
export const MODULE_KINDS: readonly string[] = [
  'storage',
  'auth',
  'sink',
  'driver',
  'db',
  'camera',
  'history',
  'link',
  'asset',
];

// Every organization administrator may configure the auth kind, which is
// administering this server's identity. The access check cannot ask about the
// organization itself, so this kind stands for it.
const ORG_ADMIN_KIND = 'auth';

const REQUESTS_CHANGED = 'core.resource/MODULE_REQUESTS_CHANGED';
const PLAN_CHANGED = 'core.resource/MODULE_PLAN_CHANGED';
const MODULES_DISABLED = 'core.resource/MODULES_DISABLED';

const SAFE_MODE = 'core.resource/SAFE_MODE';

// Safe mode refuses what MODULES_DISABLED refuses, and the page draws the
// same "modules are off" state for both.
const isModulesDisabled = (thrown: unknown): boolean =>
  thrown instanceof ApiError &&
  (thrown.code === MODULES_DISABLED || thrown.code === SAFE_MODE);

/** The state of one row: not installed, installed, or installed with a newer release on offer. */
export const stateOf = (offer: ModuleOffer): OfferState => {
  if (!offer.installed) return 'available';

  const installed = offer.installed_version ?? '';

  return installed !== '' && isNewer(offer.version, installed)
    ? 'update'
    : 'installed';
};

// What a module the plan adds is held to when its row listed no requests.
const NO_REQUESTS: ModuleRequests = {
  permissions: [],
  events: {},
  check_absent_people: false,
  records_mib: 0,
};

// The release an install asks for. A marketplace row names no address, so the
// server takes the release and its digest from the marketplace itself.
const releaseOf = (
  offer: ModuleOffer,
  version: string,
  expect: ModuleRequests | undefined,
  approveListen: boolean,
  listen: Readonly<Record<string, number>> = {},
  database?: DatabaseChoice,
  dependencies: readonly ModulePlanned[] = [],
): components['schemas']['ModuleRelease'] => {
  const fromMarketplace = offer.origin === 'marketplace';

  return {
    module: offer.module,
    version,
    approve: expect !== undefined,
    // A port takes an approval of its own (ADR-20261019-3ns rule 6).
    ...(expect !== undefined && approveListen ? { approve_listen: true } : {}),
    // The ports a person moved, granted at those numbers instead.
    ...(expect !== undefined && approveListen && Object.keys(listen).length > 0
      ? { listen: { ...listen } }
      : {}),
    ...(expect === undefined ? {} : { expect }),
    // Where the module's own database goes, only for a release that asks for
    // one. Left out, the server keeps what an installed module has.
    ...(expect?.database === true && database !== undefined
      ? { database: { ...database } }
      : {}),
    ...(fromMarketplace ? {} : { from: offer.source }),
    ...(!fromMarketplace && offer.sha256 ? { sha256: offer.sha256 } : {}),
    // The modules the plan the person was shown adds (ADR-20261027-e6k rule
    // 15), each pinned to the bytes the plan named and held to the requests
    // it showed. A row that did not list its requests showed none, so the
    // module is held to none. A plan that moved since is refused and nothing
    // is written.
    ...(dependencies.length > 0
      ? {
          dependencies: dependencies.map((one) => ({
            module: one.module,
            version: one.version,
            ...(one.sha256 ? { sha256: one.sha256 } : {}),
            ...(expect === undefined
              ? {}
              : { expect: requestsOfOffer(one) ?? NO_REQUESTS }),
            ...(expect !== undefined &&
            approveListen &&
            (one.listen?.length ?? 0) > 0
              ? { approve_listen: true }
              : {}),
          })),
        }
      : {}),
  };
};

// The key a plan is kept under: one release of one module.
const planKey = (offer: ModuleOffer, version: string): string =>
  `${offer.module}@${version}`;

// Whether a release names any module it requires, which is the only case its
// install reads a plan.
// A release that requires no module and claims no file type has an empty
// plan, and the page asks the server nothing for it.
const needsPlan = (offer: ModuleOffer): boolean =>
  (offer.requires ?? []).some((one) => (one.module ?? '') !== '') ||
  (offer.file_types ?? []).length > 0 ||
  (offer.view_types ?? []).length > 0;

// The lines a MODULE_REQUESTS_CHANGED refusal names, or undefined for any
// other refusal.
const foundOf = (thrown: unknown): string[] | undefined => {
  if (!(thrown instanceof ApiError) || thrown.code !== REQUESTS_CHANGED) {
    return undefined;
  }

  const found = thrown.params.found;

  return typeof found === 'string'
    ? found
        .split(';')
        .map((line) => line.trim())
        .filter((line) => line !== '')
    : [];
};

/** What one write to an instance changes; a key left out stays as it was. */
export interface InstanceChange {
  enabled?: boolean;
  config?: Record<string, unknown>;
  hidden_contributions?: string[];
  on_over_memory?: 'stop' | 'warn';
  /** The operator's budget in MiB; 0 clears it. */
  memory_mib?: number;
  /** The operator's switch-off; null returns to the server default. */
  switch_off?: { failures: number; window_seconds: number } | null;
  /** The largest body of each staged route the install approved, in MiB. */
  staged_routes?: { method: string; path: string; max_mib: number }[];
}

export class ModuleCatalogue {
  readonly #client: ModuleCatalogueClient;
  /** Called after an install or a removal, so the sidebar and the installed list follow. */
  readonly #changed: () => Promise<void> | void;
  /** Whether the server runs the modules it installed, as GET /api/v1/ui says. */
  readonly #modulesOn: () => boolean;
  /** Whether the server runs in safe mode, as GET /api/v1/ui says. */
  readonly #safeMode: () => boolean;

  /** The index answered MODULES_DISABLED, whatever GET /api/v1/ui said. */
  #refusedOff = $state(false);
  #items = $state.raw<readonly ModuleOffer[]>([]);
  #marketplace = $state.raw<ModuleMarketplace | undefined>(undefined);
  #loading = $state(false);
  #loaded = $state(false);
  #error = $state.raw<unknown>(undefined);
  #access = $state.raw<Readonly<Record<string, boolean>> | undefined>(
    undefined,
  );
  #operate = $state.raw<Readonly<Record<string, boolean>> | undefined>(
    undefined,
  );
  #restarting = $state<string | undefined>(undefined);
  #restartError = $state.raw<unknown>(undefined);
  #restartFailed = $state<string | undefined>(undefined);
  #acting = $state<string | undefined>(undefined);
  #actionError = $state.raw<unknown>(undefined);
  #actionModule = $state<string | undefined>(undefined);
  #pending = $state.raw<PendingApproval | undefined>(undefined);
  #outcome = $state.raw<InstallOutcome | undefined>(undefined);
  #switching = $state(false);
  #switchError = $state.raw<unknown>(undefined);
  #writing = $state<string | undefined>(undefined);
  #writeError = $state.raw<unknown>(undefined);
  #writeConflict = $state(false);
  #writeModule = $state<string | undefined>(undefined);
  #writeFor = $state<
    'switch' | 'settings' | 'visibility' | 'memory' | 'uploads' | undefined
  >(undefined);
  #plans = $state.raw<Readonly<Record<string, InstallPlan>>>({});
  // The plan reads in flight, so a second caller waits for the same answer
  // instead of reading the loading state an install must never act on. No
  // page draws it, so it is not reactive.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  readonly #planReads = new Map<string, Promise<InstallPlan>>();
  #removals = $state.raw<Readonly<Record<string, RemovalCheck>>>({});
  #whys = $state.raw<Readonly<Record<string, WhyCheck>>>({});
  #outdated = $state.raw<OutdatedCheck | undefined>(undefined);
  #memoryPool = $state.raw<ModuleMemoryPool | undefined>(undefined);
  #flight: Promise<void> | undefined;
  #accessFlight: Promise<void> | undefined;
  // The index read in flight, aborted by the next read or a reset so that
  // the newest read started is the one that lands.
  #reading: AbortController | undefined;
  #resets = 0;

  constructor(
    client: ModuleCatalogueClient,
    changed: () => Promise<void> | void = () => undefined,
    modulesOn: () => boolean = () => true,
    safeMode: () => boolean = () => false,
  ) {
    this.#client = client;
    this.#changed = changed;
    this.#modulesOn = modulesOn;
    this.#safeMode = safeMode;
  }

  /**
   * Whether this server installs modules and runs the ones it installed. The
   * operator switches them off in the configuration file (ADR-20261019-h76), and then
   * the store reads no index and no marketplace: the server refuses both.
   * GET /api/v1/ui says so, and an index that answers MODULES_DISABLED says so
   * too, for a page whose read of the settings failed.
   */
  get modulesOn(): boolean {
    return this.#modulesOn() && !this.#refusedOff;
  }

  /**
   * Whether this run is in safe mode, which is why `modulesOn` is false: the
   * screens say so instead of naming config.yaml.
   */
  get safeMode(): boolean {
    return this.#safeMode();
  }

  /** Every row the server offers. Read-only by design: the server owns it. */
  get items(): readonly ModuleOffer[] {
    return this.#items;
  }

  /** The marketplace the rows were read from, and undefined when the configuration reads none. */
  get marketplace(): ModuleMarketplace | undefined {
    return this.#marketplace;
  }

  get loading(): boolean {
    return this.#loading;
  }

  /** True once a read has answered, whether it found rows or none. */
  get loaded(): boolean {
    return this.#loaded;
  }

  /** The last refusal of the read, as it was thrown. */
  get error(): unknown {
    return this.#error;
  }

  /**
   * Whether the person may install, update and remove modules at all, and
   * undefined until the server has said. The server decides again on every
   * request, so an unknown answer leaves the actions to it.
   */
  get mayManage(): boolean | undefined {
    const access = this.#access;
    if (access === undefined) return undefined;

    const answers = Object.values(access);
    if (answers.some(Boolean)) return true;

    return answers.length === MODULE_KINDS.length ? false : undefined;
  }

  /** Whether the person may switch the marketplace, which takes an organization administrator. */
  get mayAdministerOrg(): boolean | undefined {
    return this.#access?.[ORG_ADMIN_KIND];
  }

  /** The module an install or a removal is running for, and undefined when none is. */
  get acting(): string | undefined {
    return this.#acting;
  }

  /** The last refusal of an install or a removal, as it was thrown. */
  get actionError(): unknown {
    return this.#actionError;
  }

  /** The module the last refusal of an action was for. */
  get actionModule(): string | undefined {
    return this.#actionModule;
  }

  /** The install waiting for the person to approve what it asks for. */
  get pending(): PendingApproval | undefined {
    return this.#pending;
  }

  /** What the last approved install answered. */
  get outcome(): InstallOutcome | undefined {
    return this.#outcome;
  }

  /** True while the marketplace switch is being written. */
  get switching(): boolean {
    return this.#switching;
  }

  /** The last refusal of the marketplace switch, as it was thrown. */
  get switchError(): unknown {
    return this.#switchError;
  }

  /** The namespace of the module a write is running for, and undefined when none is. */
  get writing(): string | undefined {
    return this.#writing;
  }

  /** The last refusal of a write, as it was thrown. A stale version is `writeConflict` instead. */
  get writeError(): unknown {
    return this.#writeError;
  }

  /** The namespace of the module the last write was for, so a refusal can be shown beside it. */
  get writeModule(): string | undefined {
    return this.#writeModule;
  }

  /** What the last write changed: the module's switch, or its settings. */
  get writeFor():
    'switch' | 'settings' | 'visibility' | 'memory' | 'uploads' | undefined {
    return this.#writeFor;
  }

  /** True when the last write was refused because someone changed the module meanwhile. */
  get writeConflict(): boolean {
    return this.#writeConflict;
  }

  /**
   * The plan of one release, and undefined until `readPlan` asked for it. A
   * release that requires no module has an empty plan without a request.
   */
  planOf(offer: ModuleOffer, version: string): InstallPlan | undefined {
    if (!needsPlan(offer)) {
      return { state: 'ready', dependencies: [], conflicts: [] };
    }

    return this.#plans[planKey(offer, version)];
  }

  /**
   * Asks the server which modules an install of one release adds. A plan
   * already read or being read is not asked again; `again` reads it anew, as
   * after the server said the plan moved.
   */
  async readPlan(
    offer: ModuleOffer,
    version: string,
    again = false,
  ): Promise<InstallPlan> {
    const key = planKey(offer, version);
    const reading = this.#planReads.get(key);
    if (reading !== undefined && !again) return reading;

    const known = this.planOf(offer, version);
    if (known !== undefined && known.state !== 'loading' && !again)
      return known;

    const resets = this.#resets;
    this.#plans = { ...this.#plans, [key]: { state: 'loading' } };

    const read = (async (): Promise<InstallPlan> => {
      let plan: InstallPlan;
      try {
        const answer = await this.#client.planModuleInstall({
          body: { module: offer.module, version },
        });
        plan = {
          state: 'ready',
          dependencies: answer.dependencies ?? [],
          conflicts: answer.conflicts ?? [],
        };
      } catch (thrown) {
        plan = { state: 'refused', error: thrown };
      }
      if (resets === this.#resets) {
        this.#plans = { ...this.#plans, [key]: plan };
      }

      return plan;
    })();

    this.#planReads.set(key, read);
    try {
      return await read;
    } finally {
      if (this.#planReads.get(key) === read) this.#planReads.delete(key);
    }
  }

  // The modules an install sends as its approved plan. A plan that is not
  // ready is never sent as an empty one: the install stops with the reason.
  async #approvedPlan(
    offer: ModuleOffer,
    version: string,
  ): Promise<readonly ModulePlanned[]> {
    const plan = await this.readPlan(offer, version);
    if (plan.state === 'refused') throw plan.error;
    if (plan.state !== 'ready')
      throw new Error('the install plan is not read yet');

    return plan.dependencies;
  }

  /** What a removal of one module meets, and undefined until `readRemoval` asked. */
  removalOf(namespace: string | undefined): RemovalCheck | undefined {
    return this.#removals[namespace ?? ''];
  }

  /** Asks the server what a removal of one module meets, every time it is opened. */
  async readRemoval(namespace: string | undefined): Promise<void> {
    if ((namespace ?? '') === '') return;
    const key = namespace ?? '';
    const resets = this.#resets;
    this.#removals = { ...this.#removals, [key]: { state: 'loading' } };

    let check: RemovalCheck;
    try {
      const removal = await this.#client.getModuleRemoval({
        path: { namespace: key },
      });
      check = { state: 'ready', removal };
    } catch (thrown) {
      check = { state: 'refused', error: thrown };
    }
    if (resets === this.#resets) {
      this.#removals = { ...this.#removals, [key]: check };
    }
  }

  /** What keeps one module installed, and undefined until `readWhy` asked. */
  whyOf(namespace: string | undefined): WhyCheck | undefined {
    return this.#whys[namespace ?? ''];
  }

  /** Asks the server what keeps one module installed, once until the next change. */
  async readWhy(namespace: string | undefined): Promise<void> {
    const key = namespace ?? '';
    if (key === '' || this.#whys[key] !== undefined) return;
    const resets = this.#resets;
    this.#whys = { ...this.#whys, [key]: { state: 'loading' } };

    let check: WhyCheck;
    try {
      const answer = await this.#client.getModuleWhy({
        path: { namespace: key },
      });
      check = { state: 'ready', reasons: answer.items ?? [] };
    } catch (thrown) {
      check = { state: 'refused', error: thrown };
    }
    if (resets === this.#resets) {
      this.#whys = { ...this.#whys, [key]: check };
    }
  }

  /** Each installed module against the index, and undefined until `readOutdated` asked. */
  get outdated(): OutdatedCheck | undefined {
    return this.#outdated;
  }

  /** Asks the server for every installed module against the index, once until the next change. */
  async readOutdated(): Promise<void> {
    if (this.#outdated !== undefined) return;
    const resets = this.#resets;
    this.#outdated = { state: 'loading' };

    let check: OutdatedCheck;
    try {
      const answer = await this.#client.listModuleOutdated({});
      check = { state: 'ready', rows: answer.items ?? [] };
    } catch (thrown) {
      check = { state: 'refused', error: thrown };
    }
    if (resets === this.#resets) {
      this.#outdated = check;
    }
  }

  /**
   * The memory pool the modules share and what the enabled ones ask for, and
   * undefined until `readMemory` answered or when the person may not read it.
   */
  get memoryPool(): ModuleMemoryPool | undefined {
    return this.#memoryPool;
  }

  /** Reads the pool again. A refusal (the route needs module administration) or a failure leaves it undefined: the page then shows no pool part. */
  async readMemory(): Promise<void> {
    const resets = this.#resets;
    let pool: ModuleMemoryPool | undefined;
    try {
      pool = await this.#client.getModuleMemory({});
    } catch {
      pool = undefined;
    }
    if (resets === this.#resets) this.#memoryPool = pool;
  }

  /** Whether the person may configure one kind, and the answer for any kind when it is not named. */
  mayConfigure(kind: string | undefined): boolean | undefined {
    const known =
      kind === undefined || kind === '' ? undefined : this.#access?.[kind];

    return known ?? this.mayManage;
  }

  /** Whether the person may restart instances of one kind, and undefined until the server has said. */
  mayOperate(kind: string | undefined): boolean | undefined {
    return kind === undefined || kind === ''
      ? undefined
      : this.#operate?.[kind];
  }

  /** The instance being restarted, as `kind:name`. */
  get restarting(): string | undefined {
    return this.#restarting;
  }

  /** What the last restart failed with; a refusal is an error here too. */
  get restartError(): unknown {
    return this.#restartError;
  }

  /** What the last restart failed with when it was this instance's, so another module's page says nothing. */
  restartErrorOf(instance: ResourceInstance): unknown {
    return this.#restartFailed === `${instance.kind}:${instance.name}`
      ? this.#restartError
      : undefined;
  }

  /**
   * Stops the instance and builds it again from its declaration. A call while
   * one restart is in flight sends nothing. True when the server accepted it.
   */
  async restart(instance: ResourceInstance): Promise<boolean> {
    if (this.#restarting !== undefined) return false;

    const resets = this.#resets;
    this.#restarting = `${instance.kind}:${instance.name}`;
    this.#restartError = undefined;
    this.#restartFailed = undefined;

    try {
      await this.#client.restartResource({
        path: { kind: instance.kind as ResourceKind, name: instance.name },
      });
      if (resets !== this.#resets) return false;
      await this.#changed();

      return true;
    } catch (thrown) {
      if (resets === this.#resets) {
        this.#restartError = thrown;
        this.#restartFailed = `${instance.kind}:${instance.name}`;
      }

      return false;
    } finally {
      if (resets === this.#resets) this.#restarting = undefined;
    }
  }

  /** Reads the index and what the person may do with it, once for every caller. */
  load(): Promise<void> {
    this.#accessFlight ??= this.#readAccess();
    this.#flight ??= Promise.all([this.#read(), this.#accessFlight])
      .then(() => undefined)
      .finally(() => {
        this.#flight = undefined;
      });

    return this.#flight;
  }

  /**
   * Installs a release of a row, which is also the update of an installed
   * one, approving exactly `expect`. With no `expect` the row's own requests
   * are approved, and a row that does not know them is installed switched off
   * first, so the person reads them before approving (`pending`).
   */
  install(
    offer: ModuleOffer,
    options: {
      expect?: ModuleRequests;
      version?: string;
      /** Also grants the ports the release lists. */
      approveListen?: boolean;
      /** The ports a person moved, by name; the rest keep the manifest's. */
      listen?: Readonly<Record<string, number>>;
      /** Where the module's own database goes; left out, the server decides. */
      database?: DatabaseChoice;
    } = {},
  ): Promise<boolean> {
    const version = options.version ?? offer.version;
    const expect =
      options.expect ??
      (version === offer.version ? requestsOfOffer(offer) : undefined);

    if (expect === undefined) return this.#inspect(offer, version);

    const update = offer.installed;

    return this.#act(
      offer.module,
      async () => {
        // The install adds what the plan the person was shown adds, so the
        // plan is read first when the dialog did not read it.
        const dependencies = await this.#approvedPlan(offer, version);

        return this.#client.installModule({
          body: releaseOf(
            offer,
            version,
            expect,
            options.approveListen ?? false,
            options.listen,
            options.database,
            dependencies,
          ),
        });
      },
      (answer) => {
        this.#pending = undefined;
        this.#outcome = { offer, update, answer };
      },
      (thrown) => {
        // The plan moved since it was read: the next question reads it again.
        if (thrown instanceof ApiError && thrown.code === PLAN_CHANGED) {
          const stale = planKey(offer, version);
          this.#plans = Object.fromEntries(
            Object.entries(this.#plans).filter(([key]) => key !== stale),
          );
        }
        const found = foundOf(thrown);
        // A marketplace release is held to what the marketplace lists for it
        // as well, and approving more here cannot change that list.
        if (found === undefined || offer.origin === 'marketplace') {
          return false;
        }

        this.#pending = {
          cause: 'changed',
          offer,
          version,
          requests: mergeRequests(
            expect,
            requestsOfLines(found, offer.namespace),
          ),
          found,
          namespace: offer.namespace,
        };

        return true;
      },
    );
  }

  /** Approves what the pending install asks for, in a second request. */
  approvePending(
    approveListen = false,
    listen: Readonly<Record<string, number>> = {},
    database?: DatabaseChoice,
  ): Promise<boolean> {
    const pending = this.#pending;
    if (pending === undefined) return Promise.resolve(false);

    return this.install(pending.offer, {
      expect: pending.requests,
      version: pending.version,
      approveListen,
      listen,
      ...(database === undefined ? {} : { database }),
    });
  }

  /** Leaves the pending install as the server holds it. */
  dismissPending(): void {
    this.#pending = undefined;
  }

  /** Forgets the last install's answer, once the person has read it. */
  dismissOutcome(): void {
    this.#outcome = undefined;
  }

  /**
   * Takes an installed module away. It names the module and its namespace
   * only, so a module installed by hand, which no index row offers, is removed
   * the same way. `withUnused` also removes the modules an install added for
   * it that nothing else needs, which the person chose to (ADR-20261027-e6k
   * rule 19a).
   */
  remove(removed: RemovedModule, withUnused = false): Promise<boolean> {
    const namespace = removed.namespace ?? '';
    if (namespace === '') return Promise.resolve(false);

    return this.#act(removed.module, () =>
      this.#client.uninstallModule({
        path: { namespace },
        ...(withUnused ? { query: { with_unused: true } } : {}),
      }),
    );
  }

  /**
   * Switches the marketplace off or on, then reads the index again: switched
   * off, the server lists only its own rows and fetches nothing from it.
   */
  async setMarketplace(enabled: boolean): Promise<boolean> {
    if (this.#switching || this.#acting !== undefined) return false;
    if (this.#marketplace === undefined) return false;
    if (this.#marketplace.enabled === enabled) return true;

    return this.#writeMarketplace({ enabled }, true);
  }

  /**
   * Lists the extensions the marketplace marks community, or hides them
   * again. The server answers every row either way, so the index is not read
   * again: the page hides the rows itself.
   */
  async setCommunity(shown: boolean): Promise<boolean> {
    if (this.#switching || this.#acting !== undefined) return false;
    const marketplace = this.#marketplace;
    if (marketplace === undefined) return false;
    if (marketplace.community === shown) return true;

    return this.#writeMarketplace(
      { enabled: marketplace.enabled, community: shown },
      false,
    );
  }

  async #writeMarketplace(
    body: { enabled: boolean; community?: boolean },
    reread: boolean,
  ): Promise<boolean> {
    const resets = this.#resets;
    this.#switching = true;
    this.#switchError = undefined;

    try {
      const answer = await this.#client.setModuleMarketplace({ body });
      if (resets !== this.#resets) return false;
      this.#marketplace = answer;
      if (reread) await this.#read();

      return true;
    } catch (thrown) {
      if (resets === this.#resets && isModulesDisabled(thrown)) {
        this.#switchOff();
        await this.#changed();
      } else if (resets === this.#resets) {
        this.#switchError = thrown;
      }

      return false;
    } finally {
      if (resets === this.#resets) this.#switching = false;
    }
  }

  /**
   * Writes one change to an installed module, fenced by the version the page
   * was shown: a module someone else changed meanwhile is refused with 412,
   * and the page reads it again instead of overwriting it. The config sent
   * back carries the redacted secrets, which the server reads as "keep".
   * The hidden contributions go back as they were unless the change sets them.
   */
  async writeInstance(
    instance: ResourceInstance,
    change: InstanceChange,
  ): Promise<boolean> {
    const resets = this.#resets;
    this.#writing = instance.namespace ?? instance.name;
    this.#writeModule = this.#writing;
    if (change.hidden_contributions !== undefined)
      this.#writeFor = 'visibility';
    else if (change.staged_routes !== undefined) this.#writeFor = 'uploads';
    else if (
      change.on_over_memory !== undefined ||
      change.memory_mib !== undefined ||
      change.switch_off !== undefined
    )
      this.#writeFor = 'memory';
    else this.#writeFor = change.config === undefined ? 'switch' : 'settings';
    this.#writeError = undefined;
    this.#writeConflict = false;
    const switchOff =
      change.switch_off === undefined
        ? instance.switch_off
        : (change.switch_off ?? undefined);

    try {
      await this.#client.putResource({
        path: { kind: instance.kind as ResourceKind, name: instance.name },
        header: { 'If-Match': `"${instance.version}"` },
        body: {
          type: instance.type,
          enabled: change.enabled ?? instance.enabled,
          required: instance.required,
          config: change.config ?? instance.config,
          // The write replaces the whole spec: leaving the list out clears it.
          hidden_contributions:
            change.hidden_contributions ?? instance.hidden_contributions ?? [],
          // Only a module that runs a process has a memory budget, and the
          // server refuses on_over_memory on any other instance. For one
          // that has, leaving these out resets the operator's memory budget,
          // on_over_memory and switch-off.
          ...(instance.runtime === 'service'
            ? {
                memory_mib: change.memory_mib ?? instance.memory_mib ?? 0,
                on_over_memory:
                  change.on_over_memory ?? instance.on_over_memory ?? 'stop',
              }
            : {}),
          ...(switchOff === undefined ? {} : { switch_off: switchOff }),
          // A route left out keeps its limit, so a write of another field
          // sends none.
          ...(change.staged_routes === undefined
            ? {}
            : { staged_routes: change.staged_routes }),
        },
      });

      return resets === this.#resets;
    } catch (thrown) {
      if (resets !== this.#resets) return false;
      if (isModulesDisabled(thrown)) {
        // The finally block reads the installed listing again.
        this.#switchOff();
      } else if (thrown instanceof ApiError && thrown.status === 412) {
        this.#writeConflict = true;
      } else {
        this.#writeError = thrown;
      }

      return false;
    } finally {
      if (resets === this.#resets) {
        // The write stays open until the listing is read again. A second
        // save sooner would send the version from before this one, and the
        // server would refuse the person's own change as someone else's.
        try {
          await Promise.all([this.#changed(), this.readMemory()]);
        } finally {
          if (resets === this.#resets) this.#writing = undefined;
        }
      }
    }
  }

  /** Clears the last refusal of an action, once the person has read it. */
  dismiss(): void {
    this.#actionError = undefined;
    this.#actionModule = undefined;
  }

  /** Forgets what was read for the person who left. */
  reset(): void {
    this.#resets += 1;
    this.#reading?.abort();
    this.#reading = undefined;
    this.#flight = undefined;
    this.#accessFlight = undefined;
    this.#items = [];
    this.#marketplace = undefined;
    this.#loading = false;
    this.#loaded = false;
    this.#error = undefined;
    this.#refusedOff = false;
    this.#access = undefined;
    this.#operate = undefined;
    this.#restarting = undefined;
    this.#restartError = undefined;
    this.#restartFailed = undefined;
    this.#acting = undefined;
    this.#actionError = undefined;
    this.#actionModule = undefined;
    this.#writing = undefined;
    this.#writeError = undefined;
    this.#writeConflict = false;
    this.#writeModule = undefined;
    this.#writeFor = undefined;
    this.#pending = undefined;
    this.#outcome = undefined;
    this.#switching = false;
    this.#switchError = undefined;
    this.#plans = {};
    this.#removals = {};
    this.#whys = {};
    this.#outdated = undefined;
  }

  // Installs the release switched off, so the answer names what it asks for.
  #inspect(offer: ModuleOffer, version: string): Promise<boolean> {
    return this.#act(
      offer.module,
      async () => {
        // The plan goes with an unread install too, so a release that needs
        // modules this server lacks is written switched off with them rather
        // than refused for a plan it never sent.
        const dependencies = await this.#approvedPlan(offer, version);

        return this.#client.installModule({
          body: releaseOf(
            offer,
            version,
            undefined,
            false,
            {},
            undefined,
            dependencies,
          ),
        });
      },
      (answer) => {
        const lines = answer.requests ?? [];
        this.#pending = {
          cause: 'unread',
          offer,
          version,
          requests: requestsOfLines(lines, answer.namespace),
          found: lines,
          namespace: answer.namespace,
        };
      },
    );
  }

  async #read(): Promise<void> {
    // Nothing can be installed, so there is nothing to offer. The installed
    // modules come from the resources listing, and stay removable.
    if (!this.modulesOn) {
      this.#reading?.abort();
      this.#reading = undefined;
      this.#offerNothing();

      return;
    }

    this.#reading?.abort();
    const reading = new AbortController();
    this.#reading = reading;
    const { signal } = reading;
    this.#loading = true;
    try {
      const answer = await this.#client.listModuleIndex({}, signal);
      if (signal.aborted) return;
      this.#items = answer.items ?? [];
      this.#marketplace = answer.marketplace ?? undefined;
      this.#error = undefined;
      this.#loaded = true;
    } catch (thrown) {
      if (signal.aborted) return;
      if (isModulesDisabled(thrown)) {
        this.#refusedOff = true;
        this.#offerNothing();
      } else {
        this.#error = thrown;
      }
    } finally {
      if (!signal.aborted) this.#loading = false;
    }
  }

  // The operator switched modules off after this page read the settings. The
  // page then says so, rather than this one refusal. Each caller reads the
  // installed listing again once, so no row still shows as running.
  #switchOff(): void {
    this.#refusedOff = true;
    this.#offerNothing();
  }

  // The listing while modules are off: no row and no marketplace, and no
  // error, because the refusal is the state the page explains.
  #offerNothing(): void {
    this.#items = [];
    this.#marketplace = undefined;
    this.#error = undefined;
    this.#loaded = true;
  }

  // Asks once per person which kinds they may configure and which they may
  // operate. A check that fails is left unknown rather than read as a no.
  async #readAccess(): Promise<void> {
    const resets = this.#resets;
    const ask = (action: 'configure_resource' | 'operate_resource') =>
      Promise.all(
        MODULE_KINDS.map(async (kind) => {
          try {
            const decision = await this.#client.checkAccess({
              body: { action, object_type: 'resource_kind', object_id: kind },
            });

            return [kind, decision.allowed] as const;
          } catch {
            return undefined;
          }
        }),
      );
    const [configure, operate] = await Promise.all([
      ask('configure_resource'),
      ask('operate_resource'),
    ]);
    if (resets !== this.#resets) return;

    const known = configure.filter((answer) => answer !== undefined);
    if (known.length > 0) this.#access = Object.fromEntries(known);
    const operable = operate.filter((answer) => answer !== undefined);
    if (operable.length > 0) this.#operate = Object.fromEntries(operable);
  }

  async #act<T>(
    module: string,
    send: () => Promise<T>,
    done: (answer: T) => void = () => undefined,
    refused: (thrown: unknown) => boolean = () => false,
  ): Promise<boolean> {
    if (this.#acting !== undefined || this.#switching) return false;

    const resets = this.#resets;
    this.#acting = module;
    this.#actionError = undefined;
    this.#actionModule = undefined;
    this.#outcome = undefined;

    try {
      const answer = await send();
      if (resets !== this.#resets) return false;
      done(answer);
      // What is installed moved, so every plan and removal check read
      // before it may be wrong now.
      this.#plans = {};
      this.#removals = {};
      this.#whys = {};
      this.#outdated = undefined;

      return true;
    } catch (thrown) {
      if (resets === this.#resets && isModulesDisabled(thrown)) {
        this.#pending = undefined;
        this.#switchOff();
      } else if (resets === this.#resets && !refused(thrown)) {
        this.#pending = undefined;
        this.#actionError = thrown;
        this.#actionModule = module;
      }

      return false;
    } finally {
      // A write that failed may still have changed what is installed, so
      // the reads follow every answer.
      if (resets === this.#resets) {
        // A refresh that throws must not leave the row busy.
        await Promise.allSettled([
          this.#read(),
          this.#changed(),
          this.readMemory(),
        ]);
        if (resets === this.#resets) this.#acting = undefined;
      }
    }
  }
}
