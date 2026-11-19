// What the Extensions page reads and starts: the index the server offers, an
// install, an update and a removal, through the real SDK over a fetch stub.
import { describe, expect, it, vi } from 'vitest';

import type { ResourceInstance } from '@prusa/connect-ui/data';

import { type Call, empty, json, problem, stubClient } from '../../test/stub';
import {
  MODULE_KINDS,
  ModuleCatalogue,
  type ModuleOffer,
  stateOf,
} from './module-catalogue.svelte';

const NAMESPACE = 'com.github.aux.prusa3d.parts';

const offer = (over: Partial<ModuleOffer> = {}): ModuleOffer => ({
  module: 'github.com/prusa3d/parts',
  name: 'parts',
  title: 'Parts',
  version: '0.2.0',
  source: `http://localhost:4455/api/connect/v1/extensions/${NAMESPACE}/download?version=0.2.0`,
  installed: false,
  process: true,
  requests_known: true,
  namespace: NAMESPACE,
  origin: 'marketplace',
  sha256: 'ab'.repeat(32),
  ...over,
});

// The write itself; the reads that follow it come after it in the calls.
const put = (calls: readonly Call[]): Call | undefined =>
  calls.filter((call) => call.method === 'PUT').at(-1);

const MARKETPLACE = {
  url: 'http://localhost:4455',
  browse_url: 'http://localhost:4455/connect',
  enabled: true,
  reachable: true,
  stale: false,
};

describe('the state of a row', () => {
  it('tells not installed, installed and an update apart', () => {
    expect(stateOf(offer())).toBe('available');
    expect(
      stateOf(offer({ installed: true, installed_version: '0.2.0' })),
    ).toBe('installed');
    expect(
      stateOf(offer({ installed: true, installed_version: '0.1.0' })),
    ).toBe('update');
    // A newer release installed than the one on offer is not an update.
    expect(
      stateOf(offer({ installed: true, installed_version: '0.3.0' })),
    ).toBe('installed');
    // A version the page cannot read offers no update it cannot justify.
    expect(stateOf(offer({ installed: true, installed_version: 'dev' }))).toBe(
      'installed',
    );
  });
});

