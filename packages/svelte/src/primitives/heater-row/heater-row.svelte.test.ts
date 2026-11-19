import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import HeaterRow from './heater-row.svelte';
import HeaterRows from './heater-rows.svelte';
import type { HeaterRowProps } from './types';

const editor = createRawSnippet(() => ({
  render: () => '<button type="button" aria-label="Set target">e</button>',
}));

interface Drawn {
  container: HTMLElement;
  row: HTMLElement;
  /** The state word, visible or screen-reader-only. */
  word: HTMLElement;
  /** The trailing group: the word, the reading, and the action. */
  trailing: HTMLElement;
}

const draw = async (props: Partial<HeaterRowProps> = {}): Promise<Drawn> => {
  const { container } = await render(HeaterRow, {
    props: {
      label: 'Tool 1',
      kind: 'tool',
      nozzleTemp: 234.6,
      targetTemp: 235,
      ...props,
    },
  });

  const row = container.querySelector<HTMLElement>('[data-slot="heater-row"]');
  if (row === null) throw new Error('no heater row rendered');
  const trailing = row.lastElementChild;
  if (!(trailing instanceof HTMLElement)) throw new Error('no trailing group');
  const word = trailing.firstElementChild;
  if (!(word instanceof HTMLElement)) throw new Error('no state word');

  return { container, row, word, trailing };
};

/** Which trend arrow the row drew, or `null` when the state has none. */
const arrowOf = (row: HTMLElement): string | null => {
  if (row.querySelector('svg.lucide-arrow-up')) return 'up';
  if (row.querySelector('svg.lucide-arrow-down')) return 'down';
  return null;
};

