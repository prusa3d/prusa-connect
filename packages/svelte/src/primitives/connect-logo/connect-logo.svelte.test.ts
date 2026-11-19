import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ConnectLogo from './connect-logo.svelte';
import type { ConnectLogoProps } from './types';

const hosts: HTMLElement[] = [];

/**
 * The logo inside a column wider than it, on a black ground of text, which is
 * where a stretched width or a hard-coded colour would show.
 */
const drawIn = async (props: ConnectLogoProps = {}): Promise<SVGSVGElement> => {
  const host = document.createElement('div');
  host.style.inlineSize = '800px';
  host.style.display = 'flex';
  host.style.flexDirection = 'column';
  host.style.alignItems = 'flex-start';
  host.style.color = 'rgb(0, 0, 0)';
  document.body.append(host);
  hosts.push(host);

  await render(ConnectLogo, { target: host, props });

  const logo = host.querySelector<SVGSVGElement>('[data-slot="connect-logo"]');
  if (logo === null) throw new Error('no logo rendered');
  return logo;
};

const paths = (logo: SVGSVGElement): SVGPathElement[] => [
  ...logo.querySelectorAll('path'),
];

const ratio = (logo: SVGSVGElement): number => {
  const box = logo.getBoundingClientRect();
  return box.width / box.height;
};

afterEach(() => {
  for (const host of hosts.splice(0)) host.remove();
});

describe('ConnectLogo', () => {
  it('is a picture a screen reader names Connect', async () => {
    const logo = await drawIn();

    expect(logo.getAttribute('role')).toBe('img');
    expect(logo.getAttribute('aria-label')).toBe('Connect');
    expect(logo.hasAttribute('aria-hidden')).toBe(false);
  });

  it('takes the name its caller gives it', async () => {
    const logo = await drawIn({ label: 'Prusa Connect' });

    expect(logo.getAttribute('aria-label')).toBe('Prusa Connect');
  });

  it('is decoration when the name is empty', async () => {
    const logo = await drawIn({ label: '' });

    expect(logo.getAttribute('aria-hidden')).toBe('true');
    expect(logo.hasAttribute('role')).toBe(false);
    expect(logo.hasAttribute('aria-label')).toBe(false);
  });

  it('draws one path in every shape', async () => {
    expect(paths(await drawIn())).toHaveLength(1);
    expect(paths(await drawIn({ shape: 'full' }))).toHaveLength(1);
    expect(paths(await drawIn({ shape: 'mark' }))).toHaveLength(1);
  });

  it('draws the PRUSA wordmark unless a shape is named', async () => {
    const [word] = paths(await drawIn());
    const [prusa] = paths(await drawIn({ shape: 'prusa' }));

    expect(word!.getAttribute('d')).toBe(prusa!.getAttribute('d'));
  });

  it('keeps each shape in its own proportions and not the column’s', async () => {
    // Only the height is set; the width comes from the viewBox. A logo that
    // took the column's 800 pixels would be stretched or letterboxed.
    expect(ratio(await drawIn())).toBeCloseTo(69.87 / 16.32, 1);
    expect(ratio(await drawIn({ shape: 'full' }))).toBeCloseTo(124 / 25, 1);
    expect(ratio(await drawIn({ shape: 'mark' }))).toBeCloseTo(1, 2);
  });

  it('paints the words in their ground’s colour and the mark in the logo orange', async () => {
    const [prusa] = paths(await drawIn());
    const [word] = paths(await drawIn({ shape: 'full' }));
    const [mark] = paths(await drawIn({ shape: 'mark' }));

    const probe = document.createElement('span');
    probe.style.color = 'var(--color-logo)';
    document.body.append(probe);
    const orange = getComputedStyle(probe).color;
    probe.remove();

    expect(getComputedStyle(prusa!).fill).toBe('rgb(0, 0, 0)');
    expect(getComputedStyle(word!).fill).toBe('rgb(0, 0, 0)');
    expect(getComputedStyle(mark!).fill).toBe(orange);
    expect(orange).not.toBe('rgb(0, 0, 0)');
  });

  it('grows with its size', async () => {
    const heights = [];
    for (const size of ['xs', 'sm', 'md', 'lg'] as const) {
      heights.push((await drawIn({ size })).getBoundingClientRect().height);
    }
    const [tiny, small, medium, large] = heights as [
      number,
      number,
      number,
      number,
    ];

    expect(tiny).toBeGreaterThan(0);
    expect(tiny).toBeLessThan(small);
    expect(small).toBeLessThan(medium);
    expect(medium).toBeLessThan(large);
  });

  it('has nothing axe objects to', async () => {
    await expectNoViolations(await drawIn());
    await expectNoViolations(await drawIn({ shape: 'mark', label: '' }));
  });
});
