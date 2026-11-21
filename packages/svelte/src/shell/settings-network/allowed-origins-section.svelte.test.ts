import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { AllowedOrigins } from '../../data';
import type { AllowedOrigin, AllowedOriginsClient } from '../../data';
import { StubApiError } from '../../data/stubs';
import { expectNoViolations } from '../../test/axe';
import AllowedOriginsSection from './allowed-origins-section.svelte';

interface Server {
  fixed: AllowedOrigin[];
  stored: string[];
  written: string[][];
  refusal: StubApiError | undefined;
}

const FIXED: AllowedOrigin[] = [
  { origin: 'https://farm.example', source: 'file' },
  {
    origin: 'https://slicer.example',
    source: 'extension',
    extension: 'com.github.acme.slicer',
    title: 'Web slicer',
  },
];

/** The real store over a fake client, so the section is tested with what ships. */
const storeOver = (
  stored: string[] = [],
  fixed: AllowedOrigin[] = FIXED,
): { store: AllowedOrigins; server: Server } => {
  const server: Server = { fixed, stored, written: [], refusal: undefined };
  const listed = (): { items: AllowedOrigin[] } => ({
    items: [
      ...server.stored.map((origin): AllowedOrigin => ({
        origin,
        source: 'settings',
      })),
      ...server.fixed,
    ],
  });
  const client: AllowedOriginsClient = {
    listAllowedOrigins: () => Promise.resolve(listed()),
    setAllowedOrigins: (request) => {
      server.written.push(request.body.origins);
      if (server.refusal) return Promise.reject(server.refusal);

      server.stored = request.body.origins;

      return Promise.resolve(listed());
    },
  };

  return { store: new AllowedOrigins(client), server };
};

describe('AllowedOriginsSection', () => {
  it('waits for the server before it draws the list', async () => {
    const { store } = storeOver();

    await render(AllowedOriginsSection, { props: { source: store } });

    await expect
      .element(page.getByLabelText('Reading the allowed sites'))
      .toHaveAttribute('aria-busy', 'true');
    await expect.element(page.getByRole('textbox')).not.toBeInTheDocument();
  });

  it('says where each origin comes from and removes only a stored one', async () => {
    const { store } = storeOver(['https://dashboard.example']);
    await store.load();

    await render(AllowedOriginsSection, {
      props: {
        source: store,
        extensionHref: (namespace: string) => `/extensions/${namespace}`,
      },
    });

    await expect
      .element(
        page.getByText('Named in the configuration file', { exact: false }),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('link', { name: /The extension Web slicer/ }))
      .toHaveAttribute('href', '/extensions/com.github.acme.slicer');
    await expect
      .element(
        page.getByRole('button', { name: 'Remove https://dashboard.example' }),
      )
      .toBeVisible();
    expect(
      page.getByRole('button', { name: /^Remove/ }).elements(),
    ).toHaveLength(1);
  });

  it('adds the typed origin and clears the field', async () => {
    const { store, server } = storeOver();
    await store.load();

    await render(AllowedOriginsSection, { props: { source: store } });

    const field = page.getByRole('textbox', { name: 'Address of the site' });
    await userEvent.fill(field, ' https://new.example ');
    await userEvent.click(page.getByRole('button', { name: 'Allow' }));

    await expect.element(page.getByText('https://new.example')).toBeVisible();
    await expect.element(field).toHaveValue('');
    expect(server.written).toEqual([['https://new.example']]);
  });

  it('removes a stored origin', async () => {
    const { store, server } = storeOver(['https://dashboard.example']);
    await store.load();

    await render(AllowedOriginsSection, { props: { source: store } });

    await userEvent.click(
      page.getByRole('button', { name: 'Remove https://dashboard.example' }),
    );

    await expect
      .element(page.getByText('https://dashboard.example'))
      .not.toBeInTheDocument();
    expect(server.written).toEqual([[]]);
  });

  // A viewer may read the list and not change it, and a value the server
  // refuses stays in the field.
  it('shows the refusal and keeps the typed value', async () => {
    const { store, server } = storeOver();
    await store.load();
    server.refusal = new StubApiError('core.auth/ACCESS_DENIED', 403);

    await render(AllowedOriginsSection, { props: { source: store } });

    const field = page.getByRole('textbox', { name: 'Address of the site' });
    await userEvent.fill(field, 'https://new.example');
    await userEvent.click(page.getByRole('button', { name: 'Allow' }));

    await expect.element(page.getByRole('alert')).toBeVisible();
    await expect.element(field).toHaveValue('https://new.example');
  });

  it('says so when no other site is allowed', async () => {
    const { store } = storeOver([], []);
    await store.load();

    await render(AllowedOriginsSection, { props: { source: store } });

    await expect
      .element(
        page.getByText('No other site may call this server from a browser.'),
      )
      .toBeVisible();
  });

  it('has no accessibility violations', async () => {
    const { store } = storeOver(['https://dashboard.example']);
    await store.load();

    const { container } = await render(AllowedOriginsSection, {
      props: { source: store },
    });

    await expectNoViolations(container);
  });
});
