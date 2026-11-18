import type { PrusaConnect, StreamEvent } from '@prusa/connect';
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import {
  SharedTelemetry,
  type SharedTelemetryClient,
} from '../../../data/shared-telemetry.svelte';
import { fakeClock, fakeStream, hubEvent } from '../../../data/stubs';
import { fakeDevice } from '../../../test/fakes';
import { Clock } from '../clock.svelte';
import { dashboardContextOver, fakeCore } from '../fixtures';
import TileStory from '../tile-story.svelte';
import TemperatureTile from './temperature-tile.svelte';

const sampled = (path: string, value: number): StreamEvent =>
  hubEvent(
    'com.prusa.connect.device.telemetry.sampled',
    { path, value },
    'device:core',
  );

/** A page whose one stream the test pushes frames into, at the frames' time. */
const pageWith = () => {
  const stream = fakeStream();
  const timers = fakeClock();
  const empty = () =>
    Promise.resolve({ items: [], next_cursor: '', truncated: false });
  const client = {
    events: (() => stream.subscription) as unknown as PrusaConnect['events'],
    listDeviceHistorySamples: empty,
    listDeviceHistoryFacts: empty,
  } as unknown as SharedTelemetryClient;
  const { context } = dashboardContextOver(fakeCore().client, [
    fakeDevice('core', 'CORE One'),
  ]);

  return {
    stream,
    timers,
    context: {
      ...context,
      telemetry: new SharedTelemetry(client, { timers }),
      clock: new Clock({
        now: () => Date.parse('2026-09-12T00:01:00Z'),
        every: () => () => undefined,
      }),
    },
  };
};

const legend = () =>
  page
    .getByRole('listitem')
    .elements()
    .map((item) => item.textContent.trim());

describe('TemperatureTile', () => {
  // Only an enclosed printer reports a chamber.
  it('charts the chamber of a printer that reports one', async () => {
    const { stream, timers, context } = pageWith();
    await render(TileStory, {
      props: {
        context,
        tile: TemperatureTile,
        type: 'temperature',
        config: { device: 'core' },
      },
    });
    timers.advance(100);

    await stream.push(sampled('/temperature/bed', 60));
    await stream.push(sampled('/temperature/chamber', 31.2));

    await expect.element(page.getByText('Chamber target')).toBeInTheDocument();
    expect(legend()).toContain('Chamber');
  });

  it('leaves the chamber out for a printer with none', async () => {
    const { stream, timers, context } = pageWith();
    await render(TileStory, {
      props: {
        context,
        tile: TemperatureTile,
        type: 'temperature',
        config: { device: 'core' },
      },
    });
    timers.advance(100);

    await stream.push(sampled('/temperature/bed', 60));

    await expect.element(page.getByText('Bed target')).toBeInTheDocument();
    expect(legend().some((text) => text.startsWith('Chamber'))).toBe(false);
  });
});
