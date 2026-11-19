import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { START_PRINT_VERB } from '../../data/services/print';
import {
  type Device,
  type PrintOutcome,
  type PrintRequest,
  STATE_PATH,
} from '../../domain';
import { expectNoViolations } from '../../test/axe';
import { fakeDevice } from '../../test/fakes';
import {
  jobsRead,
  nothingPrinted,
  readRefused,
  remembering,
  stillReading,
} from './fixtures';
import JobsScreen from './jobs-screen.svelte';

/** The send button, which names how many printers it sends to once one is ticked. */
const PRINT = /^Print( on \d+ printers?)?$/;

/** The printer that ran the first two prints, in the state a test names. */
const rack = (state: string): Device => ({
  ...fakeDevice('prn-rack-3', 'MK4S, print farm rack 3'),
  capabilities: [START_PRINT_VERB],
  state: { [STATE_PATH]: state },
});

const outcome = (ok: boolean): PrintOutcome => ({
  commandId: 'cmd-1',
  faultCode: ok ? undefined : 'core.printer/COMMAND_REFUSED',
  machineReason: ok ? undefined : 'the printer said no',
  ok,
  path: '/usb/Bracket_Left_Reinforced_v4_FINAL.gcode',
  transferId: undefined,
});

/** Five rows a page, so twelve prints make a pager worth pressing. */
const store = remembering({ pageSize: 5 });

/** The file link leading the first row of the table. */
const firstTableHref = (): string | null =>
  document
    .querySelector<HTMLAnchorElement>('[data-slot="table-body"] a')
    ?.getAttribute('href') ?? null;

const dataRows = (): NodeListOf<HTMLElement> =>
  document.querySelectorAll<HTMLElement>('[data-slot="data-row"]');

const panel = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="popover-content"]');

/**
 * A panel stays open after a pick, so it is shut before the next one opens.
 * The column's sort button shares the facet's name, so the trigger is the
 * button of that name outside the table.
 */
const inPanel = async (trigger: string, value: string): Promise<void> => {
  const button = page
    .getByRole('button', { name: new RegExp(`^${trigger}`) })
    .elements()
    .find((node) => node.closest('table') === null);
  if (button === undefined) throw new Error(`no ${trigger} facet`);
  await page.elementLocator(button).click();
  await expect.poll(panel).not.toBeNull();
  const surface = panel();
  if (surface === null) throw new Error('no panel opened');

  await page
    .elementLocator(surface)
    .getByRole('button', { name: new RegExp(value) })
    .click();
  await userEvent.keyboard('{Escape}');
  await expect.poll(panel).toBeNull();
};

/** Opens one row's kebab, which both row tiers draw at the end of the row. */
const menu = async (fileName: string): Promise<void> => {
  await page.getByRole('button', { name: `Actions for ${fileName}` }).click();
};

const header = (name: string) =>
  page.getByRole('columnheader', { name: new RegExp(name) });

