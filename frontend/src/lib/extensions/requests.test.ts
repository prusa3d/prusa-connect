// What a module asks for, read from an index row and from the server's lines.
import { describe, expect, it } from 'vitest';

import en from '../../locales/en.json';
import type { ModuleOffer } from '../stores/module-catalogue.svelte';
import {
  asksForDatabase,
  databaseChoiceOf,
  asksForPorts,
  isPostgresURL,
  movedPorts,
  portOfText,
  listLines,
  listRequests,
  mergeRequests,
  requestOfLine,
  requestsOfLines,
  requestsOfOffer,
  type ModuleRequests,
} from './requests';

const NAMESPACE = 'com.github.aux.acme.spools';

const offer = (over: Partial<ModuleOffer> = {}): ModuleOffer => ({
  module: 'github.com/acme/spools',
  name: 'spools',
  version: '1.0.0',
  source: '/srv/spools',
  installed: false,
  process: true,
  requests_known: true,
  ...over,
});

describe('the requests of a row', () => {
  it('are every field the row lists, with the defaults written out', () => {
    expect(requestsOfOffer(offer())).toEqual({
      permissions: [],
      events: {},
      check_absent_people: false,
      records_mib: 0,
    });
    expect(
      requestsOfOffer(
        offer({
          permissions: ['observe'],
          events: { done: 'print.done' },
          check_absent_people: true,
          records_mib: 32,
        }),
      ),
    ).toEqual({
      permissions: ['observe'],
      events: { done: 'print.done' },
      check_absent_people: true,
      records_mib: 32,
    });
  });

  it('carry the listing of people only when the row asks for it', () => {
    expect(requestsOfOffer(offer({ list_people: true }))).toEqual({
      permissions: [],
      events: {},
      check_absent_people: false,
      list_people: true,
      records_mib: 0,
    });
  });

  it('are unknown when the row says it does not know them all', () => {
    expect(
      requestsOfOffer(offer({ requests_known: false, permissions: ['x'] })),
    ).toBeUndefined();
  });
});

describe("the server's lines", () => {
  it.each([
    ['permission operate', { kind: 'permission', permission: 'operate' }],
    ['check operate', { kind: 'check', permission: 'operate' }],
    ['permission check_absent_people', { kind: 'absentPeople' }],
    ['permission list_people', { kind: 'listPeople' }],
    ['records 128 MiB', { kind: 'records', mib: 128 }],
    [
      `event done as ${NAMESPACE}.print.done`,
      { kind: 'event', endpoint: 'done', name: 'print.done' },
    ],
    ['something new', { kind: 'other', line: 'something new' }],
  ])('reads %j', (line, request) => {
    expect(requestOfLine(line, NAMESPACE)).toEqual(request);
  });

  // A namespace has dots in it, so without one the whole name is kept.
  it('keeps the whole event name when the namespace is not known', () => {
    expect(requestOfLine(`event done as ${NAMESPACE}.x`, undefined)).toEqual({
      kind: 'event',
      endpoint: 'done',
      name: `${NAMESPACE}.x`,
    });
  });

  it('turn into the fields an approval sends back', () => {
    expect(
      requestsOfLines(
        [
          'permission observe',
          'check operate',
          'permission check_absent_people',
          'permission list_people',
          'records 64 MiB',
          `event done as ${NAMESPACE}.print.done`,
        ],
        NAMESPACE,
      ),
    ).toEqual({
      permissions: ['observe'],
      checks: ['operate'],
      events: { done: 'print.done' },
      check_absent_people: true,
      list_people: true,
      records_mib: 64,
    });
    expect(listLines(null, NAMESPACE)).toEqual([]);
  });
});

describe('a call to another module', () => {
  it('reads from its line and draws as one request', () => {
    const requests = requestsOfLines(
      ['call com.github.aux.acme.slicer', 'permission write_storage'],
      undefined,
    );

    expect(requests.calls).toEqual(['com.github.aux.acme.slicer']);
    expect(listRequests(requests)).toContainEqual({
      kind: 'call',
      namespace: 'com.github.aux.acme.slicer',
    });
  });
});

