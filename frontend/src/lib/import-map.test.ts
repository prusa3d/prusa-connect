import { mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { build, createServer } from 'vite';
import { afterAll, beforeAll, expect, test } from 'vitest';

import { SHARED_SPECIFIERS } from '@prusa/connect-ui/build';

// The built document has to carry an import map, and every address in it has
// to be a file this build wrote (ADR-20260911-4p6 rule 27). A module's bundle keeps
// those specifiers bare, so an address that is missing or wrong is a module
// that fails on its first line.
//
// The build runs here rather than against `dist/`, so the gate says something
// on a checkout where nothing has been built.

const ROOT = fileURLToPath(new URL('../../', import.meta.url));

let out = '';
let html = '';

beforeAll(async () => {
  out = await mkdtemp(join(tmpdir(), 'prusa-connect-import-map-'));
  await build({
    root: ROOT,
    logLevel: 'silent',
    build: { outDir: out, emptyOutDir: true },
  });
  html = await readFile(join(out, 'index.html'), 'utf8');
}, 240_000);

// Removing the build can take longer than the default ten seconds on a busy
// CI runner, and the gate is the import map, not how fast a disk deletes.
afterAll(async () => {
  if (out !== '') await rm(out, { recursive: true, force: true });
}, 60_000);

const mapOf = (document: string): Record<string, string> => {
  const found = /<script type="importmap">(.*?)<\/script>/s.exec(document);
  expect(found).not.toBeNull();

  const parsed: unknown = JSON.parse(found?.[1] ?? '{}');
  if (
    typeof parsed !== 'object' ||
    parsed === null ||
    !('imports' in parsed) ||
    typeof parsed.imports !== 'object' ||
    parsed.imports === null
  ) {
    throw new Error('the import map holds no `imports` object');
  }

  return parsed.imports as Record<string, string>;
};

test('the map names every shared specifier and nothing else', () => {
  expect(Object.keys(mapOf(html))).toEqual([...SHARED_SPECIFIERS]);
});

test('the map comes before the first module script', () => {
  const map = html.indexOf('<script type="importmap">');
  const module = html.indexOf('<script type="module"');

  expect(map).toBeGreaterThanOrEqual(0);
  expect(module).toBeGreaterThan(map);
});

// The addresses are relative, and the document's `<base>` element says what to:
// the root, or the prefix a reverse proxy serves the app under, which the core
// writes into it. The element has to come before the map, or the map's
// addresses are read against the route the document answered at.
test('the base element names the root and comes before the map', () => {
  const base = html.indexOf('<base href="/" />');
  const map = html.indexOf('<script type="importmap">');

  expect(base).toBeGreaterThanOrEqual(0);
  expect(map).toBeGreaterThan(base);
});

test('every address in the map is a file this build wrote', async () => {
  for (const [specifier, address] of Object.entries(mapOf(html))) {
    expect(address, specifier).toMatch(/^\.\/[\w./-]+\.js$/);
    const found = await stat(join(out, address.slice(2)));
    expect(found.isFile(), `${specifier} at ${address}`).toBe(true);
  }
});

// The dev document, as `pnpm dev` serves it. `VITEST` is lifted for the one
// server, because the app's config keeps tests on the built library.
//
// The dependency scan is off because the document needs none of it. Started,
// it compiles the library's source on this thread and goes on after
// `server.close()`, and on a slow runner it held the event loop past the
// hook timeout of the `afterAll` above.
test('in the dev server the library is read from its source', async () => {
  const vitest = process.env.VITEST;
  delete process.env.VITEST;
  const server = await createServer({
    root: ROOT,
    logLevel: 'silent',
    server: { middlewareMode: true },
    optimizeDeps: { noDiscovery: true },
  });
  try {
    const document = await server.transformIndexHtml(
      '/',
      '<!doctype html><html><head></head><body></body></html>',
    );
    const map = mapOf(document);

    expect(map['@prusa/connect-ui']).toMatch(
      /^\/@fs\/.*\/packages\/svelte\/src\/index\.ts$/,
    );
    expect(map['@prusa/connect-ui/i18n']).toMatch(
      /\/packages\/svelte\/src\/i18n\/index\.ts$/,
    );
    expect(map['@prusa/connect']).toMatch(
      /\/packages\/typescript\/src\/index\.ts$/,
    );
    for (const specifier of SHARED_SPECIFIERS.filter((s) =>
      s.startsWith('@prusa/'),
    )) {
      expect(map[specifier], specifier).toMatch(/\/packages\/[a-z]+\/src\//);
    }
    expect(map['svelte/internal/client']).not.toMatch(/\.vite\/deps/);
  } finally {
    await server.close();
    process.env.VITEST = vitest;
  }
}, 60_000);
