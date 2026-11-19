// What a module asks the operator for, in the two shapes the server uses.
//
// An index row and an install's `expect` carry the requests as fields. An
// install's answer and a refusal carry them as lines in the server's own
// words: `permission operate`, `check operate`, `permission check_absent_people`,
// `permission list_people`, `call <namespace>`, `origin <origin>`,
// `public <method> <path>`, `upload <method> <path> up to 1024 MiB`,
// `header <name> (<reason>)`, `header <name>, required (<reason>)`,
// `records 128 MiB`, `memory 192 MiB`, `event <endpoint> as <namespace>.<name>`,
// `data directory`, `database of its own` and
// `listen udp 13514 as <name> (<why>)`. A page reads
// the lines back into fields, so what it sends as `expect` is exactly what it
// showed, and it draws both shapes with the same sentences.
//
// Plain functions over plain data, so a test computes them without a screen.
import type { components } from '@prusa/connect';

export type ModuleRequests = components['schemas']['ModuleRequests'];
export type ModuleListen = components['schemas']['ModuleListen'];
export type ModuleStagedRoute = components['schemas']['ModuleStagedRoute'];
export type ModuleRequestHeader = components['schemas']['ModuleRequestHeader'];
/** Where an install puts the module's own database (ADR-20261021-bqc rule 5). */
export type DatabaseChoice = components['schemas']['ModuleDatabaseChoice'];
type ModuleOffer = components['schemas']['ModuleOffer'];

/** The permission word the server writes for asking about absent people. */
const ABSENT_PEOPLE = 'check_absent_people';

/** The permission word the server writes for listing the people. */
const LIST_PEOPLE = 'list_people';

/** One request, as the confirmation draws it. */
export type ModuleRequest =
  | { readonly kind: 'permission'; readonly permission: string }
  // An action it only asks about for a person, and never takes (ADR-20261106-jnb).
  | { readonly kind: 'check'; readonly permission: string }
  | { readonly kind: 'event'; readonly endpoint: string; readonly name: string }
  | { readonly kind: 'absentPeople' }
  | { readonly kind: 'listPeople' }
  // A module whose routes this one calls (ADR-20261027-pq5).
  | { readonly kind: 'call'; readonly namespace: string }
  // A route anybody may call without signing in (ADR-20261120-xys).
  | { readonly kind: 'public'; readonly route: string }
  // A route whose body the server writes to a file, up to a limit (ADR-20261120-xys).
  | {
      readonly kind: 'staged';
      readonly method: string;
      readonly path: string;
      readonly mib: number;
    }
  // A request header the server passes to the module (ADR-20261118-nnm).
  | {
      readonly kind: 'header';
      readonly name: string;
      readonly reason: string;
      /** The module does not start without the header. */
      readonly required: boolean;
    }
  // A site whose pages may call the server from a browser (ADR-20261119-6be).
  | { readonly kind: 'origin'; readonly origin: string }
  | { readonly kind: 'records'; readonly mib: number }
  | { readonly kind: 'memory'; readonly mib: number }
  | { readonly kind: 'data' }
  | { readonly kind: 'database' }
  // A port is approved apart from the rest (ADR-20261019-3ns rule 6).
  | {
      readonly kind: 'listen';
      readonly name: string;
      readonly network: string;
      readonly port: number;
      readonly why: string;
    }
  // A line this page does not know how to read is still shown, as it came.
  | { readonly kind: 'other'; readonly line: string };

/**
 * Every request of the release a row offers, or undefined when the row does
 * not know them all and the server has to read the release first.
 */
export const requestsOfOffer = (
  offer: ModuleOffer,
): ModuleRequests | undefined =>
  offer.requests_known
    ? {
        permissions: [...(offer.permissions ?? [])],
        ...checksOf(offer.checks),
        events: { ...(offer.events ?? {}) },
        check_absent_people: offer.check_absent_people ?? false,
        ...listingPeople(offer.list_people),
        ...callsOf(offer.calls),
        ...publicOf(offer.public_routes),
        ...stagedOf(offer.staged_routes),
        ...headersOf(offer.request_headers),
        ...originsOf(offer.allowed_origins),
        records_mib: offer.records_mib ?? 0,
        ...memory(offer.memory_mib),
        ...portsAndData(offer.data, offer.listen, offer.database),
      }
    : undefined;

// The listing of people, written only when asked for, so a release that does
// not ask for it sends the same expect it always did.
const listingPeople = (
  asked: boolean | undefined,
): Pick<ModuleRequests, 'list_people'> =>
  asked === true ? { list_people: true } : {};