describe('a public route', () => {
  it('reads from its line, draws as one request and goes back as expect', () => {
    const requests = requestsOfLines(
      ['public POST /hooks/announce/', 'permission write_storage'],
      undefined,
    );

    expect(requests.public_routes).toEqual(['POST /hooks/announce/']);
    expect(listRequests(requests)).toContainEqual({
      kind: 'public',
      route: 'POST /hooks/announce/',
    });
    expect(
      requestsOfOffer(offer({ public_routes: ['POST /hooks/announce/'] })),
    ).toMatchObject({ public_routes: ['POST /hooks/announce/'] });
    expect(
      mergeRequests(requests, { public_routes: ['GET /a'] }).public_routes,
    ).toEqual(['POST /hooks/announce/', 'GET /a']);
  });
});

describe('a staged route', () => {
  const upload = {
    method: 'PUT',
    path: '/connect/app/teams/{team}/files/raw',
    max_mib: 1024,
  };

  it('reads from its line, draws as one request and goes back as expect', () => {
    const requests = requestsOfLines(
      [
        'upload PUT /connect/app/teams/{team}/files/raw up to 1024 MiB',
        'permission write_storage',
      ],
      undefined,
    );

    expect(requests.staged_routes).toEqual([upload]);
    expect(listRequests(requests)).toContainEqual({
      kind: 'staged',
      method: 'PUT',
      path: '/connect/app/teams/{team}/files/raw',
      mib: 1024,
    });
    expect(requestsOfOffer(offer({ staged_routes: [upload] }))).toMatchObject({
      staged_routes: [upload],
    });
    expect('staged_routes' in (requestsOfOffer(offer()) ?? {})).toBe(false);
  });

  it('holds each route once, with the larger limit, when two sets meet', () => {
    expect(
      mergeRequests(
        { staged_routes: [upload] },
        {
          staged_routes: [
            { ...upload, max_mib: 2048 },
            { method: 'POST', path: '/a', max_mib: 8 },
          ],
        },
      ).staged_routes,
    ).toEqual([
      { ...upload, max_mib: 2048 },
      { method: 'POST', path: '/a', max_mib: 8 },
    ]);
  });
});

describe('a request header', () => {
  const printing = {
    name: 'Print-After-Upload',
    reason: 'the slicer says whether to print the file (ADR-20261118-nnm)',
  };

  it('reads from its line with its reason, draws as one request and goes back as expect', () => {
    const requests = requestsOfLines(
      [
        'header Print-After-Upload (the slicer says whether to print the file (ADR-20261118-nnm))',
      ],
      undefined,
    );

    expect(requests.request_headers).toEqual([printing]);
    expect(listRequests(requests)).toEqual([
      { kind: 'header', ...printing, required: false },
    ]);
    expect(
      requestsOfOffer(offer({ request_headers: [printing] })),
    ).toMatchObject({ request_headers: [printing] });
    expect('request_headers' in (requestsOfOffer(offer()) ?? {})).toBe(false);
  });

  it('reads a required header from its line and keeps it required', () => {
    const requests = requestsOfLines(
      [
        'header Print-After-Upload, required (the slicer says whether to print)',
      ],
      undefined,
    );

    expect(requests.request_headers).toEqual([
      {
        name: 'Print-After-Upload',
        reason: 'the slicer says whether to print',
        required: true,
      },
    ]);
    expect(listRequests(requests)).toEqual([
      {
        kind: 'header',
        name: 'Print-After-Upload',
        reason: 'the slicer says whether to print',
        required: true,
      },
    ]);
  });

  it('holds each header once, by name, when two sets meet', () => {
    expect(
      mergeRequests(
        { request_headers: [printing] },
        {
          request_headers: [
            { ...printing, reason: 'another' },
            { name: 'Overwrite', reason: 'replaces a file' },
          ],
        },
      ).request_headers,
    ).toEqual([printing, { name: 'Overwrite', reason: 'replaces a file' }]);
  });
});