describe('the module catalogue', () => {
  it('tells not read from empty', async () => {
    const { client } = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
    });
    const store = new ModuleCatalogue(client);

    expect(store.loaded).toBe(false);
    expect(store.items).toEqual([]);

    await store.load();

    expect(store.loaded).toBe(true);
    expect(store.items).toEqual([]);
    expect(store.marketplace).toBeUndefined();
  });

  it('reads the rows and the marketplace once for two callers', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
    });
    const store = new ModuleCatalogue(stub.client);

    await Promise.all([store.load(), store.load()]);

    expect(stub.count('GET /api/v1/modules/index')).toBe(1);
    expect(store.items).toHaveLength(1);
    expect(store.marketplace?.browse_url).toBe('http://localhost:4455/connect');
  });

  it('keeps a refusal of the read as it was thrown', async () => {
    const { client } = stubClient({
      'GET /api/v1/modules/index': problem(403, 'core.authz/ACCESS_DENIED'),
    });
    const store = new ModuleCatalogue(client);

    await store.load();

    expect(store.loaded).toBe(false);
    expect(store.error).toBeDefined();
    expect(store.loading).toBe(false);
  });

  it('installs a marketplace row without an address, so the server takes the marketplace release', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [offer()] }),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const changed = vi.fn();
    const store = new ModuleCatalogue(stub.client, changed);

    expect(await store.install(offer())).toBe(true);

    const sent = stub.calls.find((call) => call.method === 'POST');
    // Exactly what the row lists is approved, so a release that asks for
    // more is refused instead of granted.
    expect(JSON.parse(sent?.body ?? '{}')).toEqual({
      module: 'github.com/prusa3d/parts',
      version: '0.2.0',
      approve: true,
      expect: {
        permissions: [],
        events: {},
        check_absent_people: false,
        records_mib: 0,
      },
    });
    // The index and the installed list are read again.
    expect(stub.count('GET /api/v1/modules/index')).toBe(1);
    expect(changed).toHaveBeenCalledOnce();
    expect(store.acting).toBeUndefined();
  });

  it('sends where the database goes only for a release that asks for one (ADR-20261021-bqc)', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const store = new ModuleCatalogue(stub.client);
    const choice = {
      mode: 'postgres' as const,
      dsn: 'postgres://db.lan/parts',
    };

    await store.install(offer({ database: true }), { database: choice });
    await store.install(offer(), { database: choice });

    const posts = stub.calls.filter((call) => call.method === 'POST');
    expect(JSON.parse(posts[0]?.body ?? '{}')).toMatchObject({
      database: choice,
      expect: { database: true },
    });
    expect('database' in JSON.parse(posts[1]?.body ?? '{}')).toBe(false);
  });

  it('installs a local row from the address it names', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const store = new ModuleCatalogue(stub.client);

    const local = offer({ origin: 'local', source: '/srv/parts.tar.gz' });
    delete local.sha256;
    await store.install(local);

    const sent = stub.calls.find((call) => call.method === 'POST');
    expect(JSON.parse(sent?.body ?? '{}')).toMatchObject({
      from: '/srv/parts.tar.gz',
    });
  });

  it('keeps a refused install for its row and starts nothing else meanwhile', async () => {
    let answer: (() => void) | undefined;
    const stub = stubClient({
      'POST /api/v1/modules': () =>
        new Promise((resolve) => {
          answer = () => {
            resolve(problem(400, 'core.validation/MODULE_DIGEST_MISMATCH'));
          };
        }),
    });
    const store = new ModuleCatalogue(stub.client);

    const first = store.install(offer());
    await vi.waitFor(() => {
      expect(answer).toBeDefined();
    });
    expect(store.acting).toBe('github.com/prusa3d/parts');

    // A second act while one runs is refused here, not sent.
    expect(await store.remove(offer({ installed: true }))).toBe(false);
    expect(stub.calls.filter((call) => call.method === 'DELETE')).toEqual([]);

    answer?.();
    expect(await first).toBe(false);

    expect(store.acting).toBeUndefined();
    expect(store.actionModule).toBe('github.com/prusa3d/parts');
    expect(store.actionError).toBeDefined();

    store.dismiss();
    expect(store.actionError).toBeUndefined();
  });

  it('removes an installed module by its namespace', async () => {
    const stub = stubClient({
      [`DELETE /api/v1/modules/${NAMESPACE}`]: empty(204),
      'GET /api/v1/modules/index': json(200, { items: [] }),
    });
    const store = new ModuleCatalogue(stub.client);

    expect(await store.remove(offer({ installed: true }))).toBe(true);
    expect(stub.count(`DELETE /api/v1/modules/${NAMESPACE}`)).toBe(1);

    // A row with no namespace cannot be removed from here.
    const unnamed = offer();
    delete unnamed.namespace;
    expect(await store.remove(unnamed)).toBe(false);
  });

  it('forgets everything for the person who left', async () => {
    const { client } = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
    });
    const store = new ModuleCatalogue(client);
    await store.load();

    store.reset();

    expect(store.items).toEqual([]);
    expect(store.marketplace).toBeUndefined();
    expect(store.loaded).toBe(false);
  });

  it('switches the marketplace off, then reads the index again', async () => {
    let enabled = true;
    const stub = stubClient({
      'GET /api/v1/modules/index': () =>
        json(200, {
          items: enabled ? [offer()] : [],
          marketplace: { ...MARKETPLACE, enabled },
        }),
      'PUT /api/v1/modules/marketplace': () => {
        enabled = false;

        return json(200, { ...MARKETPLACE, enabled: false, reachable: false });
      },
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    expect(await store.setMarketplace(false)).toBe(true);

    expect(store.marketplace?.enabled).toBe(false);
    expect(store.items).toEqual([]);
    expect(store.switching).toBe(false);
    expect(stub.count('GET /api/v1/modules/index')).toBe(2);
  });

  // The server answers every row whatever the community switch says, so
  // moving it writes the switch with the marketplace's own and reads nothing.
  it('lists community extensions without reading the index again', async () => {
    let body: unknown;
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: { ...MARKETPLACE, community: false },
      }),
      'PUT /api/v1/modules/marketplace': (call) => {
        body = JSON.parse(call.body ?? 'null');

        return json(200, { ...MARKETPLACE, community: true });
      },
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    expect(await store.setCommunity(true)).toBe(true);

    expect(body).toEqual({ enabled: true, community: true });
    expect(store.marketplace?.community).toBe(true);
    expect(store.switching).toBe(false);
    expect(stub.count('GET /api/v1/modules/index')).toBe(1);
    expect(await store.setCommunity(true)).toBe(true);
    expect(stub.count('PUT /api/v1/modules/marketplace')).toBe(1);
  });

  // A switch that already says what is asked writes nothing, and a server
  // that names no marketplace has no switch to write.
  it('writes nothing when the switch is already where it is asked to be, or there is none', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [],
        marketplace: MARKETPLACE,
      }),
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    expect(await store.setMarketplace(true)).toBe(true);
    expect(stub.count('PUT /api/v1/modules/marketplace')).toBe(0);

    const none = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
    });
    const bare = new ModuleCatalogue(none.client);
    await bare.load();

    expect(await bare.setMarketplace(false)).toBe(false);
    expect(none.count('PUT /api/v1/modules/marketplace')).toBe(0);
  });

  it('keeps a refused switch as it was thrown, and the marketplace as the server holds it', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'PUT /api/v1/modules/marketplace': problem(
        403,
        'core.authz/ACCESS_DENIED',
      ),
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    expect(await store.setMarketplace(false)).toBe(false);

    expect(store.switchError).toBeDefined();
    expect(store.marketplace?.enabled).toBe(true);
    expect(store.items).toHaveLength(1);
    expect(store.switching).toBe(false);
  });

  // The switch and an install write the same overlay, so neither starts
  // while the other runs.
  it('starts no switch while an install runs', async () => {
    let finish: (answer: Response) => void = () => undefined;
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'POST /api/v1/modules': () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    const installing = store.install(offer());
    await vi.waitFor(() => {
      expect(store.acting).toBe(offer().module);
    });

    expect(await store.setMarketplace(false)).toBe(false);
    expect(stub.count('PUT /api/v1/modules/marketplace')).toBe(0);

    finish(json(201, { instance: 'sink:parts' }));
    await installing;
  });

  // The person left while an install ran. What it answers is theirs, not the
  // next person's: no refusal, no busy row and no index read is kept.
  it('keeps nothing of an action that ends after the person left', async () => {
    let finish: (answer: Response) => void = () => undefined;
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'POST /api/v1/modules': () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    });
    const store = new ModuleCatalogue(stub.client);
    await store.load();

    const installing = store.install(offer());
    // The install is under way once its request has left, which is after the
    // plan it carries was read.
    await vi.waitFor(() => {
      expect(store.acting).toBe(offer().module);
      expect(stub.count('POST /api/v1/modules')).toBe(1);
    });

    store.reset();
    finish(problem(500, 'core.internal/UNEXPECTED'));

    expect(await installing).toBe(false);
    expect(store.acting).toBeUndefined();
    expect(store.actionError).toBeUndefined();
    expect(store.items).toEqual([]);
    expect(stub.count('GET /api/v1/modules/index')).toBe(1);
  });
});

