import { toast } from 'svelte-sonner';
import { afterEach, describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { t } from '../../i18n';
import { expectNoViolations } from '../../test/axe';
import Toaster from './sonner.svelte';

const stack = (): HTMLElement => {
  const found = document.querySelector<HTMLElement>('[data-sonner-toaster]');
  if (found === null) throw new Error('no toast stack rendered');
  return found;
};

const toastFor = (words: string): HTMLElement => {
  const found = [
    ...document.querySelectorAll<HTMLElement>('[data-sonner-toast]'),
  ].find((item) => item.textContent.includes(words));
  if (found === undefined) throw new Error(`no toast saying ${words}`);
  return found;
};

const buttonIn = (toast: HTMLElement): HTMLButtonElement => {
  const found = toast.querySelector('button');
  if (found === null) throw new Error('the toast has no close button');
  return found;
};

const glyph = (kind: string): string | null => {
  const svg = document.querySelector(
    `[data-sonner-toast][data-type="${kind}"] [data-icon] svg`,
  );
  return svg === null ? null : svg.getAttribute('class');
};

/**
 * Counts the observers that are watching, so an effect that forgets to
 * disconnect one shows up as a number that never comes back down.
 */
const watchObservers = () => {
  const Native = window.MutationObserver;
  let live = 0;

  window.MutationObserver = class extends Native {
    #watching = false;

    override observe(target: Node, options?: MutationObserverInit): void {
      if (!this.#watching) {
        this.#watching = true;
        live += 1;
      }
      super.observe(target, options);
    }

    override disconnect(): void {
      if (this.#watching) {
        this.#watching = false;
        live -= 1;
      }
      super.disconnect();
    }
  };

  return {
    get live(): number {
      return live;
    },
    restore(): void {
      window.MutationObserver = Native;
    },
  };
};

afterEach(async () => {
  // The queue is a module-level store every mounted toaster reads, so a toast
  // left over from one test is on screen in the next.
  toast.dismiss();
  document.documentElement.classList.remove('dark');
  await page.viewport(1280, 800);
});

describe('Toaster', () => {
  it('shows a toast a page raises, and reads it out', async () => {
    await render(Toaster, {});

    toast('Print queued');

    await expect.element(page.getByText('Print queued')).toBeVisible();
    const region = stack().closest('section');
    expect(region?.getAttribute('aria-live')).toBe('polite');
    // svelte-sonner names the stack in English of its own unless it is told
    // otherwise, and the name reaches a screen reader.
    expect(region?.getAttribute('aria-label')).toContain(
      t('primitives.toaster.label'),
    );
  });

  it('gives each kind of toast its own glyph', async () => {
    await render(Toaster, {});

    toast.success('Print finished');
    toast.error('Print failed');
    toast.warning('Bed cooling');
    toast.info('Filament low');
    toast.loading('Slicing');
    await expect.element(page.getByText('Slicing')).toBeVisible();

    expect(glyph('success')).toContain('lucide-circle-check');
    expect(glyph('error')).toContain('lucide-octagon-x');
    expect(glyph('warning')).toContain('lucide-triangle-alert');
    expect(glyph('info')).toContain('lucide-info');
    expect(glyph('loading')).toContain('lucide-loader-circle');

    // Every glyph is the same size, and the waiting one is the Spinner,
    // which turns.
    for (const kind of ['success', 'error', 'warning', 'info', 'loading']) {
      expect(glyph(kind)).toContain('size-4');
    }
    expect(
      document.querySelector(
        '[data-sonner-toast][data-type="loading"] [data-icon] [data-slot="spinner"]',
      ),
    ).not.toBeNull();
  });

  it('starts in the theme the root element is in', async () => {
    document.documentElement.classList.add('dark');

    await render(Toaster, {});

    expect(stack().getAttribute('data-sonner-theme')).toBe('dark');
  });

  it('follows the root element when the theme changes under it', async () => {
    await render(Toaster, {});
    expect(stack().getAttribute('data-sonner-theme')).toBe('light');

    document.documentElement.classList.add('dark');
    await expect
      .poll(() => stack().getAttribute('data-sonner-theme'))
      .toBe('dark');

    document.documentElement.classList.remove('dark');
    await expect
      .poll(() => stack().getAttribute('data-sonner-theme'))
      .toBe('light');
  });

  it('stops watching the root element when it goes away', async () => {
    const observers = watchObservers();
    const baseline = observers.live;

    try {
      for (let round = 0; round < 5; round += 1) {
        const mounted = await render(Toaster, {});
        expect(observers.live).toBe(baseline + 1);
        await mounted.unmount();
        expect(observers.live).toBe(baseline);
      }
    } finally {
      observers.restore();
    }
  });

  it('paints the toast from this theme rather than from sonner defaults', async () => {
    await render(Toaster, {});

    toast('Print queued');
    await expect.element(page.getByText('Print queued')).toBeVisible();

    const root = getComputedStyle(document.documentElement);
    const surface = getComputedStyle(stack());
    expect(surface.getPropertyValue('--normal-bg')).toBe(
      root.getPropertyValue('--popover'),
    );
    expect(surface.getPropertyValue('--normal-text')).toBe(
      root.getPropertyValue('--popover-foreground'),
    );
    expect(surface.getPropertyValue('--normal-border')).toBe(
      root.getPropertyValue('--border'),
    );

    // sonner's own stylesheet rounds a toast at 8px. The theme's small corner
    // is 2px, and the toast is what actually gets drawn.
    expect(getComputedStyle(toastFor('Print queued')).borderRadius).toBe(
      root.getPropertyValue('--radius-sm'),
    );
  });

  it('fits a 320px screen', async () => {
    await page.viewport(320, 640);
    await render(Toaster, {});

    toast('Print queued');
    await expect.element(page.getByText('Print queued')).toBeVisible();

    const box = toastFor('Print queued').getBoundingClientRect();
    expect(box.left).toBeGreaterThanOrEqual(0);
    expect(box.right).toBeLessThanOrEqual(320);
  });

  it('closes a toast from its close button', async () => {
    await render(Toaster, { props: { closeButton: true } });

    toast.success('Nozzle calibrated');
    await expect.element(page.getByText('Nozzle calibrated')).toBeVisible();

    // A toast dismissed in an earlier test can still be on screen, because the
    // queue is shared, so the button is taken from the toast that holds the
    // words rather than from the page.
    const close = buttonIn(toastFor('Nozzle calibrated'));
    expect(close.getAttribute('aria-label')).toBe(
      t('primitives.toaster.close'),
    );
    await userEvent.click(close);

    await expect
      .poll(() => document.body.textContent.includes('Nozzle calibrated'))
      .toBe(false);
  });

  it('is reachable from the keyboard and closes from there', async () => {
    await render(Toaster, { props: { closeButton: true } });

    toast.success('Nozzle calibrated');
    await expect.element(page.getByText('Nozzle calibrated')).toBeVisible();

    // A toast is not in the tab order until sonner's own shortcut, alt+T,
    // moves focus into the stack. Without it a person on a keyboard would have
    // to tab through the whole page to reach a message that closes by itself.
    await userEvent.keyboard('{Alt>}t{/Alt}');
    expect(stack().contains(document.activeElement)).toBe(true);

    // From the stack, one tab lands on the toast and the next on its close
    // button.
    await userEvent.tab();
    expect(document.activeElement).toBe(toastFor('Nozzle calibrated'));

    await userEvent.tab();
    expect(document.activeElement).toBe(
      buttonIn(toastFor('Nozzle calibrated')),
    );

    await userEvent.keyboard('{Enter}');
    await expect
      .poll(() => document.body.textContent.includes('Nozzle calibrated'))
      .toBe(false);
  });

  it('has no accessibility violation', async () => {
    const { container } = await render(Toaster, {});

    toast.success('Print finished');
    await expect.element(page.getByText('Print finished')).toBeVisible();

    await expectNoViolations(container);
  });
});
