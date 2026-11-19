import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig } from 'vitest/config';

// The lane that draws this module's page in front of a server that is really
// running. `extensions/parts/scripts/check.sh` starts that server, installs the module
// into it and then runs this.
//
// A config of its own, because `pnpm run check` cannot run it: it needs a
// server this package does not start, and it reads the address and the
// password out of the environment.
//
// The browser calls `/api/...` on the test server's own origin and Vite
// forwards it to the core. So the page runs same-origin, the sign-in cookie is
// set on the origin the page is on, and the request the page makes is the
// request a browser would make with no proxy in front of it.
const core = process.env.PCL_CORE_URL ?? 'http://127.0.0.1:18098';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  // Vite exposes only the variables whose names carry a prefix it knows, so
  // the two the check script sets are written in here by name.
  define: {
    'import.meta.env.PCL_DEMO_USER': JSON.stringify(
      process.env.PCL_DEMO_USER ?? 'demo',
    ),
    'import.meta.env.PCL_DEMO_PASSWORD': JSON.stringify(
      process.env.PCL_DEMO_PASSWORD ?? '',
    ),
  },
  server: {
    proxy: {
      '/api': { target: core, changeOrigin: true },
    },
  },
  test: {
    name: 'live',
    include: ['src/live/*.live.test.ts'],
    setupFiles: ['./src/test/setup.ts'],
    testTimeout: 60_000,
    hookTimeout: 60_000,
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      // Wide enough for the folder tree to sit beside the listing: below the
      // page's `@4xl` it starts shut behind a toggle.
      instances: [
        { browser: 'chromium', viewport: { width: 1280, height: 900 } },
      ],
      screenshotFailures: false,
      locators: { testIdAttribute: 'data-part' },
    },
  },
});