/** A refusal carrying the params the server names, as a problem document. */
const refusal = (
  status: number,
  code: string,
  params: Record<string, string>,
): Response =>
  new Response(
    JSON.stringify({
      type: 'about:blank',
      status,
      code,
      title: 'refused',
      detail: `the server refused with ${code}`,
      params,
      retryable: false,
    }),
    { status, headers: { 'content-type': 'application/problem+json' } },
  );

/** The answer to an install, with the fields a test names. */
const installation = (over: Record<string, unknown> = {}) => ({
  instance: 'sink:parts',
  type: `${NAMESPACE}/parts`,
  namespace: NAMESPACE,
  kind: 'sink',
  version: '0.2.0',
  runtime: 'service',
  root: '/srv/modules/parts',
  enabled: true,
  verified: 3,
  running: true,
  restart: false,
  ...over,
});

const bodyOf = (call: { body: string | undefined } | undefined): unknown =>
  JSON.parse(call?.body ?? '{}');

describe('what an install grants', () => {
  it('approves every request the row lists, and keeps what the server answered', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': json(
        201,
        installation({ withheld: ['permission operate'] }),
      ),
    });
    const store = new ModuleCatalogue(stub.client);
    const row = offer({
      permissions: ['observe', 'operate'],
      events: { done: 'print.done' },
      check_absent_people: true,
      records_mib: 64,
    });

    expect(await store.install(row)).toBe(true);

    expect(bodyOf(stub.calls.find((call) => call.method === 'POST'))).toEqual({
      module: row.module,
      version: '0.2.0',
      approve: true,
      expect: {
        permissions: ['observe', 'operate'],
        events: { done: 'print.done' },
        check_absent_people: true,
        records_mib: 64,
      },
    });
    expect(store.outcome?.update).toBe(false);
    expect(store.outcome?.answer.withheld).toEqual(['permission operate']);

    store.dismissOutcome();
    expect(store.outcome).toBeUndefined();
  });

  // A row that does not know its requests is installed switched off first,
  // and the second request approves what the first one's answer named.
  it('reads the requests first when the row does not know them, then approves exactly those', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': (call) =>
        (bodyOf(call) as { approve?: boolean }).approve === true
          ? json(201, installation())
          : json(
              201,
              installation({
                enabled: false,
                running: false,
                requests: [
                  'event done as com.github.aux.prusa3d.parts.print.done',
                  'permission check_absent_people',
                  'permission observe',
                  'records 128 MiB',
                ],
              }),
            ),
    });
    const store = new ModuleCatalogue(stub.client);
    const row = offer({
      origin: 'local',
      source: '/srv/parts',
      requests_known: false,
    });

    expect(await store.install(row)).toBe(true);

    const posts = () => stub.calls.filter((call) => call.method === 'POST');
    expect(bodyOf(posts()[0])).toEqual({
      module: row.module,
      version: '0.2.0',
      approve: false,
      from: '/srv/parts',
      sha256: 'ab'.repeat(32),
    });
    expect(store.pending?.cause).toBe('unread');
    expect(store.outcome).toBeUndefined();

    expect(await store.approvePending()).toBe(true);

    expect(bodyOf(posts()[1])).toMatchObject({
      approve: true,
      expect: {
        permissions: ['observe'],
        events: { done: 'print.done' },
        check_absent_people: true,
        records_mib: 128,
      },
    });
    expect(store.pending).toBeUndefined();
    expect(store.outcome?.answer.running).toBe(true);
  });

  it('asks again with what the release added when a local release asks for more', async () => {
    let calls = 0;
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': () => {
        calls += 1;

        return calls === 1
          ? refusal(409, 'core.resource/MODULE_REQUESTS_CHANGED', {
              name: 'github.com/prusa3d/parts',
              source: 'the request expected',
              found: 'permission operate; records 64 MiB',
            })
          : json(201, installation());
      },
    });
    const store = new ModuleCatalogue(stub.client);
    const row = offer({
      origin: 'local',
      source: '/srv/parts',
      permissions: ['observe'],
    });

    expect(await store.install(row)).toBe(false);

    // Nothing was installed, and the difference waits for the person.
    expect(store.actionError).toBeUndefined();
    expect(store.pending?.cause).toBe('changed');
    expect(store.pending?.found).toEqual([
      'permission operate',
      'records 64 MiB',
    ]);

    expect(await store.approvePending()).toBe(true);
    const second = stub.calls.filter((call) => call.method === 'POST')[1];
    expect(bodyOf(second)).toMatchObject({
      expect: { permissions: ['observe', 'operate'], records_mib: 64 },
    });
  });

  // The marketplace lists what its release asks for, and approving more here
  // does not change that list, so the refusal is said as it is.
  it('keeps a changed marketplace release as a refusal', async () => {
    const stub = stubClient({
      'POST /api/v1/modules': refusal(
        409,
        'core.resource/MODULE_REQUESTS_CHANGED',
        { found: 'permission operate' },
      ),
    });
    const store = new ModuleCatalogue(stub.client);

    expect(await store.install(offer())).toBe(false);

    expect(store.pending).toBeUndefined();
    expect(store.actionError).toBeDefined();
  });

  it('marks an update, so the page lists what the new release added', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': json(
        201,
        installation({ added: ['permission operate'] }),
      ),
    });
    const store = new ModuleCatalogue(stub.client);

    await store.install(offer({ installed: true, installed_version: '0.1.0' }));

    expect(store.outcome?.update).toBe(true);
    expect(store.outcome?.answer.added).toEqual(['permission operate']);
  });

  // A release a link names that is not the row's own has requests the row
  // does not list, so it is read first too.
  it('reads the requests first for a release other than the row offers', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/modules': json(
        201,
        installation({ enabled: false, running: false, requests: [] }),
      ),
    });
    const store = new ModuleCatalogue(stub.client);

    await store.install(offer(), { version: '0.1.0' });

    expect(bodyOf(stub.calls.find((call) => call.method === 'POST'))).toEqual({
      module: 'github.com/prusa3d/parts',
      version: '0.1.0',
      approve: false,
    });
    expect(store.pending?.version).toBe('0.1.0');
  });
});

