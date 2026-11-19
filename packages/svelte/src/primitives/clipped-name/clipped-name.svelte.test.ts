import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ClippedName from './clipped-name.svelte';
import type { ClippedNameProps } from './types';

const LONG = 'Bracket_Left_Reinforced_v4_FINAL.gcode';
const SIBLING = 'Bracket_Left_Reinforced_v6_verified.gcode';

const hosts: HTMLElement[] = [];

/**
 * The name in a box too narrow for it, which is the only place the clipping
 * happens. A name in an unbounded box never overflows and never clips.
 */
const drawIn = async (
  width: string,
  props: ClippedNameProps,
): Promise<HTMLElement> => {
  const host = document.createElement('div');
  host.style.inlineSize = width;
  host.style.fontFamily = 'monospace';
  document.body.append(host);
  hosts.push(host);

  await render(ClippedName, { target: host, props });

  const name = host.querySelector<HTMLElement>('[data-slot="clipped-name"]');
  if (name === null) throw new Error('no name rendered');
  return name;
};

afterEach(() => {
  for (const host of hosts.splice(0)) host.remove();
});

describe('ClippedName', () => {
  it('keeps the tail whole and clips the head', async () => {
    const name = await drawIn('120px', { name: LONG });
    const [head, tail] = [...name.children] as HTMLElement[];

    expect(head?.textContent).toBe('Bracket_Left_Reinforced_v4');
    expect(tail?.textContent).toBe('_FINAL.gcode');

    // The head is the part that gave way: its text is wider than the box it
    // was left with. The tail was never asked to shrink.
    expect(head!.scrollWidth).toBeGreaterThan(head!.clientWidth);
    expect(tail!.scrollWidth).toBe(tail!.clientWidth);
    expect(tail!.getBoundingClientRect().width).toBeGreaterThan(0);
  });

  it('offers the whole name to a pointer resting anywhere on it', async () => {
    const name = await drawIn('120px', { name: LONG });

    expect(name.getAttribute('title')).toBe(LONG);
    expect(name.textContent).toBe(LONG);
  });

  it('tells two names apart that share everything but their tail', async () => {
    const first = await drawIn('120px', { name: LONG });
    const second = await drawIn('120px', { name: SIBLING });

    const tailOf = (element: Element): string | null =>
      element.lastElementChild?.textContent ?? null;

    expect(tailOf(first)).toBe('_FINAL.gcode');
    expect(tailOf(second)).toBe('rified.gcode');
    expect(tailOf(first)).not.toBe(tailOf(second));
  });

  it('leaves a name shorter than its own tail alone', async () => {
    const name = await drawIn('120px', { name: 'benchy.3mf' });

    expect(name.children).toHaveLength(0);
    expect(name.textContent).toBe('benchy.3mf');
    // Nothing is hidden, so there is nothing a title would add.
    expect(name.getAttribute('title')).toBeNull();
  });

  it('keeps as many characters as the caller asks for', async () => {
    const name = await drawIn('120px', { name: LONG, tailLength: 22 });

    expect(name.lastElementChild?.textContent).toBe('nforced_v4_FINAL.gcode');
    expect(name.lastElementChild?.textContent).toHaveLength(22);
    expect(name.textContent).toBe(LONG);
  });

  // A flex row or an auto table column sizes the name by its minimum. Below
  // the tail the name would be clipped to nothing and drawn over its
  // neighbour, so the tail is that minimum.
  it('never gives up its tail in a box narrower than the tail', async () => {
    const name = await drawIn('10px', { name: LONG });
    name.parentElement!.style.display = 'flex';
    const [head, tail] = [...name.children] as HTMLElement[];

    // The head keeps room for its ellipsis, so the cut stays visible.
    expect(head!.getBoundingClientRect().width).toBeGreaterThan(0);
    expect(head!.scrollWidth).toBeGreaterThan(head!.clientWidth);
    expect(name.getBoundingClientRect().width).toBeGreaterThanOrEqual(
      head!.getBoundingClientRect().width + tail!.scrollWidth,
    );
    expect(tail!.scrollWidth).toBe(tail!.clientWidth);
  });

  // A head narrower than a character and the ellipsis clips the ellipsis,
  // and the character left reads as the start of a shorter name.
  it('ends a fully squeezed head in its ellipsis', async () => {
    const name = await drawIn('10px', { name: LONG });
    name.parentElement!.style.display = 'flex';
    const head = name.firstElementChild as HTMLElement;

    const probe = document.createElement('span');
    probe.style.font = getComputedStyle(head).font;
    probe.style.whiteSpace = 'pre';
    probe.textContent = `${LONG[0] ?? ''}\u2026`;
    name.parentElement!.append(probe);
    const needed = probe.getBoundingClientRect().width;
    probe.remove();

    expect(head.getBoundingClientRect().width).toBeGreaterThanOrEqual(needed);
  });

  it('keeps a name whole whose head would be three characters or fewer', async () => {
    const name = await drawIn('120px', { name: 'v2tangle.bgcode' });

    expect(name.children).toHaveLength(0);
    expect(name.textContent).toBe('v2tangle.bgcode');
  });

  it('draws head and tail touching in a box wider than the name', async () => {
    const name = await drawIn('2000px', { name: LONG });
    const [head, tail] = [...name.children] as HTMLElement[];

    expect(tail!.getBoundingClientRect().left).toBe(
      head!.getBoundingClientRect().right,
    );
  });

  it('has nothing axe objects to', async () => {
    const name = await drawIn('120px', { name: LONG });

    await expectNoViolations(name);
  });
});
