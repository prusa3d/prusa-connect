import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig } from 'vitest/config';

// The tests do not read `vite.config.ts`: that file is the library build, and
// merging a `build.lib` into a test run would ask Vitest to bundle the module
// rather than run it.
//
// Every test here touches runes or the DOM, so every one runs in a real
// Chromium (svelte-testing.md).
//
// Tailwind is in this config and the setup file loads the theme. One of these
// tests measures where the header and the list sit at two widths, and a
// measurement on an unstyled page measures nothing: the card layout below
// `@md` and the table above it are both container queries, and a container
// query needs the compiled stylesheet to apply at all.
export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  test: {
    name: 'browser',
    include: ['src/**/*.svelte.test.ts'],
    // The live lane has a config of its own, because it needs a server this
    // package does not start.
    exclude: ['src/live/**'],
    setupFiles: ['./src/test/setup.ts'],
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
      // The page marks its parts with `data-part`, the same attribute the
      // library's own components carry, so a test scopes a query to one part
      // of the page without a second set of attributes put there for it.
      locators: { testIdAttribute: 'data-part' },
    },
  },
});
