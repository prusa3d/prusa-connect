import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { START_PRINT_VERB } from '../../data/services/print';
import { StubApiError } from '../../data/stubs';
import type {
  Device,
  PrintOutcome,
  PrintRequest,
  StateTransition,
} from '../../domain';
import { STATE_PATH } from '../../domain';
import { expectNoViolations } from '../../test/axe';
import { fakeDevice } from '../../test/fakes';
import PrintDialogOpener from '../../test/print-dialog-opener.svelte';
import PrintDialogWithSlots from '../../test/print-dialog-with-slots.svelte';
import PrintSectionProbe from '../../test/print-section-probe.svelte';
import { SlotRegistry } from '../../slots/registry.svelte';
import PrintDialog from './print-dialog.svelte';
import type { PrintableFile, PrintDialogProps } from './types';

const printerOf = (
  id: string,
  name: string,
  state: string,
  over: Partial<Device> = {},
): Device => ({
  ...fakeDevice(id, name),
  capabilities: [START_PRINT_VERB],
  state: { [STATE_PATH]: state },
  ...over,
});

const file: PrintableFile = {
  details: {
    estimated_seconds: 5400,
    filament_g: 24.5,
    material: 'PLA',
    printer_model: 'MK4IS',
  },
  name: 'bracket.bgcode',
  source: 'models/bracket.bgcode',
  storage: 'local',
};

const started: PrintOutcome = {
  commandId: 'cmd-1',
  faultCode: undefined,
  machineReason: undefined,
  ok: true,
  path: 'bracket.bgcode',
  transferId: 'transfer-1',
};

const props = (over: Partial<PrintDialogProps> = {}): PrintDialogProps => ({
  devices: [
    printerOf('idle-one', 'MK4S #1042', 'IDLE'),
    printerOf('busy-one', 'MK4S #1043', 'PRINTING'),
  ],
  file,
  onOpenChange: () => undefined,
  onPrint: () => Promise.resolve(started),
  open: true,
  ...over,
});

const body = () => page.elementLocator(document.body);

// The button names how many printers it sends to once one is ticked.
const printButton = () =>
  body().getByRole('button', { name: /^Print( on \d+ printers?)?$/ });

/** A promise the test resolves, so an answer can be made to arrive late. */
const deferred = <T>() => {
  let settle: (value: T) => void = () => undefined;
  const promise = new Promise<T>((resolve) => {
    settle = resolve;
  });

  return {
    promise,
    settle: (value: T) => {
      settle(value);
    },
  };
};

