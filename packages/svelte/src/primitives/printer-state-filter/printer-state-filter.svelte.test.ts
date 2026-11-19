import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Input from '../input/input.svelte';
import PrinterStateFilter from './printer-state-filter.svelte';
import type { PrinterFacetDimension, PrinterStateFilterProps } from './types';

const STATES = ['PRINTING', 'IDLE', 'PAUSED', 'ERROR'];
const TIERS = ['critical', 'attention', 'normal'];

const label = (value: string): string =>
  value.charAt(0).toUpperCase() + value.slice(1).toLowerCase();

const dimension = (
  values: string[],
  changed: string[][],
  selected: string[] = [],
): PrinterFacetDimension => ({
  values,
  counts: new Map(values.map((value, index) => [value, index + 1])),
  selected,
  onSelectedChange: (next) => changed.push(next),
  labelFor: label,
});

interface Drawn {
  trigger: HTMLButtonElement;
  /** Every selection each dimension handed back, oldest first. */
  states: string[][];
  tiers: string[][];
}

const draw = async (
  props: Partial<PrinterStateFilterProps> = {},
  selected: { state?: string[]; tier?: string[] } = {},
): Promise<Drawn> => {
  const states: string[][] = [];
  const tiers: string[][] = [];
  const { container } = await render(PrinterStateFilter, {
    props: {
      severity: dimension(TIERS, tiers, selected.tier ?? []),
      state: dimension(STATES, states, selected.state ?? []),
      ...props,
    },
  });

  const trigger = container.querySelector<HTMLButtonElement>(
    '[data-slot="facet-popover-trigger"]',
  );
  if (trigger === null) throw new Error('no facet rendered');
  return { trigger, states, tiers };
};

/** The panel, which bits-ui portals to the end of the document. */
const panel = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>('[data-slot="popover-content"]');

const open = async (trigger: HTMLButtonElement): Promise<HTMLElement> => {
  await page.elementLocator(trigger).click();
  await expect.poll(panel).not.toBeNull();
  const surface = panel();
  if (surface === null) throw new Error('no panel opened');
  return surface;
};

const rows = (surface: HTMLElement): HTMLButtonElement[] => [
  ...surface.querySelectorAll<HTMLButtonElement>('[data-slot="facet-row"]'),
];

describe('PrinterStateFilter', () => {
  it('heads both dimensions and divides them, worst first', async () => {
    const { trigger } = await draw();
    const surface = await open(trigger);

    const headings = [...surface.querySelectorAll('p')].map((node) =>
      node.textContent.trim(),
    );
    expect(headings).toStrictEqual(['Severity', 'State']);
    expect(surface.querySelectorAll('.border-t')).toHaveLength(1);
    expect(rows(surface)).toHaveLength(TIERS.length + STATES.length);
  });

  it('hands each dimension back its own next selection', async () => {
    const { trigger, states, tiers } = await draw();
    const surface = await open(trigger);

    const critical = rows(surface).find((row) =>
      row.textContent.includes('Critical'),
    );
    critical?.click();

    expect(tiers).toStrictEqual([['critical']]);
    expect(states).toStrictEqual([]);
  });

  it('names the dimension and the choice on the trigger, across both groups', async () => {
    const { trigger } = await draw({}, { tier: ['critical'], state: ['IDLE'] });

    // The summary reads in labels and spans both dimensions, because the
    // trigger answers what the list is narrowed by.
    expect(trigger.textContent.replace(/\s+/g, ' ').trim()).toBe(
      'State: Critical +1',
    );
  });

  it('filters on raw state alone when no tier is given', async () => {
    const { trigger } = await draw({ severity: undefined });
    const surface = await open(trigger);

    expect(
      [...surface.querySelectorAll('p')].map((node) => node.textContent),
    ).toStrictEqual(['State']);
    expect(rows(surface)).toHaveLength(STATES.length);
  });

  it('leaves out the search box while the two dimensions are short', async () => {
    // Seven values in all, which is under the threshold: below it a search is
    // furniture.
    const { trigger } = await draw();
    const surface = await open(trigger);

    expect(surface.querySelector('[data-slot="input"]')).toBeNull();
  });

  it('adds the search box past eight values across both dimensions', async () => {
    const changed: string[][] = [];
    const { trigger } = await draw({
      state: dimension(
        [...STATES, 'OFFLINE', 'FINISHED', 'ATTENTION', 'BUSY', 'READY'],
        changed,
      ),
    });
    const surface = await open(trigger);
    const box = surface.querySelector<HTMLInputElement>('[data-slot="input"]');

    expect(box?.getAttribute('aria-label')).toBe('Search printer states');
  });

  it('opens a panel of the caller width, on the row it stands in', async () => {
    const { trigger } = await draw({ size: 'sm', width: 300 });
    const input = await render(Input, {
      props: { size: 'sm', 'aria-label': 'sm' },
    });
    const row = getComputedStyle(input.container.querySelector('input')!);

    expect(getComputedStyle(trigger).height).toBe(row.height);
    await input.unmount();

    const surface = await open(trigger);
    expect(getComputedStyle(surface).width).toBe('300px');
  });

  it('has nothing axe objects to', async () => {
    const { trigger } = await draw({}, { tier: ['critical'] });
    const surface = await open(trigger);

    await expectNoViolations(surface);
  });
});