describe('an install with the modules it requires', () => {
  const READER_SHA = 'cd'.repeat(32);
  const needsReader = offer({
    origin: 'local',
    requires: [{ module: 'com.github.aux.acme.reader', version: '^1.2' }],
  });
  const plannedReader = {
    module: 'github.com/acme/reader',
    name: 'reader',
    version: '1.2.0',
    source: '/srv/modules/reader',
    installed: false,
    requests_known: true,
    namespace: 'com.github.aux.acme.reader',
    sha256: READER_SHA,
    permissions: ['observe'],
    listen: [{ name: 'feed', port: 9100, network: 'tcp', why: 'the feed' }],
    required_by: ['github.com/prusa3d/parts ^1.2'],
  };

  it('sends each module of the plan with its digest and the requests it showed', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [needsReader] }),
      'POST /api/v1/modules/plan': json(200, { dependencies: [plannedReader] }),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const store = new ModuleCatalogue(stub.client, vi.fn());

    expect(await store.install(needsReader, { approveListen: true })).toBe(
      true,
    );

    const sent = JSON.parse(
      stub.calls.find(
        (call) => call.method === 'POST' && call.path === '/api/v1/modules',
      )?.body ?? '{}',
    ) as { approve?: boolean; dependencies?: unknown[] };
    expect(sent.dependencies).toEqual([
      {
        module: 'github.com/acme/reader',
        version: '1.2.0',
        sha256: READER_SHA,
        expect: expect.objectContaining({
          permissions: ['observe'],
        }) as unknown,
        approve_listen: true,
      },
    ]);
  });

  it('never sends an empty plan for a plan the server refused', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [needsReader] }),
      'POST /api/v1/modules/plan': problem(
        409,
        'core.resource/MODULE_DEPENDENCY_CONFLICT',
      ),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const store = new ModuleCatalogue(stub.client, vi.fn());

    expect(await store.install(needsReader)).toBe(false);
    expect(stub.count('POST /api/v1/modules')).toBe(0);
    expect(store.actionError).toBeDefined();
  });

  it('waits for a plan already being read instead of acting on the loading state', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [needsReader] }),
      'POST /api/v1/modules/plan': json(200, { dependencies: [plannedReader] }),
      'POST /api/v1/modules': json(201, { instance: 'sink:parts' }),
    });
    const store = new ModuleCatalogue(stub.client, vi.fn());

    const reading = store.readPlan(needsReader, needsReader.version);
    expect(store.planOf(needsReader, needsReader.version)?.state).toBe(
      'loading',
    );
    const installed = store.install(needsReader);
    await reading;

    expect(await installed).toBe(true);
    expect(stub.count('POST /api/v1/modules/plan')).toBe(1);
    const sent = JSON.parse(
      stub.calls.find(
        (call) => call.method === 'POST' && call.path === '/api/v1/modules',
      )?.body ?? '{}',
    ) as { approve?: boolean; dependencies?: unknown[] };
    expect(sent.dependencies).toHaveLength(1);
  });

  it('sends the plan with an install that reads the requests first', async () => {
    const unread = { ...needsReader, requests_known: false };
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [unread] }),
      'POST /api/v1/modules/plan': json(200, { dependencies: [plannedReader] }),
      'POST /api/v1/modules': json(201, {
        instance: 'sink:parts',
        requests: [],
      }),
    });
    const store = new ModuleCatalogue(stub.client, vi.fn());

    await store.install(unread);

    const sent = JSON.parse(
      stub.calls.find(
        (call) => call.method === 'POST' && call.path === '/api/v1/modules',
      )?.body ?? '{}',
    ) as { approve?: boolean; dependencies?: unknown[] };
    expect(sent.approve).toBe(false);
    expect(sent.dependencies).toEqual([
      {
        module: 'github.com/acme/reader',
        version: '1.2.0',
        sha256: READER_SHA,
      },
    ]);
  });
});

