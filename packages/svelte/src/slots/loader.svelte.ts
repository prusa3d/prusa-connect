// Reading the listing, deciding what may load, and loading it.
//
// Moved from pull request 185, packages/core/src/plugins/host.svelte.ts and
// load.ts, 2026-09-11. Three things come across: the admission order, which
// is check first and import second, so a bundle that fails the check never
// runs; one module at a time, so the sidebar is the same menu on every
// reload; and refusals kept as state, because "it is installed and it is not
// running" is the question an operator asks first.
//
// What stays behind is PR 185's static `plugins/index.json` and its
// `assetBase` closure. The core's own listing carries every enabled instance
// with its `ui` object and the URL its files are served at, so there is
// nothing for the page to assemble (earlier research and
// recommendation 17).
import type { PrusaConnect } from '@prusa/connect';
import type { Component } from 'svelte';

import type { ReadonlyFleet } from '../domain/fleet';
import { type ModulePrincipal, readPrincipal } from '../domain/principal';
import {
  forgetCatalogue,
  locale,
  registerCatalogue,
  SOURCE_LOCALE,
  translateFor,
} from '../i18n';
import { hiddenEntries } from '../module-hidden';
import { modulePageHref } from '../module-href';
import { LIBRARY_VERSION } from '../version';
import { buildRegister } from './generated';
import {
  ComponentUnavailable,
  type ModuleComponents,
  type ModuleHost,
  type ModuleSetup,
} from './host';
import { dynamicImport, type ModuleImport } from './import-module';
import {
  createModuleStore,
  forgetModuleCaches,
  type ModuleStorage,
  type ScopedStore,
} from './module-store.svelte';
import {
  catalogueUrl,
  type CatalogueReader,
  readCatalogue,
} from './read-catalogue';
import {
  type SlotRefusal,
  type SlotRegistry,
  UndeclaredRegistration,
} from './registry.svelte';
import type { ModuleUI } from './types';
import { satisfiesRange } from './version-range';

/** The resources listing, as the SDK answers it. */
type ResourceListing = Awaited<ReturnType<PrusaConnect['listResources']>>;

/** One module the loader ran, in the order it ran them. */
export interface LoadedModule {
  namespace: string;
  ui: ModuleUI;
}

export interface ModuleLoaderOptions {
  /** The app's `PrusaConnect`, in session mode. It is what a module receives. */
  client: PrusaConnect;
  registry: SlotRegistry;
  fleet: ReadonlyFleet;
  /** Read from the core when it is not given. */
  principal?: ModulePrincipal;
  /** This library's version. It defaults to the one the package publishes. */
  version?: string;
  /** Defaults to a dynamic `import()` of the entry URL. */
  importModule?: ModuleImport;
  /**
   * Defaults to a store scoped to the namespace and to the person, which
   * keeps its records in the core through `client`.
   */
  createStore?: (namespace: string, principal: ModulePrincipal) => ScopedStore;
  /**
   * Where module stores cache their records. It defaults to the browser's
   * `localStorage`, and `null` caches nothing.
   */
  storage?: ModuleStorage | null;
  /** Defaults to reading `locales/<lang>.json` beside the module's entry. */
  readCatalogue?: CatalogueReader;
}

const asDetail = (thrown: unknown): string =>
  thrown instanceof Error ? thrown.message : String(thrown);

const setupOf = (module: unknown): ModuleSetup | undefined => {
  if (typeof module !== 'object' || module === null) return undefined;
  // An imported module crosses into our code as `unknown`; this narrows it to
  // the one export that is read (typescript.md).
  const exported = (module as { default?: unknown }).default;

  return typeof exported === 'function' ? (exported as ModuleSetup) : undefined;
};

/**
 * Activation is lazy (ADR-20260911-4p6 rule 25). The sidebar is drawn from the
 * listing alone, and this runs when a module's page first opens or its first
 * slot first renders. `activate` is safe to call from every outlet on the
 * screen: the first call does the work and the rest are handed its promise.
 * A dashboard calls `activateFor` with the modules of the cards it draws,
 * and loads no other bundle (ADR-20261008-sww rule 25).
 */