describe('HeaterRow', () => {
  it('speaks the state it does not draw, and draws the one it must', async () => {
    const heating = await draw({ nozzleTemp: 62, action: editor });

    expect(heating.word.textContent.trim()).toBe('Heating');
    expect(heating.word.classList.contains('sr-only')).toBe(true);
    expect(arrowOf(heating.row)).toBe('up');

    // Off and at-temperature both carry no tint and no arrow, so off is the
    // one state a sighted reader would otherwise have no mark for.
    const off = await draw({
      nozzleTemp: 23,
      targetTemp: undefined,
    });

    expect(off.word.textContent.trim()).toBe('Off');
    expect(off.word.classList.contains('sr-only')).toBe(false);
    expect(arrowOf(off.row)).toBeNull();
  });

  it('gives heating and cooling two glyphs and not one turned over', async () => {
    const up = await draw({ nozzleTemp: 62, action: editor });
    const down = await draw({
      nozzleTemp: 240,
      targetTemp: 170,
      action: editor,
    });

    expect(arrowOf(up.row)).toBe('up');
    expect(arrowOf(down.row)).toBe('down');
    expect(down.word.textContent.trim()).toBe('Cooling');
  });

  it('draws no arrow inside the tolerance, and none with no reading', async () => {
    const settled = await draw({ action: editor });

    expect(arrowOf(settled.row)).toBeNull();
    expect(settled.word.textContent.trim()).toBe('At temperature');

    const silent = await draw({
      nozzleTemp: undefined,
      targetTemp: undefined,
      action: editor,
    });

    expect(arrowOf(silent.row)).toBeNull();
    expect(silent.word.textContent.trim()).toBe('Not reporting');
    expect(silent.trailing.textContent).toContain('—');
  });

  it('carries the target itself only when nothing else can', async () => {
    const alone = await draw({ nozzleTemp: 62 });

    expect(alone.row.getAttribute('title')).toBe('Target 235°C');
    expect(alone.word.textContent.trim()).toBe('Heating, target 235°C');

    const commanded = await draw({ nozzleTemp: 62, action: editor });

    // The action's own accessible name confirms the target, so the row stays
    // quiet and the two can never disagree.
    expect(commanded.row.hasAttribute('title')).toBe(false);
    expect(commanded.word.textContent.trim()).toBe('Heating');
  });

  // A chamber is a heater against its target and off with none, so a warm XL
  // enclosure with no target shows neither cooling nor arrived.
  it('reads a chamber against its target, and as off with none', async () => {
    const chamber = { label: 'Chamber', kind: 'chamber' as const };

    const warming = await draw({ ...chamber, nozzleTemp: 30, targetTemp: 45 });
    expect(warming.word.textContent.trim()).toBe('Heating, target 45°C');
    expect(arrowOf(warming.row)).toBe('up');

    const held = await draw({ ...chamber, nozzleTemp: 50, targetTemp: 45 });
    expect(held.word.textContent.trim()).toBe('Cooling, target 45°C');

    for (const targetTemp of [undefined, 0]) {
      const idle = await draw({ ...chamber, nozzleTemp: 45, targetTemp });

      expect(idle.word.textContent.trim()).toBe('Off');
      expect(idle.row.hasAttribute('title')).toBe(false);
    }
  });

  it('marks the feeding head by weight, and by nothing else', async () => {
    const feeding = await draw({ active: true });
    const idle = await draw();

    const labelOf = (drawn: Drawn): HTMLElement => {
      const first = drawn.row.firstElementChild?.children[1];
      if (!(first instanceof HTMLElement)) throw new Error('no label');
      return first;
    };

    expect(labelOf(feeding).classList.contains('font-semibold')).toBe(true);
    expect(labelOf(idle).classList.contains('font-medium')).toBe(true);
    expect(feeding.row.getAttribute('aria-current')).toBe('true');
    expect(idle.row.hasAttribute('aria-current')).toBe(false);
  });

  it('writes the loaded position on the material line', async () => {
    const { row } = await draw({
      material: 'PETG',
      filamentPositions: [
        { position: 1, loaded: false },
        { position: 2, loaded: true },
        { position: 3, loaded: false },
        { position: 4, loaded: false },
        { position: 5, loaded: false },
      ],
    });

    expect(row.textContent).toContain('PETG · position 2 of 5');
    // No strip of badges. That needs a row to wrap into, and this row is one
    // line tall.
    expect(row.querySelector('ul, ol, [role="list"]')).toBeNull();
  });

  it('leaves out the material line for a heater that holds nothing', async () => {
    const { row } = await draw({
      label: 'Chamber',
      kind: 'chamber',
      nozzleTemp: 45.2,
      targetTemp: 45,
    });

    expect(row.querySelector('svg[data-slot="control-icon"]')).not.toBeNull();
    expect(row.querySelector('.font-mono.truncate')).toBeNull();
  });

  it('gives up the material line before the reading and the trigger', async () => {
    const host = document.createElement('div');
    host.className = 'w-heater-row';
    document.body.append(host);

    await render(HeaterRow, {
      target: host,
      props: {
        label: 'Tool 2',
        kind: 'tool',
        nozzleTemp: 28,
        targetTemp: undefined,
        material: 'A very long material name that must truncate',
        action: editor,
      },
    });

    const row = host.querySelector<HTMLElement>('[data-slot="heater-row"]');
    if (row === null) throw new Error('no heater row rendered');
    const trailing = row.lastElementChild as HTMLElement;

    // A flex row with visible overflow never reports a `scrollWidth` past its
    // own box even while a `shrink-0` child spills out of it, so the check is
    // the trailing group's right edge against the row's.
    expect(trailing.getBoundingClientRect().right).toBeLessThanOrEqual(
      row.getBoundingClientRect().right + 0.5,
    );

    host.remove();
  });

  it('has nothing axe objects to', async () => {
    const { container } = await draw({ active: true, action: editor });

    await expectNoViolations(container);
  });
});

describe('HeaterRows', () => {
  const rows = createRawSnippet(() => ({ render: () => '<div>a</div>' }));

  it('stacks a lone row and flows two into columns', async () => {
    const one = await render(HeaterRows, {
      props: { rows: 1, children: rows },
    });
    const grid = one.container.querySelector('[data-slot="heater-rows"]');
    expect(grid?.classList.contains('grid-cols-1')).toBe(true);

    const many = await render(HeaterRows, {
      props: { rows: 2, children: rows },
    });
    const flowing = many.container.querySelector('[data-slot="heater-rows"]');
    expect(flowing?.classList.contains('grid-cols-1')).toBe(false);
    // No track is ever narrower than a row needs, and an empty one collapses.
    expect(getComputedStyle(flowing!).gridTemplateColumns).not.toBe('');
  });
});