describe('what the person may do', () => {
  const access =
    (allowed: (kind: string) => boolean) =>
    (call: { body: string | undefined }) => {
      const kind = (bodyOf(call) as { object_id: string }).object_id;

      return json(200, {
        allowed: allowed(kind),
        actions: [],
        authorization_epoch: 1,
      });
    };

  it('is unknown until the server has said, and left to the server when it cannot say', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
    });
    const store = new ModuleCatalogue(stub.client);

    expect(store.mayManage).toBeUndefined();
    await store.load();

    // Every check answered 404 here: nothing is refused on the page's own say.
    expect(store.mayManage).toBeUndefined();
    expect(store.mayConfigure('sink')).toBeUndefined();
    expect(store.mayAdministerOrg).toBeUndefined();
  });

  it('asks about every kind a module is declared under, once per person', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/access/check': access((kind) => kind === 'sink'),
    });
    const store = new ModuleCatalogue(stub.client);

    await Promise.all([store.load(), store.load()]);
    await store.load();

    expect(stub.count('POST /api/v1/access/check')).toBe(
      MODULE_KINDS.length * 2,
    );
    expect(store.mayManage).toBe(true);
    expect(store.mayConfigure('sink')).toBe(true);
    expect(store.mayConfigure('driver')).toBe(false);
    // A row whose kind is not named takes the answer for any kind.
    expect(store.mayConfigure(undefined)).toBe(true);
    expect(store.mayAdministerOrg).toBe(false);

    store.reset();
    expect(store.mayManage).toBeUndefined();
  });

  it('says no once every kind is refused', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': problem(403, 'core.authz/ACCESS_DENIED'),
      'POST /api/v1/access/check': access(() => false),
    });
    const store = new ModuleCatalogue(stub.client);

    await store.load();

    expect(store.mayManage).toBe(false);
    expect(store.mayConfigure('sink')).toBe(false);
  });
});