export class ModuleLoader {
  readonly #options: ModuleLoaderOptions;
  readonly registry: SlotRegistry;
  #loaded = $state<LoadedModule[]>([]);
  #ready = $state(false);
  #discovered = $state(false);
  #unread = $state(false);
  #flight: Promise<void> | undefined;
  // The listing and the principal of the current run, read once and shared
  // by activate, activateFor and discover. A read that failed is dropped, so
  // the next call reads again.
  #discovery: Promise<Discovery | undefined> | undefined;
  // The run the listing was last read for, so a full activation marks the
  // loader ready only for the run it belongs to.
  #epoch = 0;
  // The modules the current run tried to load, admitted or not, so no call
  // loads one twice. Nothing draws it, so it is a plain set.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  #attempted = new Set<string>();
  // The tail of the calls that load modules, which run one after another.
  #chain: Promise<void> = Promise.resolve();
  // Counts the resets. A run that started before a reset admits nothing.
  #runs = 0;
  // The stores handed to modules, so a reset can stop them.
  #stores: ScopedStore[] = [];
  // Aborted when the modules of the current run stop belonging to the page.
  // Every host of the run carries its signal, and every request of the run
  // goes through a client bound to it.
  #admitted = new AbortController();
  // The modules whose setup is running now, each as `<run>/<namespace>`, so
  // a load from inside one is refused instead of waiting for a chain that
  // waits for it. Nothing draws it, so it is a plain set.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  #settingUp = new Set<string>();
  // The namespaces whose catalogues a run merged, including a run a reset
  // ended part way, so the reset forgets every one of them. Nothing draws
  // it, so it is a plain set and not a reactive one.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  #catalogued = new Set<string>();

  constructor(options: ModuleLoaderOptions) {
    this.#options = options;
    this.registry = options.registry;
  }

  /** The modules that ran, in the order they ran. */
  get loaded(): readonly LoadedModule[] {
    return this.#loaded;
  }

  /** False until the listing has been read, so a screen waits rather than flashes. */
  get ready(): boolean {
    return this.#ready;
  }

  /**
   * True once this run's listing read has finished, answered or failed.
   * Until then the registry does not know which cards the modules declare,
   * so a card whose type it does not know yet is still loading, not unknown.
   */
  get discovered(): boolean {
    return this.#discovered;
  }

  /**
   * True when this run's last listing read failed, so a card whose type no
   * module declares may belong to a module the core could not name, and is
   * not known to be missing.
   */
  get unread(): boolean {
    return this.#unread;
  }

  /** Why a module is not on the screen. The registry holds the one list. */
  get refusals(): readonly SlotRefusal[] {
    return this.registry.refusals();
  }

  /** Runs once. A second call is handed the first call's promise. */
  activate(): Promise<void> {
    this.#flight ??= this.#queue(() => true).then(() => {
      if (this.#epoch === this.#runs) this.#ready = true;
    });

