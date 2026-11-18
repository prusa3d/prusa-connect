import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { formatting, formatUnit, locale } from '../../i18n';
import { expectNoViolations } from '../../test/axe';
import LineChart from './line-chart.svelte';

const points = Array.from({ length: 20 }, (_, index) => ({
  at: Date.parse('2026-09-23T09:00:00.000Z') + index * 1000,
  value: 200 + index,
}));

describe('LineChart', () => {
  it('names itself to a reader who cannot see it', async () => {
    await render(LineChart, {
      props: {
        label: 'Nozzle temperature over the last two hours',
        series: [{ id: 'nozzle', label: 'Nozzle', points }],
      },
    });

    await expect
      .element(
        page.getByRole('img', {
          name: 'Nozzle temperature over the last two hours',
        }),
      )
      .toBeVisible();
  });

  it('draws one path per series', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: [
          { id: 'nozzle', label: 'Nozzle', points },
          { id: 'bed', label: 'Bed', points },
        ],
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(2);
  });

  // The temperature chart draws three heaters and the target of each.
  it('gives each of six lines its own colour', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: ['a', 'b', 'c', 'd', 'e', 'f'].map((id) => ({
          id,
          label: id,
          points,
        })),
      },
    });

    const strokes = [
      ...container.querySelectorAll('path[data-slot="line-chart-series"]'),
    ].map((path) => path.getAttribute('class'));
    expect(new Set(strokes).size).toBe(6);
  });

  it('draws no line for a series with no points, and does not throw', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: [{ id: 'nozzle', label: 'Nozzle', points: [] }],
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(0);
  });

  // Nothing in a `Chart` composes a legend, so N coloured lines are N
  // anonymous lines until this draws one.
  it('names every line it drew', async () => {
    await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: [
          { id: 'nozzle', label: 'Nozzle', points },
          { id: 'bed', label: 'Bed', points },
        ],
      },
    });

    await expect.element(page.getByText('Nozzle')).toBeVisible();
    await expect.element(page.getByText('Bed')).toBeVisible();
  });

  // A `Spline` draws a path between points, so the first minutes after a
  // history store is declared would otherwise draw nothing at all.
  it('draws a lone reading as a point', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: [{ id: 'nozzle', label: 'Nozzle', points: points.slice(0, 1) }],
      },
    });

    expect(
      container.querySelectorAll('circle[data-slot="line-chart-series"]'),
    ).not.toHaveLength(0);
  });

  it('breaks no accessibility rule', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        series: [{ id: 'nozzle', label: 'Nozzle', points }],
      },
    });

    await expectNoViolations(container);
  });
});

describe('LineChart extras', () => {
  it('reaches the threshold with its value axis', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Memory',
        series: [{ id: 'mem', label: 'Memory', points }],
        threshold: { value: 400, label: 'Budget' },
      },
    });

    const top = Math.max(
      ...texts(container, 'line-chart-value').map((text) =>
        Number.parseFloat(text.replace(/[^\d.]/g, '')),
      ),
    );
    expect(top).toBeGreaterThanOrEqual(400);
    expect(
      container.querySelector('[data-slot="line-chart-threshold"]'),
    ).not.toBeNull();
  });

  it('draws a region behind every path', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Memory',
        series: [{ id: 'mem', label: 'Memory', points }],
        regions: [{ from: points[3]!.at, to: points[6]!.at, label: 'Down' }],
      },
    });

    const region = container.querySelector('[data-slot="line-chart-region"]');
    const path = container.querySelector('[data-slot="line-chart-series"]');
    expect(region).not.toBeNull();
    expect(
      region!.compareDocumentPosition(path!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('draws a marker as a tick and a dot, and names it to a reader', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Memory',
        unit: 'MB',
        series: [{ id: 'mem', label: 'Memory', points }],
        threshold: { value: 400, label: 'Budget' },
        markers: [
          { at: points[5]!.at, label: 'Restart' },
          { at: points[12]!.at, label: 'Crash' },
          { at: points[0]!.at - 60_000, label: 'Outside' },
        ],
        regions: [{ from: points[3]!.at, to: points[6]!.at, label: 'Down' }],
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-marker"]'),
    ).toHaveLength(2);
    const lines = [...container.querySelectorAll('ul.sr-only li')].map((one) =>
      one.textContent.trim(),
    );
    expect(lines.some((line) => line.startsWith('Budget'))).toBe(true);
    expect(lines.some((line) => line.startsWith('Restart at'))).toBe(true);
    expect(lines.some((line) => line.startsWith('Crash at'))).toBe(true);
    expect(lines.some((line) => line.startsWith('Outside'))).toBe(false);
    expect(lines.some((line) => line.startsWith('Down, from'))).toBe(true);
    const path = container.querySelector('path[data-slot="line-chart-series"]');
    expect(path?.getAttribute('d')?.match(/M/g)?.length).toBeGreaterThan(2);
  });

  it('takes a series of only gaps for no reading', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Memory',
        series: [
          {
            id: 'mem',
            label: 'Memory',
            points: [
              { at: 0, value: Number.NaN },
              { at: 1000, value: Number.NaN },
            ],
          },
        ],
      },
    });

    expect(
      container.querySelectorAll('[data-slot="line-chart-series"]'),
    ).toHaveLength(0);
    expect(container.textContent).not.toContain('Infinity');
  });

  it('draws a gap as a break in the line, not as zero', async () => {
    const gapped = points.map((point, index) => ({
      ...point,
      value: index === 10 ? Number.NaN : point.value,
    }));
    const { container } = await render(LineChart, {
      props: {
        label: 'Memory',
        series: [{ id: 'mem', label: 'Memory', points: gapped }],
      },
    });

    const path = container.querySelector('path[data-slot="line-chart-series"]');
    expect(path?.getAttribute('d')?.match(/M/g)).toHaveLength(2);
    expect(path?.getAttribute('d')).not.toContain('NaN');
  });
});

