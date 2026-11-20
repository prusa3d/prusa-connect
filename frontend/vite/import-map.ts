// The app's half of runtime sharing: ship one copy of Svelte, the library and
// the SDK, and tell the browser where each one is (ADR-20260911-4p6 rule 27).
//
// Moved from pull request 185, packages/core/src/plugins/vite.ts, 2026-09-11.
// Three things come across: the emitted chunks carry `preserveSignature:
// 'strict'`, so an export no page of ours touches is still there for a module
// to reach; Svelte is kept out of dependency pre-bundling, so the dev server
// serves it from one stable address; and the map is prepended to `<head>`, so
// it precedes the first module script the document loads.
//
// What is different here is the shape of the host. PR 185 wrote fixed file
// names and a SvelteKit `handle` hook, because SvelteKit owns its own HTML.
// This app is plain Vite (ADR-20260911-4p6 rule 34), so `transformIndexHtml` answers
// in the dev server and `generateBundle` rewrites the built document, and the
// chunks carry a content hash instead of a fixed name.
//
// Why a module bundle needs it: the bundle is a static file Vite never
// transforms, so its bare imports reach the browser as written. Two copies of
// the Svelte runtime on one page crash with `effect_orphan` (earlier research
// finding 25), so the map is what keeps the page at one.
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import type { Alias, Plugin, ResolvedConfig, ViteDevServer } from 'vite';

// The library's source, by a relative path, because this file is loaded as
// part of `vite.config.ts` and Vite leaves every bare import to Node. The
// path keeps one list: the app emits a chunk per entry, and the library's
// `moduleBuild()` marks the same entries external. The extension is written
// out because Vite's own config loader asks for it.
import {
  chunkNameOf,
  SHARED_SPECIFIERS,
} from '../../packages/svelte/build/specifiers.ts';

/**
 * The two packages this checkout links, so the dev server may serve a file
 * from either. It refuses a path outside its allow list with a 403, and some
 * of the map's addresses are subpaths no page of this app imports, so nothing
 * else would put them in the module graph.
 */
const LINKED_PACKAGES = fileURLToPath(
  new URL('../../packages', import.meta.url),
);

/**
 * Where the dev server answers for a file it resolved. The `?v=` a resolver
 * appends is part of the address and is kept: the same file under two query
 * strings is two modules to the browser, which is two runtimes.
 */
const devUrlOf = (id: string, root: string): string =>
  id.startsWith(`${root}/`) ? id.slice(root.length) : `/@fs/${id}`;

/** The map as one script tag, with `<` escaped so an address cannot end it. */
const tagFor = (imports: Record<string, string>): string =>
  JSON.stringify({ imports }).replaceAll('<', '\\u003c');

/**
 * Whether the linked packages are read from `src/` rather than `dist/`. The
 * dev server reads source, so an edit to a component or a TypeScript module
 * reaches the app through HMR with no package build; a stylesheet
 * (`app.css`, `tokens.css`) still comes from `dist/`, so a token or theme
 * edit still needs one. A test run reads what ships, because a test that
 * passes against source proves nothing about `dist/`.
 */
const fromSource = (command: 'build' | 'serve'): boolean =>
  command === 'serve' && process.env.VITEST === undefined;

/**
 * Each linked subpath, mapped to its file under `src/`. The list is read from
 * the packages' `exports` maps: a subpath whose `types` is `./dist/<p>.d.ts`
 * has its source at `./src/<p>.ts`. A subpath with no such file, such as
 * `./build` or a stylesheet, resolves to what ships. An alias and not an
 * export condition, because a condition in `exports` ships to every consumer
 * and names files the published package does not carry.
 */
export const sourceAliases = (): Alias[] =>
  ['svelte', 'typescript'].flatMap((dir) => {
    const root = join(LINKED_PACKAGES, dir);
    const { name, exports } = JSON.parse(
      readFileSync(join(root, 'package.json'), 'utf8'),
    ) as { name: string; exports: Record<string, string | { types?: string }> };

    return Object.entries(exports).flatMap(([subpath, target]) => {
      const types = typeof target === 'object' ? target.types : undefined;
      const stem = types?.match(/^\.\/dist\/(.+)\.d\.ts$/)?.[1];
      const file = stem === undefined ? '' : join(root, 'src', `${stem}.ts`);
      if (file === '' || !existsSync(file)) return [];

      const specifier = name + subpath.slice(1);
      const exact = specifier.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      return [{ find: new RegExp(`^${exact}$`), replacement: file }];
    });
  });

const LIBRARY_SOURCE_ENTRIES = [
  'index.html',
  '../packages/svelte/src/index.ts',
  '../packages/svelte/src/*/index.ts',
  // The dashboard screen, the one subpath a level deeper (ADR-20261008-sww rule 24).
  '../packages/svelte/src/shell/dashboard/index.ts',
  // The first-run wizard, the other subpath a level deeper.
  '../packages/svelte/src/shell/setup-wizard/index.ts',
];

/**
 * `page` names the built document the map goes into. The app's is
 * `index.html`; Storybook's preview is `iframe.html`.
 */
