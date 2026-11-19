import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { SendOutcome } from '../../../data';
import type { Device } from '../../../domain';
import { toast } from '../../../primitives';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import PrinterFilesPanel from './printer-files-panel.svelte';
import type {
  PrinterFileEntry,
  PrinterFilesCommandPort,
  PrinterFilesPanelProps,
  PrinterFilesSource,
} from './types';

const entry = (
  over: Partial<PrinterFileEntry> & Pick<PrinterFileEntry, 'name' | 'path'>,
): PrinterFileEntry => ({
  folder: false,
  modified: '2026-09-12T10:30:00Z',
  read_only: false,
  size: 4_182_016,
  ...over,
});

const FOLDER = entry({
  folder: true,
  name: 'models',
  path: 'usb/models',
  size: 0,
});
const PLATE = entry({ name: 'bracket.bgcode', path: 'usb/bracket.bgcode' });
/** What the folder holds, which only an expanded row lists. */
const INSIDE = entry({ name: 'cube.bgcode', path: 'usb/models/cube.bgcode' });
const SETTINGS = entry({
  name: 'prusa_printer_settings.ini',
  path: 'usb/prusa_printer_settings.ini',
  size: 2048,
});

/** What the empty path answers: the drives, which the tab opens inside. */
const DRIVES = [
  entry({ folder: true, name: 'usb', path: 'usb', size: 0 }),
  entry({ folder: true, name: 'sd', path: 'sd', size: 0 }),
];

// A source whose expanded folders behave like the store's: expanding one
// lists what the fixture holds under it.
const source = (over: Partial<PrinterFilesSource> = {}): PrinterFilesSource => {
  let expanded = $state.raw<
    Record<string, readonly PrinterFileEntry[] | undefined>
  >({});

  return {
    directory: 'usb',
    entries: [FOLDER, PLATE, SETTINGS],
    filesError: undefined,
    filesLoaded: true,
    filesLoading: false,
    filesReadingSince: undefined,
    filesUnread: false,
    filesRefreshing: false,
    get expanded() {
      return expanded;
    },
    expand: (path: string) => {
      expanded = { ...expanded, [path]: [INSIDE] };

      return Promise.resolve();
    },
    collapse: (path: string) => {
      expanded = Object.fromEntries(
        Object.entries(expanded).filter(([held]) => held !== path),
      );
    },
    changedAt: () => Promise.resolve(),
    load: () => Promise.resolve(),
    openFolder: () => Promise.resolve(),
    ...over,
  };
};

// A printer that will take both file verbs and is standing still.
const printer = (): Device => ({
  ...fakeDevice('printer-1', 'MK4S'),
  capabilities: ['core.command.start_print', 'core.command.delete_file'],
  state: { '/state': 'IDLE' },
});

const props = (
  over: Partial<PrinterFilesPanelProps> = {},
): PrinterFilesPanelProps => ({
  device: printer(),
  files: source(),
  commands: { onCommand: () => Promise.resolve({ kind: 'accepted' as const }) },
  ...over,
});

// A port whose sends wait until the test answers them, oldest first.
const heldPort = (): {
  port: PrinterFilesCommandPort;
  onCommand: ReturnType<typeof vi.fn>;
  answer: (outcome: SendOutcome) => Promise<void>;
} => {
  const waiting: ((outcome: SendOutcome) => void)[] = [];
  const onCommand = vi.fn(
    () =>
      new Promise<SendOutcome>((resolve) => {
        waiting.push(resolve);
      }),
  );

  return {
    port: { onCommand },
    onCommand,
    answer: async (outcome) => {
      await vi.waitFor(() => {
        expect(waiting.length).toBeGreaterThan(0);
      });
      waiting.shift()?.(outcome);
    },
  };
};

// A printer that offers both delete verbs.
const deleter = (): Device => ({
  ...printer(),
  capabilities: ['core.command.delete_file', 'core.command.delete_folder'],
});