    return this.#flight;
  }

  /**
   * Loads only the modules named, which is what a dashboard does: it imports
   * the bundles whose cards it draws and no other (ADR-20261008-sww rule 25). A
   * module this run already loaded, or refused, is not loaded again, and a
   * later `activate()` loads only the rest.
   */
  activateFor(namespaces: readonly string[]): Promise<void> {
    // A lookup this call reads and drops, never state a component reads.
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    const wanted = new Set(namespaces);

    return this.#queue((namespace) => wanted.has(namespace));
  }

  /**
   * Reads the listing and records what every module declares, without
   * importing any bundle. A card picker reads the declared cards from the
   * registry after this, before any module runs. A second call is handed the
   * first call's promise.
   */
  discover(): Promise<void> {
    return this.#discover().then(() => undefined);
  }

  /**
   * Reads the listing again and replaces the module set. Everything a
   * previous run registered is forgotten first, a run still going included,
   * so a reload does not leave two copies of a page in the sidebar.
   */
  reload(): Promise<void> {
    this.#end();
    this.#loaded = [];
    this.#ready = false;
    this.#flight = undefined;

    return this.activate();
  }

  /**
   * Forgets every module, the records their stores cached in this browser,
   * and the person they were admitted for. The app calls it when the person
   * signed in changes. Nothing runs again until an outlet asks, and the next
   * run reads the listing and the principal for the new person.
   */
  reset(): void {
    this.#end();
    this.#loaded = [];
    this.#ready = false;
    this.#flight = undefined;
    forgetModuleCaches(this.#options.storage);
  }

  // Ends the current run: a run still going admits nothing more, every host
  // it handed out is aborted with every request of its client, every store it
  // built is stopped, and everything it registered and merged is forgotten.
  #end(): void {
    this.#runs += 1;
    this.#admitted.abort();
    this.#admitted = new AbortController();
    for (const store of this.#stores) closeStore(store);
    this.#stores = [];
    this.registry.clear();
    for (const namespace of this.#catalogued) forgetCatalogue(namespace);
    this.#catalogued.clear();
    this.#discovery = undefined;
    this.#discovered = false;
    this.#unread = false;
    this.#attempted.clear();
    this.#chain = Promise.resolve();
  }

  /**
   * The components one module may draw from other modules
   * (ADR-20261027-wvj rule 22). Each check that fails rejects with the reason
   * it failed for, so the importing bundle draws its own fallback.
   */
  #componentsFor(
    namespace: string,
    ui: ModuleUI,
    discovery: Discovery,
  ): ModuleComponents {
    const unavailable = (
      reason: ConstructorParameters<typeof ComponentUnavailable>[0],
      module: string,
      name: string,
    ) => Promise.reject(new ComponentUnavailable(reason, module, name));

    return {
      load: async <Props extends Record<string, unknown>>(
        module: string,
        name: string,
      ) => {
        const imported = (ui.imports ?? []).some(
          (held) => held.module === module && held.name === name,
        );
        if (!imported) return unavailable('undeclared', module, name);

        // A component already registered is answered at once, whatever
        // else is loading.
        const held = this.registry.component(module, name);
        if (held !== undefined) {
          // The registry holds every component as the narrowest type; the
          // props are the exporting module's to publish, and the caller
          // names them.
          return held as unknown as Component<Props>;
        }

        if (this.#settingUp.has(`${String(this.#runs)}/${namespace}`)) {
          return unavailable('setup', module, name);
        }
        if (this.registry.refused(module)) {
          return unavailable('failed', module, name);
        }

        const provider = discovery.instances.find(
          (instance) => instance.namespace === module,
        );
        if (provider === undefined) return unavailable('absent', module, name);
        if (!(provider.ui.exports ?? []).some((held) => held.name === name)) {
          return unavailable('not-exported', module, name);
        }

        await this.activateFor([module]);

        const component = this.registry.component(module, name);
        if (component === undefined) {
          return unavailable(
            this.registry.refused(module) ? 'failed' : 'unregistered',
            module,
            name,
          );
        }

        // The registry holds every component as the narrowest type; the
        // props are the exporting module's to publish, and the caller names
        // them.
        return component as unknown as Component<Props>;
      },
    };
  }

  /**
   * Loads every module the filter keeps and this run has not tried yet, in
   * the listing's order. Calls are chained, so two of them never load one
   * module twice and the order a module registers in never depends on which
   * call came first.
   */
  #queue(keep: (namespace: string) => boolean): Promise<void> {
    const run = this.#runs;
    const next = this.#chain.then(async () => {
      if (run !== this.#runs) return;
      await this.#load(run, keep);
    });
    this.#chain = next.catch(() => undefined);

    return next;
  }

  /**
   * The listing and the principal of this run, read once. A read that failed
   * is not kept, so a core that was briefly away is asked again by the next
   * call, and not only after a reset.
   */
  #discover(): Promise<Discovery | undefined> {
    if (this.#discovery !== undefined) return this.#discovery;

    const read = this.#readListing(this.#runs);
    this.#discovery = read;
    void read.then((found) => {
      if (found === undefined && this.#discovery === read) {
        this.#discovery = undefined;
      }
    });

    return read;
  }

  async #readListing(run: number): Promise<Discovery | undefined> {
    const stale = (): boolean => run !== this.#runs;
    const { version = LIBRARY_VERSION } = this.#options;
    this.#epoch = run;
    const client = this.#options.client.withSignal(this.#admitted.signal);

    let listing: ResourceListing;
    let principal: ModulePrincipal;
    try {
      listing = await client.listResources();
      principal = this.#options.principal ?? (await readPrincipal(client));
    } catch {
      // A deployment with no module the person may see answers nothing to
      // load, and that is the default rather than an error.
      if (!stale()) {
        this.#discovered = true;
        this.#unread = true;
      }
      return undefined;
    }
    if (stale()) return undefined;

    const admissible: Instance[] = [];
    for (const instance of listing.items ?? []) {
      const { namespace, ui } = instance;
      if (namespace === undefined || ui === undefined) continue;

      this.registry.declare(namespace, ui.contributes, hiddenEntries(instance));
      this.registry.declareExports(
        namespace,
        (ui.exports ?? []).map((exported) => exported.name),
      );

      // A bundle that only offers themes runs no script, so there is nothing
      // to import. Its themes are read from the listing by the Modules store.
      const entry = ui.entry_url;
      if (entry === undefined || entry === '') continue;

      if (!satisfiesRange(version, ui.library)) {
        this.registry.refuse({
          namespace,
          reason: 'library-mismatch',
          range: ui.library,
          version,
        });
        continue;
      }

      admissible.push({
        namespace,
        self: `${instance.kind}:${instance.name}`,
        entry,
        ui,
        version: instance.module?.version,
      });
    }
    this.#discovered = true;
    this.#unread = false;

    return { principal, client, instances: admissible };
  }

  async #load(
    run: number,
    keep: (namespace: string) => boolean,
  ): Promise<void> {
    const signal = this.#admitted.signal;
    const stale = (): boolean => run !== this.#runs;
    const discovery = await this.#discover();
    if (stale() || discovery === undefined) return;
    // The client every request of this run goes through. A reset or a reload
    // aborts what it has in flight and refuses what it is asked after, so a
    // module cannot act with the authority of the next person's cookie.
    const { client, principal } = discovery;

    const {
      fleet,
      importModule = dynamicImport,
      storage,
      createStore = (namespace: string, person: ModulePrincipal) =>
        createModuleStore(namespace, {
          client,
          person: person.id,
          ...(storage === undefined ? {} : { storage }),
        }),
      readCatalogue: catalogueOf = readCatalogue,
    } = this.#options;

    /**
     * A module's own texts, merged under its namespace (frontend-i18n.md
     * rule 7). They are read from `locales/` beside the entry, which is what
     * a module built on this library's Vite plugin writes.
     *
     * Two files at most: English, because it is the source every other
     * language falls back to, and the language on screen when that is not
     * English. A file that is not there is not a fault; the module has not
     * been translated yet.
     */
    const merge = async (namespace: string, entry: string): Promise<void> => {
      const wanted =
        locale.current === SOURCE_LOCALE
          ? [SOURCE_LOCALE]
          : [SOURCE_LOCALE, locale.current];

      for (const language of wanted) {
        try {
          const messages = await catalogueOf(
            catalogueUrl(entry, language),
            signal,
          );
          // A run a reset ended registers nothing, so it cannot put its
          // words back after the reset forgot them.
          if (stale()) return;
          if (messages) {
            this.#catalogued.add(namespace);
            registerCatalogue(namespace, language, messages);
          }
        } catch {
          // A module with no catalogue draws the English its code carries. It
          // is never a reason to drop the module.
        }
      }
    };

    // One at a time rather than in parallel. A module contributes its pages
    // in the order it is loaded, and a sidebar whose order depends on which
    // bundle answered first would be a different menu on every reload.
    for (const { namespace, self, entry, ui, version } of discovery.instances) {
      if (stale()) return;
      if (!keep(namespace) || this.#attempted.has(namespace)) continue;
      this.#attempted.add(namespace);

      try {
        // The catalogue is merged before the bundle runs, so a module that
        // draws in its own setup already has its words.
        await merge(namespace, entry);
        if (stale()) return;

        let imported: unknown;
        try {
          imported = await importModule(importUrl(entry, version));
        } catch {
          if (stale()) return;
          imported = await importModule(importUrl(entry, version, Date.now()));
        }
        const setup = setupOf(imported);
        if (stale()) return;
        if (!setup) {
          throw new Error(`${entry} default-exports no setup function`);
        }

        const store = createStore(namespace, principal);
        this.#stores.push(store);
        // A module registers after a reset only if its setup was still
        // running then. The registration is for the person who left, so it
        // is dropped.
        const host: ModuleHost = {
          client,
          principal,
          fleet,
          store,
          t: translateFor(namespace),
          signal,
          namespace,
          self,
          pageHref: (path) => modulePageHref(namespace, path),
          uploadToPrinter: (deviceId, files, action) => {
            if (!stale()) {
              this.registry.requestUpload({
                namespace,
                action,
                deviceId,
                files,
              });
            }
          },
          sendToPrinter: (deviceId, stored, action) => {
            if (!stale()) {
              this.registry.requestUpload({
                namespace,
                action,
                deviceId,
                files: [],
                stored,
              });
            }
          },
          components: this.#componentsFor(namespace, ui, discovery),
          register: buildRegister({
            page: (path, component) => {
              if (!stale())
                this.registry.registerPage(namespace, path, component);
            },
            section: (slot, id, component) => {
              if (!stale()) {
                this.registry.registerSection(namespace, slot, id, component);
              }
            },
            action: (slot, id, run) => {
              if (!stale())
                this.registry.registerAction(namespace, slot, id, run);
            },
            card: (id, component, options) => {
              if (!stale()) {
                this.registry.registerCard(namespace, id, component, options);
              }
            },
            component: (name, component) => {
              if (!stale()) {
                this.registry.registerComponent(namespace, name, component);
              }
            },
          }),
        };

        const setting = `${String(run)}/${namespace}`;
        this.#settingUp.add(setting);
        try {
          await setup(host);
        } finally {
          this.#settingUp.delete(setting);
        }
        if (stale()) return;
        this.#loaded = [...this.#loaded, { namespace, ui }];
      } catch (thrown) {
        if (stale()) return;
        // A module that throws while registering is dropped whole: its
        // namespace is forgotten, so a half-registered module does not leave
        // one working section and one missing.
        this.registry.forget(namespace);
        forgetCatalogue(namespace);
        if (!(thrown instanceof UndeclaredRegistration)) {
          this.registry.refuse({
            namespace,
            reason: 'threw',
            detail: asDetail(thrown),
          });
        }
      }
    }
  }
}

