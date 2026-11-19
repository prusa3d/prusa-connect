// The fleet-stats extension, installed on the story core. Storybook serves its
// built bundle at `modules/fleet-stats/` beside the preview document, and
// `iframe.html` carries the import map the bundle loads Svelte and the library
// through (see `.storybook/main.ts`). A story installs it with the overrides
// below; the history it reads is answered for every story, since no core
// screen reads it.
import { fleetOf, type FleetSize } from '../../.storybook/fleet';
import fixtures from '../ux/fixtures/api.json';
import { json } from '../../src/test/stub';

export const FLEET_STATS_NAMESPACE = 'com.github.aux.prusa3d.fleet-stats';

// Read once, as this file loads, before any story moves the address to a route
// of the app. A relative entry would resolve against the loader's own chunk.
// The tests that read this file in Node have no address, and fetch nothing.
const SITE = new URL(
  '.',
  'location' in globalThis ? location.href : 'http://story.test/',
).href;

/** Where the bundle's files are, so `fetch` reads its catalogues from there. */
export const FLEET_STATS_FILES = `${SITE}modules/fleet-stats/`;

const JOB_SETTLED = 'com.prusa.connect.device.job.settled';
const OUTCOMES = ['finished', 'finished', 'finished', 'failed', 'cancelled'];
const HOUR = 3_600_000;
const DAYS = 60;

/**
 * Sixty days of settled jobs for each printer of the fleet, ending now, so
 * every range the page offers has a period before it to compare against.
 * The numbers come from a fixed seed, so a story draws the same each time.
 */
const settledJobs = (size: FleetSize) => {
  let seed = 7;
  const next = (): number => {
    seed = (seed * 48_271) % 2_147_483_647;
    return seed / 2_147_483_647;
  };
  const now = Date.now();
  const facts = [];

  for (const device of fleetOf(size)) {
    let at = now - DAYS * 24 * HOUR;
    while (at < now) {
      const started = at + next() * 6 * HOUR;
      const ended = started + (1 + next() * 7) * HOUR;
      if (ended >= now) break;
      const outcome = OUTCOMES[Math.floor(next() * OUTCOMES.length)];
      facts.push({
        at: new Date(ended).toISOString(),
        epoch: 'story',
        origin: `device:${device.id}`,
        sequence: facts.length + 1,
        type: JOB_SETTLED,
        payload: {
          job_id: `job-${String(facts.length + 1)}`,
          display_name: `part-${String(facts.length % 17)}.bgcode`,
          started: new Date(started).toISOString(),
          ended: new Date(ended).toISOString(),
          outcome,
          machine_state: outcome === 'finished' ? 'FINISHED' : 'STOPPED',
        },
      });
      at = ended;
    }
  }

  return facts;
};

/** `GET /api/v1/history/facts`: one page, which the page filters by range. */
export const historyAnswer = (size: FleetSize): Response =>
  json(200, { items: settledJobs(size), cursor: '', incomplete: false });

export const statsJobsAnswer = (size: FleetSize): Response =>
  json(200, {
    items: settledJobs(size).map(({ origin, payload }) => ({
      device_id: origin.slice('device:'.length),
      id: payload.job_id,
      display_name: payload.display_name,
      started: payload.started,
      ended: payload.ended,
      outcome: payload.outcome,
      state: payload.machine_state,
    })),
    cursor: '',
  });

interface Listing {
  readonly items: unknown[];
}

const listing = JSON.parse(fixtures['GET /api/v1/resources'].body) as Listing;

/** The resources listing with fleet-stats installed, as `module add` leaves it. */
const installed = (): Response =>
  json(200, {
    ...listing,
    items: [
      ...listing.items,
      {
        kind: 'asset',
        name: 'fleet-stats',
        namespace: FLEET_STATS_NAMESPACE,
        uid: 'uid-fleet-stats',
        enabled: true,
        source: 'overlay',
        version: '0.1.4',
        title: 'Fleet stats',
        summary:
          'Job KPIs, a utilization heatmap and a failure breakdown for the fleet, read from history.',
        codeowner: 'Prusa Research',
        status: { state: 'ready', serving: true, conditions: [] },
        ui: {
          entry_url: `${FLEET_STATS_FILES}index.js`,
          library: '>=0.0.1 <1.0.0',
          contributes: {
            pages: [
              { path: 'fleet-stats', title: 'Statistics', icon: 'chart-line' },
            ],
            device_card: [{ id: 'fleet-stats', title: 'Success rate' }],
            device_section: [{ id: 'fleet-stats', title: 'Print statistics' }],
          },
        },
      },
    ],
  });

/** Overrides that install fleet-stats; spread them into a story's own. */
export const withFleetStats = {
  'GET /api/v1/resources': installed,
  // The keys the module keeps for this person: none saved yet.
  [`GET /api/v1/modules/${FLEET_STATS_NAMESPACE}/state`]: () =>
    json(200, { items: [] }),
};
