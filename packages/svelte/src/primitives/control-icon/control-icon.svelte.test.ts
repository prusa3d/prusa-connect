import type { Component } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ArrowDownIcon from './arrow-down-icon.svelte';
import ArrowLeftIcon from './arrow-left-icon.svelte';
import ArrowRightIcon from './arrow-right-icon.svelte';
import ArrowUpIcon from './arrow-up-icon.svelte';
import ChamberIcon from './chamber-icon.svelte';
import DisableSteppersIcon from './disable-steppers-icon.svelte';
import HomeIcon from './home-icon.svelte';
import MeshBedLevelingIcon from './mesh-bed-leveling-icon.svelte';
import RestartIcon from './restart-icon.svelte';
import SpeedIcon from './speed-icon.svelte';
import TemperatureIcon from './temperature-icon.svelte';
import type { ControlIconProps } from './types';
import ZAxisIcon from './z-axis-icon.svelte';

type Glyph = Component<ControlIconProps>;

const glyphs: [string, Glyph][] = [
  ['ArrowDownIcon', ArrowDownIcon],
  ['ArrowLeftIcon', ArrowLeftIcon],
  ['ArrowRightIcon', ArrowRightIcon],
  ['ArrowUpIcon', ArrowUpIcon],
  ['ChamberIcon', ChamberIcon],
  ['DisableSteppersIcon', DisableSteppersIcon],
  ['HomeIcon', HomeIcon],
  ['MeshBedLevelingIcon', MeshBedLevelingIcon],
  ['RestartIcon', RestartIcon],
  ['SpeedIcon', SpeedIcon],
  ['TemperatureIcon', TemperatureIcon],
  ['ZAxisIcon', ZAxisIcon],
];

const hosts: HTMLElement[] = [];

/** Draws one glyph in a box of its own and hands back the `<svg>`. */
const draw = async (
  glyph: Glyph,
  props: ControlIconProps = {},
): Promise<SVGSVGElement> => {
  const host = document.createElement('div');
  document.body.append(host);
  hosts.push(host);

  await render(glyph, { target: host, props });

  const svg = host.querySelector('svg');
  if (svg === null) throw new Error('no glyph rendered');
  return svg;
};

afterEach(() => {
  for (const host of hosts.splice(0)) host.remove();
});

describe('the control glyphs', () => {
  it('draws every glyph under one slot, from paths of its own', async () => {
    for (const [name, glyph] of glyphs) {
      const svg = await draw(glyph);

      expect(svg.getAttribute('data-slot'), name).toBe('control-icon');
      expect(svg.getAttribute('viewBox'), name).not.toBeNull();
      // An empty <svg> renders a zero-area box and looks like a gap, not a
      // fault, so a dropped path would go unnoticed.
      expect(svg.querySelectorAll('path').length, name).toBeGreaterThan(0);
      expect([...svg.classList].sort(), name).toEqual(['shrink-0', 'size-4']);
    }
  });

  it('paints in the colour of whatever it sits in', async () => {
    const svg = await draw(HomeIcon);
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');
    host.style.color = 'rgb(220, 38, 38)';

    const painted = svg.querySelector('path');
    if (painted === null) throw new Error('the glyph draws no path');

    expect(getComputedStyle(painted).fill).toBe('rgb(220, 38, 38)');
    expect(getComputedStyle(painted).fill).toBe(getComputedStyle(svg).color);
  });

  it('is square, and gives way to a size the caller names', async () => {
    const small = (await draw(HomeIcon)).getBoundingClientRect();
    expect(small.height).toBeCloseTo(small.width, 1);

    // Nothing merges the two size utilities. The caller's wins because
    // Tailwind emits size-4 before size-8.
    const large = await draw(HomeIcon, { class: 'size-8' });
    const named = document.createElement('div');
    named.className = 'size-8';
    document.body.append(named);
    const width = named.getBoundingClientRect().width;
    named.remove();
    expect(large.getBoundingClientRect().width).toBeCloseTo(width, 1);
  });

  it('keeps its size in a row that has run out of width', async () => {
    const svg = await draw(HomeIcon);
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');
    const width = svg.getBoundingClientRect().width;
    host.style.display = 'flex';
    host.style.inlineSize = '24px';

    const label = document.createElement('div');
    label.style.inlineSize = '200px';
    host.append(label);

    expect(svg.getBoundingClientRect().width).toBeCloseTo(width, 1);
  });

  it('takes the attributes its caller sets', async () => {
    const svg = await draw(HomeIcon, {
      'aria-hidden': 'true',
      'data-testid': 'jog-home',
      role: 'img',
    });

    expect(svg.getAttribute('aria-hidden')).toBe('true');
    expect(svg.getAttribute('data-testid')).toBe('jog-home');
    expect(svg.getAttribute('role')).toBe('img');
  });

  it('has nothing axe objects to', async () => {
    const svg = await draw(HomeIcon, { 'aria-hidden': 'true' });
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');

    await expectNoViolations(host);
  });
});
