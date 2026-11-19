import { playwright } from '@vitest/browser-playwright';
import { defineConfig, mergeConfig } from 'vitest/config';

import viteConfig from './vite.config.ts';
import { chromiumGcArgs } from './src/test/chromium-gc.ts';

const TOUCH = 'src/**/*.touch.svelte.test.ts';

// A component test runs in a real Chromium through Playwright
// (svelte-testing.md), and the `.svelte.` infix is what routes it there.
// A test of what a finger gets (`pointer: coarse`) has a `.touch.` infix and
// runs in a page of its own with a touch screen. Touch emulation switched on
// over CDP and off again leaves a headless Chromium on Linux with no hover and
// no pointer for every file that page runs after it, and Tailwind's `hover:`
// waits on `(hover: hover)`.
const browser = (name: string, include: string[], hasTouch: boolean) => ({
  extends: true,
  test: {
    name,
    include,
    exclude: hasTouch ? [] : [TOUCH],
    setupFiles: ['./src/test/setup.ts'],
    // A retried assertion waits five seconds, not the one second the
    // browser default gives. On a loaded runner a panel closes late
    // and the default reported it as still open.
    expect: { poll: { timeout: 5000 } },
    browser: {
      enabled: true,
      headless: true,
      provider: playwright({
        launchOptions: { args: chromiumGcArgs },
        contextOptions: { hasTouch },
      }),
      instances: [{ browser: 'chromium' as const }],
      screenshotFailures: false,
    },
  },
});

// The token tests read the stylesheet from disk and run the Tailwind compiler,
// so they run under Node.
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      projects: [
        browser('browser', ['src/**/*.svelte.test.ts'], false),
        browser('browser-touch', [TOUCH], true),
        {
          extends: true,
          test: {
            name: 'node',
            include: ['src/**/*.test.ts', 'build/**/*.test.ts'],
            exclude: ['src/**/*.svelte.test.ts'],
            environment: 'node',
          },
        },
      ],
    },
  }),
);
