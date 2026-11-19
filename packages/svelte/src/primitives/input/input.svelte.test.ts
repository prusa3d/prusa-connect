import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Input from './input.svelte';

const control = (): HTMLInputElement => {
  const found = document.querySelector<HTMLInputElement>('[data-slot="input"]');
  if (found === null) throw new Error('no input rendered');
  return found;
};

describe('Input', () => {
  it('takes what is typed into it', async () => {
    await render(Input, { props: { 'aria-label': 'Printer name' } });

    await userEvent.type(page.getByRole('textbox'), 'MK4S');

    expect(control().value).toBe('MK4S');
  });

  it('is named by the label that points at it', async () => {
    const host = document.createElement('div');
    host.innerHTML = '<label for="printer-name">Printer name</label>';
    document.body.append(host);

    await render(Input, {
      target: host,
      props: { id: 'printer-name', value: 'MK4S' },
    });

    await expect
      .element(page.getByRole('textbox', { name: 'Printer name' }))
      .toHaveValue('MK4S');
    await expectNoViolations(host);

    host.remove();
  });

  // One disabled look for every field: the textarea's, greyed text in an
  // unfilled box.
  it('greys its text when disabled and leaves the box unfilled', async () => {
    const live = await render(Input, {
      props: { 'aria-label': 'Printer name' },
    });
    const liveStyle = getComputedStyle(control());
    const liveColour = liveStyle.color;
    expect(liveStyle.backgroundColor).not.toBe('rgba(0, 0, 0, 0)');
    await live.unmount();

    await render(Input, {
      props: { 'aria-label': 'Printer name', disabled: true },
    });
    const dead = getComputedStyle(control());

    expect(dead.color).not.toBe(liveColour);
    expect(dead.opacity).toBe('1');
    expect(dead.backgroundColor).toBe('rgba(0, 0, 0, 0)');
  });

  it('takes nothing while it is disabled', async () => {
    await render(Input, {
      props: { 'aria-label': 'Printer name', disabled: true },
    });

    await userEvent.tab();

    expect(document.activeElement).not.toBe(control());
    expect(control().value).toBe('');
  });

  // The density may draw several steps at one height. A larger step is never the
  // shorter one, and none falls under the 24px a pointer target needs.
  it('never shrinks from one step of the control scale to the next', async () => {
    const heightOf = async (size: 'sm' | 'md' | 'lg'): Promise<number> => {
      const host = document.createElement('div');
      document.body.append(host);
      await render(Input, {
        target: host,
        props: { size, 'aria-label': size },
      });
      const found = host.querySelector('input');
      if (found === null) throw new Error('no input rendered');
      const height = found.getBoundingClientRect().height;
      host.remove();
      return height;
    };

    const [small, medium, large] = await Promise.all([
      heightOf('sm'),
      heightOf('md'),
      heightOf('lg'),
    ]);

    expect(small).toBeLessThanOrEqual(medium);
    expect(medium).toBeLessThanOrEqual(large);
    expect(small).toBeGreaterThanOrEqual(24);
  });
});
