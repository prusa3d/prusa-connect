import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import SelectFacetPopoverProbe from '../../test/select-facet-popover-probe.svelte';
import SelectFacet from './select-facet.svelte';
import type { SelectFacetOption, SelectFacetProps } from './types';

const STATES: SelectFacetOption[] = [
  { value: 'PRINTING', label: 'PRINTING', count: 6 },
  { value: 'IDLE', label: 'IDLE', count: 4 },
  { value: 'ERROR', label: 'ERROR', count: 0 },
];

interface Drawn {
  container: HTMLElement;
  /** Every value the list offers, in the order it draws them. */
  rows: () => HTMLButtonElement[];
  /** The values `onToggle` was called with, oldest first. */
  toggled: string[];
}

const draw = async (props: Partial<SelectFacetProps> = {}): Promise<Drawn> => {
  const toggled: string[] = [];
  const { container } = await render(SelectFacet, {
    props: {
      options: STATES,
      selected: [],
      onToggle: (value: string) => toggled.push(value),
      ...props,
    },
  });

  return {
    container,
    rows: () => [
      ...container.querySelectorAll<HTMLButtonElement>(
        '[data-slot="facet-row"]',
      ),
    ],
    toggled,
  };
};

const named = (rows: HTMLButtonElement[], text: string) => {
  const found = rows.find((row) => row.textContent.includes(text));
  if (found === undefined) throw new Error(`no row for ${text}`);
  return found;
};

