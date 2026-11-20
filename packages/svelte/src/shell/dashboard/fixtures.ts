// A dashboard screen over fakes, for the screen's tests and its stories.
//
// The stores are the real ones: the dashboards store, the editor, the card
// catalogue over a real slot registry with the core's cards in it. What is
// fake is the core at the far end, which keeps its dashboards in a map and
// answers the way the routes do (ADR-20261008-sww rules 1 to 11).
import type { Device, ReadonlyFleet } from '../../domain';
import {
  type Dashboard,
  type DashboardDraftBody,
  DashboardEditor,
  Dashboards,
  type DashboardsStoreClient,
  SharedTelemetry,
  type SharedTelemetryClient,
} from '../../data';
import { SlotRegistry } from '../../slots';
import { CardCatalogue, type CardLoader } from './card-catalogue';
import { CameraFrames, type CameraStore } from './cameras.svelte';
import { registerCoreCards } from './cards';
import { Clock, type Ticker } from './clock.svelte';
import type { DashboardContext } from './context';
import { FleetViews } from './fleet-views.svelte';

/** What the fake core grants: the access checks it answers yes to. */
export type Grant = 'operate_resource' | 'configure_resource';

export interface FakeCore {
  /** The dashboards it holds, by id. */
  readonly held: Map<number, Dashboard>;
  /** The default dashboard's id. */
  defaultId: number | null;
  /** Every write the core was asked for, by operation, in order. */
  readonly writes: string[];
}

/** One stored dashboard, as `get-dashboard` answers it. */
export const storedDashboard = (
  id: number,
  name: string,
  cards: Dashboard['cards'] = [],
  over: Partial<Dashboard> = {},
): Dashboard => ({
  id,
  name,
  visibility: 'everyone',
  is_default: false,
  columns: 12,
  density: 'comfortable',
  cards,
  revision: 1,
  created: '2026-10-08T09:00:00Z',
  created_by: 'user:demo',
  changed: '2026-10-08T09:00:00Z',
  changed_by: 'user:demo',
  ...over,
});

/** A refusal shaped as the SDK throws it, with the registry code on it. */
const refusal = (status: number, code: string): Error =>
  Object.assign(new Error(`${code}: refused`), { status, code });

const NOT_FOUND = 'core.resource/DASHBOARD_NOT_FOUND';
const CHANGED = 'core.resource/DASHBOARD_CHANGED';

/**
 * A core that keeps dashboards in a map. The cast names the SDK type, as the
 * other fakes in this library do: a fake of the whole client cannot be
 * written out.
 */
export const fakeCore = (
  dashboards: readonly Dashboard[] = [],
  {
    grants = ['operate_resource', 'configure_resource'],
    defaultId = null,
  }: { grants?: readonly Grant[]; defaultId?: number | null } = {},
): { core: FakeCore; client: DashboardsStoreClient } => {
  const core: FakeCore = {
    held: new Map(dashboards.map((dashboard) => [dashboard.id, dashboard])),
    defaultId,
    writes: [],
  };
  let next = Math.max(0, ...dashboards.map((dashboard) => dashboard.id)) + 1;

  const answer = (dashboard: Dashboard): Dashboard => ({
    ...dashboard,
    is_default: dashboard.id === core.defaultId,
  });

  const client = {
    checkAccess: ({ body }: { body: { action: string } }) =>
      Promise.resolve({
        allowed: (grants as readonly string[]).includes(body.action),
      }),
    listDashboards: () =>
      Promise.resolve({
        items: [...core.held.values()]
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((dashboard) => ({
            id: dashboard.id,
            name: dashboard.name,
            visibility: dashboard.visibility,
            is_default: dashboard.id === core.defaultId,
            cards: dashboard.cards.length,
            changed: dashboard.changed,
            changed_by: dashboard.changed_by,
          })),
        total: core.held.size,
        default_id: core.defaultId,
      }),
    getDashboard: ({ path }: { path: { dashboard: string } }) => {
      const held = core.held.get(Number(path.dashboard));

      return held === undefined
        ? Promise.reject(refusal(404, NOT_FOUND))
        : Promise.resolve(answer(held));
    },
    createDashboard: ({ body }: { body: DashboardDraftBody }) => {
      core.writes.push('create');
      const made = storedDashboard(next, body.name, [...body.cards], {
        ...body,
        cards: [...body.cards],
        id: next,
        revision: 1,
      });
      next += 1;
      core.held.set(made.id, made);

      return Promise.resolve(answer(made));
    },
    replaceDashboard: ({
      path,
      body,
    }: {
      path: { dashboard: string };
      body: DashboardDraftBody & { revision: number };
    }) => {
      core.writes.push('replace');
      const held = core.held.get(Number(path.dashboard));
      if (held === undefined) return Promise.reject(refusal(404, NOT_FOUND));
      if (held.revision !== body.revision)
        return Promise.reject(refusal(409, CHANGED));
      const saved = {
        ...held,
        ...body,
        cards: [...body.cards],
        revision: held.revision + 1,
      };
      core.held.set(saved.id, saved);

      return Promise.resolve(answer(saved));
    },
    deleteDashboard: ({ path }: { path: { dashboard: string } }) => {
      core.writes.push('delete');
      const id = Number(path.dashboard);
      core.held.delete(id);
      if (core.defaultId === id) core.defaultId = null;

      return Promise.resolve();
    },
    setDefaultDashboard: ({ body }: { body: { id: number | null } }) => {
      core.writes.push('default');
      core.defaultId = body.id;

      return Promise.resolve({ default_id: body.id });
    },
  };

  return { core, client: client as unknown as DashboardsStoreClient };
};

