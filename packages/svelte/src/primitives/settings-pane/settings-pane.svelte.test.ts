import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import SettingsPane from './settings-pane.svelte';

afterEach(() => {
  document.documentElement.classList.remove('dark');
});
import SettingsRow from './settings-row.svelte';

const snippet = (html: string) =>
  createRawSnippet(() => ({ render: () => html }));

const nextFrame = () =>
  new Promise<void>((resolve) => {
    requestAnimationFrame(() => {
      resolve();
    });
  });

describe('SettingsPane', () => {
  it('names itself by its own heading, so a screen reader announces it', async () => {
    await render(SettingsPane, {
      props: {
        title: 'People',
        description: 'Who may sign in to this server.',
        children: snippet('<p>Three accounts</p>'),
      },
    });

    const pane = page.getByRole('region', { name: 'People' });
    await expect.element(pane).toBeVisible();
    await expect
      .element(page.getByText('Who may sign in to this server.'))
      .toBeVisible();
  });

  it('puts what saves the pane in a band of its own', async () => {
    await render(SettingsPane, {
      props: {
        title: 'People',
        children: snippet('<p>Three accounts</p>'),
        actions: snippet('<button type="button">Save</button>'),
      },
    });

    const band = document.querySelector('[data-slot="settings-pane-actions"]');
    if (band === null) throw new Error('no action band rendered');
    const save = page.getByRole('button', { name: 'Save' });
    await expect.element(save).toBeVisible();
    expect(band.contains(save.element())).toBe(true);
  });

  it('draws no band when there is nothing to save', async () => {
    await render(SettingsPane, {
      props: { title: 'People', children: snippet('<p>Three accounts</p>') },
    });

    expect(
      document.querySelector('[data-slot="settings-pane-actions"]'),
    ).toBeNull();
    expect(
      document.querySelector('[data-slot="settings-pane-message"]'),
    ).toBeNull();
  });

  it('announces a failure and lets a plain message wait its turn', async () => {
    const failed = await render(SettingsPane, {
      props: {
        title: 'People',
        message: 'The accounts could not be read.',
        messageIsFailure: true,
      },
    });

    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('The accounts could not be read.');
    await failed.unmount();

    await render(SettingsPane, {
      props: { title: 'People', message: 'Saved.' },
    });
    await expect.element(page.getByRole('status')).toHaveTextContent('Saved.');
  });

  it('names a row without giving its control a second name', async () => {
    await render(SettingsRow, {
      props: {
        label: 'Discovery',
        description: 'Let this server find peers on the network.',
        control: snippet(
          '<input type="checkbox" aria-label="Discovery on this network" />',
        ),
      },
    });

    // The label is text, not a `<label>`: the control carries its own name and
    // a second one would be read out as well.
    expect(document.querySelectorAll('label')).toHaveLength(0);
    await expect
      .element(
        page.getByRole('checkbox', { name: 'Discovery on this network' }),
      )
      .toBeVisible();
  });

  it('stacks a row in a narrow pane and puts the control beside it in a wide one', async () => {
    const host = document.createElement('div');
    host.style.inlineSize = '20rem';
    document.body.appendChild(host);

    try {
      await render(SettingsPane, {
        target: host,
        props: { title: 'Fractal', children: snippet('<div>rows</div>') },
      });
      const pane = document.querySelector('[data-slot="settings-pane-body"]');
      if (pane === null) throw new Error('no pane body rendered');

      await render(SettingsRow, {
        target: pane,
        props: { label: 'Discovery', control: snippet('<span>on</span>') },
      });
      const row = document.querySelector('[data-slot="settings-row"]');
      if (row === null) throw new Error('no row rendered');
      expect(getComputedStyle(row).flexDirection).toBe('column');

      host.style.inlineSize = '48rem';
      await nextFrame();
      expect(getComputedStyle(row).flexDirection).toBe('row');
    } finally {
      host.remove();
    }
  });
});

it.each([false, true])(
  'keeps the panel surface steady during hover and focus (dark=%s)',
  async (dark) => {
    document.documentElement.classList.toggle('dark', dark);
    await render(SettingsPane, {
      props: {
        title: 'Password',
        children: snippet('<input aria-label="Password value" />'),
      },
    });
    const pane = page.getByRole('region', { name: 'Password' });
    const panel = pane.element();
    const shadow = getComputedStyle(panel).boxShadow;

    await userEvent.hover(pane);
    expect(getComputedStyle(panel).boxShadow).toBe(shadow);

    const input = page.getByRole('textbox', { name: 'Password value' });
    await input.click();
    expect(panel.matches(':focus-within')).toBe(true);
    expect(getComputedStyle(panel).boxShadow).toBe(shadow);
    expect(getComputedStyle(panel, '::after').content).toBe('none');
  },
);
