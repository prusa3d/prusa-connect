import { describe, expect, it, vi } from 'vitest';

import type { ReadonlyFleet } from '../domain/fleet';
import { fakeLoaderClient } from '../test/fakes';
import PlainCard from '../test/plain-card.svelte';
import PlainSection from '../test/plain-section.svelte';
import PlainPage from '../test/plain-page.svelte';
import { translateFor } from '../i18n';
import { defineModule, type ModuleSetup } from './host';
import type { ModuleImport } from './import-module';
import { createModuleLoader } from './loader.svelte';
import { SlotRegistry } from './registry.svelte';
import type { ModuleUI } from './types';

// The fleet as an application hands it in, before it has read anything. The
// loader only passes it through to the host object, so the store itself would
// buy the test nothing.
const unreadFleet: ReadonlyFleet = {
  devices: [],
  loading: false,
  loaded: false,
  error: undefined,
  device: () => undefined,
};

const uiFor = (namespace: string, library = '>=0.1.0 <1.0.0'): ModuleUI => ({
  entry_url: `/api/v1/modules/${namespace}/ui/index.js`,
  library,
  contributes: {
    pages: [{ path: namespace, title: namespace, hidden: false }],
  },
});

const loaderWith = (
  modules: readonly { namespace: string; ui: ModuleUI }[],
  setups: Record<string, ModuleSetup>,
) => {
  const registry = new SlotRegistry();
  const imported: string[] = [];
  const importModule: ModuleImport = (url) => {
    imported.push(url);
    const found = Object.entries(setups).find(([namespace]) =>
      url.includes(`/modules/${namespace}/`),
    );

    return Promise.resolve(found ? { default: found[1] } : {});
  };

  const loader = createModuleLoader({
    client: fakeLoaderClient(modules),
    registry,
    fleet: unreadFleet,
    version: '0.1.0',
    importModule,
    createStore: () => ({
      get: () => undefined,
      set: () => undefined,
      delete: () => undefined,
      keys: () => [],
    }),
  });

  return { loader, registry, imported };
};

