import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Slider from './slider.svelte';

describe('Slider', () => {
  it('moves by one step per arrow key and reports the value', async () => {
    const onValueChange = vi.fn();
    await render(Slider, {
      props: {
        type: 'single',
        value: 50,
        onValueChange,
        'aria-label': 'Volume',
      },
    });

    const thumb = page.getByRole('slider', { name: 'Volume' });
    thumb.element().focus();
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '50');

    await userEvent.keyboard('{ArrowRight}');
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '51');
    expect(onValueChange).toHaveBeenLastCalledWith(51);

    await userEvent.keyboard('{ArrowLeft}{ArrowLeft}');
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '49');
  });

  it('reaches the bounds on Home and End', async () => {
    await render(Slider, {
      props: { type: 'single', value: 50, min: 0, max: 100 },
    });

    const thumb = page.getByRole('slider');
    thumb.element().focus();
    await expect.element(thumb).toHaveAttribute('aria-valuemax', '100');

    await userEvent.keyboard('{End}');
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '100');

    await userEvent.keyboard('{Home}');
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '0');
  });

  it('draws one thumb per value', async () => {
    const { container } = await render(Slider, {
      props: { type: 'multiple', value: [25, 75], 'aria-label': 'Price range' },
    });

    const thumbs = container.querySelectorAll('[data-slot="slider-thumb"]');
    expect(thumbs.length).toBe(2);
    expect(thumbs[0]?.getAttribute('aria-valuenow')).toBe('25');
    expect(thumbs[1]?.getAttribute('aria-valuenow')).toBe('75');
  });

  it('blocks the callback while disabled', async () => {
    const onValueChange = vi.fn();
    await render(Slider, {
      props: {
        type: 'single',
        value: 50,
        disabled: true,
        onValueChange,
        'aria-label': 'Volume',
      },
    });

    const thumb = page.getByRole('slider');
    thumb.element().focus();
    await userEvent.keyboard('{ArrowRight}');

    expect(onValueChange).not.toHaveBeenCalled();
    await expect.element(thumb).toHaveAttribute('aria-valuenow', '50');
  });

  // `border-transparent` on a disabled thumb left nothing on the track for a
  // reader to see, so the disabled thumb takes a border colour of its own.
  it('keeps a disabled thumb visible against the track', async () => {
    const { container } = await render(Slider, {
      props: {
        type: 'single',
        value: 50,
        disabled: true,
        'aria-label': 'Volume',
      },
    });

    const thumb = container.querySelector('[data-slot="slider-thumb"]');
    expect(thumb?.getAttribute('data-disabled')).toBe('');
    const painted = getComputedStyle(thumb!);
    expect(painted.borderTopColor).not.toBe('rgba(0, 0, 0, 0)');
  });

  // The track lays itself out from the orientation, and the primitive sets it
  // on the root alone.
  it('turns the track with the slider', async () => {
    const { container } = await render(Slider, {
      props: {
        type: 'single',
        value: 50,
        orientation: 'vertical',
        'aria-label': 'Volume',
      },
    });

    const track = container.querySelector('[data-slot="slider-track"]');
    expect(track?.getAttribute('data-orientation')).toBe('vertical');
  });

  it('fills the range in the primary colour under the primary tone', async () => {
    const probe = document.createElement('div');
    probe.style.backgroundColor = 'var(--primary)';
    document.body.append(probe);
    const primary = getComputedStyle(probe).backgroundColor;
    probe.remove();

    const fills = new Map<string, string>();
    for (const tone of ['default', 'primary'] as const) {
      const { container, unmount } = await render(Slider, {
        props: { type: 'single', value: 50, tone, 'aria-label': 'Speed' },
      });
      const range = container.querySelector<HTMLElement>(
        '[data-slot="slider-range"]',
      );
      fills.set(tone, getComputedStyle(range!).backgroundColor);
      await unmount();
    }

    expect(fills.get('primary')).toBe(primary);
    expect(fills.get('default')).not.toBe(primary);
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(Slider, {
      props: { type: 'multiple', value: [25, 75], 'aria-label': 'Price range' },
    });

    await expectNoViolations(container);
  });
});