describe('a site whose pages may call the server', () => {
  it('reads from its line, is sent back as expect and draws as one request', () => {
    const requests = requestsOfLines(
      ['origin https://slicer.example', 'origin https://slicer.example'],
      undefined,
    );

    expect(requests.allowed_origins).toEqual(['https://slicer.example']);
    expect(listRequests(requests)).toContainEqual({
      kind: 'origin',
      origin: 'https://slicer.example',
    });
  });

  it('is left out of the expect of a release that asks for none', () => {
    expect(
      requestsOfLines(['permission observe'], undefined),
    ).not.toHaveProperty('allowed_origins');
  });
});

describe('two sets of requests together', () => {
  it('hold everything either holds, once', () => {
    expect(
      mergeRequests(
        { permissions: ['observe'], records_mib: 64 },
        {
          permissions: ['observe', 'operate'],
          events: { a: 'b' },
          check_absent_people: true,
          list_people: true,
          records_mib: 16,
        },
      ),
    ).toEqual({
      permissions: ['observe', 'operate'],
      events: { a: 'b' },
      check_absent_people: true,
      list_people: true,
      records_mib: 64,
    });
  });

  it('are listed permissions first, and the default room not at all', () => {
    expect(
      listRequests({
        permissions: ['observe'],
        checks: ['operate'],
        events: { done: 'print.done' },
        check_absent_people: true,
        list_people: true,
        records_mib: 0,
      }).map((request) => request.kind),
    ).toEqual(['permission', 'check', 'event', 'absentPeople', 'listPeople']);
  });
});

describe('ports and a data directory', () => {
  const syslog = {
    name: 'syslog',
    network: 'udp' as const,
    port: 13514,
    why: 'printer log lines',
  };

  it('reads a port line back into the port it names, so it can be sent as expect', () => {
    expect(
      requestOfLine(
        'listen udp 13514 as syslog (printer log lines)',
        undefined,
      ),
    ).toEqual({ kind: 'listen', ...syslog });
    expect(requestOfLine('data directory', undefined)).toEqual({
      kind: 'data',
    });

    const read = requestsOfLines(
      ['listen udp 13514 as syslog (printer log lines)', 'data directory'],
      undefined,
    );
    expect(read.listen).toEqual([syslog]);
    expect(read.data).toBe(true);
  });

  it('says a release asks for a port, and lists ports after the other requests', () => {
    const requests = { permissions: ['observe'], data: true, listen: [syslog] };

    expect(asksForPorts(requests)).toBe(true);
    expect(asksForPorts({ permissions: ['observe'] })).toBe(false);
    expect(listRequests(requests).map((one) => one.kind)).toEqual([
      'permission',
      'data',
      'listen',
    ]);
  });

  it('merges two sets of ports by name', () => {
    const metrics = { ...syslog, name: 'metrics', port: 8514 };

    expect(
      mergeRequests({ listen: [syslog] }, { listen: [syslog, metrics] }).listen,
    ).toEqual([syslog, metrics]);
  });
});

describe('ports a person moves', () => {
  const asked = [
    { name: 'syslog', network: 'udp' as const, port: 13514, why: 'logs' },
    { name: 'metrics', network: 'udp' as const, port: 8514, why: 'metrics' },
  ];

  it('reads a port from 1 to 65535 and nothing else', () => {
    expect(portOfText(' 5514 ')).toBe(5514);
    expect(portOfText('0')).toBeUndefined();
    expect(portOfText('65536')).toBeUndefined();
    expect(portOfText('55.4')).toBeUndefined();
    expect(portOfText('')).toBeUndefined();
  });

  it('moves only a port typed as another valid number', () => {
    expect(movedPorts(asked, { syslog: '5514', metrics: '8514' })).toEqual({
      syslog: 5514,
    });
    expect(movedPorts(asked, { syslog: 'abc' })).toEqual({});
  });
});