describe('ModuleLoader', () => {
  describe('importing a bundle', () => {
    const loaderOver = (
      importModule: ModuleImport,
      registry = new SlotRegistry(),
    ) => ({
      registry,
      loader: createModuleLoader({
        client: fakeLoaderClient([
          { namespace: 'spools', ui: uiFor('spools'), release: '0.2.0' },
        ]),
        registry,
        fleet: unreadFleet,
        version: '0.1.0',
        storage: null,
        readCatalogue: () => Promise.resolve(undefined),
        importModule,
      }),
    });
    const setup = {
      default: defineModule((host) => {
        host.register.page('spools', PlainPage);
      }),
    };

    it('names the release in the address, so a new release is a new import', async () => {
      const urls: string[] = [];
      const { loader } = loaderOver((url) => {
        urls.push(url);

        return Promise.resolve(setup);
      });

      await loader.activate();

      expect(urls).toEqual(['/api/v1/modules/spools/ui/index.js?v=0.2.0']);
    });

    it('imports again once, under a fresh address, when the first import is rejected', async () => {
      const urls: string[] = [];
      const { loader, registry } = loaderOver((url) => {
        urls.push(url);

        return urls.length === 1
          ? Promise.reject(new Error('Importing a module script failed.'))
          : Promise.resolve(setup);
      });

      await loader.activate();

      expect(urls).toHaveLength(2);
      expect(urls[1]).toMatch(/\?v=0\.2\.0&retry=\d+$/);
      expect(registry.pages()).toHaveLength(1);
      expect(loader.refusals).toEqual([]);
    });

    it('refuses the module when the second import is rejected too', async () => {
      const urls: string[] = [];
      const { loader } = loaderOver((url) => {
        urls.push(url);

        return Promise.reject(new Error('Importing a module script failed.'));
      });

      await loader.activate();

      expect(urls).toHaveLength(2);
      expect(loader.refusals).toEqual([
        {
          namespace: 'spools',
          reason: 'threw',
          detail: 'Importing a module script failed.',
        },
      ]);
    });
  });

  // ADR-20260911-4p6 rules 19 and 24.
  it('refuses a library range this version does not satisfy, and never imports it', async () => {
    const { loader, registry, imported } = loaderWith(
      [{ namespace: 'spools', ui: uiFor('spools', '>=2.0.0 <3.0.0') }],
      {},
    );

    await loader.activate();

    expect(imported).toEqual([]);
    expect(registry.pages()).toHaveLength(0);
    expect(loader.refusals).toEqual([
      {
        namespace: 'spools',
        reason: 'library-mismatch',
        range: '>=2.0.0 <3.0.0',
        version: '0.1.0',
      },
    ]);
    expect(loader.ready).toBe(true);
  });

  it('imports nothing for a bundle that only offers themes, and refuses nothing', async () => {
    const { loader, imported } = loaderWith(
      [
        {
          namespace: 'themes',
          ui: {
            library: '>=2.0.0 <3.0.0',
            contributes: {},
            themes: [
              {
                id: 'ocean',
                title: 'Ocean',
                stylesheet_url: '/api/v1/modules/themes/ui/ocean.css',
              },
            ],
          },
        },
      ],
      {},
    );

    await loader.activate();

    expect(imported).toEqual([]);
    expect(loader.refusals).toEqual([]);
    expect(loader.ready).toBe(true);
  });

  // ADR-20260911-4p6 rule 26.
  it('records a module that throws and loads the ones after it', async () => {
    const { loader, registry } = loaderWith(
      [
        { namespace: 'broken', ui: uiFor('broken') },
        { namespace: 'spools', ui: uiFor('spools') },
      ],
      {
        broken: defineModule(() => {
          throw new Error('boom');
        }),
        spools: defineModule((host) => {
          host.register.page('spools', PlainPage);
        }),
      },
    );

    await loader.activate();

    expect(loader.refusals).toEqual([
      { namespace: 'broken', reason: 'threw', detail: 'boom' },
    ]);
    expect(loader.loaded.map((module) => module.namespace)).toEqual(['spools']);
    expect(registry.pages().map((page) => page.namespace)).toEqual(['spools']);
  });

  // ADR-20260911-4p6 rule 21: a module that oversteps is dropped whole, not in part.
  it('drops a module that registers what its manifest does not declare', async () => {
    const { loader, registry } = loaderWith(
      [{ namespace: 'spools', ui: uiFor('spools') }],
      {
        spools: defineModule((host) => {
          host.register.page('spools', PlainPage);
          host.register.page('secrets', PlainPage);
        }),
      },
    );

    await loader.activate();

    expect(registry.pages()).toHaveLength(0);
    expect(loader.loaded).toHaveLength(0);
    expect(loader.refusals).toEqual([
      {
        namespace: 'spools',
        reason: 'undeclared',
        slot: 'page',
        name: 'secrets',
      },
    ]);
  });

  it('records a bundle that default-exports no setup function', async () => {
    const { loader } = loaderWith(
      [{ namespace: 'empty', ui: uiFor('empty') }],
      {},
    );

    await loader.activate();

    expect(loader.refusals[0]?.reason).toBe('threw');
  });

  // ADR-20260911-4p6 rule 25: one at a time, so the sidebar is the same on every load.
  it('loads one module at a time, in the listing order', async () => {
    const order: string[] = [];
    const slow =
      (namespace: string): ModuleSetup =>
      async (host) => {
        await Promise.resolve();
        order.push(namespace);
        host.register.page(namespace, PlainPage);
      };
    const { loader, registry } = loaderWith(
      [
        { namespace: 'first', ui: uiFor('first') },
        { namespace: 'second', ui: uiFor('second') },
      ],
      { first: slow('first'), second: slow('second') },
    );

    await loader.activate();

    expect(order).toEqual(['first', 'second']);
    expect(registry.sidebarItems().map((item) => item.href)).toEqual([
      '/m/first/first',
      '/m/second/second',
    ]);
  });

  it('runs once however many outlets ask', async () => {
    const { loader, imported } = loaderWith(
      [{ namespace: 'spools', ui: uiFor('spools') }],
      {
        spools: defineModule((host) => {
          host.register.page('spools', PlainPage);
        }),
      },
    );

    await Promise.all([loader.activate(), loader.activate()]);
    await loader.activate();

    expect(imported).toHaveLength(1);
  });

  it('hands the module the client, the principal, the store, the fleet and its own t', async () => {
    let seen: string[] = [];
    const { loader } = loaderWith(
      [{ namespace: 'spools', ui: uiFor('spools') }],
      {
        spools: defineModule((host) => {
          seen = Object.keys(host).sort();
          expect(host.principal.id).toBe('user:alice');
          expect(host.namespace).toBe('spools');
          expect(host.self).toBe('sink:spools');
          expect(host.fleet.loaded).toBe(false);
          host.register.page('spools', PlainPage);
        }),
      },
    );

    await loader.activate();

    // ADR-20260911-4p6 rule 20: no `fetch` and no token in the object.
    expect(seen).toEqual([
      'client',
      'components',
      'fleet',
      'namespace',
      'pageHref',
      'principal',
      'register',
      'self',
      'sendToPrinter',
      'signal',
      'store',
      't',
      'uploadToPrinter',
    ]);
  });
});