describe('SelectFacet', () => {
  it('draws one row per option and reports the one that was picked', async () => {
    const { rows, toggled } = await draw();

    expect(rows()).toHaveLength(3);

    named(rows(), 'PRINTING').click();

    expect(toggled).toStrictEqual(['PRINTING']);
  });

  it('keeps an option nothing would return, dimmed and unpickable', async () => {
    const { rows, toggled } = await draw();

    const dead = named(rows(), 'ERROR');
    expect(dead.disabled).toBe(true);

    dead.click();
    expect(toggled).toStrictEqual([]);
  });

  it('narrows the list by search and says so when it finds nothing', async () => {
    const { container, rows } = await draw({
      searchable: true,
      searchAriaLabel: 'Search states',
    });

    const box = page.getByRole('searchbox', { name: 'Search states' });
    await box.fill('idl');

    expect(rows()).toHaveLength(1);
    expect(rows()[0]?.textContent).toContain('IDLE');

    await box.fill('zzz');

    expect(rows()).toHaveLength(0);
    expect(container.textContent).toContain('No match for');
  });

  it('keeps the real panel open while Escape empties the search', async () => {
    await render(SelectFacetPopoverProbe);
    await page.getByRole('button', { name: 'State' }).click();
    const panel = () =>
      document.body.querySelector('[data-slot="popover-content"]');
    const box = page.getByRole('searchbox', { name: 'Search states' });
    await box.fill('pl');

    await userEvent.keyboard('{Escape}');
    expect(box.element()).toHaveProperty('value', '');
    expect(panel()).not.toBeNull();

    await userEvent.keyboard('{Escape}');
    await expect.poll(panel).toBeNull();
  });

  it('matches a rich label on the search text a caller passes', async () => {
    const { rows } = await draw({
      options: [
        { value: 'mk4s', label: 'MK4S', searchText: 'Original Prusa MK4S' },
      ],
      searchable: true,
      searchAriaLabel: 'Search states',
    });

    await page.getByRole('searchbox', { name: 'Search states' }).fill('prusa');

    expect(rows()).toHaveLength(1);
  });

  it('heads each group and parts them, and drops a group search empties', async () => {
    const picked: string[] = [];
    const group = (id: string, heading: string, value: string) => ({
      id,
      heading,
      options: [{ value, label: value, count: 1 }],
      selected: [],
      onToggle: (name: string) => picked.push(name),
    });
    const { container, rows } = await draw({
      options: undefined,
      groups: [group('state', 'State', 'IDLE'), group('type', 'Type', 'MK4S')],
      searchable: true,
      searchAriaLabel: 'Search states',
    });

    const headings = [...container.querySelectorAll('p')].map((node) =>
      node.textContent.trim(),
    );
    expect(headings).toStrictEqual(['State', 'Type']);
    expect(container.querySelectorAll('.border-t')).toHaveLength(1);

    await page.getByRole('searchbox', { name: 'Search states' }).fill('mk4');

    expect(rows()).toHaveLength(1);
    expect(
      [...container.querySelectorAll('p')].map((node) => node.textContent),
    ).toStrictEqual(['Type']);
  });

  it('clears every dimension from the All row, in single mode', async () => {
    const cleared: string[] = [];
    const { rows } = await draw({
      mode: 'single',
      allLabel: 'All printers',
      allCount: 22,
      selected: [],
      onToggle: (value: string) => cleared.push(value),
    });

    const all = rows()[0];
    if (all === undefined) throw new Error('no All row rendered');
    expect(all.textContent).toContain('All printers');
    // The All row is ticked while nothing narrows the list.
    expect(all.getAttribute('aria-pressed')).toBe('true');
    expect(all.textContent).toContain('22');

    all.click();
    expect(cleared).toStrictEqual(['']);
  });

  it('draws no All row in multiple mode, and no count where the options carry none', async () => {
    const plain = await draw();
    expect(plain.rows()[0]?.textContent).toContain('PRINTING');

    const single = await draw({
      mode: 'single',
      options: STATES.map((state) => ({
        value: state.value,
        label: state.label,
      })),
      allCount: 22,
    });
    expect(single.rows()[0]?.textContent).not.toContain('22');
  });

  it('offers to create a value no option carries, and never a duplicate', async () => {
    const made: string[] = [];
    const { container } = await draw({
      searchable: true,
      searchAriaLabel: 'Search states',
      onCreate: (name: string) => made.push(name),
    });

    const create = () =>
      container.querySelector<HTMLButtonElement>(
        '[data-slot="select-facet-create"]',
      );

    expect(create()).toBeNull();

    const box = page.getByRole('searchbox', { name: 'Search states' });
    await box.fill('printing');

    // The query names a value the list already has, so it is picked and never
    // made again.
    expect(create()).toBeNull();

    await box.fill('Office');

    expect(create()?.textContent).toContain('Create');
    expect(create()?.textContent).toContain('Office');

    create()?.click();

    expect(made).toStrictEqual(['Office']);
    // The query is spent, so the list is whole again and the search box empty.
    await expect.poll(create).toBeNull();
  });

  it('caps the scrolling viewport rather than the box around it', async () => {
    const many = Array.from({ length: 40 }, (_value, index) => ({
      value: `v${String(index)}`,
      label: `Value ${String(index)}`,
      count: 1,
    }));
    const { container } = await draw({ options: many });

    const viewport = container.querySelector<HTMLElement>(
      '[data-slot="scroll-area-viewport"]',
    );
    if (viewport === null) throw new Error('no viewport rendered');

    expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);
    // A long value wraps inside the panel instead of widening it.
    expect(viewport.scrollWidth).toBe(viewport.clientWidth);
  });

  it('reaches every row from the keyboard', async () => {
    const { rows, toggled } = await draw();

    // The scrolling viewport takes focus first: a browser only scrolls a
    // container that is, or contains, the focused element, and the scroll area
    // gives it a `tabindex` for that.
    await userEvent.tab();
    await userEvent.tab();
    expect(document.activeElement).toBe(rows()[0]);
    await userEvent.keyboard(' ');

    expect(toggled).toStrictEqual(['PRINTING']);
  });

  it('has nothing axe objects to', async () => {
    const { container } = await draw({
      searchable: true,
      searchAriaLabel: 'Search states',
      selected: ['IDLE'],
    });

    await expectNoViolations(container);
  });
});