/** A fleet that holds what it was given and never reads anything. */
export const heldFleet = (devices: readonly Device[]): ReadonlyFleet => ({
  devices,
  loading: false,
  loaded: true,
  error: undefined,
  device: (id) => devices.find((device) => device.id === id),
});

/** A camera store with no cameras, which reads nothing. */
const NO_CAMERAS: CameraStore = {
  items: [],
  loaded: true,
  loading: false,
  error: undefined,
  load: () => Promise.resolve(),
  listen: () => () => undefined,
  stillUrl: () => '',
  streamUrl: () => '',
  watch: () => Promise.resolve(''),
  socket: () => {
    throw new Error('no live socket in a fixture');
  },
};

/** A telemetry client whose stream never says anything. */
const QUIET_TELEMETRY = {
  events: () => ({
    close: () => undefined,
    [Symbol.asyncIterator]: () => ({
      next: () => new Promise<never>(() => undefined),
    }),
  }),
  readDeviceHistory: () => Promise.resolve({ items: [] }),
} as unknown as SharedTelemetryClient;

/** A clock stopped at one instant, so a card's time never moves under a test. */
const STOPPED: Ticker = {
  now: () => Date.UTC(2026, 9, 8, 9, 0, 0),
  every: () => () => undefined,
};

/** A loader with no module to load, whose listing was read and was empty. */
const NO_MODULES: CardLoader = {
  activateFor: () => Promise.resolve(),
  discover: () => Promise.resolve(),
  discovered: true,
  unread: false,
};

/** Where the screen sent the person, in order, for a test to read. */
export interface Moves {
  readonly opened: { id: number; edit: boolean; replace: boolean }[];
  readonly kiosk: boolean[];
  home: number;
  addPrinter: number;
}

/** A whole context over a fake core and a held fleet. */
export const dashboardContextOver = (
  client: DashboardsStoreClient,
  devices: readonly Device[] = [],
  loader: CardLoader = NO_MODULES,
): { context: DashboardContext; moves: Moves } => {
  const registry = new SlotRegistry();
  registerCoreCards(registry);
  const cards = new CardCatalogue(registry, loader);
  const moves: Moves = { opened: [], kiosk: [], home: 0, addPrinter: 0 };

  return {
    moves,
    context: {
      dashboards: new Dashboards(client),
      editor: new DashboardEditor((type) => cards.leastOf(type)),
      printers: new FleetViews(heldFleet(devices)),
      telemetry: new SharedTelemetry(QUIET_TELEMETRY),
      cameras: new CameraFrames(NO_CAMERAS),
      clock: new Clock(STOPPED),
      cards,
      navigation: {
        open: (id, options) => {
          moves.opened.push({
            id,
            edit: options?.edit ?? false,
            replace: options?.replace ?? false,
          });
        },
        kiosk: (on) => {
          moves.kiosk.push(on);
        },
        link: (id, kiosk) =>
          `https://printers.example/dashboards/${String(id)}${kiosk ? '?kiosk=1' : ''}`,
        home: () => {
          moves.home += 1;
        },
        checkLeave: () => () => undefined,
        addPrinter: () => {
          moves.addPrinter += 1;
        },
      },
    },
  };
};