// A browser can sign out and sign in as somebody else without the page
// reloading. The modules admitted for the first person are then forgotten.
describe('ModuleLoader and a dashboard', () => {
  const spoolCard = {
    id: 'spool',
    title: 'Spool',
    description: 'The spool on one printer.',
    size: { w: 4, h: 3 },
    least: { w: 2, h: 2 },
    settings: [],
  };
  const withCard = (namespace: string): ModuleUI => ({
    ...uiFor(namespace),
    contributes: { cards: [spoolCard] },
  });
  const registering = defineModule((host) => {
    host.register.card('spool', PlainCard);
  });

  // ADR-20261008-sww rule 25: a dashboard reads every declared card and imports nothing.
  it('declares every card on discover, and imports no bundle', async () => {
    const { loader, registry, imported } = loaderWith(
      [{ namespace: 'spools', ui: withCard('spools') }],
      { spools: registering },
    );

    await loader.discover();

    expect(imported).toEqual([]);
    expect(registry.declaredCards().map((card) => card.type)).toEqual([
      'spools/spool',
    ]);
  });

  it('says the listing was read only once the read finished', async () => {
    const { loader } = loaderWith(
      [{ namespace: 'spools', ui: withCard('spools') }],
      {},
    );

    expect(loader.discovered).toBe(false);
    const reading = loader.discover();
    expect(loader.discovered).toBe(false);
    await reading;
    expect(loader.discovered).toBe(true);

    loader.reset();
    expect(loader.discovered).toBe(false);
  });

  it('reads the listing again after a read that failed', async () => {
    const registry = new SlotRegistry();
    const client = fakeLoaderClient([
      { namespace: 'spools', ui: withCard('spools') },
    ]);
    const answer = client.listResources.bind(client);
    const listResources = vi
      .fn<typeof client.listResources>()
      .mockRejectedValueOnce(new Error('core away'))
      .mockImplementation(answer);
    Object.assign(client, { listResources });
    const loader = createModuleLoader({
      client,
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      importModule: () => Promise.resolve({}),
    });

    await loader.discover();
    expect(loader.discovered).toBe(true);
    expect(loader.unread).toBe(true);
    expect(registry.declaredCards()).toEqual([]);

    await loader.discover();
    expect(loader.unread).toBe(false);
    expect(listResources).toHaveBeenCalledTimes(2);
    expect(registry.declaredCards().map((card) => card.type)).toEqual([
      'spools/spool',
    ]);

    // A listing that answered is kept for the run.
    await loader.discover();
    expect(listResources).toHaveBeenCalledTimes(2);
  });

  it('loads only the modules named, and none of them twice', async () => {
    const { loader, registry, imported } = loaderWith(
      [
        { namespace: 'spools', ui: withCard('spools') },
        { namespace: 'other', ui: withCard('other') },
      ],
      { spools: registering, other: registering },
    );

    await Promise.all([
      loader.activateFor(['spools']),
      loader.activateFor(['spools']),
    ]);

    expect(imported).toEqual(['/api/v1/modules/spools/ui/index.js']);
    expect(registry.card('spools/spool')?.component).toBe(PlainCard);
    expect(registry.card('other/spool')).toBeUndefined();
    expect(loader.ready).toBe(false);

    await loader.activate();

    expect(imported).toEqual([
      '/api/v1/modules/spools/ui/index.js',
      '/api/v1/modules/other/ui/index.js',
    ]);
    expect(loader.loaded.map((module) => module.namespace)).toEqual([
      'spools',
      'other',
    ]);
    expect(loader.ready).toBe(true);
  });
});

