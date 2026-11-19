// A stub host: everything the module receives, built over a fetch that answers
// from a table.
//
// The client is a real `PrusaConnect` in session mode, not a fake of one. That
// is what makes the tests worth running: the request the page causes is the
// request the browser would send, so the path, the method, the body and the
// absence of an Authorization header are all readable here.
import { PrusaConnect } from '@prusa/connect';
import type { Device, ReadonlyFleet } from '@prusa/connect-ui/domain';
import { registerCatalogue, translateFor } from '@prusa/connect-ui/i18n';
import type { ModuleHost } from '@prusa/connect-ui/slots';
import {
  buildRegister,
  modulePageHref,
  noComponents,
} from '@prusa/connect-ui/slots';
import type { Component } from 'svelte';

import english from '../locales/en.json';

/** The namespace the module is installed under in every test. */
export const NAMESPACE = 'com.github.aux.prusa3d.buddy-debug-logs';

/** The module's mount, which every one of its routes is under. */
export const MOUNT = `/api/v1/modules/${NAMESPACE}`;

/** One request the module made, as the stub recorded it. */
export interface Call {
  readonly method: string;
  readonly path: string;
  readonly search: string;
  readonly authorization: string | null;
  readonly body: unknown;
}

/** One answer. A refusal carries the module's own problem document. */
export interface Answer {
  readonly status: number;
  readonly body?: unknown;
}

/**
 * A route answers from a fixed list, or from a function of the request. The
 * function may answer later, for a case that acts while a request is on its
 * way.
 */
export type Route =
  Answer | readonly Answer[] | ((call: Call) => Answer | Promise<Answer>);

export interface StubOptions {
  /** Who the host says is calling. */
  readonly principal?: string;
  /** The answers, keyed by `METHOD path`. A key here replaces the default. */
  readonly routes?: Readonly<Record<string, Route>>;
  /** The printers the host says this person can see. */
  readonly fleet?: ReadonlyFleet;
}

export interface Stub {
  host: ModuleHost;
  calls: Call[];
  pages: Record<string, Component>;
  /** The sections the module registered, by `slot:id`. */
  sections: Record<string, Component<{ deviceId: string }>>;
  /** The calls made to one `METHOD path`. */
  made(key: string): Call[];
}

/** A refusal in the module's own shape. */
export const refusal = (
  status: number,
  code: string,
  detail = code,
): Answer => ({
  status,
  body: {
    type: 'about:blank',
    status,
    code,
    title: code,
    detail,
    params: {},
    retryable: false,
  },
});

/** A status with both ports granted and listening. */
export const STATUS = {
  streams: [
    {
      stream: 'syslog',
      listener: { granted: true, network: 'udp', port: 13514, listening: true },
      counts: { received: 12, unreadable: 1, records: 11, dropped: 0 },
      // Read when the module is set up, so the stream counts as receiving.
      last_received: new Date().toISOString(),
      bytes: 2048,
      limit: 33554432,
      segments: 1,
    },
    {
      stream: 'metrics',
      listener: { granted: true, network: 'udp', port: 8514, listening: true },
      counts: { received: 40, unreadable: 0, records: 40, dropped: 0 },
      last_received: new Date().toISOString(),
      bytes: 8192,
      limit: 100663296,
      segments: 1,
    },
  ],
  retention_hours: 24,
  max_mib: 128,
  storing: true,
};

/** Two lines from one printer, newest first. */
export const LINES = [
  {
    cursor: 'c2',
    at: '2026-10-19T14:03:22Z',
    device_id: 'mk4',
    mac: '10:9c:70:2a:bb:cd',
    ip: '192.168.1.20',
    severity: 'error',
    component: 'Connect',
    text: 'Connection lost',
  },
  {
    cursor: 'c1',
    at: '2026-10-19T14:03:20Z',
    device_id: 'mk4',
    mac: '10:9c:70:2a:bb:cd',
    severity: 'info',
    component: 'GUI',
    text: 'Screen changed',
  },
];

/** One printer, as the fleet store holds it. Only the fields the views read. */
const PRINTER = { id: 'mk4', name: 'MK4 bench' } as unknown as Device;

