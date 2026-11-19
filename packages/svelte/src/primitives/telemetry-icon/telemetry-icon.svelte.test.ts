import type { Component } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import HeatbedIcon from './heatbed-icon.svelte';
import NozzleIcon from './nozzle-icon.svelte';
import SpoolIcon from './spool-icon.svelte';
import type { TelemetryIconProps } from './types';

type Glyph = Component<TelemetryIconProps>;

const glyphs: [string, Glyph][] = [
  ['HeatbedIcon', HeatbedIcon],
  ['NozzleIcon', NozzleIcon],
  ['SpoolIcon', SpoolIcon],
];

const hosts: HTMLElement[] = [];

/** Draws one glyph in a box of its own and hands back the `<svg>`. */
const draw = async (
  glyph: Glyph,
  props: TelemetryIconProps = {},
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

describe('the telemetry glyphs', () => {
  it('draws every glyph under one slot, from paths of its own', async () => {
    for (const [name, glyph] of glyphs) {
      const svg = await draw(glyph);

      expect(svg.getAttribute('data-slot'), name).toBe('telemetry-icon');
      expect(svg.getAttribute('viewBox'), name).not.toBeNull();
      // An empty <svg> renders a zero-area box and looks like a gap, not a
      // fault, so a dropped path would go unnoticed.
      expect(svg.querySelectorAll('path').length, name).toBeGreaterThan(0);
      expect([...svg.classList].sort(), name).toEqual(['shrink-0', 'size-4']);
    }
  });

  it('paints in the colour of whatever it sits in', async () => {
    const svg = await draw(NozzleIcon);
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');
    host.style.color = 'rgb(220, 38, 38)';

    const painted = svg.querySelector('path');
    if (painted === null) throw new Error('the glyph draws no path');

    expect(getComputedStyle(painted).fill).toBe('rgb(220, 38, 38)');
    expect(getComputedStyle(painted).fill).toBe(getComputedStyle(svg).color);
  });

  it('is square, and gives way to a size the caller names', async () => {
    const small = (await draw(SpoolIcon)).getBoundingClientRect();
    expect(small.height).toBeCloseTo(small.width, 1);

    // Nothing merges the two size utilities. The caller's wins because
    // Tailwind emits size-4 before size-8.
    const large = await draw(SpoolIcon, { class: 'size-8' });
    const named = document.createElement('div');
    named.className = 'size-8';
    document.body.append(named);
    const width = named.getBoundingClientRect().width;
    named.remove();
    expect(large.getBoundingClientRect().width).toBeCloseTo(width, 1);
  });

  it('keeps its size beside a reading that fills the row', async () => {
    const svg = await draw(HeatbedIcon);
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');
    const width = svg.getBoundingClientRect().width;
    host.style.display = 'flex';
    host.style.inlineSize = '24px';

    const reading = document.createElement('div');
    reading.style.inlineSize = '200px';
    host.append(reading);

    expect(svg.getBoundingClientRect().width).toBeCloseTo(width, 1);
  });

  it('takes the attributes its caller sets', async () => {
    const svg = await draw(NozzleIcon, {
      'aria-hidden': 'true',
      'data-testid': 'nozzle',
      role: 'img',
    });

    expect(svg.getAttribute('aria-hidden')).toBe('true');
    expect(svg.getAttribute('data-testid')).toBe('nozzle');
    expect(svg.getAttribute('role')).toBe('img');
  });

  it('has nothing axe objects to', async () => {
    const svg = await draw(NozzleIcon, { 'aria-hidden': 'true' });
    const host = svg.parentElement;
    if (host === null) throw new Error('the glyph has no box');

    await expectNoViolations(host);
  });
});
