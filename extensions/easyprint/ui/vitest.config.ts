import { svelte } from '@sveltejs/vite-plugin-svelte';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig } from 'vitest/config';

// Every test runs in a real Chromium.
export default defineConfig({
  plugins: [svelte()],
  // A test reads ../module.yaml, which sits one folder above this project.
  server: { fs: { allow: ['..'] } },
  test: {
    name: 'browser',
    include: ['src/**/*.test.ts'],
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      instances: [{ browser: 'chromium' }],
      screenshotFailures: false,
    },
  },
});
