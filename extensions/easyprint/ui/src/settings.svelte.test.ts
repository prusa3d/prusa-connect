import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { EASYPRINT_ORIGIN, pairHref } from './easyprint';
import { holdHost } from './host';
import setup from './index';
import Settings from './settings.svelte';
import { NAMESPACE, stubHost } from './test/host';

describe('the EasyPrint settings tab', () => {
  it('registers the one tab module.yaml declares', async () => {
    const { host, registry } = stubHost({ origins: [EASYPRINT_ORIGIN] });

    await setup(host);

    expect(registry.sections('settings').map((entry) => entry.id)).toEqual([
      'easyprint',
    ]);
    expect(registry.sections('settings')[0]?.namespace).toBe(NAMESPACE);
  });

  it('links to EasyPrint with the address this page was opened on', async () => {
    const { host } = stubHost({ origins: [EASYPRINT_ORIGIN] });
    holdHost(host);

    await render(Settings);

    const link = page.getByRole('link', {
      name: 'Add this server to EasyPrint',
    });
    await expect
      .element(link)
      .toHaveAttribute('href', pairHref(window.location.origin));
    await expect.element(link).toHaveAttribute('target', '_blank');
    await expect.element(link).toHaveAttribute('rel', 'noopener noreferrer');
    await expect
      .element(page.getByLabelText('The address EasyPrint will call'))
      .toHaveValue(window.location.origin);
  });

  it('says nothing is missing when the server answers EasyPrint', async () => {
    const { host, requests } = stubHost({ origins: [EASYPRINT_ORIGIN] });
    holdHost(host);

    const { container } = await render(Settings);

    await expect.poll(() => requests.length).toBeGreaterThan(0);
    expect(container.querySelector('[data-part="not-allowed"]')).toBeNull();
  });

  // An extension switched on without an approved install was granted nothing.
  it('warns when the server does not answer the origin of EasyPrint', async () => {
    const { host } = stubHost({ origins: ['https://other.example'] });
    holdHost(host);

    await render(Settings);

    await expect
      .element(page.getByText('This server does not answer EasyPrint yet'))
      .toBeVisible();
  });

  it('warns nobody who may not read the allowed origins', async () => {
    const { host, requests } = stubHost({ refused: 403 });
    holdHost(host);

    const { container } = await render(Settings);

    await expect.poll(() => requests.length).toBeGreaterThan(0);
    expect(container.querySelector('[data-part="not-allowed"]')).toBeNull();
  });

  // The test page itself is served over plain HTTP.
  it('says what plain HTTP costs on a page opened over it', async () => {
    const { host } = stubHost({ origins: [EASYPRINT_ORIGIN] });
    holdHost(host);

    await render(Settings);

    await expect
      .element(page.getByText('This page is open over plain HTTP'))
      .toBeVisible();
  });
});
