<!--
  The printers page over the story core. States it branches on: the listing
  (populated, empty, loading, failed, refused) and the nearby-printers panel
  (printers found, none found, lookup failed; the panel draws nothing while
  it is still looking or when none are found). The listing
  shows a farm of twelve. A printer's card line, its tag filter
  and a retire in flight need product state a story cannot set, so they are
  not drawn.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { components } from '@prusa/connect';

  import { failed, pending, refused, emptyList } from '../../../test/story/api';
  import { devicesAnswer } from '../../../.storybook/fleet';
  import { json } from '../../test/stub';
  import AppAt from '../../../test/story/app-at.svelte';

  const { Story } = defineMeta({ title: 'Screens/Printers' });

  const NEARBY = 'GET /api/v1/discovery/nearby';

  const fleetTools = async (): Promise<Response> => {
    const body = (await devicesAnswer(
      'many',
    ).json()) as components['schemas']['DevicePage'];
    const items = body.items ?? [];
    const states = [
      ...new Set(
        items.map((device) => {
          const state = device.state['/state'];
          return typeof state === 'string' ? state : 'UNKNOWN';
        }),
      ),
    ];
    const models = [...new Set(items.map((device) => device.model))];
    return json(200, {
      ...body,
      filters: [
        { param: 'search', kind: 'text', max_length: 100 },
        {
          param: 'state',
          kind: 'multi',
          values: states.map((value) => ({
            value,
            count: items.filter((device) => device.state['/state'] === value)
              .length,
          })),
        },
        {
          param: 'model',
          kind: 'multi',
          values: models.map((value) => ({
            value,
            count: items.filter((device) => device.model === value).length,
          })),
        },
      ],
      sorts: ['urgency', 'name', 'model', 'state'].map((field) => ({
        field,
        default_order: 'asc',
      })),
      issues: {
        affected: 2,
        counts: {},
        by_severity: { error: 1, warning: 1, info: 0 },
      },
    });
  };
</script>

<Story name="Populated"><AppAt path="/printers" /></Story>

<Story name="Fleet tools advertised by server">
  <AppAt path="/printers" overrides={{ 'GET /api/v1/devices': fleetTools }} />
</Story>

<Story name="Empty">
  <AppAt path="/printers" overrides={{ 'GET /api/v1/devices': emptyList }} />
</Story>

<Story name="Loading">
  <AppAt path="/printers" overrides={{ 'GET /api/v1/devices': pending }} />
</Story>

<Story name="Failed">
  <AppAt path="/printers" overrides={{ 'GET /api/v1/devices': failed }} />
</Story>

<Story name="Refused">
  <AppAt path="/printers" overrides={{ 'GET /api/v1/devices': refused }} />
</Story>

<Story name="Nothing nearby">
  <AppAt path="/printers" overrides={{ [NEARBY]: emptyList }} />
</Story>

<Story name="Nearby lookup failed">
  <AppAt path="/printers" overrides={{ [NEARBY]: failed }} />
</Story>
