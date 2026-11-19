import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page as browser } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import PageHeader from './page-header.svelte';

describe('PageHeader', () => {
  it('names the page as the one heading it is', async () => {
    await render(PageHeader, { props: { title: 'Printers' } });

    await expect
      .element(browser.getByRole('heading', { level: 1 }))
      .toHaveTextContent('Printers');
  });

  it('draws the one action a screen usually has', async () => {
    let pressed = 0;
    await render(PageHeader, {
      props: {
        title: 'Printers',
        action: {
          label: 'Add printer',
          onclick: () => {
            pressed += 1;
          },
        },
      },
    });

    await browser.getByRole('button', { name: 'Add printer' }).click();

    expect(pressed).toBe(1);
  });

  it('draws no action region when a screen gives none', async () => {
    await render(PageHeader, { props: { title: 'Printers' } });

    expect(document.querySelector('[data-slot="page-header-actions"]')).toBe(
      null,
    );
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(PageHeader, {
      props: {
        title: 'Printers',
        action: { label: 'Add printer', onclick: () => undefined },
      },
    });

    await expectNoViolations(container);
  });
});

describe('PageHeader actions row', () => {
  it('keeps the height of one button so a late action moves nothing', async () => {
    const rowHeight = (): number =>
      document
        .querySelector<HTMLElement>('[data-slot="page-header-actions"]')!
        .getBoundingClientRect().height;

    const waiting = await render(PageHeader, {
      props: {
        title: 'Materials',
        actions: createRawSnippet(() => ({ render: () => '<span></span>' })),
      },
    });
    const empty = rowHeight();
    await waiting.unmount();

    await render(PageHeader, {
      props: {
        title: 'Materials',
        action: { label: 'Add material', onclick: () => undefined },
      },
    });

    expect(rowHeight()).toBeCloseTo(empty, 1);
  });
});
