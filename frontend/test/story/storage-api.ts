import type { components } from '../../../packages/typescript/src/generated/core-api';
import { json, problem } from '../../src/test/stub';
import fixtures from '../ux/fixtures/api.json';
import { storageThumbnails } from './storage-thumbnails';

type Entry = components['schemas']['StorageEntry'];

function fixture(body: string): Entry[] {
  return (JSON.parse(body) as { items: Entry[] }).items;
}

const root = '/api/v1/storages/local';
const parentOf = (path: string) =>
  path.slice(0, Math.max(0, path.lastIndexOf('/')));
const leaf = (path: string) => path.slice(path.lastIndexOf('/') + 1);

const initial = [
  ...fixture(fixtures['GET /api/v1/storages/local/files'].body),
  ...fixture(
    fixtures['GET /api/v1/storages/local/files?path=q3_rush_jobs'].body,
  ),
  {
    name: 'model-with-supports.bgcode',
    path: 'model-with-supports.bgcode',
    folder: false,
    size: 1677721,
    modified: '2026-09-19T15:00:00Z',
  },
  {
    name: 'spacer.bgcode',
    path: 'examples/spacer.bgcode',
    folder: false,
    size: 286412,
    modified: '2026-09-18T09:12:00Z',
  },
] satisfies Entry[];

function validPath(path: unknown): path is string {
  return (
    typeof path === 'string' &&
    path
      .split('/')
      .every((part) => part !== '' && part !== '.' && part !== '..')
  );
}

export function storyStorage() {
  const entries = new Map(initial.map((entry) => [entry.path, entry]));

  return async (
    method: string,
    url: URL,
    input: RequestInfo | URL,
    init?: RequestInit,
  ) => {
    const { pathname, searchParams } = url;
    const path = searchParams.get('path') ?? '';

    if (method === 'GET' && pathname === root + '/files') {
      const items = [...entries.values()].filter(
        (entry) => parentOf(entry.path) === path,
      );
      return path === '' || entries.get(path)?.folder || items.length > 0
        ? json(200, { items })
        : problem(404, 'core.api/NOT_FOUND');
    }

    if (method === 'GET' && pathname === root + '/stat') {
      const entry = entries.get(path);
      if (!entry) return undefined;
      const preview = storageThumbnails.get(path);
      return json(200, {
        ...entry,
        ...(preview && {
          print: {
            ...entry.print,
            thumbnail_width: preview.width,
            thumbnail_height: preview.height,
          },
        }),
      });
    }

    if (method === 'DELETE' && pathname === root + '/files') {
      if (!entries.has(path)) return problem(404, 'core.api/NOT_FOUND');
      for (const key of entries.keys()) {
        if (key === path || key.startsWith(path + '/')) entries.delete(key);
      }
      return new Response(null, { status: 204 });
    }

    const creating = pathname === root + '/folders';
    if (method !== 'POST' || (!creating && pathname !== root + '/moves')) {
      return undefined;
    }

    const request =
      input instanceof Request && init === undefined
        ? input
        : new Request(url, init);
    const body = (await request.json()) as {
      path?: unknown;
      from?: unknown;
      to?: unknown;
    };
    const target = creating ? body.path : body.to;
    if (!validPath(target)) return problem(400, 'core.api/INVALID_REQUEST');
    if (entries.has(target)) return problem(409, 'core.api/CONFLICT');

    if (creating) {
      entries.set(target, {
        path: target,
        name: leaf(target),
        folder: true,
        size: 0,
        modified: new Date().toISOString(),
      });
    } else {
      const from = body.from;
      if (!validPath(from) || !entries.has(from))
        return problem(404, 'core.api/NOT_FOUND');
      if (target.startsWith(from + '/'))
        return problem(409, 'core.api/CONFLICT');
      const moved = [...entries.values()].filter(
        (entry) => entry.path === from || entry.path.startsWith(from + '/'),
      );
      for (const entry of moved) {
        entries.delete(entry.path);
        const next = target + entry.path.slice(from.length);
        entries.set(next, { ...entry, path: next, name: leaf(next) });
      }
    }
    return json(200, { path: target });
  };
}
