import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { CommandEntry } from '../../../data';
import type { StateTransition } from '../../../domain';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import PrinterControlsPanel from '../controls/printer-controls-panel.svelte';
import PrinterCommandsPanel from './printer-commands-panel.svelte';
import type { CommandsSource } from './types';

const CAPABILITIES = [
  'core.command.home',
  'core.command.home#axis',
  'core.command.pause_print',
  'core.command.stop_print',
];

const TRANSITIONS: readonly StateTransition[] = [
  { from: 'PRINTING', to: 'PAUSED', verb: 'core.command.pause_print' },
  { from: 'PRINTING', to: 'STOPPED', verb: 'core.command.stop_print' },
];

const printer = (state = 'PRINTING') => ({
  ...fakeDevice('printer-1', 'MK4S #1042'),
  capabilities: CAPABILITIES,
  state: { '/state': state },
});

const fakeCommands = (over: Partial<CommandsSource> = {}): CommandsSource => ({
  answer: undefined,
  pending: undefined,
  clear: () => undefined,
  error: undefined,
  limit: 100,
  loadPresets: () => Promise.resolve(),
  log: [],
  presets: [],
  refused: 0,
  runPreset: () => Promise.resolve(),
  send: () => Promise.resolve({ kind: 'accepted' as const }),
  sendGcode: () => Promise.resolve(),
  sending: false,
  ...over,
});

const entry = (over: Partial<CommandEntry> = {}): CommandEntry => ({
  id: 1,
  kind: 'command',
  sent: 'core.command.home',
  ok: true,
  answer: 'ok',
  code: undefined,
  reason: undefined,
  ignored: [],
  at: Date.UTC(2026, 8, 17, 9, 30),
  ...over,
});