// The actions only asked about, written only when there are some, so a
// release that checks none sends the same expect it always did.
const checksOf = (
  checks: readonly string[] | null | undefined,
): Pick<ModuleRequests, 'checks'> =>
  (checks ?? []).length > 0 ? { checks: [...(checks ?? [])] } : {};

// The modules called, written only when there are some, so a release that
// calls none sends the same expect it always did.
const callsOf = (
  calls: readonly string[] | null | undefined,
): Pick<ModuleRequests, 'calls'> =>
  (calls ?? []).length > 0 ? { calls: [...(calls ?? [])] } : {};

// The public routes, written only when there are some, so a release that
// has none sends the same expect it always did.
const publicOf = (
  routes: readonly string[] | null | undefined,
): Pick<ModuleRequests, 'public_routes'> =>
  (routes ?? []).length > 0 ? { public_routes: [...(routes ?? [])] } : {};

// The staged routes, written only when there are some, so a release that
// has none sends the same expect it always did.
const stagedOf = (
  routes: readonly ModuleStagedRoute[] | null | undefined,
): Pick<ModuleRequests, 'staged_routes'> =>
  (routes ?? []).length > 0
    ? { staged_routes: (routes ?? []).map((route) => ({ ...route })) }
    : {};

// The request headers, written only when there are some, so a release that
// asks for none sends the same expect it always did.
const headersOf = (
  headers: readonly ModuleRequestHeader[] | null | undefined,
): Pick<ModuleRequests, 'request_headers'> =>
  (headers ?? []).length > 0
    ? { request_headers: (headers ?? []).map((header) => ({ ...header })) }
    : {};

// Each header once, by name, the first reason kept.
const mergeHeaders = (
  a: readonly ModuleRequestHeader[],
  b: readonly ModuleRequestHeader[],
): ModuleRequestHeader[] => [
  ...a,
  ...b.filter((header) => !a.some((held) => held.name === header.name)),
];

/** The largest body a staged route may take, in MiB: the core's ceiling. */
export const MAX_STAGED_MIB = 4096;

/**
 * The limit a person typed for a staged route, or undefined when it is not
 * one: a whole number of MiB from 1 to MAX_STAGED_MIB.
 */
export const stagedOfText = (text: string): number | undefined => {
  if (!/^\d{1,4}$/u.test(text.trim())) return undefined;
  const mib = Number(text.trim());

  return mib >= 1 && mib <= MAX_STAGED_MIB ? mib : undefined;
};

/** Whether two staged routes name one method and one path. */
export const sameRoute = (
  a: Pick<ModuleStagedRoute, 'method' | 'path'>,
  b: Pick<ModuleStagedRoute, 'method' | 'path'>,
): boolean => a.method === b.method && a.path === b.path;

// Each route once, with the larger limit where both name it.
const mergeStaged = (
  a: readonly ModuleStagedRoute[],
  b: readonly ModuleStagedRoute[],
): ModuleStagedRoute[] => {
  const out: ModuleStagedRoute[] = [];

  for (const route of [...a, ...b]) {
    const index = out.findIndex((held) => sameRoute(held, route));
    const held = out[index];
    if (held === undefined) out.push({ ...route });
    else
      out[index] = { ...held, max_mib: Math.max(held.max_mib, route.max_mib) };
  }

  return out;
};

// The origins asked for, written only when there are some, so a release that
// asks for none sends the same expect it always did.
const originsOf = (
  origins: readonly string[] | null | undefined,
): Pick<ModuleRequests, 'allowed_origins'> =>
  (origins ?? []).length > 0 ? { allowed_origins: [...(origins ?? [])] } : {};

// The memory recommendation, written only when there is one, so a release
// that makes none sends the same expect it always did.
const memory = (mib: number | undefined): Pick<ModuleRequests, 'memory_mib'> =>
  mib !== undefined && mib > 0 ? { memory_mib: mib } : {};

// The directory, the database and the ports, written only when asked for, so
// a release that asks for none of them sends the same expect it always did.
const portsAndData = (
  data: boolean | undefined,
  listen: readonly ModuleListen[] | null | undefined,
  database?: boolean,
): Pick<ModuleRequests, 'data' | 'listen' | 'database'> => ({
  ...(data === true ? { data: true } : {}),
  ...(database === true ? { database: true } : {}),
  ...((listen ?? []).length > 0 ? { listen: [...(listen ?? [])] } : {}),
});

