import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import KeyValueList from './key-value-list.svelte';
import type { KeyValueRow } from './types';

const rows: KeyValueRow[] = [
  {
    id: 'serial',
    label: 'Serial',
    value: 'CZPX1234',
    mono: true,
    copyable: true,
  },
  { id: 'model', label: 'Model', value: 'CORE One' },
  { id: 'firmware', label: 'Firmware' },
];

const nextFrame = () =>
  new Promise<void>((resolve) => {
    requestAnimationFrame(() => {
      resolve();
    });
  });

afterEach(() => {
  vi.restoreAllMocks();
});

describe('KeyValueList', () => {
  it('widens the name column to the width a caller names', async () => {
    await render(KeyValueList, { props: { rows, labelWidth: '14rem' } });

    expect(
      document.querySelector('dl')?.style.getPropertyValue('--kv-label'),
    ).toBe('14rem');
  });

  it('draws each name and its reading as a pair', async () => {
    await render(KeyValueList, { props: { rows } });

    const list = document.querySelector('dl');
    expect(list?.querySelectorAll('dt')).toHaveLength(3);
    expect(list?.querySelectorAll('dd')).toHaveLength(3);
    await expect.element(page.getByText('CORE One')).toBeVisible();
  });

  it('says a reading is absent rather than drawing a zero', async () => {
    await render(KeyValueList, {
      props: { rows, absentText: 'No reading yet' },
    });

    await expect.element(page.getByText('No reading yet')).toBeVisible();
    const name = Array.from(document.querySelectorAll('dt')).find(
      (one) => one.textContent === 'Firmware',
    );
    expect((name?.nextElementSibling?.textContent ?? '').trim()).toBe(
      'No reading yet',
    );
  });

  it('copies a reading with a button named for its own row', async () => {
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined);
    await render(KeyValueList, { props: { rows } });

    // Only the row that asked for one gets a copy button.
    expect(document.querySelectorAll('button')).toHaveLength(1);

    await page.getByRole('button', { name: 'Copy Serial' }).click();

    expect(writeText).toHaveBeenCalledWith('CZPX1234');
    // The copy button says what it did in its own live region.
    await expect
      .poll(() => document.querySelector('[role="status"]')?.textContent)
      .toBe('Copied to clipboard');
  });

  it('draws a key in the mono face and a reading in the body face', async () => {
    await render(KeyValueList, { props: { rows } });

    const values = document.querySelectorAll<HTMLElement>(
      '[data-slot="key-value-value"]',
    );
    const [serial, model] = Array.from(values);
    if (serial === undefined || model === undefined) {
      throw new Error('no values rendered');
    }
    expect(getComputedStyle(serial).fontFamily).not.toBe(
      getComputedStyle(model).fontFamily,
    );
  });

  // An identifier has no words to break on.
  it('keeps an identifier on one line and offers the whole of it', async () => {
    await render(KeyValueList, {
      props: {
        rows: [
          {
            id: 'uuid',
            label: 'Printer UUID',
            value: '34e039fc-13fd-4f4a-b16b-92392643f63a',
            mono: true,
            copyable: true,
          },
          { id: 'model', label: 'Model', value: 'CORE One' },
        ],
      },
    });

    const values = document.querySelectorAll<HTMLElement>(
      '[data-slot="key-value-value"]',
    );
    const [uuid, model] = Array.from(values);
    if (uuid === undefined || model === undefined) {
      throw new Error('no values rendered');
    }
    expect(getComputedStyle(uuid).whiteSpace).toBe('nowrap');
    expect(getComputedStyle(uuid).textOverflow).toBe('ellipsis');
    expect(uuid.title).toBe('34e039fc-13fd-4f4a-b16b-92392643f63a');
    // Prose and numbers still wrap, and carry no tooltip of their own.
    expect(getComputedStyle(model).whiteSpace).toBe('normal');
    expect(model.title).toBe('');
  });

  it('stacks a pair in a narrow region and puts them on one line in a wide one', async () => {
    const host = document.createElement('div');
    host.style.inlineSize = '20rem';
    document.body.appendChild(host);

    try {
      await render(KeyValueList, { target: host, props: { rows } });
      const row = document.querySelector('[data-slot="key-value-row"]');
      if (row === null) throw new Error('no row rendered');
      expect(getComputedStyle(row).flexDirection).toBe('column');

      host.style.inlineSize = '40rem';
      await nextFrame();
      expect(getComputedStyle(row).flexDirection).toBe('row');
    } finally {
      host.remove();
    }
  });
});
