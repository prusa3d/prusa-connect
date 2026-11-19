// The pool bar: an unknown pool is said so and never drawn as zero, hidden
// modules are one segment, and a promise past the pool warns.
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import MemoryPool from './memory-pool.svelte';

const MIB = 1_048_576;

describe('MemoryPool', () => {
  it('an unknown pool says unknown, never zero', async () => {
    await render(MemoryPool, {
      props: { pool: { committed_bytes: 128 * MIB }, budgets: [] },
    });

    await expect
      .element(page.getByText(/Pool unknown: the server could not read/u))
      .toBeVisible();
    expect(
      document.querySelector('[data-part="pool-summary"]')?.textContent,
    ).not.toMatch(/free|of/u);
    expect(document.querySelector('[data-part="pool-bar"]')).toBeNull();
  });

  it('hidden modules are one segment', async () => {
    await render(MemoryPool, {
      props: {
        pool: { committed_bytes: 400 * MIB, pool_bytes: 1024 * MIB },
        budgets: [
          { key: 'a', title: 'Spools', bytes: 128 * MIB },
          { key: 'b', title: 'Nozzle watch', bytes: 128 * MIB },
        ],
      },
    });

    const segments = document.querySelectorAll('[data-part="pool-segment"]');
    expect(segments).toHaveLength(3);
    expect(segments[2]?.getAttribute('data-other')).toBe('true');
    await expect
      .element(page.getByText('Other modules: 144 MiB'))
      .toBeVisible();
  });

  it('gives each module its own colour, matching its legend swatch', async () => {
    await render(MemoryPool, {
      props: {
        pool: { committed_bytes: 256 * MIB, pool_bytes: 1024 * MIB },
        budgets: [
          { key: 'a', title: 'Spools', bytes: 128 * MIB },
          { key: 'b', title: 'Nozzle watch', bytes: 128 * MIB },
        ],
      },
    });

    const fill = (part: string) =>
      [...document.querySelectorAll(`[data-part="${part}"]`)].map(
        (one) => /bg-[\w-]+/u.exec(one.className)?.[0],
      );
    expect(fill('pool-segment')).toEqual(['bg-primary', 'bg-info']);
    expect(fill('pool-swatch')).toEqual(['bg-primary', 'bg-info']);
  });

  it('a promise past the pool warns and says by how much', async () => {
    await render(MemoryPool, {
      props: {
        pool: { committed_bytes: 300 * MIB, pool_bytes: 256 * MIB },
        budgets: [{ key: 'a', title: 'Spools', bytes: 300 * MIB }],
      },
    });

    await expect.element(page.getByText('Over by 44 MiB')).toBeVisible();
  });

  it('is a settings pane with the summary in its header band', async () => {
    await render(MemoryPool, {
      props: {
        pool: { committed_bytes: 256 * MIB, pool_bytes: 1024 * MIB },
        budgets: [{ key: 'a', title: 'Spools', bytes: 256 * MIB }],
      },
    });

    const header = document.querySelector('[data-slot="settings-pane-header"]');
    expect(header?.textContent).toContain('Module memory');
    expect(header?.querySelector('[data-part="pool-summary"]')).not.toBeNull();
  });

  it('draws nothing without a pool', async () => {
    await render(MemoryPool, { props: { pool: undefined, budgets: [] } });

    expect(document.querySelector('[data-part="memory-pool"]')).toBeNull();
  });
});