describe('a write to an installed module', () => {
  const spools = {
    kind: 'asset',
    name: 'spools',
    namespace: 'com.github.aux.acme.spools',
    type: 'com.github.aux.acme.spools.spools',
    enabled: true,
    required: false,
    config: { shelf_size: 4, token: 'xxxxx' },
    version: '7',
    source: 'overlay',
  } as unknown as ResourceInstance;

  it('takes a switch-on refused with MODULES_DISABLED as modules off', async () => {
    const changed = vi.fn();
    const { client } = stubClient({
      'PUT /api/v1/resources/asset/spools': problem(
        409,
        'core.resource/MODULES_DISABLED',
      ),
    });
    const store = new ModuleCatalogue(client, changed, () => true);

    expect(await store.writeInstance(spools, { enabled: true })).toBe(false);

    expect(store.modulesOn).toBe(false);
    expect(store.writeError).toBeUndefined();
    expect(changed).toHaveBeenCalledOnce();
  });

  it('stays writing until the module is read again', async () => {
    // The version the page holds comes from that read. A second save before
    // it would send the old version and be refused with 412 as someone
    // else's change.
    let reloaded: () => void = () => undefined;
    const changed = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          reloaded = resolve;
        }),
    );
    const { client } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
    });
    const store = new ModuleCatalogue(client, changed);

    const written = store.writeInstance(spools, { enabled: false });
    await vi.waitFor(() => {
      expect(changed).toHaveBeenCalledOnce();
    });
    expect(store.writing).toBe(spools.namespace);

    reloaded();

    expect(await written).toBe(true);
    expect(store.writing).toBeUndefined();
  });

  it('sends the spec it was shown with the change', async () => {
    const changed = vi.fn();
    const { client, calls } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, {
        ...spools,
        enabled: false,
      }),
    });
    const store = new ModuleCatalogue(client, changed);

    expect(await store.writeInstance(spools, { enabled: false })).toBe(true);

    expect(JSON.parse(put(calls)?.body ?? '{}')).toEqual({
      type: spools.type,
      enabled: false,
      required: false,
      config: { shelf_size: 4, token: 'xxxxx' },
      hidden_contributions: [],
    });
    expect(changed).toHaveBeenCalledOnce();
  });

  // The server refuses on_over_memory on an instance that runs no process, so
  // a write that carried it could not switch a bundle-only module off.
  it('a module with a process sends its memory keys, and one without sends none', async () => {
    const { client, calls } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
    });
    const store = new ModuleCatalogue(client, vi.fn());

    await store.writeInstance(spools, { enabled: false });
    expect(JSON.parse(put(calls)?.body ?? '{}')).not.toHaveProperty(
      'on_over_memory',
    );
    expect(JSON.parse(put(calls)?.body ?? '{}')).not.toHaveProperty(
      'memory_mib',
    );

    await store.writeInstance(
      { ...spools, runtime: 'service' },
      { enabled: false },
    );
    expect(JSON.parse(put(calls)?.body ?? '{}')).toMatchObject({
      memory_mib: 0,
      on_over_memory: 'stop',
    });
  });

  it('a write keeps the hidden contributions unless it sets them', async () => {
    const { client, calls } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
    });
    const store = new ModuleCatalogue(client, vi.fn());
    const hidden = { ...spools, hidden_contributions: ['device_card/shelf'] };
    const sent = () =>
      (
        JSON.parse(put(calls)?.body ?? '{}') as {
          hidden_contributions: string[];
        }
      ).hidden_contributions;

    await store.writeInstance(hidden, { enabled: false });
    expect(sent()).toEqual(['device_card/shelf']);

    await store.writeInstance(hidden, { config: { shelf_size: 6 } });
    expect(sent()).toEqual(['device_card/shelf']);

    await store.writeInstance(hidden, { hidden_contributions: [] });
    expect(sent()).toEqual([]);
    expect(store.writeFor).toBe('visibility');
  });

  it('a switch flip keeps the memory budget, on_over_memory and switch-off', async () => {
    const { client, calls } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
    });
    const store = new ModuleCatalogue(client, vi.fn());
    const switchOff = { failures: 0, window_seconds: 3600 };

    await store.writeInstance(
      {
        ...spools,
        runtime: 'service',
        memory_mib: 256,
        on_over_memory: 'warn',
        switch_off: switchOff,
      },
      { enabled: false },
    );

    expect(JSON.parse(put(calls)?.body ?? '{}')).toMatchObject({
      memory_mib: 256,
      on_over_memory: 'warn',
      switch_off: switchOff,
    });

    await store.writeInstance(
      { ...spools, switch_off: switchOff },
      { switch_off: null },
    );
    expect(JSON.parse(put(calls)?.body ?? '{}')).not.toHaveProperty(
      'switch_off',
    );
    expect(store.writeFor).toBe('memory');
  });

  it('sends the staged limits only on a write that changes them', async () => {
    const { client, calls } = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
    });
    const store = new ModuleCatalogue(client, vi.fn());
    const upload = { method: 'PUT', path: '/files/{name}', max_mib: 1024 };
    const staged = { ...spools, staged_routes: [upload] };

    await store.writeInstance(staged, { enabled: false });
    expect(JSON.parse(put(calls)?.body ?? '{}')).not.toHaveProperty(
      'staged_routes',
    );

    await store.writeInstance(staged, {
      staged_routes: [{ ...upload, max_mib: 2048 }],
    });
    expect(JSON.parse(put(calls)?.body ?? '{}')).toMatchObject({
      enabled: true,
      staged_routes: [{ ...upload, max_mib: 2048 }],
    });
    expect(store.writeFor).toBe('uploads');
  });

  it('a write reads the memory pool again', async () => {
    const stub = stubClient({
      'PUT /api/v1/resources/asset/spools': json(200, spools),
      'GET /api/v1/modules/memory': json(200, {
        pool_bytes: 512 * 1024 * 1024,
        committed_bytes: 128 * 1024 * 1024,
        source: 'configured',
      }),
    });
    const store = new ModuleCatalogue(stub.client, vi.fn());

    await store.writeInstance(spools, { enabled: true });

    expect(stub.count('GET /api/v1/modules/memory')).toBe(1);
    expect(store.memoryPool?.committed_bytes).toBe(128 * 1024 * 1024);
  });

  it('a write refused for a stale version says so and re-reads', async () => {
    const changed = vi.fn();
    const { client } = stubClient({
      'PUT /api/v1/resources/asset/spools': problem(
        412,
        'core.resource/VERSION_CONFLICT',
      ),
    });
    const store = new ModuleCatalogue(client, changed);

    expect(
      await store.writeInstance(spools, { config: { shelf_size: 6 } }),
    ).toBe(false);

    expect(store.writeConflict).toBe(true);
    expect(store.writeError).toBeUndefined();
    expect(changed).toHaveBeenCalledOnce();
  });
});

