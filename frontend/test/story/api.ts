// The core an app story talks to: `fetch` answers from the UX fixture table,
// with the devices of the chosen fleet size and the story's own rows on top.
// A story installs it before the app's first request and restores the real
// `fetch` when it goes, since Storybook draws every story in one page.
import fixtures from '../ux/fixtures/api.json';
import type { components } from '../../../packages/typescript/src/generated/core-api';
import {
  copiedFrom,
  devicesAnswer,
  fleetOf,
  type FleetSize,
} from '../../.storybook/fleet';
import { FLEET_STATS_FILES, historyAnswer } from './fleet-stats';
import { materialReading, materialsList } from './materials-catalogue';
import recorded from './recorded.json';
import { storyStorage } from './storage-api';
import {
  type Answer,
  json,
  problem,
  stream,
  type Stream,
} from '../../src/test/stub';

export type { FleetSize };

/**
 * Rows a story puts over the table, keyed `"<METHOD> <path>"` as `api.json`
 * keys are, with the query when the row answers one query only. A function is
 * asked on every request, so `pending` in it keeps every request waiting.
 */
export type Overrides = Record<
  string,
  Answer | ((query: URLSearchParams) => Answer | Promise<Answer>)
>;

interface Row {
  readonly status: number;
  readonly type: string;
  readonly body: string;
}

// `recorded.json` holds what the demo core answers for the requests every
// screen makes that the UX table leaves out.
const table: Record<string, Row> = { ...recorded, ...fixtures };

type StoryJob = NonNullable<components['schemas']['JobPage']['items']>[number];
const finishedJobs = JSON.parse(
  fixtures['GET /api/v1/jobs?phase=ended&limit=200'].body,
) as {
  items: StoryJob[];
};

const endedJobs = finishedJobs.items.map((job, index) => {
  if (index !== 5 && index !== 10) return job;
  return {
    ...job,
    state: index === 5 ? 'ERROR' : 'STOPPED',
    progress: index === 5 ? 43 : 68,
    ended: new Date(
      Date.parse(job.started) + (index === 5 ? 17 : 32) * 60_000,
    ).toISOString(),
  };
});

const DEVICE_ID = /^\S+ \/api\/v1\/devices\/([^/?]+)/;

