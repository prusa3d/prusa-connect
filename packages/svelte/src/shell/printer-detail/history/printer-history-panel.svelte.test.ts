import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import PrinterHistoryPanel from './printer-history-panel.svelte';

const nothing = {
  rows: [],
  loading: false,
  loaded: true,
  error: undefined,
  incomplete: false,
  more: false,
};

const noSeries = {
  series: {},
  since: undefined,
  loading: false,
  loaded: true,
  error: undefined,
  truncated: false,
  incomplete: false,
};

const points = Array.from({ length: 10 }, (_, index) => ({
  at: Date.parse('2026-09-23T09:00:00.000Z') + index * 1000,
  value: 200 + index,
}));

const NOT_CONFIGURED = {
  code: 'core.resource/HISTORY_NOT_CONFIGURED',
  status: 501,
  params: {},
  message: 'no history store is declared',
  traceId: undefined,
};

const STATE_ROW = {
  at: '2026-09-23T10:00:00.000Z',
  epoch: 'one',
  origin: 'device:printer-1',
  payload: {},
  sequence: 1,
  type: 'com.prusa.connect.device.state.changed',
};

describe('PrinterHistoryPanel', () => {
  // A deployment that declared no history store is the state a fresh install
  // is in. It must not read as a printer that did nothing.
  it('says the server keeps no past where the provider refused', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: {
          ...nothing,
          error: {
            code: 'core.resource/HISTORY_NOT_CONFIGURED',
            traceId: undefined,
            status: 501,
            params: {},
            message: 'no history store is declared',
          },
        },
        samples: noSeries,
      },
    });

    await expect
      .element(page.getByText('This server keeps no past'))
      .toBeVisible();
  });

  // A provider that keeps a coarser step than this tab's raw two-hour window
  // refuses rather than approximating. Read as "nothing happened" it would
  // look exactly like a printer that was switched off; the registry's own
  // action line says so.
  it('draws the fault where the provider refuses the query, not the empty window', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: {
          ...nothing,
          error: {
            code: 'core.resource/HISTORY_QUERY_REFUSED',
            traceId: undefined,
            status: 422,
            params: { coarsest_step: '1m' },
            message: 'the provider keeps only minute-resolution samples',
          },
        },
        samples: noSeries,
      },
    });

    await expect.element(page.getByText(/switched off/)).toBeVisible();
    expect(
      page.getByText('Nothing happened in this window').elements(),
    ).toHaveLength(0);
  });

  it('says the window held nothing where the store answered nothing', async () => {
    await render(PrinterHistoryPanel, {
      props: { facts: nothing, samples: noSeries },
    });

    await expect
      .element(page.getByText('Nothing happened in this window'))
      .toBeVisible();
  });

  // The server reports a hole rather than answering a quiet one, and dropping
  // the flag throws away the only signal that the log is short.
  it('marks a window the server did not see whole', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: {
          ...nothing,
          incomplete: true,
          rows: [
            {
              at: '2026-09-23T10:00:00.000Z',
              epoch: 'one',
              origin: 'device:printer-1',
              payload: {},
              sequence: 1,
              type: 'com.prusa.connect.device.state.changed',
            },
          ],
        },
        samples: noSeries,
      },
    });

    await expect.element(page.getByText(/did not see/)).toBeVisible();
  });

  it('labels a temperature axis in whole steps of 5', async () => {
    const noisy = points.map((one, index) => ({
      ...one,
      value: 59.6 + (index % 3) * 0.35,
    }));
    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/temperature/bed': noisy } },
      },
    });

    const labels = [
      ...container.querySelectorAll('[data-slot="line-chart-value"]'),
    ].map((one) => Number.parseFloat(one.textContent));

    expect(labels.length).toBeGreaterThan(1);
    expect(labels.every((one) => one % 5 === 0)).toBe(true);
  });

  it('draws a line for every metric the store holds', async () => {
    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          series: {
            '/tools/0/nozzle_temp': points,
            '/temperature/bed': points,
          },
        },
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(2);
  });

  // Without this the chart shows the oldest part of the window and says only
  // that the window "holds more than was read".
  it('says which part of the window the chart is', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          truncated: true,
          series: { '/tools/0/nozzle_temp': points },
        },
      },
    });

    await expect.element(page.getByText(/most recent part/)).toBeVisible();
  });

  // The join is the reason this tab reads the stored window at all, and no
  // test proved the panel ever asked for it.
  it('continues a stored line into the live readings', async () => {
    const live = {
      at: Date.parse('2026-09-23T09:01:00.000Z'),
      value: 240,
    };

    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/tools/0/nozzle_temp': points } },
        live: {
          values: { '/tools/0/nozzle_temp': 240, '/temperature/bed': 60 },
          points: (metric: string) =>
            metric === '/tools/0/nozzle_temp' ? [live] : [],
        },
      },
    });

    // The bed reported live and was never stored, so it is drawn from the
    // live side alone or not at all.
    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(1);
    await expect
      .element(page.getByText('Nozzle, tool 1', { exact: true }))
      .toBeVisible();
  });

  // Two failed reads with one code crash an each block keyed on the code.
  it('draws one alert where both reads failed the same way', async () => {
    const fault = {
      traceId: undefined,
      code: 'core.transport/NETWORK',
      status: 0,
      params: {},
      message: 'the server could not be reached',
    };

    await render(PrinterHistoryPanel, {
      props: {
        facts: { ...nothing, error: fault },
        samples: { ...noSeries, error: fault },
      },
    });

    expect(page.getByRole('alert').elements()).toHaveLength(1);
  });

  // A failed `Read more` keeps the rows it already had, and a panel that
  // drew the alert instead of them would throw that care away.
  it('keeps the rows it has beside a failed read', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: {
          ...nothing,
          more: true,
          error: {
            code: 'core.transport/NETWORK',
            traceId: undefined,
            status: 0,
            params: {},
            message: 'the server could not be reached',
          },
          rows: [
            {
              at: '2026-09-23T10:00:00.000Z',
              epoch: 'one',
              origin: 'device:printer-1',
              payload: {},
              sequence: 1,
              type: 'com.prusa.connect.device.state.changed',
            },
          ],
        },
        samples: noSeries,
      },
    });

    expect(page.getByRole('alert').elements()).toHaveLength(1);
    await expect.element(page.getByText('Events')).toBeVisible();
  });

  it('says what an event is in the words the server gave', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: {
          ...nothing,
          rows: [
            {
              at: '2026-09-23T10:00:00.000Z',
              epoch: 'one',
              origin: 'device:printer-1',
              payload: {},
              sequence: 1,
              type: 'com.prusa.connect.device.state.changed',
            },
          ],
        },
        samples: noSeries,
        describeType: () => 'A device changed what it is doing.',
      },
    });

    await expect
      .element(page.getByText(/A device changed what it is doing/))
      .toBeVisible();
  });

  // A chart holding a temperature and a millimetre at once has a y axis that
  // measures neither, so a reader who has chosen nothing gets the two
  // readings that share a unit.
  it('draws the usual readings and names the rest in the legend', async () => {
    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          series: {
            '/tools/0/nozzle_temp': points,
            '/temperature/bed': points,
            '/axis/x': points,
          },
        },
        onDrawnChange: () => undefined,
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(2);
    await expect
      .element(page.getByRole('button', { name: 'X axis' }))
      .toHaveAttribute('aria-pressed', 'false');
  });

  it('hands the host every reading a click turned on in the unit drawn', async () => {
    let chosen: readonly string[] = [];

    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          series: {
            '/tools/0/nozzle_temp': points,
            '/temperature/bed': points,
          },
        },
        drawn: ['/tools/0/nozzle_temp'],
        onDrawnChange: (next: readonly string[]) => {
          chosen = next;
        },
      },
    });

    await getByRole('button', { name: 'Bed' }).click();

    expect(chosen).toEqual(['/tools/0/nozzle_temp', '/temperature/bed']);
  });

  // The chart has one value axis, so a temperature and a millimetre cannot
  // share it. A reading in another unit takes the place of the ones drawn.
  it('draws a reading in another unit in place of the ones drawn', async () => {
    let chosen: readonly string[] = [];

    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          series: {
            '/tools/0/nozzle_temp': points,
            '/temperature/bed': points,
            '/axis/x': points,
          },
        },
        drawn: ['/tools/0/nozzle_temp', '/temperature/bed'],
        onDrawnChange: (next: readonly string[]) => {
          chosen = next;
        },
      },
    });

    await getByRole('button', { name: 'X axis' }).click();

    expect(chosen).toEqual(['/axis/x']);
  });

  // A remembered reading this build cannot draw is dropped from the choice and
  // not only from the legend. Left in the choice it was invisible and still
  // counted: every later toggle read it as a reading in another unit, so
  // clicking a second temperature replaced the chart instead of adding to it.
  it('adds to the chart beside a remembered reading nothing can draw', async () => {
    let chosen: readonly string[] = [];

    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: noSeries,
        reported: {
          '/temperature/bed': 60,
          '/tools/0/nozzle_temp': 215,
          '/job/progress': 42,
        },
        drawn: ['/temperature/bed', '/job/progress'],
        onDrawnChange: (next: readonly string[]) => {
          chosen = next;
        },
      },
    });

    await getByRole('button', { name: 'Nozzle, tool 1' }).click();

    expect(chosen).toEqual(['/temperature/bed', '/tools/0/nozzle_temp']);
  });

  // The server is asked for the readings the chart will draw, so a remembered
  // reading this build cannot draw is not read either.
  it('asks only for the remembered readings it can draw', async () => {
    const asked: string[][] = [];

    await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: noSeries,
        reported: { '/temperature/bed': 60 },
        drawn: ['/temperature/bed', '/job/progress'],
        onOpen: (metrics: readonly string[]) => {
          asked.push([...metrics]);
        },
      },
    });

    expect(asked).toEqual([['/temperature/bed']]);
  });

  // What is read and what is drawn come from one answer. A remembered choice
  // this build can draw nothing of reads as no choice at all, so the usual
  // readings are both asked for and drawn, rather than asked for and left off.
  it('draws the usual readings it asks for when the whole choice is stale', async () => {
    const asked: string[][] = [];

    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          series: {
            '/tools/0/nozzle_temp': points,
            '/temperature/bed': points,
          },
        },
        drawn: ['/job/progress', '/dialog/id'],
        onDrawnChange: () => undefined,
        onOpen: (metrics: readonly string[]) => {
          asked.push([...metrics]);
        },
      },
    });

    expect(asked).toEqual([['/tools/0/nozzle_temp', '/temperature/bed']]);
    await expect
      .element(getByRole('button', { name: 'Bed' }))
      .toHaveAttribute('aria-pressed', 'true');
    await expect
      .element(getByRole('button', { name: 'Nozzle, tool 1' }))
      .toHaveAttribute('aria-pressed', 'true');
  });

  // A reader who turned every line off asked for that, and it is not an older
  // build's leftovers. The empty ask is the data layer's "read everything".
  it('keeps an empty choice the reader made', async () => {
    const asked: string[][] = [];

    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/temperature/bed': points } },
        drawn: [],
        onDrawnChange: () => undefined,
        onOpen: (metrics: readonly string[]) => {
          asked.push([...metrics]);
        },
      },
    });

    expect(asked).toEqual([[]]);
    await expect
      .element(getByRole('button', { name: 'Bed' }))
      .toHaveAttribute('aria-pressed', 'false');
  });

  // The read asks only for the readings drawn, so a legend built from what it
  // returned offered nothing else, and a reading turned off with no points
  // left the legend for good.
  it('offers every reading the printer holds, read or not', async () => {
    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/temperature/bed': points } },
        reported: {
          '/state': 'IDLE',
          '/temperature/bed': 60,
          '/axis/z': 10,
          '/tools/0/nozzle_temp': 215,
        },
        drawn: ['/temperature/bed', '/speed'],
        onDrawnChange: () => undefined,
      },
    });

    await expect
      .element(getByRole('button', { name: 'Z axis' }))
      .toHaveAttribute('aria-pressed', 'false');
    await expect
      .element(getByRole('button', { name: 'Nozzle, tool 1' }))
      .toHaveAttribute('aria-pressed', 'false');
    await expect
      .element(getByRole('button', { name: 'Speed' }))
      .toHaveAttribute('aria-pressed', 'true');
    await expect
      .element(getByRole('button', { name: '/state' }))
      .not.toBeInTheDocument();
  });

  // The choice is remembered in this browser, so a reading an older build
  // offered is still in the remembered list after an upgrade. There is no unit
  // to draw it in, and a legend entry reading `/job/progress` names a path
  // rather than a reading.
  it('drops a remembered reading nothing can draw rather than naming its path', async () => {
    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/temperature/bed': points } },
        reported: { '/temperature/bed': 60 },
        drawn: [
          '/temperature/bed',
          '/job/progress',
          '/job/time_printing',
          '/dialog/id',
        ],
        onDrawnChange: () => undefined,
      },
    });

    await expect
      .element(getByRole('button', { name: 'Bed' }))
      .toHaveAttribute('aria-pressed', 'true');
    for (const path of ['/job/progress', '/job/time_printing', '/dialog/id']) {
      await expect
        .element(getByRole('button', { name: path }))
        .not.toBeInTheDocument();
    }
  });

  it('keeps the legend while the chosen reading has nothing to draw', async () => {
    const { getByRole } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: noSeries,
        reported: { '/temperature/bed': 60, '/axis/x': 10 },
        drawn: ['/axis/x'],
        onDrawnChange: () => undefined,
      },
    });

    await expect
      .element(getByRole('button', { name: 'Bed' }))
      .toHaveAttribute('aria-pressed', 'false');
  });

  it('says nothing of a truncated window while no line is drawn', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: {
          ...noSeries,
          truncated: true,
          series: { '/temperature/bed': points },
        },
        drawn: [],
        onDrawnChange: () => undefined,
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Bed' }))
      .toBeVisible();
    await expect
      .element(page.getByText(/most recent part/))
      .not.toBeInTheDocument();
  });

  // The panel waits in the shape of what it will draw, rather than flashing
  // "Nothing happened" before either read answers.
  it('draws its own loading shape and neither empty state while a read is in flight', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: { ...nothing, loaded: false },
        samples: noSeries,
      },
    });

    await expect
      .element(page.getByRole('status', { name: 'Loading the events' }))
      .toBeVisible();
    expect(
      page.getByText('Nothing happened in this window').elements(),
    ).toHaveLength(0);
    expect(page.getByText('This server keeps no past').elements()).toHaveLength(
      0,
    );
  });
  // A deployment may keep one half and not the other. The half it keeps is
  // still worth drawing.
  it('draws the measurements where only the events were refused', async () => {
    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: { ...nothing, error: NOT_CONFIGURED },
        samples: { ...noSeries, series: { '/tools/0/nozzle_temp': points } },
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(1);
    await expect
      .element(page.getByText('This server keeps no past'))
      .toBeVisible();
  });

  it('draws the events where only the measurements were refused', async () => {
    await render(PrinterHistoryPanel, {
      props: {
        facts: { ...nothing, rows: [STATE_ROW] },
        samples: { ...noSeries, error: NOT_CONFIGURED },
        describeType: () => 'A device changed what it is doing.',
      },
    });

    await expect
      .element(page.getByText(/A device changed what it is doing/))
      .toBeVisible();
    await expect
      .element(page.getByText('This server keeps no past'))
      .toBeVisible();
  });

  // The tab reads when it opens, and asks for what the chart will draw.
  it('asks for the readings it draws when it opens', async () => {
    const opened: (readonly string[])[] = [];

    await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: noSeries,
        onOpen: (metrics: readonly string[]) => {
          opened.push(metrics);
        },
      },
    });

    expect(opened).toEqual([['/tools/0/nozzle_temp', '/temperature/bed']]);
  });

  it('asks for the readings a reader chose when it opens', async () => {
    const opened: (readonly string[])[] = [];

    await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: noSeries,
        drawn: ['/axis/x'],
        onOpen: (metrics: readonly string[]) => {
          opened.push(metrics);
        },
      },
    });

    expect(opened).toEqual([['/axis/x']]);
  });

  // The window is fixed when the tab opens, and live readings keep coming.
  // The axis has to reach the newest one, or the line runs off the card.
  it('keeps a live reading past the window inside the chart', async () => {
    const from = Date.parse('2026-09-23T09:00:00.000Z');
    const late = { at: from + 3_600_000, value: 250 };

    const { container } = await render(PrinterHistoryPanel, {
      props: {
        facts: nothing,
        samples: { ...noSeries, series: { '/tools/0/nozzle_temp': points } },
        span: { from, to: from + 10_000 },
        live: {
          values: { '/tools/0/nozzle_temp': 250 },
          points: (metric: string) =>
            metric === '/tools/0/nozzle_temp' ? [late] : [],
        },
      },
    });

    const svg = container.querySelector('svg[role="img"], svg');
    const line = container.querySelector('[data-slot="line-chart-series"]');
    expect(svg).not.toBeNull();
    expect(line).not.toBeNull();
    const box = svg?.getBoundingClientRect();
    const drawn = line?.getBoundingClientRect();
    expect(drawn?.right).toBeLessThanOrEqual((box?.right ?? 0) + 1);
  });
});
