<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { DESCRIBED_BED } from '../../../test/readings';
  import PrinterHistoryPanel from './printer-history-panel.svelte';
  import type { FactsSource, SamplesSource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Printer History Panel',
    component: PrinterHistoryPanel,
  });

  const START = Date.parse('2026-09-23T09:00:00.000Z');

  const points = (base: number) =>
    Array.from({ length: 24 }, (_, index) => ({
      at: START + index * 60_000,
      value: base + Math.round(Math.sin(index / 3) * 8),
    }));

  const factsOf = (over: Partial<FactsSource> = {}): FactsSource => ({
    rows: [
      {
        at: '2026-09-23T09:05:00.000Z',
        epoch: 'one',
        origin: 'device:printer-1',
        payload: {},
        sequence: 1,
        type: 'com.prusa.connect.device.state.changed',
      },
      {
        at: '2026-09-23T09:20:00.000Z',
        epoch: 'one',
        origin: 'device:printer-1',
        payload: {},
        sequence: 2,
        type: 'com.prusa.connect.device.state.changed',
      },
    ],
    loading: false,
    loaded: true,
    error: undefined,
    incomplete: false,
    more: false,
    ...over,
  });

  const samplesOf = (over: Partial<SamplesSource> = {}): SamplesSource => ({
    series: {
      '/tools/0/nozzle_temp': points(215),
      '/temperature/bed': points(60),
    },
    since: undefined,
    loading: false,
    loaded: true,
    error: undefined,
    truncated: false,
    incomplete: false,
    ...over,
  });

  const span = { from: START, to: START + 24 * 60_000 };

  // The printer describes its bed, which names the line and gives its unit.
  const reported = { ...DESCRIBED_BED };

  const FAULT = {
    code: 'core.transport/NETWORK',
    status: undefined,
    params: {},
    message: 'the server could not be reached',
    traceId: undefined,
  };
</script>

<Story name="Past Window" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf()}
    samples={samplesOf()}
    {span}
  />
</Story>

<Story name="Bed Alone" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf()}
    samples={samplesOf({
      series: {
        '/temperature/bed': Array.from({ length: 24 }, (_, index) => ({
          at: START + index * 60_000,
          value: 59.6 + ((index * 7) % 8) * 0.1,
        })),
      },
    })}
    drawn={['/temperature/bed']}
    {span}
  />
</Story>

<Story name="More Events Wait" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({ more: true })}
    samples={samplesOf()}
    {span}
    onMore={() => undefined}
  />
</Story>

<Story name="Server Did Not See It Whole" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({ incomplete: true })}
    samples={samplesOf({ incomplete: true, truncated: true })}
    {span}
  />
</Story>

<Story name="Nothing Happened" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({ rows: [] })}
    samples={samplesOf({ series: {} })}
    {span}
  />
</Story>

<Story name="Still Reading" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({ rows: [], loaded: false, loading: true })}
    samples={samplesOf({ series: {}, loaded: false, loading: true })}
    {span}
  />
</Story>

<Story name="No History Store" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({
      rows: [],
      error: {
        code: 'core.resource/HISTORY_NOT_CONFIGURED',
        status: 501,
        params: {},
        message: 'no history store is declared',
        traceId: undefined,
      },
    })}
    samples={samplesOf({ series: {} })}
    {span}
  />
</Story>

<Story name="Provider Refused The Query" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({
      rows: [],
      error: {
        code: 'core.resource/HISTORY_QUERY_REFUSED',
        status: 422,
        params: { coarsest_step: '1m' },
        message: 'the provider keeps only minute-resolution samples',
        traceId: undefined,
      },
    })}
    samples={samplesOf({ series: {} })}
    {span}
  />
</Story>

<Story name="Both Reads Failed" asChild>
  <PrinterHistoryPanel
    {reported}
    facts={factsOf({ rows: [], error: FAULT })}
    samples={samplesOf({ series: {}, error: FAULT })}
    {span}
  />
</Story>
