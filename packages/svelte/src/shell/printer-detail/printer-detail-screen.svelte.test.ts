import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import { t } from '../../i18n';
import { fakeDevice } from '../../test/fakes';
import PrinterDetailScreen from './printer-detail-screen.svelte';
import type { PrinterAdministration } from './info/types';
import type { PrinterDetailScreenProps, TelemetrySource } from './types';

const telemetry: TelemetrySource = {
  value: () => undefined,
  points: () => [],
};

const props = (
  over: Partial<PrinterDetailScreenProps> = {},
): PrinterDetailScreenProps => ({
  deviceId: 'printer-1',
  device: {
    ...fakeDevice('printer-1', 'MK4S'),
    state: { '/state': 'PRINTING' },
  },
  telemetry,
  commands: { onCommand: () => undefined },
  printerCount: 1,
  tab: 'status',
  onTabChange: () => undefined,
  printersHref: '/printers',
  ...over,
});

const administration: PrinterAdministration = {
  pending: () => false,
  error: undefined,
  failed: undefined,
  tags: (record) => record,
  editTags: () => Promise.resolve(true),
  rename: () => Promise.resolve(true),
  retire: () => Promise.resolve(true),
};

describe('PrinterDetailScreen', () => {
  it('offers a pencil beside the name to a caller who may administer the printer', async () => {
    const base = props();
    await render(PrinterDetailScreen, {
      props: {
        ...base,
        device: { ...base.device!, actions: ['administer'] },
        administration,
      },
    });

    await expect
      .element(
        page.getByRole('button', {
          name: t('printerDetailInfo.name.iconLabel'),
        }),
      )
      .toBeVisible();
  });

  it('offers no pencil to a caller who may not', async () => {
    await render(PrinterDetailScreen, { props: props({ administration }) });

    await expect
      .element(
        page.getByRole('button', {
          name: t('printerDetailInfo.name.iconLabel'),
        }),
      )
      .not.toBeInTheDocument();
  });

  it('names the printer as the page heading and the trail leads back', async () => {
    await render(PrinterDetailScreen, { props: props() });

    expect(document.querySelector('h1')?.textContent.trim()).toBe('MK4S');
    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toHaveAttribute('href', '/printers');
  });

  it('draws the five tabs and reports the one pressed', async () => {
    const onTabChange = vi.fn();
    const { container } = await render(PrinterDetailScreen, {
      props: props({ onTabChange }),
    });

    const labels = [
      ...container.querySelectorAll('[data-slot="tabs-trigger"]'),
    ].map((trigger) => trigger.textContent.trim());
    expect(labels).toEqual(['Now', 'Controls', 'Files', 'Printer', 'History']);

    await page.getByRole('tab', { name: 'Files' }).click();
    expect(onTabChange).toHaveBeenCalledWith('files');
  });

  it('selects Controls for a legacy Advanced address', async () => {
    await render(PrinterDetailScreen, {
      props: props({ tab: 'commands' }),
    });

    await expect
      .element(page.getByRole('tab', { name: 'Controls', exact: true }))
      .toHaveAttribute('aria-selected', 'true');
    await expect
      .element(page.getByRole('tab', { name: 'Advanced', exact: true }))
      .not.toBeInTheDocument();
  });

  // A skeleton that never resolves reads as a broken page.
  it('names a tab this build does not draw', async () => {
    const { container } = await render(PrinterDetailScreen, {
      props: props({ tab: 'commands' }),
    });

    await expect.element(page.getByText('Not built yet')).toBeVisible();
    expect(container.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(
      0,
    );
  });

  it('draws the state the printer reported', async () => {
    await render(PrinterDetailScreen, { props: props() });

    await expect.element(page.getByText('Printing')).toBeVisible();
  });

  // A read of the drive holds up the printer's other answers.
  it('shows the reading-the-drive badge on the printer', async () => {
    await render(PrinterDetailScreen, {
      props: props({
        commands: { onCommand: () => undefined, readingDrive: true },
      }),
    });

    await expect.element(page.getByText('Reading the drive')).toBeVisible();
  });

  it('shows no such badge while no drive is read', async () => {
    await render(PrinterDetailScreen, { props: props() });

    await expect.element(page.getByText('Printing')).toBeVisible();
    expect(page.getByText('Reading the drive').elements()).toHaveLength(0);
  });

  // The whole page waits as the header's own shape, not as a grey rectangle.
  it('draws the header skeleton while the record is on its way', async () => {
    await render(PrinterDetailScreen, { props: props({ loading: true }) });

    expect(document.querySelector('h1')).toBeNull();
  });

  // The path names the printer.
  it('keeps the trail while the record is still on its way', async () => {
    await render(PrinterDetailScreen, {
      props: props({ device: undefined, loading: true }),
    });

    expect(document.querySelector('h1')).toBeNull();
    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toHaveAttribute('href', '/printers');
  });

  // The fleet answered and holds no such printer.
  it('names a printer the server does not hold', async () => {
    await render(PrinterDetailScreen, { props: props({ device: undefined }) });

    await expect
      .element(page.getByText('This printer is not here'))
      .toBeVisible();
    expect(
      document.querySelectorAll('[data-slot="tabs-trigger"]'),
    ).toHaveLength(0);
    await expect
      .element(page.getByRole('link', { name: 'Printers' }).last())
      .toHaveAttribute('href', '/printers');
  });

  it('shows a failed record read as a fault, not as a gone printer', async () => {
    await render(PrinterDetailScreen, {
      props: props({
        device: undefined,
        fault: { code: 'core.device/BROKEN', status: 500, message: 'boom' },
      }),
    });

    await expect.element(page.getByText('core.device/BROKEN')).toBeVisible();
    expect(document.body.textContent).not.toContain('This printer is not here');
  });

  it('names a printer whose record read was refused as gone', async () => {
    await render(PrinterDetailScreen, {
      props: props({
        device: undefined,
        fault: { code: 'core.device/NOT_FOUND', status: 404 },
      }),
    });

    await expect
      .element(page.getByText('This printer is not here'))
      .toBeVisible();
    expect(document.body.textContent).not.toContain('core.device/NOT_FOUND');
  });

  // A refusal renders by its code, with the server's own line under it.
  it('names a fault the server answered with', async () => {
    const { container } = await render(PrinterDetailScreen, {
      props: props({
        fault: { code: 'core.device/NOT_FOUND', message: 'no such device' },
      }),
    });

    expect(container.textContent).toContain('core.device/NOT_FOUND');
  });

  // The peer is a fact of the instance.
  it('offers the other printer only when the instance holds two', async () => {
    const onSelectPeer = vi.fn();
    const { unmount } = await render(PrinterDetailScreen, { props: props() });
    expect(
      page.getByRole('button', { name: /CORE One/ }).elements(),
    ).toHaveLength(0);
    await unmount();

    await render(PrinterDetailScreen, {
      props: props({
        printerCount: 2,
        peer: { device: fakeDevice('printer-2', 'CORE One'), side: 'after' },
        onSelectPeer,
      }),
    });

    // A group and not a tablist: the control changes the subject of the whole
    // screen and has no panel of its own.
    await page.getByRole('button', { name: /CORE One/ }).click();
    expect(onSelectPeer).toHaveBeenCalledTimes(1);
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(PrinterDetailScreen, { props: props() });

    await expectNoViolations(container);
  });
});