describe('ModuleLoader reset', () => {
  const cardUi = (namespace: string): ModuleUI => ({
    entry_url: `/api/v1/modules/${namespace}/ui/index.js`,
    library: '>=0.1.0 <1.0.0',
    contributes: {
      device_card: [{ id: 'spool', title: 'Spool' }],
    },
  });

  const loaderFor = (who: () => string, setup: ModuleSetup) => {
    const registry = new SlotRegistry();
    const loader = createModuleLoader({
      client: fakeLoaderClient(
        [{ namespace: 'spools', ui: cardUi('spools') }],
        who,
      ),
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      importModule: () => Promise.resolve({ default: setup }),
      storage: null,
    });

    return { loader, registry };
  };

  it('forgets the first person modules and admits them again for the next', async () => {
    let who = 'user:ada';
    const principals: string[] = [];
    const { loader, registry } = loaderFor(
      () => who,
      defineModule((host) => {
        principals.push(host.principal.id);
        host.register.deviceCard('spool', PlainSection);
      }),
    );
    await loader.activate();
    expect(registry.sections('device-card')).toHaveLength(1);

    loader.reset();
    who = 'user:grace';

    expect(registry.sections('device-card')).toEqual([]);
    expect(loader.loaded).toEqual([]);
    expect(loader.ready).toBe(false);

    await loader.activate();
    expect(principals).toEqual(['user:ada', 'user:grace']);
    expect(registry.sections('device-card')).toHaveLength(1);
  });

  it('drops what a setup still running at the reset registers', async () => {
    let finish: (() => void) | undefined;
    const { loader, registry } = loaderFor(
      () => 'user:ada',
      defineModule(
        (host) =>
          new Promise<void>((resolve) => {
            finish = () => {
              host.register.deviceCard('spool', PlainSection);
              resolve();
            };
          }),
      ),
    );
    const run = loader.activate();
    await vi.waitFor(() => {
      expect(finish).toBeDefined();
    });

    loader.reset();
    finish?.();
    await run;

    expect(registry.sections('device-card')).toEqual([]);
    expect(loader.loaded).toEqual([]);
    expect(loader.ready).toBe(false);
  });
});

describe('ModuleLoader stores', () => {
  // A module's records live in the core, one set per person, so the store
  // the loader hands a module writes through the client.
  it('hands a module a store that keeps its records in the core', async () => {
    const written: string[] = [];
    const client = fakeLoaderClient([
      { namespace: 'spools', ui: uiFor('spools') },
    ]);
    Object.assign(client, {
      listModuleState: () => Promise.resolve({ items: [] }),
      putModuleState: (request: {
        path: { namespace: string; key: string };
        body: { value: unknown };
      }) => {
        written.push(`${request.path.namespace}/${request.path.key}`);

        return Promise.resolve({ key: request.path.key, value: 'grams' });
      },
    });
    const loader = createModuleLoader({
      client,
      registry: new SlotRegistry(),
      fleet: unreadFleet,
      version: '0.1.0',
      importModule: () =>
        Promise.resolve({
          default: defineModule((host) => {
            host.store.set('unit', 'grams');
            host.register.page('spools', PlainPage);
          }),
        }),
      storage: null,
    });

    await loader.activate();

    expect(written).toEqual(['spools/unit']);
  });
});

