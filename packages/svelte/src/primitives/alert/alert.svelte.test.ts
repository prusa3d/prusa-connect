import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import AlertProbe from '../../test/alert-probe.svelte';
import { expectNoViolations } from '../../test/axe';
import type { AlertVariant } from './types';
import { variants } from './variants';

const VARIANTS: AlertVariant[] = [
  'default',
  'destructive',
  'success',
  'warning',
  'info',
];

/** The width of the first grid column, which is the icon's. */
const iconColumn = (alert: Element): number =>
  Number.parseFloat(getComputedStyle(alert).gridTemplateColumns.split(' ')[0]!);

describe('Alert', () => {
  // A screen reader reads a live region as soon as it appears. That is the
  // whole point of the role, and losing it makes the message silent.
  it('is a live region a screen reader reads unprompted', async () => {
    await render(AlertProbe, {
      props: {
        variant: 'destructive',
        title: 'Nozzle clog detected',
        description: 'The print has been paused.',
      },
    });

    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('Nozzle clog detected The print has been paused.');
  });

  it('gives each variant its own attribute and its own colours', async () => {
    const looks = new Set<string>();

    for (const variant of VARIANTS) {
      const { container, unmount } = await render(AlertProbe, {
        props: { variant, title: variant },
      });
      const alert = container.querySelector('[data-slot="alert"]')!;
      expect(alert.getAttribute('data-variant')).toBe(variant);

      const style = getComputedStyle(alert);
      looks.add(`${style.borderTopColor}/${style.color}`);
      await unmount();
    }

    // One rendering per variant. An entry that falls back to another's
    // utilities collapses this set.
    expect(looks.size).toBe(VARIANTS.length);
  });

  it('names every variant the map declares', () => {
    expect([...VARIANTS].sort()).toEqual(Object.keys(variants).sort());
  });

  // The icon column is zero wide until an icon is there, so a caller adds one
  // by writing it and nothing else.
  it('opens the icon column for a direct svg child alone', async () => {
    const plain = await render(AlertProbe, {
      props: { title: 'Print complete' },
    });
    expect(
      iconColumn(plain.container.querySelector('[data-slot="alert"]')!),
    ).toBe(0);
    await plain.unmount();

    const withIcon = await render(AlertProbe, {
      props: { title: 'Print complete', icon: true },
    });
    expect(
      iconColumn(withIcon.container.querySelector('[data-slot="alert"]')!),
    ).toBeGreaterThan(0);
  });

  // The description names no colour of its own, so a status alert reads in one
  // tone from title to last line.
  it('paints the description in the alert colour', async () => {
    const { container } = await render(AlertProbe, {
      props: {
        variant: 'success',
        title: 'Print complete',
        description: 'benchy.3mf finished after 47 minutes.',
      },
    });

    const alert = container.querySelector('[data-slot="alert"]')!;
    const description = container.querySelector(
      '[data-slot="alert-description"]',
    )!;
    expect(getComputedStyle(description).color).toBe(
      getComputedStyle(alert).color,
    );
  });

  it('has no accessibility violation', async () => {
    const { container } = await render(AlertProbe, {
      props: {
        variant: 'warning',
        title: 'Filament running low',
        description: 'Less than 2 meters remain on the active spool.',
        icon: true,
      },
    });

    await expectNoViolations(container);
  });
});