const oneFleet: ReadonlyFleet = {
  devices: [PRINTER],
  loading: false,
  loaded: true,
  error: undefined,
  device: (id) => (id === PRINTER.id ? PRINTER : undefined),
};

const bodyOf = (init: RequestInit | undefined): unknown => {
  if (typeof init?.body !== 'string') return init?.body;

  try {
    return JSON.parse(init.body) as unknown;
  } catch {
    return init.body;
  }
};

/** The address one call went to, whichever of the three shapes fetch was given. */
const addressOf = (input: RequestInfo | URL): string => {
  if (typeof input === 'string') return input;

  return input instanceof URL ? input.href : input.url;
};

/** Builds a host whose fetch answers each call from the table. */
export const stubHost = (options: StubOptions = {}): Stub => {
  registerCatalogue(NAMESPACE, 'en', english);

  const calls: Call[] = [];
  const seen = new Map<string, number>();

  const defaults: Record<string, Route> = {
    'GET /api/v1/ui': {
      status: 200,
      body: {
        enabled: true,
        title: 'Prusa Connect',
        default_language: 'en',
        theme: 'system',
        printer_address: 'https://192.168.1.5:8443',
        printer_admission: 'approval',
        printer_custom_ca: false,
        dashboards: true,
        diagnostics: true,
      },
    },
    [`GET ${MOUNT}/status`]: { status: 200, body: STATUS },
    [`GET ${MOUNT}/sources`]: { status: 200, body: { sources: [] } },
    [`GET ${MOUNT}/logs`]: {
      status: 200,
      body: { lines: LINES, newest: 'c2' },
    },
    [`GET ${MOUNT}/metrics/names`]: {
      status: 200,
      body: { names: [], truncated: false },
    },
  };

  const table = { ...defaults, ...options.routes };

  const answer = (key: string, call: Call): Answer | Promise<Answer> => {
    const route = table[key];
    if (route === undefined) return refusal(404, 'test/NO_ROUTE', key);
    if (typeof route === 'function') return route(call);
    if (!Array.isArray(route)) return route as Answer;

    const list = route as readonly Answer[];
    const at = seen.get(key) ?? 0;
    seen.set(key, at + 1);

    return list[Math.min(at, list.length - 1)] ?? refusal(500, 'test/EMPTY');
  };

  const fetcher: typeof globalThis.fetch = (input, init) => {
    const url = new URL(addressOf(input), 'http://module.test');
    const method = init?.method ?? 'GET';
    const call: Call = {
      method,
      path: url.pathname,
      search: url.search,
      authorization: new Headers(init?.headers).get('authorization'),
      body: bodyOf(init),
    };
    calls.push(call);

    return Promise.resolve(answer(`${method} ${url.pathname}`, call)).then(
      (reply) =>
        new Response(
          reply.body === undefined ? null : JSON.stringify(reply.body),
          {
            status: reply.status,
            headers: {
              'content-type':
                reply.status >= 400
                  ? 'application/problem+json'
                  : 'application/json',
            },
          },
        ),
    );
  };

  const pages: Record<string, Component> = {};
  const sections: Record<string, Component<{ deviceId: string }>> = {};

  const host: ModuleHost = {
    client: new PrusaConnect({
      auth: { kind: 'session' },
      fetch: fetcher,
      retryDelay: 0,
    }),
    principal: { id: options.principal ?? 'user:demo', grants: [] },
    fleet: options.fleet ?? oneFleet,
    store: {
      get: () => undefined,
      set: () => undefined,
      delete: () => undefined,
      keys: () => [],
    },
    t: translateFor(NAMESPACE),
    signal: new AbortController().signal,
    pageHref: (path) => modulePageHref(NAMESPACE, path),
    // This module draws no other module's component.
    components: noComponents,
    register: buildRegister({
      page: (path, component) => {
        pages[path] = component;
      },
      section: (slot, id, component) => {
        sections[`${slot}:${id}`] = component as Component<{
          deviceId: string;
        }>;
      },
      action: () => undefined,
    }),
  };

  return {
    host,
    calls,
    pages,
    sections,
    made: (key) =>
      calls.filter((call) => `${call.method} ${call.path}` === key),
  };
};