describe('JobsScreen', () => {
  // Wide enough for the seven columns the table's container query asks for.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  it("names itself and draws every printer's prints as one list", async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await expect
      .element(page.getByRole('heading', { level: 1 }))
      .toHaveTextContent('Jobs');
    await expect
      .element(page.getByText(/Page 1 of 3 . showing 5 of 12/))
      .toBeVisible();
  });

  it('opens on the newest print and reorders on a header', async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await expect
      .element(header('Started'))
      .toHaveAttribute('aria-sort', 'descending');
    await expect
      .element(header('Duration'))
      .toHaveAttribute('aria-sort', 'none');
    expect(firstTableHref()).toBe('/jobs/prn-rack-3%3A42');

    // A press on the sorted header reverses it, and the oldest print leads.
    await page.getByRole('button', { name: 'Started', exact: true }).click();
    await expect
      .element(header('Started'))
      .toHaveAttribute('aria-sort', 'ascending');
    await expect.poll(firstTableHref).toBe('/jobs/prn-mk3s%3A89');

    // The other header takes the order ascending, and a second press reverses
    // it, longest first.
    await page.getByRole('button', { name: 'Duration', exact: true }).click();
    await expect
      .element(header('Duration'))
      .toHaveAttribute('aria-sort', 'ascending');
    await page.getByRole('button', { name: 'Duration', exact: true }).click();
    await expect
      .element(header('Duration'))
      .toHaveAttribute('aria-sort', 'descending');
    await expect
      .element(header('Started'))
      .toHaveAttribute('aria-sort', 'none');
    await expect.poll(firstTableHref).toBe('/jobs/prn-rack-3%3A40');
  });

  it('narrows by file name and returns to the first page', async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await page.getByRole('button', { name: 'Next' }).click();
    await expect.element(page.getByText(/Page 2 of 3/)).toBeVisible();

    await page.getByRole('searchbox', { name: 'Search prints' }).fill('benchy');

    await expect
      .element(page.getByText(/Page 1 of 1 . showing 2 of 2/))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Next' }))
      .toBeDisabled();
  });

  it('says a filter hid the list rather than that nothing has printed', async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await inPanel('Printer', 'MINI\\+');
    await page
      .getByRole('searchbox', { name: 'Search prints' })
      .fill('bracket');

    await expect
      .element(page.getByText('No prints match these filters'))
      .toBeVisible();
    expect(page.getByText('No prints yet').query()).toBe(null);
  });

  it('puts a column away in both views at once', async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await expect.element(header('Started')).toBeVisible();
    await inPanel('Fields', 'Started');

    expect(header('Started').query()).toBe(null);
    await expect
      .element(page.getByRole('button', { name: 'Fields, 1 hidden' }))
      .toBeInTheDocument();

    // The same choice in the stacked rows.
    await page.getByRole('radio', { name: 'Rows' }).click();
    await expect.poll(() => page.getByRole('table').query()).toBe(null);
    expect(dataRows()).toHaveLength(5);
    expect(dataRows()[0]?.textContent).not.toContain('2026');
  });

  // Rows already are the narrow layout, so a table that turned into rows on a
  // narrow screen would leave the two views looking the same.
  it('keeps the table where the columns do not fit, scrolled sideways', async () => {
    await page.viewport(420, 900);
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await expect
      .element(page.getByRole('radio', { name: 'Table' }))
      .toHaveAttribute('aria-checked', 'true');
    await expect.element(page.getByRole('table')).toBeVisible();

    await page.getByRole('radio', { name: 'Rows' }).click();
    await expect.poll(() => page.getByRole('table').query()).toBe(null);
    expect(dataRows()[0]?.querySelector('a')?.getAttribute('href')).toBe(
      '/jobs/prn-rack-3%3A42',
    );
  });

  it('draws its own skeleton while the first answer is on its way', async () => {
    await render(JobsScreen, { props: { jobs: stillReading(), store } });

    await expect
      .element(page.getByRole('status', { name: 'Loading prints' }))
      .toBeVisible();
    // A filter set before the rows arrive still applies to them.
    await expect
      .element(page.getByRole('searchbox', { name: 'Search prints' }))
      .toBeVisible();
    expect(page.getByRole('table').query()).toBe(null);
    expect(page.getByText('No prints yet').query()).toBe(null);
  });

  it('says nothing has printed yet, and offers no filter to blame', async () => {
    await render(JobsScreen, { props: { jobs: nothingPrinted(), store } });

    await expect.element(page.getByText('No prints yet')).toBeVisible();
    expect(page.getByText('No prints match these filters').query()).toBe(null);
    expect(page.getByRole('searchbox', { name: 'Search prints' }).query()).toBe(
      null,
    );
  });

  it('draws the refusal the server gave rather than an empty list', async () => {
    await render(JobsScreen, { props: { jobs: readRefused(), store } });

    await expect
      .element(page.getByText('core.auth/ACCESS_DENIED'))
      .toBeVisible();
    expect(page.getByText('No prints yet').query()).toBe(null);
    expect(page.getByText('No prints match these filters').query()).toBe(null);
    expect(page.getByRole('table').query()).toBe(null);
  });

  it('opens the print dialog on the printer that ran the print', async () => {
    const sent: PrintRequest[] = [];
    const onPrinted = vi.fn();
    await render(JobsScreen, {
      props: {
        jobs: jobsRead(),
        store,
        devices: [rack('IDLE')],
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.resolve(outcome(true));
        },
        onPrinted,
      },
    });

    await menu('Bracket_Left_Reinforced_v4_FINAL.gcode');
    await page.getByRole('menuitem', { name: 'Print again' }).click();

    const dialog = page.getByRole('dialog');
    await expect
      .element(
        dialog.getByRole('checkbox', { name: /MK4S, print farm rack 3/ }),
      )
      .toBeChecked();
    await dialog.getByRole('button', { name: PRINT }).click();

    // The printer already holds the file, so nothing names a storage.
    expect(sent).toEqual([
      {
        deviceId: 'prn-rack-3',
        path: '/usb/Bracket_Left_Reinforced_v4_FINAL.gcode',
      },
    ]);
    await expect.poll(() => onPrinted.mock.calls).toEqual([[['prn-rack-3']]]);
  });

  // The rules are the print dialog's, so a printer with a part on its bed
  // is not sent the file and the reader is told why.
  it('does not send the file to a printer that cannot take it', async () => {
    const onPrint = vi.fn();
    await render(JobsScreen, {
      props: { jobs: jobsRead(), store, devices: [rack('FINISHED')], onPrint },
    });

    await menu('Bracket_Left_Reinforced_v4_FINAL.gcode');
    await page.getByRole('menuitem', { name: 'Print again' }).click();

    const dialog = page.getByRole('dialog');
    await expect
      .element(
        dialog.getByText('This printer has a finished print on its bed.'),
      )
      .toBeVisible();
    await expect
      .element(dialog.getByRole('button', { name: PRINT }))
      .toBeDisabled();
    expect(onPrint).not.toHaveBeenCalled();
  });

  it('shows the code of a print the printer refused', async () => {
    await render(JobsScreen, {
      props: {
        jobs: jobsRead(),
        store,
        devices: [rack('IDLE')],
        onPrint: () => Promise.resolve(outcome(false)),
      },
    });

    await menu('Bracket_Left_Reinforced_v4_FINAL.gcode');
    await page.getByRole('menuitem', { name: 'Print again' }).click();
    await page.getByRole('dialog').getByRole('button', { name: PRINT }).click();

    await expect
      .element(page.getByText('core.printer/COMMAND_REFUSED'))
      .toBeVisible();
  });

  // A print still running is not a print to start again.
  it('offers no Print again on a print that is still running', async () => {
    await render(JobsScreen, {
      props: {
        jobs: jobsRead(),
        store,
        devices: [rack('PRINTING')],
        onPrint: vi.fn(),
      },
    });

    await menu('Camera_Mount_Bracket_v3.gcode');
    await expect
      .element(page.getByRole('menuitem', { name: 'Open printer' }))
      .toBeVisible();
    expect(
      page.getByRole('menuitem', { name: /Print again/ }).elements(),
    ).toHaveLength(0);
  });

  // Nothing wired to take the row is the one case with no item at all.
  it('holds the printer link alone where nothing can send a file', async () => {
    await render(JobsScreen, { props: { jobs: jobsRead(), store } });

    await menu('Bracket_Left_Reinforced_v4_FINAL.gcode');
    await expect
      .element(page.getByRole('menuitem', { name: 'Open printer' }))
      .toHaveAttribute('href', '/printers/prn-rack-3');
    expect(
      page.getByRole('menuitem', { name: /Print again/ }).elements(),
    ).toHaveLength(0);
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(JobsScreen, {
      props: { jobs: jobsRead(), store },
    });

    await expectNoViolations(container);
  });
});