describe('PrintDialog', () => {
  it('names the file and what the slicer said about the print', async () => {
    await render(PrintDialog, { props: props() });

    await expect.element(body().getByText('bracket.bgcode')).toBeVisible();
    await expect.element(body().getByText('Estimated time')).toBeVisible();
    await expect.element(body().getByText('PLA')).toBeVisible();
    await expect.element(body().getByText('MK4IS')).toBeVisible();
  });

  it('discards a replacement request when the open file changes', async () => {
    const onPrint = vi.fn(() =>
      Promise.reject(new StubApiError('core.printer/FILE_NAME_TAKEN', 409)),
    );
    const { rerender } = await render(PrintDialog, {
      props: props({ onPrint }),
    });

    await printButton().click();
    await expect
      .element(body().getByRole('button', { name: 'Replace' }))
      .toBeVisible();

    await rerender({
      file: { ...file, name: 'other.bgcode', source: 'models/other.bgcode' },
    });
    await expect
      .element(body().getByRole('button', { name: 'Replace' }))
      .not.toBeInTheDocument();
    await expect.element(body().getByText('other.bgcode')).toBeVisible();
    expect(onPrint).toHaveBeenCalledTimes(1);
  });

  it('reports printers that become unavailable before replacement', async () => {
    const devices = [
      printerOf('idle-one', 'MK4S #1042', 'IDLE'),
      printerOf('idle-two', 'MK4S #1043', 'IDLE'),
    ];
    const onPrinted = vi.fn();
    const onPrint = vi.fn((request: PrintRequest) =>
      request.replace === true
        ? Promise.resolve(started)
        : Promise.reject(new StubApiError('core.printer/FILE_NAME_TAKEN', 409)),
    );
    const { rerender } = await render(PrintDialog, {
      props: props({ devices, onPrint, onPrinted }),
    });
    await body().getByRole('checkbox', { name: 'MK4S #1042' }).click();
    await body().getByRole('checkbox', { name: 'MK4S #1043' }).click();
    await printButton().click();
    const replace = body().getByRole('button', { name: 'Replace' });
    await expect.element(replace).toBeVisible();
    await rerender({
      devices: [
        printerOf('idle-one', 'MK4S #1042', 'IDLE'),
        printerOf('idle-two', 'MK4S #1043', 'PRINTING'),
      ],
    });
    await expect
      .element(body().getByText('Only 1 printer can replace the file now.'))
      .toBeVisible();
    await replace.click();
    await vi.waitFor(() => {
      expect(onPrint).toHaveBeenCalledTimes(3);
    });
    expect(onPrint.mock.calls[2]?.[0]).toMatchObject({
      deviceId: 'idle-one',
      replace: true,
    });
    await expect.element(body().getByRole('dialog')).toBeVisible();
    expect(onPrinted).not.toHaveBeenCalled();
  });

  it('draws no facts for a file that said nothing', async () => {
    await render(PrintDialog, {
      props: props({ file: { ...file, details: undefined } }),
    });

    expect(body().getByText('Estimated time').elements()).toHaveLength(0);
  });

  it('offers a waiting printer and blocks a busy one, with the reason', async () => {
    await render(PrintDialog, { props: props() });

    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1042/ }))
      .not.toBeDisabled();
    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1043/ }))
      .toBeDisabled();
    await expect
      .element(body().getByText('This printer is printing.'))
      .toBeVisible();
  });

  // A finished print is still on the bed. The reason has to say that, not
  // that the machine is unavailable.
  it('says a finished print is still on the bed', async () => {
    await render(PrintDialog, {
      props: props({
        devices: [printerOf('done-one', 'MK4S #1044', 'FINISHED')],
      }),
    });

    await expect
      .element(
        body().getByText('This printer has a finished print on its bed.'),
      )
      .toBeVisible();
    await expect.element(printButton()).toBeDisabled();
  });

  // The kind that declares its own moves is the authority on its own machine.
  it('names the states a device kind allows a print to start from', async () => {
    const transitions: StateTransition[] = [
      { from: 'IDLE', to: 'PRINTING', verb: START_PRINT_VERB },
    ];
    await render(PrintDialog, {
      props: props({
        devices: [printerOf('ready-one', 'Echo #1', 'READY')],
        transitions,
      }),
    });

    await expect
      .element(body().getByText('This works when the printer is IDLE.'))
      .toBeVisible();
  });

  it('sends the file to the printer that was picked', async () => {
    const sent: PrintRequest[] = [];
    await render(PrintDialog, {
      props: props({
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.resolve(started);
        },
      }),
    });

    await printButton().click();

    expect(sent).toEqual([
      {
        deviceId: 'idle-one',
        path: 'bracket.bgcode',
        source: 'models/bracket.bgcode',
        storage: 'local',
      },
    ]);
  });

  // A file the printer already holds, which is what Print again sends, names
  // no storage and is started under its own path on the machine.
  it('sends a file the printer holds by its path alone', async () => {
    const sent: PrintRequest[] = [];
    await render(PrintDialog, {
      props: props({
        file: { name: 'bracket.bgcode', source: 'usb/bracket.bgcode' },
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.resolve(started);
        },
      }),
    });

    await printButton().click();

    expect(sent).toEqual([
      { deviceId: 'idle-one', path: 'usb/bracket.bgcode' },
    ]);
  });

  it('takes the reader to the printer once it has the file', async () => {
    const onPrinted = vi.fn();
    const onOpenChange = vi.fn();
    await render(PrintDialog, { props: props({ onOpenChange, onPrinted }) });

    await printButton().click();

    await expect.poll(() => onPrinted.mock.calls).toEqual([[['idle-one']]]);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  // One free printer is picked for the reader, because there is nothing to
  // decide. Two are not: guessing which machine a print goes to is the one
  // guess this panel must not make.
  it('picks nothing when more than one printer is free', async () => {
    const onPrint = vi.fn();
    await render(PrintDialog, {
      props: props({
        devices: [
          printerOf('idle-one', 'MK4S #1042', 'IDLE'),
          printerOf('idle-two', 'MK4S #1043', 'IDLE'),
        ],
        onPrint,
      }),
    });

    await expect.element(printButton()).toBeDisabled();

    await body()
      .getByRole('checkbox', { name: /MK4S #1043/ })
      .click();
    await expect.element(printButton()).not.toBeDisabled();
    expect(onPrint).not.toHaveBeenCalled();
  });

  it('sends the file to every printer ticked', async () => {
    const sent: string[] = [];
    const onPrinted = vi.fn();
    await render(PrintDialog, {
      props: props({
        devices: [
          printerOf('idle-one', 'MK4S #1042', 'IDLE'),
          printerOf('idle-two', 'MK4S #1043', 'IDLE'),
        ],
        onPrint: (request: PrintRequest) => {
          sent.push(request.deviceId);

          return Promise.resolve(started);
        },
        onPrinted,
      }),
    });

    await body()
      .getByRole('checkbox', { name: /MK4S #1042/ })
      .click();
    await body()
      .getByRole('checkbox', { name: /MK4S #1043/ })
      .click();
    await printButton().click();

    await expect
      .poll(() => onPrinted.mock.calls)
      .toEqual([[['idle-one', 'idle-two']]]);
    expect(sent).toEqual(['idle-one', 'idle-two']);
  });

  // One printer refusing must not hide that the others started, and a second
  // press must not send the file again to a printer already printing it.
  it('stays open on the printers that failed and unticks the ones that started', async () => {
    const onPrinted = vi.fn();
    await render(PrintDialog, {
      props: props({
        devices: [
          printerOf('idle-one', 'MK4S #1042', 'IDLE'),
          printerOf('idle-two', 'MK4S #1043', 'IDLE'),
        ],
        onPrint: (request: PrintRequest) =>
          Promise.resolve(
            request.deviceId === 'idle-one'
              ? started
              : {
                  ...started,
                  faultCode: 'core.printer/COMMAND_REFUSED',
                  machineReason: 'printer is busy',
                  ok: false,
                },
          ),
        onPrinted,
      }),
    });

    await body()
      .getByRole('checkbox', { name: /MK4S #1042/ })
      .click();
    await body()
      .getByRole('checkbox', { name: /MK4S #1043/ })
      .click();
    await printButton().click();

    await expect.element(body().getByText('printer is busy')).toBeVisible();
    // The failure names its printer above the row that names it too.
    await expect
      .poll(
        () => body().getByText('MK4S #1043', { exact: true }).elements().length,
      )
      .toBe(2);
    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1042/ }))
      .not.toBeChecked();
    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1043/ }))
      .toBeChecked();
    expect(onPrinted).not.toHaveBeenCalled();
  });

  // A printer holding a different file under the name is a question, asked
  // once for every such printer, and only those are sent to again.
  it('asks once before replacing the file on the printers that hold the name', async () => {
    const sent: PrintRequest[] = [];
    const onPrinted = vi.fn();
    await render(PrintDialog, {
      props: props({
        devices: [
          printerOf('idle-one', 'MK4S #1042', 'IDLE'),
          printerOf('idle-two', 'MK4S #1043', 'IDLE'),
        ],
        onPrint: (request: PrintRequest) => {
          sent.push(request);
          if (request.deviceId === 'idle-two' && request.replace !== true) {
            return Promise.reject(
              new StubApiError('core.printer/FILE_NAME_TAKEN', 409),
            );
          }

          return Promise.resolve(started);
        },
        onPrinted,
      }),
    });

    await body()
      .getByRole('checkbox', { name: /MK4S #1042/ })
      .click();
    await body()
      .getByRole('checkbox', { name: /MK4S #1043/ })
      .click();
    await printButton().click();

    const question = body().getByRole('alertdialog');
    await expect.element(question).toBeVisible();
    // The name reaches the line inside bidi isolates, which are left out here.
    const asked = question
      .element()
      .textContent.replaceAll(/[\u2066-\u2069]/gu, '');
    expect(asked).toContain(
      'A different file named bracket.bgcode is already on this printer. Replace it?',
    );
    expect(
      body().getByText('core.printer/FILE_NAME_TAKEN').elements(),
    ).toHaveLength(0);

    await question.getByRole('button', { name: 'Replace' }).click();

    await expect.poll(() => onPrinted.mock.calls).toEqual([[['idle-two']]]);
    expect(sent.map((one) => [one.deviceId, one.replace])).toEqual([
      ['idle-one', undefined],
      ['idle-two', undefined],
      ['idle-two', true],
    ]);
  });

  it('keeps the printer and its file when the person cancels the question', async () => {
    const sent: PrintRequest[] = [];
    await render(PrintDialog, {
      props: props({
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.reject(
            new StubApiError('core.printer/FILE_NAME_TAKEN', 409),
          );
        },
      }),
    });

    await printButton().click();
    const question = body().getByRole('alertdialog');
    await question.getByRole('button', { name: 'Cancel' }).click();

    await expect.element(question).not.toBeInTheDocument();
    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1042/ }))
      .toBeChecked();
    expect(sent).toHaveLength(1);
  });

  // The model is a warning only: a close model may print the file well.
  it('warns about a printer of another model than the file was sliced for', async () => {
    await render(PrintDialog, {
      props: props({
        devices: [
          printerOf('idle-one', 'MK4S #1042', 'IDLE', { model: 'MK4IS' }),
          printerOf('idle-two', 'XL #7', 'IDLE', { model: 'XL' }),
        ],
      }),
    });

    await expect
      .element(body().getByText('This file was sliced for MK4IS.'))
      .toBeVisible();
    expect(
      body().getByText('This file was sliced for MK4IS.').elements(),
    ).toHaveLength(1);
    await expect
      .element(body().getByRole('checkbox', { name: /XL #7/ }))
      .not.toBeDisabled();
  });

  // The fleet is live. A machine that becomes free while the panel is open
  // must not take the pick away from the machine the reader chose, and must
  // not become the pick by itself.
  it('keeps the pick the reader made while the fleet moves under it', async () => {
    const sent: PrintRequest[] = [];
    const onPrint = (request: PrintRequest) => {
      sent.push(request);

      return Promise.resolve(started);
    };
    const { rerender } = await render(PrintDialog, {
      props: props({
        devices: [printerOf('idle-one', 'MK4S #1042', 'IDLE')],
        onPrint,
      }),
    });

    // Another machine finishes and sorts above the picked one by name.
    await rerender({
      devices: [
        printerOf('idle-one', 'MK4S #1042', 'IDLE'),
        printerOf('free-two', 'AAA #1', 'IDLE'),
      ],
    });
    await printButton().click();

    await expect
      .poll(() => sent.map((one) => one.deviceId))
      .toEqual(['idle-one']);
  });

  // An answer that arrives after the reader shut the panel belongs to a send
  // they have left behind. It must not open a printer's page, and it must not
  // write into the panel the next file opens.
  it('ignores an answer that arrives after the panel was shut', async () => {
    const late = deferred<PrintOutcome>();
    const onPrinted = vi.fn();
    const onOpenChange = vi.fn();
    const { rerender } = await render(PrintDialog, {
      props: props({
        onOpenChange,
        onPrint: () => late.promise,
        onPrinted,
      }),
    });

    await printButton().click();
    await rerender({ open: false });
    late.settle(started);
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(onPrinted).not.toHaveBeenCalled();
  });

  it('carries no refusal from one send into the next panel', async () => {
    const late = deferred<PrintOutcome>();
    const { rerender } = await render(PrintDialog, {
      props: props({ onPrint: () => late.promise }),
    });

    await printButton().click();
    await rerender({ open: false });
    late.settle({
      commandId: 'cmd-2',
      faultCode: 'core.printer/COMMAND_REFUSED',
      machineReason: 'printer is busy',
      ok: false,
      path: 'bracket.bgcode',
      transferId: undefined,
    });
    await new Promise((resolve) => setTimeout(resolve, 20));
    await rerender({ open: true });

    expect(
      body().getByText('core.printer/COMMAND_REFUSED').elements(),
    ).toHaveLength(0);
    await expect.element(printButton()).not.toBeDisabled();
  });

  // The printer answered and did not take the command. That is a 200 with
  // `ok` false, so a caller that read the status alone would call it a
  // success.
  it('stays open and names the code when the printer refuses', async () => {
    const onPrinted = vi.fn();
    await render(PrintDialog, {
      props: props({
        onPrint: () =>
          Promise.resolve({
            commandId: 'cmd-2',
            faultCode: 'core.printer/COMMAND_REFUSED',
            machineReason: 'printer is busy',
            ok: false,
            path: 'bracket.bgcode',
            transferId: undefined,
          }),
        onPrinted,
      }),
    });

    await printButton().click();

    await expect
      .element(body().getByText('core.printer/COMMAND_REFUSED'))
      .toBeVisible();
    await expect.element(body().getByText('printer is busy')).toBeVisible();
    expect(onPrinted).not.toHaveBeenCalled();
  });

  // The registry says what to do about each code, and a refusal a reader
  // cannot act on is half an answer.
  it('says what the reader can do about the refusal', async () => {
    await render(PrintDialog, {
      props: props({
        onPrint: () =>
          Promise.reject(
            Object.assign(new Error('the printer is already fetching a file'), {
              code: 'core.printer/TRANSFER_BUSY',
            }),
          ),
      }),
    });

    await printButton().click();

    await expect
      .element(body().getByText('core.printer/TRANSFER_BUSY'))
      .toBeVisible();
    await expect
      .element(body().getByText(/Wait for the transfer in flight to finish/))
      .toBeVisible();
  });

  // The thumbnail is the first control in the panel. Focus landing on it would
  // open its preview over the printers the reader came to pick from.
  it('opens on the first free printer with no preview over it', async () => {
    await render(PrintDialogOpener, {
      props: {
        devices: [
          printerOf('busy-one', 'MK4S #1043', 'PRINTING'),
          printerOf('idle-one', 'MK4S #1042', 'IDLE'),
          printerOf('idle-two', 'MK4S #1044', 'IDLE'),
        ],
        file: {
          ...file,
          thumbnailUrl:
            'data:image/svg+xml;utf8,' +
            encodeURIComponent(
              '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"/>',
            ),
        },
        onPrint: () => Promise.resolve(started),
      },
    });

    await body().getByRole('button', { name: 'Print this file' }).click();

    await expect
      .element(body().getByRole('button', { name: /Preview of/ }))
      .toBeVisible();

    await expect
      .element(body().getByRole('checkbox', { name: /MK4S #1042/ }))
      .toHaveFocus();
    expect(
      document.body.querySelector('[data-slot="popover-content"]'),
    ).toBeNull();
  });

  it('says so when no printer can start a print, and sends nothing', async () => {
    const onPrint = vi.fn();
    await render(PrintDialog, {
      props: props({
        devices: [printerOf('busy-one', 'MK4S #1043', 'PRINTING')],
        onPrint,
      }),
    });

    await expect
      .element(
        body().getByText(
          'No printer on this server can start a print right now.',
        ),
      )
      .toBeVisible();
    await expect.element(printButton()).toBeDisabled();
    expect(onPrint).not.toHaveBeenCalled();
  });

  // The one free printer is ticked when the panel opens, and the tick stays
  // the reader's to undo. So a printer that goes into ERROR under an open
  // panel is still in the set, and a button that read the size of the set was
  // pressable beside the line saying nothing can start a print.
  it('disables Print when the one printer ticked stops being free', async () => {
    const onPrint = vi.fn();
    const { rerender } = await render(PrintDialog, {
      props: props({
        devices: [printerOf('idle-one', 'MK4S #1042', 'IDLE')],
        onPrint,
      }),
    });

    await expect.element(printButton()).toBeEnabled();

    await rerender({ devices: [printerOf('idle-one', 'MK4S #1042', 'ERROR')] });

    await expect
      .element(
        body().getByText(
          'No printer on this server can start a print right now.',
        ),
      )
      .toBeVisible();
    await expect.element(printButton()).toBeDisabled();
    expect(onPrint).not.toHaveBeenCalled();
  });

  // A farm's worth of printers is longer than any screen. Without a cap on
  // the panel and a scroll region inside it, the Print button sits under the
  // fold and the reader cannot reach it at all.
  it('keeps the Print button on the screen with a long list of printers', async () => {
    const many = Array.from({ length: 40 }, (_, at) => {
      const name = String(at);

      return printerOf(
        `p${name}`,
        `MK4S #${name}`,
        at === 0 ? 'IDLE' : 'PRINTING',
      );
    });
    await render(PrintDialog, { props: props({ devices: many }) });

    const button = printButton().element();
    const box = button.getBoundingClientRect();

    expect(box.bottom).toBeLessThanOrEqual(window.innerHeight);
    expect(box.top).toBeGreaterThanOrEqual(0);
  });

  it('has nothing axe objects to', async () => {
    await render(PrintDialog, { props: props() });

    await expectNoViolations(document.body);
  });

  describe('a module section', () => {
    const registryWithSection = (): SlotRegistry => {
      const registry = new SlotRegistry();
      registry.declare('planner', {
        print_section: [{ id: 'later', title: 'Print later' }],
      });
      registry.registerSection(
        'planner',
        'print-section',
        'later',
        PrintSectionProbe,
      );

      return registry;
    };

    it('draws below the printers with the file reference and the pick', async () => {
      await render(PrintDialogWithSlots, {
        props: { ...props(), registry: registryWithSection() },
      });

      const section = page.getByTestId('print-section');
      await expect
        .element(section)
        .toHaveTextContent('local:models/bracket.bgcode for idle-one');
      await expect
        .element(page.getByRole('heading', { name: 'Print later' }))
        .toBeInTheDocument();
    });

    it('closes the dialog when the section asks', async () => {
      const onOpenChange = vi.fn();
      await render(PrintDialogWithSlots, {
        props: { ...props({ onOpenChange }), registry: registryWithSection() },
      });

      await page.getByRole('button', { name: 'Close from section' }).click();

      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    // An operator hides the section (ADR-20261021-3dp): it registers and is not drawn.
    it('is not drawn when the operator hides it', async () => {
      const registry = registryWithSection();
      registry.hide('planner', ['print_section/later']);
      await render(PrintDialogWithSlots, {
        props: { ...props(), registry },
      });

      await expect.element(page.getByRole('dialog')).toBeInTheDocument();
      expect(page.getByTestId('print-section').query()).toBeNull();
    });

    // A file already on the printer is not a file of this server's storage,
    // so there is no reference to hand a module.
    it('is not drawn for a file the printer holds', async () => {
      await render(PrintDialogWithSlots, {
        props: {
          ...props({ file: { ...file, storage: undefined } }),
          registry: registryWithSection(),
        },
      });

      await expect.element(page.getByRole('dialog')).toBeInTheDocument();
      expect(page.getByTestId('print-section').query()).toBeNull();
    });
  });
});