describe('restarting an instance', () => {
  const parts = { kind: 'sink', name: 'parts' } as ResourceInstance;

  it('sends one request however many callers ask', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/resources/sink/parts/restart': empty(204),
    });
    const store = new ModuleCatalogue(stub.client);

    const answers = await Promise.all([
      store.restart(parts),
      store.restart(parts),
    ]);

    expect(answers).toEqual([true, false]);
    expect(stub.count('POST /api/v1/resources/sink/parts/restart')).toBe(1);
    expect(store.restarting).toBeUndefined();
  });

  it('keeps a refusal as the restart error', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/resources/sink/parts/restart': problem(
        403,
        'core.authorization/FORBIDDEN',
      ),
    });
    const store = new ModuleCatalogue(stub.client);

    expect(await store.restart(parts)).toBe(false);
    expect(store.restartError).toBeDefined();
    expect(store.restartErrorOf(parts)).toBeDefined();
    // Another module's page says nothing of it.
    expect(
      store.restartErrorOf({ kind: 'sink', name: 'other' } as ResourceInstance),
    ).toBeUndefined();
  });

  it('asks whether the person may operate each kind', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'POST /api/v1/access/check': (call: { body: string | undefined }) => {
        const asked = JSON.parse(call.body ?? '{}') as { action: string };

        return json(200, {
          allowed: asked.action === 'operate_resource',
          actions: [],
          authorization_epoch: 1,
        });
      },
    });
    const store = new ModuleCatalogue(stub.client);

    expect(store.mayOperate('sink')).toBeUndefined();
    await store.load();

    expect(store.mayOperate('sink')).toBe(true);
    expect(store.mayConfigure('sink')).toBe(false);
  });
});

