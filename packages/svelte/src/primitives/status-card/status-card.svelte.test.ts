import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import StatusCard from './status-card.svelte';
import type { StatusTone } from './types';

const snippet = (html: string) =>
  createRawSnippet(() => ({ render: () => html }));

const dot = (): HTMLElement => {
  const found = document.querySelector<HTMLElement>('[data-slot="status-dot"]');
  if (found === null) throw new Error('no dot rendered');
  return found;
};

describe('StatusCard', () => {
  it('draws the title, the reading and the line under it', async () => {
    await render(StatusCard, {
      props: {
        title: 'Nozzle',
        value: '215 °C',
        caption: 'Target 215 °C',
      },
    });

    await expect
      .element(page.getByRole('heading', { name: 'Nozzle' }))
      .toBeVisible();
    await expect.element(page.getByText('215 °C')).toBeVisible();
    await expect.element(page.getByText('Target 215 °C')).toBeVisible();
  });

  // Two tones may be painted alike (idle draws as quiet), so each
  // tone is checked for its own word and a painted dot, not a colour of its
  // own.
  it('gives each tone its own word and a painted dot', async () => {
    const tones = [
      'active',
      'done',
      'idle',
      'attention',
      'fault',
      'quiet',
    ] satisfies StatusTone[];
    for (const tone of tones) {
      const rendered = await render(StatusCard, {
        props: { title: 'Fractal', tone },
      });
      const card = document.querySelector<HTMLElement>(
        '[data-slot="status-card"]',
      );
      expect(card?.dataset.tone).toBe(tone);
      expect(getComputedStyle(dot()).backgroundColor).not.toBe(
        'rgba(0, 0, 0, 0)',
      );
      await rendered.unmount();
    }
  });

  it('reads the state out, because the dot alone says nothing', async () => {
    await render(StatusCard, {
      props: { title: 'Fractal', tone: 'fault', toneLabel: 'Not reachable' },
    });

    expect(dot().getAttribute('aria-hidden')).toBe('true');
    await expect
      .element(page.getByRole('heading', { name: 'Fractal' }))
      .toBeVisible();
    expect(document.body.textContent).toContain('Not reachable');
  });

  it('draws the arrow the trend names, and says the trend in words', async () => {
    const up = await render(StatusCard, {
      props: {
        title: 'Prints',
        trend: 'up',
        trendLabel: '3 more than yesterday',
      },
    });
    const rising = document.querySelector<HTMLElement>(
      '[data-slot="status-trend"]',
    );
    expect(rising?.dataset.variant).toBe('up');
    await expect.element(page.getByText('3 more than yesterday')).toBeVisible();
    await up.unmount();

    await render(StatusCard, {
      props: { title: 'Prints', trend: 'down', trendLabel: '1 less' },
    });
    const falling = document.querySelector<HTMLElement>(
      '[data-slot="status-trend"]',
    );
    expect(falling?.dataset.variant).toBe('down');
  });

  it('draws no footnote when there is neither a trend nor a time', async () => {
    await render(StatusCard, { props: { title: 'Fractal' } });

    expect(document.querySelector('[data-slot="status-footnote"]')).toBeNull();
    expect(document.querySelector('[data-slot="status-value"]')).toBeNull();
  });

  it('takes a badge, a body and one action from the caller', async () => {
    await render(StatusCard, {
      props: {
        title: 'Fractal',
        badge: snippet('<span>On</span>'),
        children: snippet('<p>Two peers</p>'),
        action: snippet('<button type="button">Open</button>'),
      },
    });

    expect(document.querySelector('[data-slot="status-badge"]')).not.toBeNull();
    await expect.element(page.getByText('Two peers')).toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Open' }))
      .toBeVisible();
  });
});

describe('StatusCard figure and heading', () => {
  it('draws a quiet reading muted and a loud one in the foreground', async () => {
    await render(StatusCard, {
      props: { title: 'Idle', tone: 'quiet', value: '0' },
    });
    await render(StatusCard, {
      props: { title: 'Busy', tone: 'active', value: '3' },
    });
    const [quiet, active] = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="status-value"]'),
    ].map((value) => getComputedStyle(value).color);
    expect(quiet).not.toBe(active);
  });

  it('takes the heading level it is given', async () => {
    await render(StatusCard, { props: { title: 'Idle', headingLevel: 'h3' } });
    expect(document.querySelector('h3')?.textContent.trim()).toBe('Idle');
  });

  it('scales a fluid figure with the card', async () => {
    await render(StatusCard, {
      props: { title: 'Idle', value: '3', figure: 'fluid' },
    });
    const value = document.querySelector<HTMLElement>('[data-figure="fluid"]')!;
    expect(getComputedStyle(value).lineHeight).not.toBe('normal');
    expect(value.className).not.toContain('text-display');
  });
});