describe('ModuleLoader signal', () => {
  // A module opens a stream and a timer in its setup and ties both to the
  // host's signal. A reset must stop both, because both belong to the person
  // who left.
  it('stops what a module opened in its setup on a reset', async () => {
    const opened = { stream: false, ticks: 0 };
    const registry = new SlotRegistry();
    const loader = createModuleLoader({
      client: fakeLoaderClient([{ namespace: 'spools', ui: uiFor('spools') }]),
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      importModule: () =>
        Promise.resolve({
          default: defineModule((host) => {
            const stream = new AbortController();
            opened.stream = true;
            host.signal.addEventListener('abort', () => {
              stream.abort();
              opened.stream = false;
            });

            const timer = setInterval(() => {
              opened.ticks += 1;
            }, 5);
            host.signal.addEventListener('abort', () => {
              clearInterval(timer);
            });
            host.register.page('spools', PlainPage);
          }),
        }),
    });
    await loader.activate();
    await vi.waitFor(() => {
      expect(opened.ticks).toBeGreaterThan(0);
    });

    loader.reset();
    const ticksAtReset = opened.ticks;
    await new Promise((resolve) => setTimeout(resolve, 30));

    expect(opened.stream).toBe(false);
    expect(opened.ticks).toBe(ticksAtReset);
  });

  it('aborts the previous run and stops its stores on a reload', async () => {
    const signals: AbortSignal[] = [];
    let closed = 0;
    // A store with a `close`, which the loader calls when the run ends.
    const store = {
      get: () => undefined,
      set: () => undefined,
      delete: () => undefined,
      keys: () => [],
      close: () => {
        closed += 1;
      },
    };
    const loader = createModuleLoader({
      client: fakeLoaderClient([{ namespace: 'spools', ui: uiFor('spools') }]),
      registry: new SlotRegistry(),
      fleet: unreadFleet,
      version: '0.1.0',
      createStore: () => store,
      importModule: () =>
        Promise.resolve({
          default: defineModule((host) => {
            signals.push(host.signal);
            host.register.page('spools', PlainPage);
          }),
        }),
    });
    await loader.activate();

    await loader.reload();

    expect(signals.map((signal) => signal.aborted)).toEqual([true, false]);
    expect(closed).toBe(1);
  });

  it('registers no catalogue for a run a reset ended', async () => {
    let answer: (() => void) | undefined;
    const registry = new SlotRegistry();
    const loader = createModuleLoader({
      client: fakeLoaderClient([{ namespace: 'late', ui: uiFor('late') }]),
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      readCatalogue: () =>
        new Promise((resolve) => {
          answer = () => {
            resolve({ 'late.title': 'Late' });
          };
        }),
      importModule: () =>
        Promise.resolve({ default: defineModule(() => undefined) }),
    });
    const run = loader.activate();
    await vi.waitFor(() => {
      expect(answer).toBeDefined();
    });

    loader.reset();
    answer?.();
    await run;

    expect(translateFor('late')('late.title')).toBe('late.title');
  });
});