/** Whether the requests name a database of the module's own. */
export const asksForDatabase = (
  requests: ModuleRequests | undefined,
): boolean => requests?.database === true;

/**
 * Whether a person typed a connection string the server takes for a
 * Postgres database: a postgres:// or postgresql:// URL with a host and a
 * database name.
 */
export const isPostgresURL = (text: string): boolean => {
  const trimmed = text.trim();
  if (!/^postgres(?:ql)?:\/\//u.test(trimmed)) return false;

  try {
    const parsed = new URL(trimmed);

    return parsed.host !== '' && parsed.pathname.replace(/\//gu, '') !== '';
  } catch {
    return false;
  }
};

/** The ports the requests name, in the order the manifest lists them. */
export const portsOf = (
  requests: ModuleRequests | undefined,
): readonly ModuleListen[] => requests?.listen ?? [];

/**
 * The port a person typed, or undefined when it is not one: a whole number
 * from 1 to 65535.
 */
export const portOfText = (text: string): number | undefined => {
  if (!/^\d{1,5}$/u.test(text.trim())) return undefined;
  const port = Number(text.trim());

  return port >= 1 && port <= 65_535 ? port : undefined;
};

/**
 * The ports a person moved, by name, for `listen` beside `approve_listen`.
 * A port left as asked, or typed as something that is not a port, is not
 * moved, so the release keeps the port its manifest names.
 */
export const movedPorts = (
  asked: readonly ModuleListen[],
  typed: Readonly<Record<string, string>>,
): Record<string, number> => {
  const out: Record<string, number> = {};

  for (const port of asked) {
    const chosen = portOfText(typed[port.name] ?? '');
    if (chosen !== undefined && chosen !== port.port) out[port.name] = chosen;
  }

  return out;
};

/** Whether the requests name a port, which takes its own approval. */
export const asksForPorts = (requests: ModuleRequests | undefined): boolean =>
  (requests?.listen ?? []).length > 0;

/**
 * The places a person may pick for a module's database. `keep` leaves an
 * installed module's database where it is, by sending no choice.
 */
export type DatabaseMode = 'keep' | DatabaseChoice['mode'];

/** What an install sends for the place a person picked, or nothing for `keep`. */
export const databaseChoiceOf = (
  mode: DatabaseMode,
  dsn: string,
): DatabaseChoice | undefined => {
  switch (mode) {
    case 'keep':
      return undefined;
    case 'postgres':
      return { mode, dsn: dsn.trim() };
    case 'sqlite':
    case 'provision':
      return { mode };
  }
};

const DATA = 'data directory';
const DATABASE = 'database of its own';
const RECORDS = /^records (\d+) MiB$/u;
const MEMORY = /^memory (\d+) MiB$/u;
const LISTEN = /^listen (udp|tcp) (\d+) as (\S+) \((.*)\)$/u;
const EVENT = /^event (\S+) as (\S+)$/u;
const CALL = /^call (\S+)$/u;
const ORIGIN = /^origin (\S+)$/u;
const CHECK = /^check (\S+)$/u;
const PUBLIC = /^public (\S+ \S+)$/u;
const STAGED = /^upload (\S+) (\S+) up to (\d+) MiB$/u;
const HEADER = /^header ([^\s,]+)(, required)? \((.*)\)$/u;

/** One line of the server's, read back into a request. */
export const requestOfLine = (
  line: string,
  namespace: string | undefined,
): ModuleRequest => {
  const text = line.trim();

  if (text.startsWith('permission ')) {
    const permission = text.slice('permission '.length).trim();

    if (permission === ABSENT_PEOPLE) return { kind: 'absentPeople' };
    if (permission === LIST_PEOPLE) return { kind: 'listPeople' };

    return { kind: 'permission', permission };
  }

  const check = CHECK.exec(text);
  if (check?.[1] !== undefined) return { kind: 'check', permission: check[1] };

  const call = CALL.exec(text);
  if (call?.[1] !== undefined) return { kind: 'call', namespace: call[1] };

  const route = PUBLIC.exec(text);
  if (route?.[1] !== undefined) return { kind: 'public', route: route[1] };

  const staged = STAGED.exec(text);
  if (
    staged?.[1] !== undefined &&
    staged[2] !== undefined &&
    staged[3] !== undefined
  ) {
    return {
      kind: 'staged',
      method: staged[1],
      path: staged[2],
      mib: Number(staged[3]),
    };
  }

  const header = HEADER.exec(text);
  if (header?.[1] !== undefined && header[3] !== undefined) {
    return {
      kind: 'header',
      name: header[1],
      reason: header[3],
      required: header[2] !== undefined,
    };
  }

  const origin = ORIGIN.exec(text);
  if (origin?.[1] !== undefined) return { kind: 'origin', origin: origin[1] };

  if (text === DATA) return { kind: 'data' };
  if (text === DATABASE) return { kind: 'database' };

  const listen = LISTEN.exec(text);
  if (
    listen?.[1] !== undefined &&
    listen[2] !== undefined &&
    listen[3] !== undefined &&
    listen[4] !== undefined
  ) {
    return {
      kind: 'listen',
      name: listen[3],
      network: listen[1],
      port: Number(listen[2]),
      why: listen[4],
    };
  }

  const records = RECORDS.exec(text);
  if (records?.[1] !== undefined) {
    return { kind: 'records', mib: Number(records[1]) };
  }

  const memoryLine = MEMORY.exec(text);
  if (memoryLine?.[1] !== undefined) {
    return { kind: 'memory', mib: Number(memoryLine[1]) };
  }

  const event = EVENT.exec(text);
  if (event?.[1] !== undefined && event[2] !== undefined) {
    // The name is written after the namespace, and a namespace has dots in
    // it, so only the namespace the page knows can be taken off.
    const lead = namespace === undefined ? '' : `${namespace}.`;
    const name =
      lead !== '' && event[2].startsWith(lead)
        ? event[2].slice(lead.length)
        : event[2];

    return { kind: 'event', endpoint: event[1], name };
  }

  return { kind: 'other', line: text };
};

/** The requests a list of the server's lines names, as fields. */
export const requestsOfLines = (
  lines: readonly string[],
  namespace: string | undefined,
): ModuleRequests => {
  const out: Required<
    Omit<
      ModuleRequests,
      | 'data'
      | 'listen'
      | 'list_people'
      | 'database'
      | 'calls'
      | 'public_routes'
      | 'staged_routes'
      | 'request_headers'
      | 'checks'
      | 'memory_mib'
      | 'allowed_origins'
    >
  > = {
    permissions: [],
    events: {},
    check_absent_people: false,
    records_mib: 0,
  };
  let data = false;
  let listPeople = false;
  let database = false;
  const calls: string[] = [];
  const routes: string[] = [];
  let staged: ModuleStagedRoute[] = [];
  let headers: ModuleRequestHeader[] = [];
  const origins: string[] = [];
  const checks: string[] = [];
  let memoryMiB = 0;
  const listen: ModuleListen[] = [];

  for (const line of lines) {
    const request = requestOfLine(line, namespace);
    switch (request.kind) {
      case 'permission':
        out.permissions?.push(request.permission);
        break;
      case 'check':
        if (!checks.includes(request.permission)) {
          checks.push(request.permission);
        }
        break;
      case 'event':
        out.events[request.endpoint] = request.name;
        break;
      case 'absentPeople':
        out.check_absent_people = true;
        break;
      case 'listPeople':
        listPeople = true;
        break;
      case 'call':
        if (!calls.includes(request.namespace)) calls.push(request.namespace);
        break;
      case 'public':
        if (!routes.includes(request.route)) routes.push(request.route);
        break;
      case 'staged':
        staged = mergeStaged(staged, [
          { method: request.method, path: request.path, max_mib: request.mib },
        ]);
        break;
      case 'header':
        headers = mergeHeaders(headers, [
          {
            name: request.name,
            reason: request.reason,
            ...(request.required ? { required: true } : {}),
          },
        ]);
        break;
      case 'origin':
        if (!origins.includes(request.origin)) origins.push(request.origin);
        break;
      case 'records':
        out.records_mib = Math.max(out.records_mib, request.mib);
        break;
      case 'memory':
        memoryMiB = Math.max(memoryMiB, request.mib);
        break;
      case 'data':
        data = true;
        break;
      case 'database':
        database = true;
        break;
      case 'listen':
        if (!listen.some((port) => port.name === request.name)) {
          listen.push({
            name: request.name,
            network: request.network === 'tcp' ? 'tcp' : 'udp',
            port: request.port,
            why: request.why,
          });
        }
        break;
      case 'other':
        break;
    }
  }

  return {
    ...out,
    ...checksOf(checks),
    ...listingPeople(listPeople),
    ...callsOf(calls),
    ...publicOf(routes),
    ...stagedOf(staged),
    ...headersOf(headers),
    ...originsOf(origins),
    ...memory(memoryMiB),
    ...portsAndData(data, listen, database),
  };
};

/** Both sets of requests together, so a second approval holds the first one's too. */
export const mergeRequests = (
  a: ModuleRequests,
  b: ModuleRequests,
): ModuleRequests => ({
  permissions: [
    ...new Set([...(a.permissions ?? []), ...(b.permissions ?? [])]),
  ],
  ...checksOf([...new Set([...(a.checks ?? []), ...(b.checks ?? [])])]),
  events: { ...(a.events ?? {}), ...(b.events ?? {}) },
  check_absent_people:
    (a.check_absent_people ?? false) || (b.check_absent_people ?? false),
  ...listingPeople((a.list_people ?? false) || (b.list_people ?? false)),
  ...callsOf([...new Set([...(a.calls ?? []), ...(b.calls ?? [])])]),
  ...publicOf([
    ...new Set([...(a.public_routes ?? []), ...(b.public_routes ?? [])]),
  ]),
  ...stagedOf(mergeStaged(a.staged_routes ?? [], b.staged_routes ?? [])),
  ...headersOf(mergeHeaders(a.request_headers ?? [], b.request_headers ?? [])),
  ...originsOf([
    ...new Set([...(a.allowed_origins ?? []), ...(b.allowed_origins ?? [])]),
  ]),
  records_mib: Math.max(a.records_mib ?? 0, b.records_mib ?? 0),
  ...memory(Math.max(a.memory_mib ?? 0, b.memory_mib ?? 0)),
  ...portsAndData(
    (a.data ?? false) || (b.data ?? false),
    [
      ...(a.listen ?? []),
      ...(b.listen ?? []).filter(
        (port) => !(a.listen ?? []).some((held) => held.name === port.name),
      ),
    ],
    (a.database ?? false) || (b.database ?? false),
  ),
});

/** The requests as a list to draw, in a fixed order: permissions, checks, events, people, the list of people, calls, public routes, staged routes, request headers, origins, room, memory, directory, database, ports. */
export const listRequests = (requests: ModuleRequests): ModuleRequest[] => [
  ...(requests.permissions ?? []).map((permission): ModuleRequest => ({
    kind: 'permission',
    permission,
  })),
  ...(requests.checks ?? []).map((permission): ModuleRequest => ({
    kind: 'check',
    permission,
  })),
  ...Object.entries(requests.events ?? {}).map(
    ([endpoint, name]): ModuleRequest => ({ kind: 'event', endpoint, name }),
  ),
  ...(requests.check_absent_people
    ? [{ kind: 'absentPeople' } satisfies ModuleRequest]
    : []),
  ...(requests.list_people
    ? [{ kind: 'listPeople' } satisfies ModuleRequest]
    : []),
  ...(requests.calls ?? []).map((namespace): ModuleRequest => ({
    kind: 'call',
    namespace,
  })),
  ...(requests.public_routes ?? []).map((route): ModuleRequest => ({
    kind: 'public',
    route,
  })),
  ...(requests.staged_routes ?? []).map((route): ModuleRequest => ({
    kind: 'staged',
    method: route.method,
    path: route.path,
    mib: route.max_mib,
  })),
  ...(requests.request_headers ?? []).map((header): ModuleRequest => ({
    kind: 'header',
    name: header.name,
    reason: header.reason,
    required: header.required === true,
  })),
  ...(requests.allowed_origins ?? []).map((origin): ModuleRequest => ({
    kind: 'origin',
    origin,
  })),
  ...((requests.records_mib ?? 0) > 0
    ? [
        {
          kind: 'records',
          mib: requests.records_mib ?? 0,
        } satisfies ModuleRequest,
      ]
    : []),
  ...((requests.memory_mib ?? 0) > 0
    ? [
        {
          kind: 'memory',
          mib: requests.memory_mib ?? 0,
        } satisfies ModuleRequest,
      ]
    : []),
  ...(requests.data ? [{ kind: 'data' } satisfies ModuleRequest] : []),
  ...(requests.database ? [{ kind: 'database' } satisfies ModuleRequest] : []),
  ...(requests.listen ?? []).map((port): ModuleRequest => ({
    kind: 'listen',
    ...port,
  })),
];

/** The server's lines as a list to draw. */
export const listLines = (
  lines: readonly string[] | null | undefined,
  namespace: string | undefined,
): ModuleRequest[] =>
  (lines ?? []).map((line) => requestOfLine(line, namespace));