// Ticks two files and confirms their delete.
const deleteBoth = async (): Promise<void> => {
  await page.getByRole('checkbox', { name: 'Select bracket.bgcode' }).click();
  await page
    .getByRole('checkbox', { name: 'Select prusa_printer_settings.ini' })
    .click();
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Delete', exact: true })
    .click();
};

describe('PrinterFilesPanel', () => {
  // Wide enough for every column and every row action; a phone-width pane
  // folds them, which the narrow cases below set up for themselves.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  it('draws one row per entry, with the size and the date formatted', async () => {
    const { container } = await render(PrinterFilesPanel, { props: props() });

    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();
    // 4182016 bytes is four mebibytes, and nothing in the panel writes the unit.
    await expect.element(page.getByText('4.0 MB')).toBeVisible();
    expect(container.textContent).toContain('Sep 12, 2026');
  });

  it('lists the folders first, then the files by name', async () => {
    const files = source({
      entries: [
        {
          folder: false,
          name: 'a-fw.bbf',
          path: 'usb/a-fw.bbf',
          read_only: false,
          size: 1024,
        },
        PLATE,
        FOLDER,
        entry({
          name: 'z-new.bgcode',
          path: 'usb/z-new.bgcode',
          modified: '2026-09-20T10:30:00Z',
        }),
      ],
    });
    const { container } = await render(PrinterFilesPanel, {
      props: props({ files }),
    });

    await expect.element(page.getByText('z-new.bgcode')).toBeVisible();
    const names = [...container.querySelectorAll('tbody tr')].map(
      (row) => row.textContent,
    );

    expect(
      names.map((text) => /models|z-new|bracket|a-fw/.exec(text)?.[0]),
    ).toEqual(['models', 'a-fw', 'bracket', 'z-new']);
  });

  // The page around the panel already draws a breadcrumb.
  it('names its folder trail something the page trail is not called', async () => {
    const { container } = await render(PrinterFilesPanel, { props: props() });

    const trail = container.querySelector('[data-slot="breadcrumb"]');
    expect(trail?.getAttribute('aria-label')).toBe('Folders on this printer');
    await expectNoViolations(container);
  });

  it('opens the folder a row is pressed on', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    await render(PrinterFilesPanel, {
      props: props({ files: source({ openFolder }) }),
    });

    await page.getByRole('button', { name: 'models', exact: true }).click();
    expect(openFolder).toHaveBeenCalledWith('usb/models');
  });

  // A narrow pane used to pin the preview to one width, so the slider moved
  // and nothing changed.
  it('resizes the file previews with the thumbnail slider in a narrow pane', async () => {
    await page.viewport(640, 900);
    const { container } = await render(PrinterFilesPanel, { props: props() });
    const width = () =>
      container.querySelector('.aspect-video')?.getBoundingClientRect().width ??
      0;
    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();
    const before = width();

    await page.getByRole('slider', { name: 'Thumbnail size' }).click();
    await userEvent.keyboard('{End}');

    await vi.waitFor(() => {
      expect(width()).toBeGreaterThan(before);
    });
  });

  // A list preview is hidden on a phone-width pane, so the slider that sizes
  // it goes too.
  it('hides the thumbnail slider where the list previews are hidden', async () => {
    await page.viewport(375, 900);
    const { container } = await render(PrinterFilesPanel, { props: props() });
    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();

    const slider = container.querySelector('[data-slot="slider"]');
    expect(slider).not.toBeNull();
    expect(slider?.getClientRects()).toHaveLength(0);
  });

  // The trail is rooted at the drive.
  it('climbs back through the trail', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    await render(PrinterFilesPanel, {
      props: props({
        files: source({ directory: 'usb/models', openFolder }),
      }),
    });

    await page.getByRole('button', { name: 'usb' }).click();
    expect(openFolder).toHaveBeenCalledWith('usb');
  });

  // The core's listing carries no child count.
  it('expands a folder under its own row and counts what it listed', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    const files = source({ openFolder });
    const expand = vi.spyOn(files, 'expand');
    await render(PrinterFilesPanel, {
      props: props({ files }),
    });

    expect(page.getByText('1 item').elements()).toHaveLength(0);
    await expect
      .element(page.getByRole('button', { name: 'Expand models' }))
      .toHaveAttribute('aria-expanded', 'false');

    await page.getByRole('button', { name: 'Expand models' }).click();
    expect(expand).toHaveBeenCalledWith('usb/models');
    await expect.element(page.getByText('cube.bgcode')).toBeVisible();
    await expect.element(page.getByText('1 item')).toBeVisible();
    expect(openFolder).not.toHaveBeenCalled();

    await page.getByRole('button', { name: 'Collapse models' }).click();
    expect(page.getByText('cube.bgcode').elements()).toHaveLength(0);
  });

  // The bulk bar counts what is on screen.
  it('picks a file inside an expanded folder and drops it when it folds', async () => {
    await render(PrinterFilesPanel, { props: props() });

    await page.getByRole('button', { name: 'Expand models' }).click();
    await page.getByRole('checkbox', { name: 'Select cube.bgcode' }).click();
    await expect.element(page.getByText('1 selected')).toBeVisible();

    await page.getByRole('button', { name: 'Collapse models' }).click();
    expect(page.getByText('1 selected').elements()).toHaveLength(0);
  });

  // No driver declares a verb that downloads or moves a printer's file, and
  // a verb a printer does not offer has no control at all.
  it('offers no Download and no Move on a selection', async () => {
    await render(PrinterFilesPanel, { props: props() });

    await page.getByRole('button', { name: 'Expand models' }).click();
    await page.getByRole('checkbox', { name: 'Select cube.bgcode' }).click();
    await expect.element(page.getByText('1 selected')).toBeVisible();

    await expect
      .element(page.getByRole('button', { name: 'Download' }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Move to…' }))
      .not.toBeInTheDocument();
  });

  it('asks before it deletes, then sends the verb with the path', async () => {
    const onCommand = vi.fn(() =>
      Promise.resolve({ kind: 'accepted' as const }),
    );
    await render(PrinterFilesPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await page.getByRole('menuitem', { name: 'Delete' }).click();
    expect(onCommand).not.toHaveBeenCalled();

    await page.getByRole('button', { name: 'Delete' }).click();
    expect(onCommand).toHaveBeenCalledWith(
      'core.command.delete_file',
      { path: 'usb/bracket.bgcode' },
      { quiet: true },
    );
  });

  // A delete changes what the folder holds, and only the printer can say
  // what it holds now.
  // A range or "Select all" sweeps folders in, and a folder goes with all it
  // holds, so the confirmation counts the two apart.
  it('counts the files and the folders a bulk delete takes', async () => {
    await render(PrinterFilesPanel, { props: props({ device: deleter() }) });

    await page.getByRole('checkbox', { name: 'Select models' }).click();
    await page.getByRole('checkbox', { name: 'Select bracket.bgcode' }).click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();

    await expect
      .element(page.getByText('Delete 1 file and 1 folder?'))
      .toBeVisible();
  });

  it('reads the folder again once the printer answers a delete', async () => {
    const changedAt = vi.fn(() => Promise.resolve());
    let answer: (outcome: SendOutcome) => void = () => undefined;
    const onCommand = vi.fn(
      () =>
        new Promise<SendOutcome>((resolve) => {
          answer = resolve;
        }),
    );
    await render(PrinterFilesPanel, {
      props: props({ files: source({ changedAt }), commands: { onCommand } }),
    });

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await page.getByRole('menuitem', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Delete' }).click();
    expect(changedAt).not.toHaveBeenCalled();

    answer({ kind: 'accepted' });
    await vi.waitFor(() => {
      expect(changedAt).toHaveBeenCalledWith('usb/bracket.bgcode');
    });
  });

  it('keeps its rows and says it is busy while the folder is read again', async () => {
    const { container } = await render(PrinterFilesPanel, {
      props: props({ files: source({ filesRefreshing: true }) }),
    });

    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
  });

  // The printer takes one command at a time. Sent together, all but the
  // first would be dropped.
  it('deletes a selection one file after another and names what failed', async () => {
    const error = vi.spyOn(toast, 'error').mockReturnValue('');
    const held = heldPort();
    await render(PrinterFilesPanel, {
      props: props({ device: deleter(), commands: held.port }),
    });

    await deleteBoth();

    await held.answer({
      kind: 'refused',
      fault: {
        code: 'core.printer/FILE_CHANGE_REFUSED',
        traceId: undefined,
        status: undefined,
        params: {},
        message: 'no such file',
      },
    });
    await held.answer({ kind: 'unsent' });

    // The refusal names the printer and the file. Under it are what the error
    // code means, the printer's words, what to do and the code, each apart.
    await vi.waitFor(() => {
      expect(error).toHaveBeenCalledWith(
        'MK4S did not delete bracket.bgcode.',
        expect.objectContaining({
          componentProps: {
            title: 'The printer refused that change to its storage',
            reasons: [{ detail: 'no such file', names: 'bracket.bgcode' }],
            action: expect.stringContaining(
              'Read the machine reason',
            ) as unknown,
            code: 'core.printer/FILE_CHANGE_REFUSED',
          },
        }),
      );
    });
    expect(error).toHaveBeenCalledWith(
      'MK4S did not delete prusa_printer_settings.ini, because another command was still being sent to it. Try again.',
    );
    // The page's announcer would name each refusal too, so the deletes are
    // sent quiet and the panel's summary is the only word about them.
    expect(held.onCommand).toHaveBeenCalledWith(
      'core.command.delete_file',
      { path: 'usb/bracket.bgcode' },
      { quiet: true },
    );
    error.mockRestore();
  });

  // Each error code is its own toast, and each file keeps the reason the
  // printer gave for it.
  it('raises one toast per error code and keeps each reason', async () => {
    const error = vi.spyOn(toast, 'error').mockReturnValue('');
    const held = heldPort();
    await render(PrinterFilesPanel, {
      props: props({ device: deleter(), commands: held.port }),
    });

    await page.getByRole('checkbox', { name: 'Select models' }).click();
    await deleteBoth();

    const refusal = (code: string, message: string) => ({
      kind: 'refused' as const,
      fault: {
        code,
        status: undefined,
        params: {},
        message,
        traceId: undefined,
      },
    });
    await held.answer(refusal('core.printer/FILE_CHANGE_REFUSED', 'busy'));
    await held.answer(
      refusal('core.printer/FILE_CHANGE_REFUSED', 'no such file'),
    );
    await held.answer(refusal('core.authz/FORBIDDEN', 'not allowed'));

    await vi.waitFor(() => {
      expect(error).toHaveBeenCalledTimes(2);
    });
    const props0 = (
      error.mock.calls[0]?.[1] as {
        componentProps: { reasons: unknown; code: string };
      }
    ).componentProps;
    expect(props0.code).toBe('core.printer/FILE_CHANGE_REFUSED');
    expect(props0.reasons).toEqual([
      { detail: 'busy', names: 'models' },
      { detail: 'no such file', names: 'bracket.bgcode' },
    ]);
    const props1 = (
      error.mock.calls[1]?.[1] as {
        componentProps: { code: string };
      }
    ).componentProps;
    expect(props1.code).toBe('core.authz/FORBIDDEN');
    error.mockRestore();
  });

  // The deletes belong to the printer they were confirmed on. A page that
  // switches printer halfway must not send the rest to the new one.
  it('sends the rest of a delete to the printer it was confirmed on', async () => {
    const first = heldPort();
    const second = heldPort();
    const { rerender } = await render(PrinterFilesPanel, {
      props: props({ device: deleter(), commands: first.port }),
    });

    await deleteBoth();
    await rerender(props({ device: deleter(), commands: second.port }));
    await first.answer({ kind: 'accepted' });
    await first.answer({ kind: 'accepted' });

    expect(first.onCommand).toHaveBeenCalledTimes(2);
    expect(second.onCommand).not.toHaveBeenCalled();
  });

  // The extension is the only thing that tells a plate from a file sitting.
  it('offers Print on a plate and not on a settings file', async () => {
    await render(PrinterFilesPanel, {
      props: props({ files: source({ entries: [PLATE, SETTINGS] }) }),
    });

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await expect
      .element(page.getByRole('menuitem', { name: 'Print' }))
      .toBeVisible();
    await page.getByRole('menuitem', { name: 'Print' }).click();

    await page
      .getByRole('button', { name: 'Actions for prusa_printer_settings.ini' })
      .click();
    expect(
      document.querySelectorAll('[data-slot="dropdown-menu-item"]'),
    ).toHaveLength(1);
  });

  // A printer in the middle of a print keeps the act and refuses it.
  it('draws Print refused while the printer is printing', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        device: { ...printer(), state: { '/state': 'PRINTING' } },
        files: source({ entries: [PLATE] }),
      }),
    });

    await expect
      .element(page.getByRole('button', { name: /Print bracket\.bgcode/ }))
      .toBeDisabled();

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await expect
      .element(page.getByRole('menuitem', { name: /Print/ }))
      .toHaveAttribute('aria-disabled', 'true');
    await expect
      .element(page.getByText(/This works when the printer is/))
      .toBeVisible();
  });

  // A phone-width pane has room for one control per row: Print and Flash
  // fold into the row's menu and the table never scrolls sideways.
  it('folds Print and Flash into the row menu on a phone-width pane', async () => {
    await page.viewport(375, 800);
    const image = entry({
      name: 'MK4S_firmware_6.3.2.bbf',
      path: 'usb/MK4S_firmware_6.3.2.bbf',
    });
    await render(PrinterFilesPanel, {
      props: props({
        device: {
          ...printer(),
          capabilities: [
            'core.command.start_print',
            'core.command.flash',
            'core.command.delete_file',
          ],
        },
        files: source({ entries: [PLATE, image] }),
      }),
    });

    await expect.element(page.getByRole('table')).toBeVisible();
    const scroller = page.getByRole('table').element().parentElement!;
    expect(scroller.scrollWidth).toBeLessThanOrEqual(scroller.clientWidth);
    expect(
      page.getByRole('button', { name: /^Print bracket\.bgcode/ }).query(),
    ).toBeNull();
    expect(
      page
        .getByRole('button', { name: 'Flash MK4S_firmware_6.3.2.bbf' })
        .query(),
    ).toBeNull();

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await expect
      .element(page.getByRole('menuitem', { name: 'Print' }))
      .toBeVisible();
    await userEvent.keyboard('{Escape}');

    await page
      .getByRole('button', { name: 'Actions for MK4S_firmware_6.3.2.bbf' })
      .click();
    await expect
      .element(page.getByRole('menuitem', { name: 'Flash' }))
      .toBeVisible();
  });

  // An interrupted flash leaves a printer that does not boot.
  it('names a firmware image and confirms before it flashes one', async () => {
    const onCommand = vi.fn(() =>
      Promise.resolve({ kind: 'accepted' as const }),
    );
    const image = entry({
      name: 'MK4S_firmware_6.3.2.bbf',
      path: 'usb/MK4S_firmware_6.3.2.bbf',
    });
    await render(PrinterFilesPanel, {
      props: props({
        commands: { onCommand },
        device: {
          ...printer(),
          capabilities: ['core.command.flash'],
        },
        files: source({ entries: [image] }),
      }),
    });

    await expect.element(page.getByText('Firmware')).toBeVisible();
    await page
      .getByRole('button', { name: 'Flash MK4S_firmware_6.3.2.bbf' })
      .click();
    expect(onCommand).not.toHaveBeenCalled();

    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Flash', exact: true })
      .click();
    expect(onCommand).toHaveBeenCalledWith('core.command.flash', {
      path: 'usb/MK4S_firmware_6.3.2.bbf',
    });
  });

  // An administrator holds every class and still may not flash a printer they
  // do not own, so the panel reads the verbs and not the classes.
  it('offers no flash to a caller the server would refuse it', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        device: {
          ...printer(),
          capabilities: ['core.command.flash'],
          verbs: [],
        },
        files: source({
          entries: [
            entry({
              name: 'MK4S_firmware_6.3.2.bbf',
              path: 'usb/MK4S_firmware_6.3.2.bbf',
            }),
          ],
        }),
      }),
    });

    await expect.element(page.getByText('Firmware')).toBeVisible();
    await expect
      .element(
        page.getByRole('button', { name: 'Flash MK4S_firmware_6.3.2.bbf' }),
      )
      .not.toBeInTheDocument();
  });

  it('offers nothing on a printer that declared no file verbs', async () => {
    await render(PrinterFilesPanel, {
      props: props({ device: { ...printer(), capabilities: [] } }),
    });

    expect(
      document.querySelectorAll('[data-slot="dropdown-menu-trigger"]'),
    ).toHaveLength(0);
  });

  // The panel waits in the shape of the listing it will draw.
  it('draws its own loading shape and no table while the read is in flight', async () => {
    const { container } = await render(PrinterFilesPanel, {
      props: props({
        files: source({ entries: [], filesLoaded: false, filesLoading: true }),
      }),
    });

    await expect
      .element(page.getByRole('status', { name: 'Loading files' }))
      .toBeVisible();
    expect(container.querySelector('table')).toBeNull();
  });

  it('says the folder is empty', async () => {
    await render(PrinterFilesPanel, {
      props: props({ files: source({ entries: [] }) }),
    });

    await expect.element(page.getByText('This folder is empty')).toBeVisible();
  });

  // The empty path is the drives themselves.
  // A printer with no drive lists nothing at the root. That is a drive to put
  // in, said with the same words the upload uses, and asking again is offered.
  it('tells no USB drive from an empty folder, and asks the printer again', async () => {
    const onRecheck = vi.fn();
    await render(PrinterFilesPanel, {
      props: props({
        files: source({ directory: '', entries: [] }),
        uploadBlock: 'noStorage',
        onRecheck,
      }),
    });

    await expect
      .element(page.getByText('No USB drive in the printer'))
      .toBeVisible();
    await page.getByRole('button', { name: 'Check again' }).click();
    expect(onRecheck).toHaveBeenCalledOnce();
  });

  // A printer that has not answered yet is not told to find a drive.
  it('waits for a printer that has not reported its storage', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        files: source({ directory: '', entries: [] }),
        uploadBlock: 'storagesUnknown',
      }),
    });

    await expect
      .element(page.getByText("Waiting for the printer's storage"))
      .toBeVisible();
  });

  // Inside a drive the reason sits where the drop zone would be.
  it('says why nothing can be uploaded where the drop zone would be', async () => {
    await render(PrinterFilesPanel, {
      props: props({ uploadBlock: 'noServerStorage' }),
    });

    await expect
      .element(page.getByText('This server has nowhere to keep an upload'))
      .toBeVisible();
    expect(page.getByText('Choose files').elements()).toHaveLength(0);
    expect(
      page.getByRole('button', { name: 'Check again' }).elements(),
    ).toHaveLength(0);
  });

  it('renders a refusal with the registry code and the server line', async () => {
    const fault = Object.assign(new Error('the printer is not answering'), {
      code: 'core.device/NOT_FOUND',
    });
    await render(PrinterFilesPanel, {
      props: props({ files: source({ filesError: fault }) }),
    });

    await expect.element(page.getByText('core.device/NOT_FOUND')).toBeVisible();
    await expect
      .element(page.getByText('the printer is not answering'))
      .toBeVisible();
  });

  it('draws no drop zone without a taker for the files', async () => {
    await render(PrinterFilesPanel, { props: props() });

    expect(page.getByText('Choose files').elements()).toHaveLength(0);
  });

  it('names the drive the folder on screen is on', async () => {
    await render(PrinterFilesPanel, {
      props: props({ onUpload: () => undefined }),
    });

    await expect.element(page.getByText('Upload to usb')).toBeVisible();
    await expect.element(page.getByText('Choose files')).toBeVisible();
  });

  // The firmware answers a .3mf with "Unsupported file type", after the server
  // has already saved it, so the zone must turn it down before the upload.
  it('turns down a project file the printer would refuse', async () => {
    const onUpload = vi.fn();
    const { container } = await render(PrinterFilesPanel, {
      props: props({ onUpload }),
    });
    const input =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null) throw new Error('no file input rendered');

    await page
      .elementLocator(input)
      .upload([new File(['PK'], 'benchy.3mf'), new File(['G28'], 'a.bgcode')]);

    await expect
      .element(page.getByText('benchy.3mf was not accepted.', { exact: false }))
      .toBeVisible();
    expect(onUpload).toHaveBeenCalledTimes(1);
    expect(
      (onUpload.mock.calls[0]?.[0] as File[]).map((file) => file.name),
    ).toEqual(['a.bgcode']);
  });

  // M997 flashes only an image in the root of the stick, so a .bbf dropped in
  // a folder is sent to the top of the drive, and a print beside it is not.
  it('sends a firmware image to the top of the drive', async () => {
    const onUpload = vi.fn<(dropped: File[], directory: string) => void>();
    const { container } = await render(PrinterFilesPanel, {
      props: props({
        files: source({ directory: 'usb/models' }),
        onUpload,
      }),
    });
    const input =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null) throw new Error('no file input rendered');

    await page
      .elementLocator(input)
      .upload([
        new File(['BBF'], 'MK4S_8.0.0.bbf'),
        new File(['G28'], 'a.bgcode'),
      ]);

    await expect.poll(() => onUpload.mock.calls.length).toBe(2);
    expect(
      onUpload.mock.calls.map(([dropped, directory]) => [
        dropped.map((file) => file.name),
        directory,
      ]),
    ).toEqual([
      [['MK4S_8.0.0.bbf'], 'usb'],
      [['a.bgcode'], 'usb/models'],
    ]);
  });

  // The empty path is where the panel waits for the drives.
  it('draws no drop zone at the empty path', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        files: source({ directory: '', entries: [] }),
        onUpload: () => undefined,
      }),
    });

    expect(page.getByText('Choose files').elements()).toHaveLength(0);
  });

  // A transfer that ended carries the word for how it ended and the server's.
  it('reports every transfer and offers a stop to the one moving', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        transfers: {
          current: [
            {
              transferId: 't-1',
              storage: 'local',
              source: 'models/bracket.bgcode',
              path: 'usb/bracket.bgcode',
              bytesTotal: 4_182_016,
              bytesSent: 2_091_008,
              state: 'sending',
              reason: undefined,
            },
            {
              transferId: 't-2',
              storage: 'local',
              source: 'models/vase.bgcode',
              path: 'usb/vase.bgcode',
              bytesTotal: 8_388_608,
              bytesSent: 0,
              state: 'failed',
              reason: 'the printer closed the connection',
            },
          ],
          stop: () => Promise.resolve(),
        },
      }),
    });

    await expect.element(page.getByText('Sending')).toBeVisible();
    await expect.element(page.getByText('Failed')).toBeVisible();
    await expect
      .element(page.getByText('the printer closed the connection'))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: 'Stop bracket.bgcode' }).elements(),
    ).toHaveLength(1);
    expect(
      page.getByRole('button', { name: 'Stop vase.bgcode' }).elements(),
    ).toHaveLength(0);
  });

  // The whole root cause of the tab opening on a table of one folder.
  it('opens the first drive rather than listing the drives', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    await render(PrinterFilesPanel, {
      props: props({
        files: source({
          directory: '',
          entries: DRIVES,
          openFolder,
        }),
      }),
    });

    expect(openFolder).toHaveBeenCalledWith('usb');
  });

  // A drive the printer has not finished reading lists nothing yet, which is
  // not an empty folder.
  it('says the printer is reading the folder and counts the seconds', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
    try {
      vi.setSystemTime(Date.UTC(2026, 10, 12, 9, 0, 3));
      await render(PrinterFilesPanel, {
        props: props({
          files: source({
            entries: [],
            filesReadingSince: '2026-11-12T09:00:00Z',
            filesUnread: true,
          }),
        }),
      });

      await expect
        .element(
          page.getByText(
            'The printer is reading this folder. This can take a while.',
          ),
        )
        .toBeVisible();
      await expect.element(page.getByText('3 sec so far')).toBeVisible();
      await expect
        .element(page.getByRole('status', { name: 'Loading files' }))
        .toBeVisible();
      expect(page.getByText('This folder is empty').elements()).toHaveLength(0);

      vi.advanceTimersByTime(2000);
      await expect.element(page.getByText('5 sec so far')).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the rows and marks them updating while a read runs', async () => {
    const { container } = await render(PrinterFilesPanel, {
      props: props({
        files: source({ filesReadingSince: '2026-11-12T09:00:00Z' }),
      }),
    });

    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();
    await expect.element(page.getByText('Updating')).toBeVisible();
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(
      page.getByRole('status', { name: 'Loading files' }).elements(),
    ).toHaveLength(0);
  });

  it('marks an empty held folder updating while a read runs', async () => {
    await render(PrinterFilesPanel, {
      props: props({
        files: source({
          entries: [],
          filesReadingSince: '2026-11-12T09:00:00Z',
        }),
      }),
    });

    await expect.element(page.getByText('This folder is empty')).toBeVisible();
    await expect.element(page.getByText('Updating')).toBeVisible();
  });

  it('offers Retry on a failed read and asks again', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    const fault = Object.assign(new Error('the drive did not answer'), {
      code: 'core.printer/LISTING_UNFINISHED',
    });
    await render(PrinterFilesPanel, {
      props: props({
        files: source({ entries: [], filesError: fault, openFolder }),
      }),
    });

    await expect
      .element(page.getByText('core.printer/LISTING_UNFINISHED'))
      .toBeVisible();
    await page.getByRole('button', { name: 'Retry' }).click();
    expect(openFolder).toHaveBeenCalledExactlyOnceWith('usb', true);
  });

  // The server answers the rows it holds beside the fault of a re-read that
  // failed, so the rows stay under the fault.
  it('keeps the held rows under the fault of a failed re-read', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    const fault = Object.assign(new Error('the link dropped'), {
      code: 'core.printer/LISTING_UNFINISHED',
    });
    await render(PrinterFilesPanel, {
      props: props({ files: source({ filesError: fault, openFolder }) }),
    });

    await expect
      .element(page.getByText('core.printer/LISTING_UNFINISHED'))
      .toBeVisible();
    await expect.element(page.getByText('bracket.bgcode')).toBeVisible();
    await page.getByRole('button', { name: 'Retry' }).click();
    expect(openFolder).toHaveBeenCalledExactlyOnceWith('usb', true);
  });

  // A change made on the printer's screen reaches no client, so the folder
  // on screen can be read again by hand.
  it('reads the open folder again on Refresh', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    await render(PrinterFilesPanel, {
      props: props({ files: source({ openFolder }) }),
    });

    await page.getByRole('button', { name: 'Read the folder again' }).click();
    expect(openFolder).toHaveBeenCalledExactlyOnceWith('usb', true);
  });

  it('offers the other drives in the picker and opens the one chosen', async () => {
    const openFolder = vi.fn(() => Promise.resolve());
    const files = source({ directory: '', entries: DRIVES, openFolder });
    await render(PrinterFilesPanel, { props: props({ files }) });

    await page.getByRole('combobox', { name: 'Storage device' }).click();
    await page.getByRole('option', { name: 'sd' }).click();
    expect(openFolder).toHaveBeenCalledWith('sd');
  });
});