describe('ModuleLoader and a setup still running at a reset', () => {
  // A setup that awaits is still running when the person leaves. When it
  // resumes, its host's signal has already fired, so a holder that checks it
  // holds nothing, and what it registers is dropped.
  it('hands the resumed setup an aborted signal and drops what it registers', async () => {
    let resume: (() => void) | undefined;
    const seen: { aborted: boolean | undefined } = { aborted: undefined };
    const registry = new SlotRegistry();
    const loader = createModuleLoader({
      client: fakeLoaderClient([{ namespace: 'spools', ui: uiFor('spools') }]),
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      readCatalogue: () => Promise.resolve(undefined),
      importModule: () =>
        Promise.resolve({
          default: defineModule(async (host) => {
            await new Promise<void>((resolve) => {
              resume = resolve;
            });
            seen.aborted = host.signal.aborted;
            host.register.page('spools', PlainPage);
          }),
        }),
    });
    const run = loader.activate();
    await vi.waitFor(() => {
      expect(resume).toBeDefined();
    });

    loader.reset();
    resume?.();
    await run;

    expect(seen.aborted).toBe(true);
    expect(registry.pages()).toEqual([]);
  });

  // The first module registered its page and the second is still in its
  // setup when the reload starts. The reload must not leave the first page
  // behind for the new run to register a second time.
  it('leaves one copy of each page after a reload during a setup', async () => {
    let resume: (() => void) | undefined;
    let held = true;
    const registry = new SlotRegistry();
    const loader = createModuleLoader({
      client: fakeLoaderClient([
        { namespace: 'spools', ui: uiFor('spools') },
        { namespace: 'dryer', ui: uiFor('dryer') },
      ]),
      registry,
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      readCatalogue: () => Promise.resolve(undefined),
      importModule: (url) =>
        Promise.resolve({
          default: defineModule(async (host) => {
            if (url.includes('/spools/')) {
              host.register.page('spools', PlainPage);

              return;
            }
            if (held) {
              held = false;
              await new Promise<void>((resolve) => {
                resume = resolve;
              });
            }
            host.register.page('dryer', PlainPage);
          }),
        }),
    });
    const first = loader.activate();
    await vi.waitFor(() => {
      expect(resume).toBeDefined();
    });

    const second = loader.reload();
    resume?.();
    await Promise.all([first, second]);

    expect(registry.pages().map((page) => page.namespace)).toEqual([
      'spools',
      'dryer',
    ]);
  });

  it('forgets the words a run merged before a reset ended it', async () => {
    let resume: (() => void) | undefined;
    const loader = createModuleLoader({
      client: fakeLoaderClient([{ namespace: 'words', ui: uiFor('words') }]),
      registry: new SlotRegistry(),
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      readCatalogue: () => Promise.resolve({ 'words.title': 'Words' }),
      importModule: () =>
        new Promise((resolve) => {
          resume = () => {
            resolve({ default: defineModule(() => undefined) });
          };
        }),
    });
    const run = loader.activate();
    await vi.waitFor(() => {
      expect(resume).toBeDefined();
    });
    expect(translateFor('words')('words.title')).toBe('Words');

    loader.reset();
    resume?.();
    await run;

    expect(translateFor('words')('words.title')).toBe('words.title');
  });

  it('hands every module a client bound to the run, and hands the reader the signal', async () => {
    const bound: AbortSignal[] = [];
    const read: (AbortSignal | undefined)[] = [];
    const client = fakeLoaderClient([
      { namespace: 'spools', ui: uiFor('spools') },
    ]);
    Object.assign(client, {
      withSignal: (signal: AbortSignal) => {
        bound.push(signal);

        return client;
      },
    });
    const loader = createModuleLoader({
      client,
      registry: new SlotRegistry(),
      fleet: unreadFleet,
      version: '0.1.0',
      storage: null,
      readCatalogue: (_url, signal) => {
        read.push(signal);

        return Promise.resolve(undefined);
      },
      importModule: () =>
        Promise.resolve({
          default: defineModule((host) => {
            host.register.page('spools', PlainPage);
          }),
        }),
    });
    await loader.activate();

    loader.reset();

    expect(bound).toHaveLength(1);
    expect(bound[0]?.aborted).toBe(true);
    expect(read[0]).toBe(bound[0]);
  });
});
