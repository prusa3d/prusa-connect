import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// The extension is written out because Vite's own config loader asks for it.
import { importMap } from './vite/import-map.ts';

// Plain Vite, client-only, no SvelteKit and no server code (ADR-20260911-4p6 rule 34).
// `importMap()` comes first because it excludes Svelte from pre-bundling, and
// vite-plugin-svelte only stops adding Svelte back if it reads that exclusion
// first. The other two plugins are the ones the fourth consumer under
// packages/typescript/testdata/consumers/svelte-ui adds, and nothing else.
export default defineConfig({
  plugins: [importMap(), tailwindcss(), svelte()],

  // The build writes every address relative, and the entry document's
  // `<base>` element says what they are relative to: the root, or the path
  // prefix a reverse proxy serves the app under. Vite's dev server reads a
  // relative base as the root.
  base: './',

  // The component library's built entry points are scanned as well as the
  // document, so the dependency optimizer sees what the library imports before
  // anything runs. `importMap()` excludes the library from pre-bundling, and
  // Vite's cold-start scan stops at an excluded package, so `bits-ui`,
  // `svelte-sonner` and every icon the library draws with used to be first
  // seen when the browser asked for one. Vite optimized them at that point and
  // reloaded the page, and Vitest failed the test file that was loading with
  // `Vite unexpectedly reloaded a test`.
  //
  // Scanning is what fixes it rather than `optimizeDeps.include`: the icons
  // are 25 separate subpaths today and a different 25 after the next design
  // change, and a list here would go stale without anything saying so.
  optimizeDeps: {
    entries: [
      'index.html',
      '../packages/svelte/dist/index.js',
      '../packages/svelte/dist/*/index.js',
      // The dashboard screen is the one subpath a level deeper, a chunk the
      // app loads when a dashboard is drawn (ADR-20261008-sww rule 24).
      '../packages/svelte/dist/shell/dashboard/index.js',
      // The first-run wizard, a chunk the app loads only on a new install.
      '../packages/svelte/dist/shell/setup-wizard/index.js',
    ],
  },

  // `appType` is left at Vite's default, `spa`, so the dev server answers the
  // entry document for an address that names no file. That is what the core
  // does in production (`internal/transport/http/httpapi/appui.go`), and it is
  // what lets the router use real addresses rather than a hash.
  server: {
    // The app holds no origin: the SDK runs in session mode and sends every
    // request to a relative path, so the dev server forwards them to a core.
    // PCL_CORE_URL points at one, such as `make demo` on :8080.
    proxy: {
      '/api': {
        target: process.env.PCL_CORE_URL ?? 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
});
