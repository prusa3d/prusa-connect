import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import AddPrinterScreen from './add-printer-screen.svelte';
import { storedHostFits } from './presentation';
import type { AddPrinterScreenProps } from './types';

const connection = {
  host: '192.168.1.50',
  port: 8080,
  tls: false,
  wsUrl: 'ws://192.168.1.50:8080/p/ws',
  registerUrl: 'http://192.168.1.50:8080/p/register',
};

const screen = (
  over: Partial<AddPrinterScreenProps> = {},
): AddPrinterScreenProps => ({
  connection,
  admission: 'approval',
  pending: [],
  joining: [],
  joined: [],
  refused: undefined,
  printersHref: '/printers',
  onBegin: () => undefined,
  onApprove: () => undefined,
  onReject: () => undefined,
  onPair: () => undefined,
  ...over,
});

const ask = () => page.getByRole('button', { name: 'Ask the printer' });

describe('pairing a printer at its screen', () => {
  it('is offered only by a host that can pair', async () => {
    await render(AddPrinterScreen, { props: screen({ onPair: undefined }) });

    expect(page.getByText("Pair from the printer's screen").query()).toBe(null);
  });

  it('sends the printer address and the address the settings file carries', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, { props: screen({ onPair }) });

    await expect.element(ask()).toBeDisabled();

    await page.getByLabelText('Printer address').fill(' 192.168.1.20 ');
    await expect.element(ask()).toBeEnabled();
    await ask().click();

    expect(onPair).toHaveBeenCalledExactlyOnceWith({
      printer: '192.168.1.20',
      connect: { host: '192.168.1.50', port: 8080, tls: false },
    });
  });

  it('sends the server address the person changed', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, { props: screen({ onPair }) });

    await page
      .getByRole('button', { name: /The printer will connect to/ })
      .click();
    await page
      .getByRole('textbox', { name: 'Server address' })
      .fill('10.0.0.5');
    await page.getByRole('textbox', { name: 'Port', exact: true }).fill('8481');
    await page.getByRole('switch').click();
    await page.getByLabelText('Printer address').fill('10.0.0.20:8080');
    await ask().click();

    expect(onPair).toHaveBeenCalledExactlyOnceWith({
      printer: '10.0.0.20:8080',
      connect: { host: '10.0.0.5', port: 8481, tls: true },
    });
  });

  it('warns about a server name the printer cannot store', async () => {
    await render(AddPrinterScreen, { props: screen() });

    await page
      .getByRole('button', { name: /The printer will connect to/ })
      .click();
    await page
      .getByRole('textbox', { name: 'Server address' })
      .fill('printers.workshop.example.net');

    await expect
      .element(page.getByText(/stores at most 20 characters/))
      .toBeInTheDocument();
  });

  it('refuses a server address that is the printer itself', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, { props: screen({ onPair }) });

    await page.getByLabelText('Printer address').fill('192.168.1.20');
    await page
      .getByRole('button', { name: /The printer will connect to/ })
      .click();
    await page
      .getByRole('textbox', { name: 'Server address' })
      .fill('localhost');

    await expect
      .element(page.getByText(/localhost is the printer itself/))
      .toBeInTheDocument();
    await expect.element(ask()).toBeDisabled();
    await userEvent.keyboard('{Enter}');
    expect(onPair).not.toHaveBeenCalled();
  });

  it('does not send twice while a request is on its way', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, {
      props: screen({ onPair, pairingBusy: true }),
    });

    await page.getByLabelText('Printer address').fill('192.168.1.20');
    await expect.element(ask()).toBeDisabled();
    await expect.element(ask()).toHaveAttribute('aria-busy', 'true');
  });

  it('shows the code large while the question is on the printer, and withdraws it', async () => {
    const onCancelPairing = vi.fn();
    await render(AddPrinterScreen, {
      props: screen({
        onCancelPairing,
        pairing: {
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          state: 'waiting',
        },
      }),
    });

    await expect.element(page.getByText('K7QX-29FM')).toBeInTheDocument();
    await expect
      .element(page.getByText(/press Yes on the printer/))
      .toBeInTheDocument();
    expect(ask().query()).toBe(null);

    await page.getByRole('button', { name: 'Cancel' }).click();
    expect(onCancelPairing).toHaveBeenCalledOnce();
  });

  it('stays in view when the guide moves on while the printer registers', async () => {
    await render(AddPrinterScreen, {
      props: screen({
        initialStep: 'waiting',
        pairing: {
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          state: 'approved',
        },
      }),
    });

    await expect
      .element(page.getByText(/It is registering with this server/))
      .toBeInTheDocument();
  });

  it('offers to pair again after a cancel at the printer', async () => {
    const onPairAgain = vi.fn();
    await render(AddPrinterScreen, {
      props: screen({
        onPairAgain,
        pairing: {
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          state: 'refused',
        },
      }),
    });

    await expect
      .element(page.getByText('Somebody pressed Cancel on the printer'))
      .toBeInTheDocument();
    await page.getByRole('button', { name: 'Pair again' }).click();
    expect(onPairAgain).toHaveBeenCalledOnce();
  });

  it('names the settings file as the way in when the printer cannot pair', async () => {
    await render(AddPrinterScreen, {
      props: screen({
        pairingError: {
          code: 'core.printer/PAIRING_UNSUPPORTED',
          status: 422,
          message: "the printer's firmware cannot pair",
        },
      }),
    });

    await expect
      .element(page.getByText(/add the printer with the settings file/))
      .toBeInTheDocument();
    await expect
      .element(page.getByLabelText('Printer address'))
      .toBeInTheDocument();
  });

  it('has nothing axe refuses in the form or the code', async () => {
    const { container, rerender } = await render(AddPrinterScreen, {
      props: screen(),
    });
    await expectNoViolations(container);

    await rerender(
      screen({
        pairing: {
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          state: 'waiting',
        },
      }),
    );
    await expectNoViolations(container);
  });
});

describe('storedHostFits', () => {
  it('holds the firmware rule for a host name', () => {
    expect(storedHostFits('192.168.100.200')).toBe(true);
    expect(storedHostFits('shop.example.net')).toBe(true);
    expect(storedHostFits('printers.workshop.example.net')).toBe(false);
    expect(storedHostFits('a-long-name.prusa3d.com')).toBe(true);
    expect(storedHostFits('a-much-longer-name-here.prusa3d.com')).toBe(false);
    expect(storedHostFits('my-own-local.connect.prusa3d.com')).toBe(true);
  });
});