const UUID_HEAD =
  /([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-)[0-9a-f]{12}/g;

// A copied printer answers its jobs, files and history with the rows of the
// printer it was copied from, under its own id. Every other id in the rows
// takes the copy's tail as well: the jobs screen lists every printer's history
// in one keyed list, and two printers holding one job id would break it.
const copiedRow = (key: string): Row | undefined => {
  const id = DEVICE_ID.exec(key)?.[1] ?? '';
  const source = copiedFrom.get(id);
  if (source === undefined) return undefined;
  const row = table[key.replace(id, source)];

  const tail = id.slice(-12);

  return (
    row && {
      ...row,
      body: row.body
        .replaceAll(source, id)
        .replace(UUID_HEAD, (_, head: string) => `${head}${tail}`),
    }
  );
};

/** An answer that never comes: a loading state that stays loading. */
export const pending = (): Promise<never> =>
  new Promise<never>(() => undefined);

export const refused = (): Response => problem(403, 'forbidden');

export const failed = (): Response => problem(500, 'internal');

export const emptyList = (): Response =>
  json(200, { items: [], total: 0, limit: 50, offset: 0 });

const addressOf = (input: RequestInfo | URL): string => {
  if (typeof input === 'string') return input;
  return input instanceof URL ? input.href : input.url;
};

const methodOf = (input: RequestInfo | URL, init?: RequestInit): string =>
  init?.method ??
  (typeof input === 'object' && 'method' in input ? input.method : 'GET');

const fromRow = (row: Row): Response =>
  new Response(row.body, {
    status: row.status,
    headers: { 'content-type': row.type },
  });

// Read once, as this file loads, so a stub is never taken for the real one.
const original = globalThis.fetch;
const streams: Stream[] = [];

/** Puts the real `fetch` back and ends every event stream a stub opened. */
const restore = (): void => {
  globalThis.fetch = original;
  for (const events of streams.splice(0)) events.end();
};

/**
 * Answers the app from the table until the returned function is called. A
 * second install replaces the first stub, and the streams the first opened
 * stay open, so the app still mounted on them keeps its connection.
 */
export const installApi = (
  size: FleetSize,
  overrides: Overrides = {},
): (() => void) => {
  const storage = storyStorage();
  const fleet = fleetOf(size);
  const deviceIds = new Set(fleet.map((device) => device.id));
  const runningJobs = fleet.flatMap((device) =>
    device.job?.running
      ? [
          {
            ...device.job,
            device_id: device.id,
            started: '2026-09-13T11:33:00Z',
          },
        ]
      : [],
  );
  const answer = async (
    key: string,
    query: URLSearchParams,
  ): Promise<Response | undefined> => {
    const own = overrides[key];
    if (own !== undefined) {
      const given = typeof own === 'function' ? await own(query) : own;
      if (given instanceof Error) throw given;
      // A row may be asked for more than once and a body is read once.
      return given.clone();
    }
    if (key === 'GET /api/v1/devices') return devicesAnswer(size, query);
    const detailId = /^GET \/api\/v1\/devices\/([^/]+)$/.exec(key)?.[1];
    if (detailId !== undefined) {
      const device = fleetOf(size).find((record) => record.id === detailId);
      if (device !== undefined) return json(200, device);
    }
    if (key === 'GET /api/v1/jobs') {
      const phase = query.get('phase');
      if (phase === 'running' || phase === 'ended') {
        const printer = query.get('device_id');
        const items = (phase === 'running' ? runningJobs : endedJobs).filter(
          (job) =>
            deviceIds.has(job.device_id) &&
            (!printer || job.device_id === printer),
        );
        const limit = Number(query.get('limit')) || 200;
        const offset = Number(query.get('cursor')) || 0;
        const next = offset + limit;
        return json(200, {
          items: items.slice(offset, next),
          cursor: next < items.length ? String(next) : '',
          limit,
        });
      }
    }
    const jobId = /^GET \/api\/v1\/jobs\/([^/]+)$/.exec(key)?.[1];
    if (jobId) {
      const job =
        endedJobs.find((job) => job.id === jobId) ??
        runningJobs.find((job) => job.id === jobId);
      if (job !== undefined) return json(200, job);
    }
    if (key === 'GET /api/v1/history/facts') return historyAnswer(size);
    const filesId = /^GET \/api\/v1\/devices\/([^/]+)\/files$/.exec(key)?.[1];
    if (filesId !== undefined && deviceIds.has(filesId)) {
      const directory = query.get('path');
      const directoryKey = directory ? `${key}?path=${directory}` : key;
      const samplePrinter = fleetOf('one')[0]?.id;
      const sample =
        samplePrinter === undefined
          ? undefined
          : table[directoryKey.replace(filesId, samplePrinter)];
      const row = table[directoryKey] ?? copiedRow(directoryKey) ?? sample;
      return row === undefined ? undefined : fromRow(row);
    }
    const row = table[key] ?? copiedRow(key);
    const historyId = /^GET \/api\/v1\/devices\/([^/?]+)\/jobs\/history$/.exec(
      key,
    )?.[1];
    if (row?.status === 200 && historyId !== undefined) {
      const job = fleetOf(size).find(({ id }) => id === historyId)?.job;
      if (job?.state === 'FINISHED') {
        const history = JSON.parse(row.body) as {
          items: { id: string }[] | null;
        };
        const items = history.items ?? [];
        if (!items.some(({ id }) => id === job.id)) {
          return json(200, {
            ...history,
            items: [
              {
                ...job,
                device_id: historyId,
                started: '2026-09-13T11:33:00Z',
                ended: '2026-09-13T12:23:00Z',
              },
              ...items,
            ].slice(0, Number(query.get('limit') ?? items.length + 1)),
          });
        }
      }
    }

    return row === undefined ? undefined : fromRow(row);
  };

  globalThis.fetch = async (input, init) => {
    // The extension's catalogues are files Storybook serves.
    const address = addressOf(input);
    const { location } = globalThis as { location?: Location };
    const ownFile = new URL(address, location?.href ?? 'http://core.test');
    const method = methodOf(input, init);
    const path = `${method} ${ownFile.pathname}`;
    if (address.startsWith(FLEET_STATS_FILES)) {
      return original(input, init);
    }
    const url = ownFile;
    const materialsBody =
      overrides[materialsList] &&
      ((method === 'PUT' && path.includes('.materials/printers/')) ||
        (method === 'POST' &&
          (path.endsWith('.materials/custom-materials') ||
            path.endsWith('.materials/spools') ||
            path.endsWith('.materials/locations')))) &&
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as Parameters<typeof materialReading>[1])
        : undefined;

    // Held open, as the core holds it: a stream that ends or a 404 shows the
    // connection as lost on every screen.
    if (path === 'GET /api/v1/events' && overrides[path] === undefined) {
      const events = stream();
      streams.push(events);
      return events.response;
    }

    const query = decodeURIComponent(url.search);

    return (
      (query === ''
        ? undefined
        : overrides[`${path}${query}`] &&
          (await answer(`${path}${query}`, url.searchParams))) ??
      (overrides[path] ? await answer(path, url.searchParams) : undefined) ??
      (await storage(method, url, input, init)) ??
      (query === '' || path === 'GET /api/v1/jobs'
        ? undefined
        : await answer(`${path}${query}`, url.searchParams)) ??
      (await answer(path, url.searchParams)) ??
      (overrides[materialsList]
        ? materialReading(path, materialsBody)
        : undefined) ??
      problem(404, 'core.api/NOT_FOUND')
    );
  };

  return restore;
};
