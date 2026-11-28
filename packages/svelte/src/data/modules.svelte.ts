// Moved from frontend/src/lib/stores/modules.svelte.ts, 2026-09-11
import { hiddenEntries } from '../module-hidden';
import { modulePageHref } from '../module-href';
import type { ModuleThemeOption } from '../styles/module-theme.svelte';
import {
  type ModulesClient,
  readResources,
  type ResourceInstance,
} from './services/settings';

/**
 * The fault as it was thrown, and never as a sentence. A screen renders it
 * through `describeFault`, which reads the registry code and keeps the
 * server's English message beside the translation (frontend-i18n.md rule 5).
 * Flattening it to a message here would throw the code away.
 */
const asError = (thrown: unknown): Error =>
  thrown instanceof Error ? thrown : new Error(String(thrown));

/** One page a module's manifest declares, before any bundle has been imported. */
export interface AnnouncedPage {
  namespace: string;
  path: string;
  title: string;
  /** The title of the module that declares it, which heads its pages when there are several. */
  module?: string | undefined;
  hidden: boolean;
  /** The icon name the manifest wrote, when it wrote one. */
  icon?: string | undefined;
  /** Where an app serves it: `/m/<namespace>/<path>` (ADR-20260911-4p6 rule 9). */
  href: string;
}

/**
 * What this server has installed, read once from `GET /api/v1/resources`.
 *
 * It is what makes activation lazy (ADR-20260911-4p6 rule 25). The sidebar and the
 * router are drawn from this listing alone: a module's manifest says which
 * pages it will add, so the menu is the same on every reload and no bundle
 * has to run to draw it. The bundle itself is imported by the loader in
 * `./slots`, when the module's page first opens or its first slot first
 * renders.
 *
 * The loader reads the same listing again when it runs. That is one extra
 * request for a short listing, and the alternative is handing the loader a
 * listing this store read, which the loader does not take today.
 *
 * It holds the data and builds no menu: `SlotRegistry.sidebarItems()` in
 * `../slots` turns the pages into the sidebar's entries, so the store has
 * nothing to say about how the chrome is drawn.
 */
export class Modules {
  readonly #client: ModulesClient;
  #items = $state.raw<readonly ResourceInstance[]>([]);
  #loading = $state(false);
  #loaded = $state(false);
  #error = $state<Error | undefined>(undefined);
  #flight: Promise<void> | undefined;
  // Aborted when a fresh read or a reset supersedes the read in flight, so
  // that read records nothing.
  #controller: AbortController | undefined;

  constructor(client: ModulesClient) {
    this.#client = client;
  }

  /** Read-only by design: the collection is the server's (svelte-5.md). */
  get items(): readonly ResourceInstance[] {
    return this.#items;
  }

  get loading(): boolean {
    return this.#loading;
  }

  /** True once the listing has answered, which tells an empty server from an unread one. */
  get loaded(): boolean {
    return this.#loaded;
  }

  get error(): Error | undefined {
    return this.#error;
  }

  /** The instances that carry a `ui` object, which are the ones with a bundle. */
  readonly withUi: readonly ResourceInstance[] = $derived(
    this.#items.filter((item) => item.ui !== undefined),
  );

  /** Every page a module declared, whether or not its bundle has run. */
  readonly pages: readonly AnnouncedPage[] = $derived(
    this.withUi.flatMap((item) => {
      const namespace = item.namespace ?? '';
      if (namespace === '') return [];
      const hidden = hiddenEntries(item);
      // An empty title falls through as well, which `??` would keep.
      const module =
        [item.module?.title, item.name].find(
          (name) => name !== undefined && name !== '',
        ) ?? namespace;

      // A page the operator hid, or the running module hides now, is hidden
      // the way a manifest-hidden one is: no menu entry, and its route still
      // answers.
      return (item.ui?.contributes.pages ?? []).map((page) => ({
        namespace,
        path: page.path,
        title: page.title,
        module,
        hidden: page.hidden || hidden.includes(`pages/${page.path}`),
        icon: page.icon,
        href: modulePageHref(namespace, page.path),
      }));
    }),
  );

  /** Every theme a module offers, as the appearance picker lists them. */
  readonly themes: readonly ModuleThemeOption[] = $derived(
    this.withUi.flatMap((item) => {
      const namespace = item.namespace ?? '';
      if (namespace === '') return [];
      const module =
        [item.module?.title, item.name].find(
          (name) => name !== undefined && name !== '',
        ) ?? namespace;

      return (item.ui?.themes ?? []).map((theme) => ({
        key: `${namespace}/${theme.id}`,
        title: theme.title,
        module,
        href: theme.stylesheet_url,
      }));
    }),
  );

  /**
   * A second call while one read is in flight is handed that read. A `fresh`
   * call, made after a write, aborts that read and starts its own, so what
   * lands is what the server held after the write.
   */
  load(options: { fresh?: boolean } = {}): Promise<void> {
    if (this.#flight && options.fresh !== true) return this.#flight;

    this.#controller?.abort();
    const controller = new AbortController();
    this.#controller = controller;
    const flight = this.#read(controller.signal).finally(() => {
      if (this.#flight === flight) this.#flight = undefined;
    });
    this.#flight = flight;

    return flight;
  }

  /**
   * Forgets what this store read of the installed modules. The app calls it
   * when the person signed in changes, so the next person never sees the
   * pages the last one was allowed to see. A read that started before the
   * reset records nothing.
   */
  reset(): void {
    this.#controller?.abort();
    this.#controller = undefined;
    this.#flight = undefined;
    this.#items = [];
    this.#loading = false;
    this.#loaded = false;
    this.#error = undefined;
  }

  async #read(signal: AbortSignal): Promise<void> {
    this.#loading = true;
    try {
      const page = await readResources(this.#client, signal);
      if (signal.aborted) return;
      this.#items = page.items ?? [];
      this.#loaded = true;
      this.#error = undefined;
    } catch (thrown) {
      if (signal.aborted) return;
      // A deployment where this person may see no resource answers a refusal,
      // and that is an empty list rather than a broken screen.
      this.#items = [];
      this.#error = asError(thrown);
    } finally {
      if (!signal.aborted) this.#loading = false;
    }
  }
}
