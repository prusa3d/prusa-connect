import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import AddPrinterScreen from './add-printer-screen.svelte';
import {
  matchingCandidates,
  modelOfName,
  pairingAddress,
} from './presentation';
import type { AddPrinterScreenProps, NearbyCandidate } from './types';

const connection = {
  host: '192.168.1.50',
  port: 8080,
  tls: false,
  wsUrl: 'ws://192.168.1.50:8080/p/ws',
  registerUrl: 'http://192.168.1.50:8080/p/register',
};

const nearby: NearbyCandidate[] = [
  { name: 'prusa-core-one', address: '192.168.1.21', port: 80 },
  { name: 'workshop-mk4', address: '192.168.1.22', port: 8080 },
];

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
  nearby,
  ...over,
});

const pairButtons = () =>
  page.getByRole('button', { name: 'Pair', exact: true });

describe('the printers on this network', () => {
  it('lists each printer with its name and address, and its model when its name says it', async () => {
    await render(AddPrinterScreen, { props: screen() });

    await expect
      .element(page.getByText('Printers on this network'))
      .toBeInTheDocument();
    await expect.element(page.getByText('prusa-core-one')).toBeInTheDocument();
    await expect
      .element(page.getByText('CORE One, 192.168.1.21, port 80'))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('192.168.1.22, port 8080'))
      .toBeInTheDocument();
    expect(pairButtons().all()).toHaveLength(2);
  });

  it('pairs the printer chosen, at the address and port it announced', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, { props: screen({ onPair }) });

    await pairButtons().nth(1).click();

    expect(onPair).toHaveBeenCalledExactlyOnceWith({
      printer: '192.168.1.22:8080',
      connect: { host: '192.168.1.50', port: 8080, tls: false },
    });
  });

  it('narrows the list by name or address, and clears the search', async () => {
    await render(AddPrinterScreen, { props: screen() });
    const search = page.getByRole('searchbox', {
      name: 'Printer search',
    });

    await search.fill('1.22');
    await expect.element(page.getByText('workshop-mk4')).toBeInTheDocument();
    expect(page.getByText('prusa-core-one').query()).toBe(null);

    await search.fill('CORE');
    await expect.element(page.getByText('prusa-core-one')).toBeInTheDocument();
    expect(page.getByText('workshop-mk4').query()).toBe(null);

    await search.fill('nothing like it');
    await expect
      .element(page.getByText('No printer matches the search.'))
      .toBeInTheDocument();
    await page.getByRole('button', { name: 'Show every printer' }).click();
    expect(pairButtons().all()).toHaveLength(2);
  });

  it('says none were found, and keeps the address field', async () => {
    await render(AddPrinterScreen, { props: screen({ nearby: [] }) });

    await expect
      .element(page.getByText('No new printers found on this network'))
      .toBeInTheDocument();
    await expect
      .element(page.getByRole('textbox', { name: 'Printer address' }))
      .toBeInTheDocument();
  });

  it('offers only the address field to a host that lists nothing', async () => {
    await render(AddPrinterScreen, { props: screen({ nearby: undefined }) });

    await expect
      .element(page.getByRole('textbox', { name: 'Printer address' }))
      .toBeInTheDocument();
    expect(page.getByText('Printers on this network').query()).toBe(null);
  });

  it('sends nothing while the server address is the printer itself', async () => {
    const onPair = vi.fn();
    await render(AddPrinterScreen, {
      props: screen({
        onPair,
        connection: { ...connection, host: 'localhost' },
      }),
    });

    await expect.element(pairButtons().first()).toBeDisabled();
    expect(onPair).not.toHaveBeenCalled();
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(AddPrinterScreen, { props: screen() });
    await expectNoViolations(container);
  });
});

describe('the picker helpers', () => {
  it('reads the model only from a default name', () => {
    expect(modelOfName('prusa-mk4')).toBe('MK4');
    expect(modelOfName('PRUSA-CORE-ONE-L')).toBe('CORE One L');
    expect(modelOfName('workshop-mk4')).toBeUndefined();
  });

  it('writes the port only when it is not 80, and brackets IPv6', () => {
    expect(pairingAddress({ address: '192.168.1.21', port: 80 })).toBe(
      '192.168.1.21',
    );
    expect(pairingAddress({ address: '192.168.1.21', port: 8080 })).toBe(
      '192.168.1.21:8080',
    );
    expect(pairingAddress({ address: 'fd00::1', port: 81 })).toBe(
      '[fd00::1]:81',
    );
  });

  it('matches name or address, ignoring case', () => {
    expect(
      matchingCandidates(nearby, '  ').map((one) => one.name),
    ).toHaveLength(2);
    expect(
      matchingCandidates(nearby, 'Workshop').map((one) => one.name),
    ).toEqual(['workshop-mk4']);
    expect(matchingCandidates(nearby, '.21').map((one) => one.name)).toEqual([
      'prusa-core-one',
    ]);
  });
});