describe('while the operator has switched modules off', () => {
  it('reads no index, still asks what the person may do, and removes', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'POST /api/v1/access/check': json(200, { allowed: true }),
      [`DELETE /api/v1/modules/${NAMESPACE}`]: empty(204),
    });
    const store = new ModuleCatalogue(stub.client, undefined, () => false);

    await store.load();

    expect(store.modulesOn).toBe(false);
    expect(store.loaded).toBe(true);
    expect(store.items).toEqual([]);
    expect(store.marketplace).toBeUndefined();
    expect(store.error).toBeUndefined();
    expect(store.mayManage).toBe(true);
    expect(stub.count('GET /api/v1/modules/index')).toBe(0);

    // A removal reads the index again when it ends, and that read is skipped too.
    expect(await store.remove(offer({ installed: true }))).toBe(true);
    expect(stub.count(`DELETE /api/v1/modules/${NAMESPACE}`)).toBe(1);
    expect(stub.count('GET /api/v1/modules/index')).toBe(0);
  });

  it('takes an index that answers MODULES_DISABLED as modules off', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': problem(
        409,
        'core.resource/MODULES_DISABLED',
      ),
      'POST /api/v1/access/check': json(200, { allowed: true }),
    });
    const store = new ModuleCatalogue(stub.client, undefined, () => true);

    await store.load();

    expect(store.modulesOn).toBe(false);
    expect(store.loaded).toBe(true);
    expect(store.error).toBeUndefined();
    expect(store.items).toEqual([]);

    // The next person starts from the settings again.
    store.reset();
    expect(store.modulesOn).toBe(true);
  });

  it('takes an install refused with MODULES_DISABLED as modules off', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'POST /api/v1/access/check': json(200, { allowed: true }),
      'POST /api/v1/modules': problem(409, 'core.resource/MODULES_DISABLED'),
    });
    const changed = vi.fn();
    const store = new ModuleCatalogue(stub.client, changed, () => true);
    await store.load();
    expect(store.items).toHaveLength(1);

    expect(await store.install(offer())).toBe(false);
    expect(changed).toHaveBeenCalledOnce();

    expect(store.modulesOn).toBe(false);
    expect(store.items).toEqual([]);
    expect(store.marketplace).toBeUndefined();
    expect(store.actionError).toBeUndefined();
  });

  it('takes any write refused with MODULES_DISABLED as modules off, and reads the listing again', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, {
        items: [offer()],
        marketplace: MARKETPLACE,
      }),
      'POST /api/v1/access/check': json(200, { allowed: true }),
      'PUT /api/v1/modules/marketplace': problem(
        409,
        'core.resource/MODULES_DISABLED',
      ),
    });
    const changed = vi.fn();
    const store = new ModuleCatalogue(stub.client, changed, () => true);
    await store.load();

    expect(await store.setMarketplace(false)).toBe(false);

    expect(store.modulesOn).toBe(false);
    expect(store.switchError).toBeUndefined();
    expect(store.marketplace).toBeUndefined();
    expect(changed).toHaveBeenCalledOnce();
  });
});

describe('the index read after a write', () => {
  it('keeps the newest read when an older one answers last', async () => {
    const answers: ((response: Response) => void)[] = [];
    const stub = stubClient({
      'GET /api/v1/modules/index': () =>
        new Promise<Response>((resolve) => {
          answers.push(resolve);
        }),
      [`DELETE /api/v1/modules/${NAMESPACE}`]: empty(204),
    });
    const store = new ModuleCatalogue(stub.client);

    // A refresh the stream asked for is in flight when the removal ends.
    const older = store.load();
    await vi.waitFor(() => {
      expect(answers).toHaveLength(1);
    });
    const removing = store.remove(offer({ installed: true }));
    await vi.waitFor(() => {
      expect(answers).toHaveLength(2);
    });

    answers[1]?.(json(200, { items: [] }));
    expect(await removing).toBe(true);
    answers[0]?.(json(200, { items: [offer({ installed: true })] }));
    await older;

    expect(store.items).toEqual([]);
    expect(store.error).toBeUndefined();
    expect(store.loading).toBe(false);
  });

  it('reads the index again when a write fails after its effect', async () => {
    const stub = stubClient({
      'GET /api/v1/modules/index': json(200, { items: [] }),
      [`DELETE /api/v1/modules/${NAMESPACE}`]: problem(
        500,
        'core.resource/MODULE_LOCK_NOT_WRITTEN',
      ),
    });
    const changed = vi.fn();
    const store = new ModuleCatalogue(stub.client, changed);

    expect(await store.remove(offer({ installed: true }))).toBe(false);

    expect(stub.count('GET /api/v1/modules/index')).toBe(1);
    expect(changed).toHaveBeenCalledOnce();
    expect(store.actionError).toBeDefined();
    expect(store.actionModule).toBe(offer().module);
  });
});