const texts = (container: HTMLElement, slot: string): string[] =>
  [...container.querySelectorAll(`[data-slot="${slot}"]`)].map((one) =>
    one.textContent.trim(),
  );

const DAY = 24 * 3_600_000;

describe('LineChart for a screen reader', () => {
  it('reads out the latest, lowest and highest value of each line', async () => {
    await render(LineChart, {
      props: {
        label: 'Temperatures',
        unit: '°C',
        series: [
          {
            id: 'nozzle',
            label: 'Nozzle',
            points: [
              { at: 0, value: 210 },
              { at: 1000, value: 190.25 },
              { at: 2000, value: 230 },
              { at: 3000, value: 215.5 },
            ],
          },
          { id: 'bed', label: 'Bed', points, hidden: true },
        ],
      },
    });

    const celsius = (value: number): string =>
      formatUnit(value, '°C', { maximumFractionDigits: 1 });

    await expect
      .element(
        page.getByText(
          `Nozzle, latest ${celsius(215.5)}, lowest ${celsius(190.3)}, highest ${celsius(230)}`,
        ),
      )
      .toBeInTheDocument();
    // A hidden line is not drawn, so it is not read out either.
    expect(page.getByText(/^Bed, latest/).elements()).toHaveLength(0);
  });

  it('reads a line in its own unit where it has one', async () => {
    await render(LineChart, {
      props: {
        label: 'Readings',
        series: [
          {
            id: 'x',
            label: 'X axis',
            unit: 'mm',
            points: [{ at: 0, value: 12 }],
          },
        ],
      },
    });

    const mm = formatUnit(12, 'mm', { maximumFractionDigits: 1 });
    await expect
      .element(
        page.getByText(`X axis, latest ${mm}, lowest ${mm}, highest ${mm}`),
      )
      .toBeInTheDocument();
  });
});

describe('LineChart axes', () => {
  afterEach(() => {
    locale.set('en');
    formatting.clock = '24-hour';
  });

  it('writes the time labels in the chosen language', async () => {
    const from = new Date(2026, 8, 20, 15).getTime();
    const days = [
      { at: from, value: 1 },
      { at: from + 4 * DAY, value: 2 },
    ];

    locale.set('de');
    const { container } = await render(LineChart, {
      props: {
        label: 'Readings',
        series: [{ id: 'one', label: 'One', points: days }],
      },
    });

    const german = new Intl.DateTimeFormat('de', {
      day: 'numeric',
      month: 'short',
    });
    expect(texts(container, 'line-chart-time')).toEqual(
      [21, 22, 23, 24].map((date) => german.format(new Date(2026, 8, date))),
    );
  });

  it('puts a flat line in the middle of its axis', async () => {
    const { container } = await render(LineChart, {
      props: {
        label: 'Temperatures',
        unit: '°C',
        series: [
          {
            id: 'bed',
            label: 'Bed',
            points: points.map((point) => ({ ...point, value: 60 })),
          },
        ],
      },
    });

    const plot = container.querySelector('svg')?.getBoundingClientRect();
    const line = container
      .querySelector('[data-slot="line-chart-series"]')
      ?.getBoundingClientRect();
    expect(plot).toBeDefined();
    expect(line).toBeDefined();
    const middle = (plot?.top ?? 0) + (plot?.height ?? 0) / 2;
    expect(Math.abs((line?.top ?? 0) - middle)).toBeLessThanOrEqual(1);
  });

  for (const [width, clock] of [
    [320, '24-hour'],
    [320, '12-hour'],
    [800, '12-hour'],
  ] as const) {
    it(`keeps every label inside the chart at ${String(width)} pixels on a ${clock} clock`, async () => {
      await page.viewport(width, 600);
      formatting.clock = clock;
      const { container } = await render(LineChart, {
        props: {
          label: 'Positions',
          unit: 'mm',
          series: [
            {
              id: 'z',
              label: 'Z axis',
              points: [
                { at: from(0), value: 1_234.5 },
                { at: from(95), value: 20_000 },
              ],
            },
          ],
          xDomain: [from(0), from(120)],
        },
      });

      const chart = container
        .querySelector('[role="img"]')
        ?.getBoundingClientRect();
      expect(chart).toBeDefined();
      const labels = [
        ...container.querySelectorAll(
          '[data-slot="line-chart-value"], [data-slot="line-chart-time"]',
        ),
      ]
        .map((one) => one.getBoundingClientRect())
        .filter((box) => box.width > 0);

      expect(labels.length).toBeGreaterThan(4);
      for (const box of labels) {
        expect(box.left).toBeGreaterThanOrEqual(chart?.left ?? 0);
        expect(box.right).toBeLessThanOrEqual(chart?.right ?? 0);
        expect(box.top).toBeGreaterThanOrEqual(chart?.top ?? 0);
        expect(box.bottom).toBeLessThanOrEqual(chart?.bottom ?? 0);
      }

      // The time labels that are shown never overlap one another.
      const times = [
        ...container.querySelectorAll('[data-slot="line-chart-time"]'),
      ]
        .map((one) => one.getBoundingClientRect())
        .filter((box) => box.width > 0);
      expect(times.length).toBeGreaterThan(1);
      for (const [index, box] of times.slice(1).entries()) {
        expect(box.left).toBeGreaterThan(times[index]?.right ?? 0);
      }
    });
  }
});

function from(minutes: number): number {
  return Date.parse('2026-09-23T09:00:00.000Z') + minutes * 60_000;
}