describe('a database of its own', () => {
  it('reads the line back and sends it as expect (ADR-20261021-bqc rule 8)', () => {
    expect(requestOfLine('database of its own', undefined)).toEqual({
      kind: 'database',
    });

    const read = requestsOfLines(['database of its own'], undefined);
    expect(read.database).toBe(true);
    expect(asksForDatabase(read)).toBe(true);
    expect(asksForDatabase({ permissions: ['observe'] })).toBe(false);
    expect(requestsOfOffer(offer({ database: true }))?.database).toBe(true);
    expect('database' in (requestsOfOffer(offer()) ?? {})).toBe(false);
  });

  it('lists the database after the directory and before the ports, and keeps it in a merge', () => {
    const requests = {
      permissions: ['observe'],
      data: true,
      database: true,
      listen: [
        { name: 'syslog', network: 'udp' as const, port: 1, why: 'lines' },
      ],
    };

    expect(listRequests(requests).map((one) => one.kind)).toEqual([
      'permission',
      'data',
      'database',
      'listen',
    ]);
    expect(
      mergeRequests({ permissions: [] }, { database: true }).database,
    ).toBe(true);
    expect('database' in mergeRequests({}, {})).toBe(false);
  });

  it('sends the place a person picked, and nothing to keep the one it has', () => {
    expect(databaseChoiceOf('keep', 'x')).toBeUndefined();
    expect(databaseChoiceOf('sqlite', 'x')).toEqual({ mode: 'sqlite' });
    expect(databaseChoiceOf('postgres', ' postgres://db/x ')).toEqual({
      mode: 'postgres',
      dsn: 'postgres://db/x',
    });
  });

  it('takes a connection string only as a postgres URL with a host and a database', () => {
    expect(isPostgresURL('postgres://notes:pw@db.lan:5432/notes')).toBe(true);
    expect(isPostgresURL(' postgresql://db.lan/notes?sslmode=require ')).toBe(
      true,
    );
    expect(isPostgresURL('host=db.lan dbname=notes')).toBe(false);
    expect(isPostgresURL('postgres://db.lan/')).toBe(false);
    expect(isPostgresURL('mysql://db.lan/notes')).toBe(false);
  });
});

describe('a memory recommendation', () => {
  it('reads the line back and sends it as expect, so an approval holds it', () => {
    expect(requestOfLine('memory 192 MiB', undefined)).toEqual({
      kind: 'memory',
      mib: 192,
    });
    expect(requestsOfLines(['memory 192 MiB'], undefined).memory_mib).toBe(192);
    expect(requestsOfOffer(offer({ memory_mib: 192 }))?.memory_mib).toBe(192);
    expect('memory_mib' in (requestsOfOffer(offer()) ?? {})).toBe(false);
    expect(
      mergeRequests({ memory_mib: 64 }, { memory_mib: 192 }).memory_mib,
    ).toBe(192);
  });
});

describe('every kind of ask', () => {
  // A field added to ModuleRequests without a value here fails to compile.
  const ALL: Required<ModuleRequests> = {
    permissions: ['observe'],
    checks: ['operate'],
    events: { done: 'print.done' },
    check_absent_people: true,
    list_people: true,
    calls: ['com.github.aux.other'],
    public_routes: ['POST /hooks/announce/'],
    staged_routes: [{ method: 'PUT', path: '/files/{name}', max_mib: 1024 }],
    request_headers: [{ name: 'Overwrite', reason: 'replaces a file' }],
    allowed_origins: ['https://slicer.example'],
    records_mib: 16,
    memory_mib: 256,
    data: true,
    database: true,
    listen: [{ name: 'syslog', network: 'udp', port: 5514, why: 'debug logs' }],
  };

  it('draws one line for each kind, each with its own sentence', () => {
    const kinds = new Set(listRequests(ALL).map((r) => r.kind));
    expect(kinds.size).toBe(Object.keys(ALL).length);
    for (const kind of kinds) {
      expect(
        en[`app.settings.extensions.request.${kind}` as keyof typeof en],
      ).toBeTypeOf('string');
    }
  });

  it('reads every kind back from the lines the server writes', () => {
    const lines = [
      'permission observe',
      'check operate',
      `event done as ${NAMESPACE}.print.done`,
      'permission check_absent_people',
      'permission list_people',
      'call com.github.aux.other',
      'public POST /hooks/announce/',
      'upload PUT /files/{name} up to 1024 MiB',
      'header Overwrite (replaces a file)',
      'origin https://slicer.example',
      'records 16 MiB',
      'memory 256 MiB',
      'data directory',
      'database of its own',
      'listen udp 5514 as syslog (debug logs)',
    ];
    expect(listRequests(requestsOfLines(lines, NAMESPACE))).toEqual(
      listRequests(ALL),
    );
  });
});