export const importMap = ({ page = 'index.html' } = {}): Plugin => {
  let config: ResolvedConfig;
  let server: ViteDevServer | undefined;
  /** specifier to the reference id of the chunk emitted for it. */
  const emitted = new Map<string, string>();

  return {
    name: 'prusa-connect:import-map',
    enforce: 'pre',

    config: {
      // vite-plugin-svelte declares the same order, so within it the array
      // order decides and this plugin has to be listed first.
      // `configResolved` refuses the build when it is not.
      order: 'pre',
      handler: (_config, { command }) => ({
        optimizeDeps: {
          // Pre-bundling would inline Svelte into a hashed `.vite/deps`
          // chunk whose address changes on every re-optimise, and would put
          // a second copy of it beside every other entry. vite-plugin-svelte
          // reads this exclusion and stops adding Svelte to
          // `optimizeDeps.include` itself, but only if it sees it, which is
          // what the ordering is for.
          exclude: [...SHARED_SPECIFIERS],
          // Read from source in serve, so the optimizer's cold-start scan has
          // to start where the source is, for the reason the app's own
          // `optimizeDeps.entries` gives.
          ...(fromSource(command) && { entries: LIBRARY_SOURCE_ENTRIES }),
        },
        server: { fs: { allow: [LINKED_PACKAGES] } },
        ...(fromSource(command) && { resolve: { alias: sourceAliases() } }),
      }),
    },

    configResolved: (resolved) => {
      config = resolved;

      // Pre-bundled Svelte in the dev server means the app loads the runtime
      // from `.vite/deps` while the map still names the source module: two
      // runtimes, and `effect_orphan` the first time a module renders. The
      // cause is one line of the app's config, so it is named here rather
      // than left to be read out of the crash.
      const included =
        resolved.environments.client?.optimizeDeps.include ??
        resolved.optimizeDeps.include ??
        [];
      const prebundled = included.filter(
        (dep) => dep === 'svelte' || dep.startsWith('svelte/'),
      );
      if (prebundled.length > 0) {
        throw new Error(
          `importMap() has to come before svelte() in the plugins array. Svelte is pre-bundled (${prebundled.join(', ')}), so a module bundle would load a second runtime.`,
        );
      }
    },

    configureServer: (dev) => {
      server = dev;
    },

    buildStart: {
      order: 'pre',
      handler: async function buildStart() {
        emitted.clear();
        if (config.command !== 'build') return;

        for (const specifier of SHARED_SPECIFIERS) {
          const resolved = await this.resolve(specifier);
          if (!resolved) {
            throw new Error(
              `${specifier} did not resolve; is it installed in this app?`,
            );
          }

          // Without `strict` the chunk keeps only the exports the rest of the
          // build happens to use, and a module reaching for one this app's
          // own pages never touched would find it missing.
          emitted.set(
            specifier,
            this.emitFile({
              type: 'chunk',
              id: resolved.id,
              name: chunkNameOf(specifier),
              preserveSignature: 'strict',
            }),
          );
        }
      },
    },

    // The dev server's path. `head-prepend` is what puts the map before the
    // first module script, which is the browser's requirement.
    transformIndexHtml: {
      order: 'pre',
      handler: async () => {
        // Held in a local so it stays narrowed inside the closure below.
        const dev = server;
        if (!dev) return [];

        const root = config.root;
        const entries = await Promise.all(
          SHARED_SPECIFIERS.map(async (specifier) => {
            const resolved = await dev.pluginContainer.resolveId(specifier);
            if (!resolved) {
              throw new Error(
                `${specifier} did not resolve; is it installed in this app?`,
              );
            }

            return [specifier, devUrlOf(resolved.id, root)] as const;
          }),
        );

        return [
          {
            tag: 'script',
            attrs: { type: 'importmap' },
            children: tagFor(Object.fromEntries(entries)),
            injectTo: 'head-prepend',
          },
        ];
      },
    },

    // The built document's path. It runs after Vite has written the HTML and
    // Rollup has named every chunk, so the map carries the hashed addresses.
    generateBundle: {
      order: 'post',
      handler: function generateBundle(_options, bundle) {
        const document = bundle[page];
        if (document?.type !== 'asset') {
          throw new Error(`the build wrote no ${page} for the map`);
        }

        const imports = Object.fromEntries(
          [...emitted].map(([specifier, reference]) => [
            specifier,
            `${config.base}${this.getFileName(reference)}`,
          ]),
        );

        const html = String(document.source);
        const head = html.indexOf('<head>');
        if (head === -1) throw new Error(`${page} has no <head>`);

        // The app's one fallback document answers at every path (ADR-20260911-4p6
        // rule 33), so a relative address in its map names the same file on
        // every route only when the document's `<base>` element names the
        // root, and the map is read after it. The core writes a reverse
        // proxy's path prefix into that element. Storybook's preview is served
        // at one address only and needs no element.
        const base = /<base\s[^>]*>/u.exec(html);
        if (page === 'index.html' && config.base.startsWith('.') && !base) {
          throw new Error(
            `a relative base ("${config.base}") needs a <base> element in ${page}, and it has none.`,
          );
        }
        const at = base ? base.index + base[0].length : head + '<head>'.length;

        const tag = `<script type="importmap">${tagFor(imports)}</script>`;
        document.source = html.slice(0, at) + tag + html.slice(at);
      },
    },
  };
};
