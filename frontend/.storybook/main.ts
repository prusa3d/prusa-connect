// The Storybook for the whole product: the app's stories under `src/` and the
// component library's under `packages/svelte/src/`, on one page.
//
// `vite.config.ts` is read for its plugins, with `importMap()` swapped for one
// that writes the map into Storybook's preview document, `iframe.html`. The
// map is what the fleet-stats bundle, served from `modules/fleet-stats/`,
// loads Svelte and the library through, so it draws with the page's own
// runtime. The library is read from source in both the dev server and the
// build, through the same aliases `importMap()` sets in the app's dev server.
// Library stories import their components by relative path, so app code that
// imports `@prusa/connect-ui` gets the same module, not a copy from
// `dist/`.
//
// The library and the app each install their own `svelte`. The page still loads
// one runtime, because `svelte()` from vite-plugin-svelte puts `svelte` in
// `resolve.dedupe`. Two runtimes would fail with "Function called outside
// component initialization".
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import type { StorybookConfig } from '@storybook/svelte-vite';
import {
  mergeConfig,
  type Plugin,
  type PluginOption,
  searchForWorkspaceRoot,
} from 'vite';

import { storageThumbnails } from '../test/story/storage-thumbnails.ts';

import { importMap, sourceAliases } from '../vite/import-map.ts';

const here = path.dirname(fileURLToPath(import.meta.url));

// The library's own Storybook measured a build at 5 minutes 37 with the props
// tables against 10 seconds without, so a build leaves them out and the dev
// server reads them one story at a time.
const building = process.argv.includes('build');

const isImportMap = (plugin: PluginOption): boolean =>
  typeof plugin === 'object' &&
  plugin !== null &&
  'name' in plugin &&
  plugin.name === 'prusa-connect:import-map';

const filePreviews: Plugin = {
  name: 'prusa-connect:storybook-file-thumbnails',
  configureServer(server) {
    server.middlewares.use(
      '/api/v1/storages/local/thumbnail',
      (request, response, next) => {
        const url = new URL(request.url ?? '', 'http://localhost');
        const preview = storageThumbnails.get(
          url.searchParams.get('path') ?? '',
        );
        if (request.method !== 'GET' || preview === undefined) {
          next();
          return;
        }
        response.setHeader('Content-Type', 'image/png');
        response.end(
          readFileSync(path.join(here, '../test/story/assets', preview.file)),
        );
      },
    );
  },
};

const config: StorybookConfig = {
  // A self-hosted open-source tool must not phone home on every build.
  core: { disableTelemetry: true },
  stories: [
    '../src/**/*.mdx',
    '../src/**/*.stories.svelte',
    '../../packages/svelte/src/**/*.stories.svelte',
  ],
  addons: ['@storybook/addon-svelte-csf', '@storybook/addon-docs'],
  framework: {
    name: '@storybook/svelte-vite',
    options: { docgen: !building },
  },
  // The module bundle is built by `pnpm run build` in
  // `extensions/fleet-stats/ui`, which `make frontend-storybook` runs first.
  staticDirs: [
    { from: '../public/img', to: '/img' },
    { from: '../public/fonts', to: '/fonts' },
    {
      from: '../../extensions/fleet-stats/ui/dist/ui',
      to: '/modules/fleet-stats',
    },
  ],
  viteFinal: (vite) =>
    mergeConfig(
      {
        ...vite,
        plugins: [
          filePreviews,
          ...(vite.plugins ?? [])
            .flat()
            .map((p) =>
              isImportMap(p) ? importMap({ page: 'iframe.html' }) : p,
            ),
        ],
      },
      {
        resolve: {
          alias: [
            ...sourceAliases(),
            {
              find: '@prusa/connect-ui/app.css',
              replacement: path.resolve(
                here,
                '../../packages/svelte/src/styles/app.css',
              ),
            },
            {
              find: '@prusa/connect-ui/tokens.css',
              replacement: path.resolve(
                here,
                '../../packages/svelte/src/styles/tokens.css',
              ),
            },
          ],
          dedupe: ['svelte'],
        },
        // A list here replaces Vite's default of the workspace root, which
        // holds Storybook's own runtime, so the root is named again.
        server: {
          fs: {
            allow: [
              searchForWorkspaceRoot(process.cwd()),
              path.resolve(here, '../../packages'),
            ],
          },
        },
      },
    ),
};

export default config;