/** One module the listing names, admitted by the version check. */
interface Instance {
  namespace: string;
  /** The instance's kind and name, such as `sink:simple-queue`. */
  self: string;
  /** Where the bundle's entry module is served. */
  entry: string;
  ui: ModuleUI;
  /** The release the manifest declared; absent when the listing names none. */
  version: string | undefined;
}

/**
 * The address a bundle is imported from. Safari keeps a rejected import in its
 * module map per address for the life of the page, so the address changes with
 * the release, and with `retry` when one attempt already failed. The server
 * serves the entry by path and ignores the query; the bundle's own relative
 * imports resolve against the path and do not carry it.
 */
const importUrl = (
  entry: string,
  version: string | undefined,
  retry?: number,
): string => {
  const text = new URLSearchParams([
    ...(version === undefined ? [] : [['v', version]]),
    ...(retry === undefined ? [] : [['retry', String(retry)]]),
  ]).toString();
  if (text === '') return entry;

  return `${entry}${entry.includes('?') ? '&' : '?'}${text}`;
};

/** What one run read before it loaded anything. */
interface Discovery {
  principal: ModulePrincipal;
  client: PrusaConnect;
  instances: readonly Instance[];
}

// A store the loader built has a `close`. A store a test or an app hands in
// may not, and then there is nothing to stop. The narrowing cast reads one
// optional method, because a module's view of its store has no `close`
// (typescript.md).
const closeStore = (store: ScopedStore): void => {
  const closable = store as ScopedStore & { close?: () => void };
  closable.close?.();
};

/** One loader per app, built where the app builds its registry. */
export const createModuleLoader = (
  options: ModuleLoaderOptions,
): ModuleLoader => new ModuleLoader(options);