describe('PrinterCommandsPanel', () => {
  it('opens an Advanced command link and focuses the requested action', async () => {
    await render(PrinterControlsPanel, {
      props: {
        device: printer(),
        telemetry: { value: () => undefined, points: () => [] },
        commands: { onCommand: () => undefined },
        advanced: {
          commands: fakeCommands(),
          requested: true,
          highlight: 'core.command.pause_print',
        },
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Advanced', exact: true }))
      .toHaveAttribute('aria-expanded', 'true');
    await expect
      .poll(() => document.activeElement?.id)
      .toBe('command-core.command.pause_print');
  });

  it('keeps basic commands in Controls and exact commands in the disclosure', async () => {
    const { container } = await render(PrinterControlsPanel, {
      props: {
        device: {
          ...printer('IDLE'),
          capabilities: [
            'core.command.set_speed',
            'core.command.set_speed#speed',
            'core.command.cancel_printer_ready',
            'core.command.move_e',
            'core.command.move_e#length',
          ],
        },
        telemetry: { value: () => undefined, points: () => [] },
        commands: { onCommand: () => undefined },
        advanced: { commands: fakeCommands(), requested: false },
      },
    });

    const trigger = page.getByRole('button', { name: 'Advanced', exact: true });
    await expect.element(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(
      container.querySelector('[id="command-core.command.move_e"]'),
    ).toBeNull();
    await trigger.click();
    await expect
      .element(page.getByRole('button', { name: 'Move extruder', exact: true }))
      .toBeVisible();
    expect(
      container.querySelector('[id="command-core.command.set_speed"]'),
    ).toBeNull();
    expect(
      container.querySelector(
        '[id="command-core.command.cancel_printer_ready"]',
      ),
    ).not.toBeNull();
    await trigger.click();
    await expect.element(trigger).toHaveAttribute('aria-expanded', 'false');
  });

  // The light outline is a light grey: on a dark card it reads as a white line.
  it.each([
    ['dark', '--color-border'],
    ['light', '--color-panel-outline'],
  ])('outlines the Advanced row for the %s theme', async (theme, token) => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
    try {
      await render(PrinterControlsPanel, {
        props: {
          device: printer('IDLE'),
          telemetry: { value: () => undefined, points: () => [] },
          commands: { onCommand: () => undefined },
          advanced: { commands: fakeCommands(), requested: false },
        },
      });
      const trigger = page.getByRole('button', {
        name: 'Advanced',
        exact: true,
      });
      await expect.element(trigger).toBeInTheDocument();
      const probe = document.createElement('div');
      probe.style.borderTop = `1px solid var(${token})`;
      document.body.append(probe);
      const expected = getComputedStyle(probe).borderTopColor;
      probe.remove();

      expect(getComputedStyle(trigger.element()).borderTopColor).toBe(expected);
    } finally {
      document.documentElement.classList.remove('dark');
    }
  });

  it.each([
    ['speed', 'Set speed', 120],
    ['flow', 'Set flow rate', 85],
  ])(
    'sets %s locally until the operator confirms',
    async (argument, name, initial) => {
      const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
      const verb = `core.command.set_${argument}`;
      const screen = await render(PrinterCommandsPanel, {
        props: {
          commands: fakeCommands({ send }),
          device: {
            ...printer(),
            capabilities: [verb, `${verb}#${argument}`],
            state: { '/state': 'PRINTING', [`/${argument}`]: initial },
          },
        },
      });

      const action = screen.getByRole('button', { name, exact: true });
      await action.click();
      const dialog = page.getByRole('alertdialog');
      const slider = dialog.getByRole('slider', { name, exact: true });
      await expect
        .element(slider)
        .toHaveAttribute('aria-valuenow', String(initial));
      const input = dialog.getByRole('spinbutton');
      await input.fill(String(initial + 0.4));
      await userEvent.keyboard('{Enter}');
      await expect.element(input).toHaveValue(initial);
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
      await expect.poll(() => dialog.elements().length).toBe(0);
      expect(send).not.toHaveBeenCalled();

      await action.click();
      await expect
        .element(slider)
        .toHaveAttribute('aria-valuenow', String(initial));
      await dialog.getByRole('spinbutton').fill('142');
      await dialog.getByRole('button', { name, exact: true }).click();
      expect(send).toHaveBeenCalledExactlyOnceWith(verb, { [argument]: '142' });
    },
  );

  it('uses named playback buttons and confirms Stop', async () => {
    const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ send }),
        device: printer(),
        transitions: TRANSITIONS,
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Pause', exact: true }))
      .toBeEnabled();
    const stop = page.getByRole('button', { name: 'Stop print', exact: true });
    await stop.click();
    expect(send).not.toHaveBeenCalled();
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Stop print', exact: true })
      .click();
    expect(send).toHaveBeenCalledExactlyOnceWith(
      'core.command.stop_print',
      undefined,
    );
  });

  it('shows the active print’s live name and progress', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: {
          ...printer(),
          state: {
            '/state': 'PRINTING',
            '/job/display': 'Previous.gcode',
            '/job/progress': 11,
          },
        },
        transitions: TRANSITIONS,
        telemetry: {
          value: (path: string) => {
            if (path === '/job/display') return 'Current bracket.gcode';
            if (path === '/job/progress') return 42;
            return undefined;
          },
          points: () => [],
        },
      },
    });
    const progress = page.getByRole('progressbar', {
      name: 'Current bracket.gcode progress',
    });
    await expect.element(progress).toHaveAttribute('aria-valuenow', '42');
  });

  it('does not show a previous print as active on an idle printer', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: {
          ...printer('IDLE'),
          state: {
            '/state': 'IDLE',
            '/job/display': 'Previous.gcode',
            '/job/progress': 100,
          },
        },
        transitions: TRANSITIONS,
      },
    });
    await expect
      .element(page.getByRole('button', { name: 'Pause', exact: true }))
      .toBeDisabled();
    await expect.element(page.getByRole('progressbar')).not.toBeInTheDocument();
  });

  it('blocks playback when an offline printer last reported printing', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: { ...printer(), connected: false },
        transitions: TRANSITIONS,
      },
    });
    await expect
      .element(page.getByRole('button', { name: 'Pause', exact: true }))
      .toBeDisabled();
    await expect
      .element(page.getByRole('button', { name: 'Stop print', exact: true }))
      .toBeDisabled();
  });

  // The Controls panel holds these while a print runs, and Advanced sits on
  // the same page.
  it.each(['PRINTING', 'PAUSED', 'BUSY', 'ATTENTION', 'ERROR', 'OFFLINE'])(
    'holds the machine outside idle, ready, finished and stopped, with the reason Controls gives (%s)',
    async (state) => {
      const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
      const motion = [
        'core.command.disable_steppers',
        'core.command.home',
        'core.command.move',
        'core.command.move_z',
        'core.command.z_power_move',
      ];
      const { container } = await render(PrinterCommandsPanel, {
        props: {
          commands: fakeCommands({ send }),
          device: { ...printer(state), capabilities: motion },
        },
      });

      for (const verb of motion) {
        const row = container.querySelector(`[id="command-${verb}"]`);
        expect(row, verb).not.toBeNull();
        expect(row?.querySelector('button')?.disabled, verb).toBe(true);
        expect(row?.getAttribute('title'), verb).toBe(
          'The printer moves and homes only when it is idle, ready, finished or stopped.',
        );
      }
      expect(send).not.toHaveBeenCalled();
    },
  );

  it.each(['IDLE', 'READY', 'FINISHED', 'STOPPED'])(
    'moves the machine when nothing holds it (%s)',
    async (state) => {
      const motion = ['core.command.home', 'core.command.move'];
      const { container } = await render(PrinterCommandsPanel, {
        props: {
          commands: fakeCommands(),
          device: { ...printer(state), capabilities: motion },
        },
      });

      for (const verb of motion) {
        const row = container.querySelector(`[id="command-${verb}"]`);
        expect(row?.querySelector('button')?.disabled, verb).toBe(false);
      }
    },
  );

  // The extruder is not the gantry: only a running print holds it.
  it('holds the extruder while a print runs', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: {
          ...printer('PRINTING'),
          capabilities: ['core.command.move_e'],
        },
        transitions: TRANSITIONS,
      },
    });

    const row = container.querySelector('[id="command-core.command.move_e"]');
    expect(row?.querySelector('button')?.disabled).toBe(true);
    expect(row?.getAttribute('title')).toBe(
      'Available once the print finishes.',
    );
  });

  // Telemetry says the print is running before the device record does.
  it('holds motion by the state telemetry reports', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: { ...printer('IDLE'), capabilities: ['core.command.home'] },
        telemetry: {
          value: (path) => (path === '/state' ? 'PRINTING' : undefined),
          points: () => [],
        },
      },
    });

    const row = container.querySelector('[id="command-core.command.home"]');
    expect(row?.querySelector('button')?.disabled).toBe(true);
  });

  it('shows shared motion controls alongside exact commands', async () => {
    const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
    const device = {
      ...printer('IDLE'),
      build_volume: { x_mm: 250, y_mm: 220, z_mm: 270 },
      capabilities: [
        ...CAPABILITIES,
        'core.command.move',
        'core.command.move#x',
      ],
      state: { '/state': 'IDLE', '/axis/x': 100, '/axis/y': 75, '/axis/z': 25 },
    };
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ send }),
        device,
        transitions: TRANSITIONS,
      },
    });

    await expect
      .element(page.getByRole('heading', { name: 'Coordinates' }))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Home axes', exact: true }))
      .toBeVisible();
    await page.getByRole('button', { name: 'X+', exact: true }).click();
    expect(send).toHaveBeenCalledExactlyOnceWith('core.command.move', {
      x: '1',
      feedrate: '3000',
    });
    const search = page.getByRole('textbox', { name: 'Search commands' });
    await search.fill('no such command');
    await expect
      .element(page.getByRole('heading', { name: 'No matching commands' }))
      .toBeVisible();
    await search.fill('');
    await expect
      .element(page.getByRole('button', { name: 'X+', exact: true }))
      .toHaveAttribute('aria-disabled', 'true');
    expect(send).toHaveBeenCalledTimes(1);
  });

  // The one surface that lists everything.
  it('lists a refused verb with the reason the domain gives', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: printer('IDLE'),
        transitions: TRANSITIONS,
      },
    });

    await expect.element(page.getByText('Stop print')).toBeVisible();
    // Pausing and stopping both run from PRINTING.
    expect(
      page.getByText('This works when the printer is PRINTING.').elements(),
    ).toHaveLength(2);
  });

  it('sends nothing for a verb the printer refuses', async () => {
    const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ send }),
        device: printer('IDLE'),
        transitions: TRANSITIONS,
      },
    });

    const button = page.getByRole('button', {
      name: 'Stop print',
      exact: true,
    });
    await expect.element(button).toBeDisabled();
    expect(send).not.toHaveBeenCalled();
  });

  // A reset with no words of its own still warns before it is sent.
  it('asks before a send that cannot be taken back', async () => {
    const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ send }),
        device: {
          ...printer('IDLE'),
          capabilities: [...CAPABILITIES, 'core.command.factory_reset'],
        },
        transitions: TRANSITIONS,
      },
    });

    await page
      .getByRole('button', { name: 'Factory reset', exact: true })
      .click();
    expect(send).not.toHaveBeenCalled();

    await expect
      .element(page.getByText('This changes the printer and cannot be undone.'))
      .toBeVisible();
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Factory reset', exact: true })
      .click();
    expect(send).toHaveBeenCalledWith('core.command.factory_reset', undefined);
  });

  // The capability list declares the arguments as `verb#name`.
  it('collects the arguments a verb declares and sends them', async () => {
    const send = vi.fn(() => Promise.resolve({ kind: 'accepted' as const }));
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ send }),
        device: printer('IDLE'),
        transitions: TRANSITIONS,
      },
    });

    await page.getByRole('button', { name: 'Home axes', exact: true }).click();
    await page.getByLabelText('axis').fill('Z');
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Home axes', exact: true })
      .click();

    expect(send).toHaveBeenCalledWith('core.command.home', { axis: 'Z' });
  });

  // A capability key is what a bug report quotes.
  it('names a verb in words rather than by its wire token', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: printer('IDLE'),
        transitions: TRANSITIONS,
      },
    });

    await expect.element(page.getByText('Home axes')).toBeVisible();
    await expect
      .element(page.getByText('Starts the homing sequence for the named axes.'))
      .toBeVisible();
    expect(container.textContent).not.toContain('core.command.home');
    // The heading the group sits under, shared with the Controls tab.
    await expect.element(page.getByText('Movement')).toBeVisible();
  });

  // A verb another surface owns is not a verb this catalogue offers.
  it('leaves out the verbs another surface sends', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: {
          ...printer(),
          capabilities: [
            ...CAPABILITIES,
            'core.command.start_print',
            'core.command.send_gcode',
          ],
        },
        transitions: TRANSITIONS,
      },
    });

    // Neither reaches the catalogue.
    expect(container.textContent).not.toContain('Start print');
    expect(container.textContent).not.toContain('Send gcode');
    expect(container.textContent).toContain('Home axes');
  });

  // Search reads the name a person types as well as the token.
  it('narrows the list as the reader types, and says when nothing matches', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: printer(),
        transitions: TRANSITIONS,
      },
    });

    await page.getByLabelText('Search commands').fill('home');
    expect(container.textContent).not.toContain('Stop print');
    await expect.element(page.getByText('Home axes')).toBeVisible();

    await page.getByLabelText('Search commands').fill('nothing-like-this');
    await expect.element(page.getByText('No matching commands')).toBeVisible();
    await expect
      .element(page.getByText('No command matches “nothing-like-this”.'))
      .toBeVisible();
  });

  // A printer that offers no verb is an absence the panel names.
  it('says so when the printer offers no verb', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: { ...fakeDevice('printer-1'), capabilities: [] },
      },
    });

    await expect
      .element(page.getByText('This printer offers no command.'))
      .toBeVisible();
  });

  it('names the history this server does not publish', async () => {
    await render(PrinterCommandsPanel, {
      props: { commands: fakeCommands(), device: printer() },
    });

    await expect
      .element(
        page.getByText(
          'This server does not publish a command history, so this log holds only what this page sent.',
        ),
      )
      .toBeVisible();
  });

  // The third absence: the registry code travels with the server's own line.
  it('draws a refusal from the server by its code', async () => {
    await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({
          error: {
            code: 'core.printer/VERB_UNKNOWN',
            traceId: undefined,
            status: 400,
            params: {},
            message: 'This build has no such capability verb',
          },
        }),
        device: printer(),
      },
    });

    await expect
      .element(page.getByText('core.printer/VERB_UNKNOWN'))
      .toBeVisible();
  });

  // The panel waits in the shape of what it will draw.
  it('draws its own loading state and no catalogue', async () => {
    await render(PrinterCommandsPanel, {
      props: { commands: fakeCommands(), device: printer(), loading: true },
    });

    await expect
      .element(
        page.getByRole('status', {
          name: 'Reading the commands this printer accepts',
        }),
      )
      .toBeVisible();
    expect(page.getByText('Home axes').elements()).toHaveLength(0);
  });

  // A chip on the Printer tab sends the reader here for one command.
  it('marks the row a handed verb names, and no row for one it does not draw', async () => {
    const quiet = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: printer(),
        transitions: TRANSITIONS,
        highlight: 'core.command.send_gcode',
      },
    });
    const lit = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands(),
        device: printer(),
        transitions: TRANSITIONS,
        highlight: 'core.command.home',
      },
    });

    await expect
      .poll(() =>
        [...lit.container.querySelectorAll('[aria-current]')].map(
          (row) => row.id,
        ),
      )
      .toEqual(['command-core.command.home']);
    expect(quiet.container.querySelectorAll('[aria-current]')).toHaveLength(0);
  });

  // The printer answers a command only once it has finished reading its drive.
  it('says a pending command waits for the drive read', async () => {
    const hint = 'The printer is finishing a read of its drive.';
    const { rerender } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({
          pending: 'core.command.pause_print',
          sending: true,
        }),
        device: printer(),
        transitions: TRANSITIONS,
        readingDrive: true,
      },
    });

    await expect.element(page.getByText(hint)).toBeVisible();
    expect(page.getByText(hint).elements()).toHaveLength(1);

    await rerender({ readingDrive: false });
    await expect.element(page.getByText(hint)).not.toBeInTheDocument();
  });

  // The command rows live only inside Controls' Advanced disclosure.
  it('says a pending command waits for the drive read inside Controls', async () => {
    const hint = 'The printer is finishing a read of its drive.';
    await render(PrinterControlsPanel, {
      props: {
        device: printer(),
        telemetry: { value: () => undefined, points: () => [] },
        commands: { onCommand: () => undefined, readingDrive: true },
        advanced: {
          commands: fakeCommands({
            pending: 'core.command.pause_print',
            sending: true,
          }),
          requested: true,
        },
      },
    });

    await expect.element(page.getByText(hint)).toBeVisible();
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(PrinterCommandsPanel, {
      props: {
        commands: fakeCommands({ log: [entry()], refused: 0 }),
        device: printer(),
        transitions: TRANSITIONS,
      },
    });

    await expectNoViolations(container);
  });
});
