<!--
  The add-printer guide over the story core. States it branches on: nothing
  waiting, printers waiting for approval, and the registrations list failing
  or being refused. Joining, joined and refused-at-the-door rows come only from
  registration events on the live stream, which the story core holds open and
  never feeds, so those states are drawn by the library's own stories.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({ title: 'Screens/Printers/Add printer' });

  const LIST = 'GET /api/v1/registrations';

  // The guide draws nothing until the server's settings name how it admits
  // printers, and the story core's table does not hold them.
  const ui = () =>
    json(200, {
      api_reference: '/docs',
      dashboards: true,
      default_language: 'en',
      diagnostics: true,
      enabled: true,
      modules: true,
      printer_admission: 'approval',
      printer_custom_ca: false,
      theme: 'system',
      title: 'Prusa Connect',
      upgrades: true,
    });

  const waiting = () =>
    json(200, {
      items: [
        {
          code: 'K7M2-93QX',
          fingerprint: 'b1:7e:09:4c:a2:d5',
          model: 'COREONE',
          firmware: '6.3.0',
          serial: 'CZPX1024X004K12345',
        },
        {
          code: 'R4T8-11WZ',
          fingerprint: '5d:c0:e1:88:3f:20',
          model: 'MK4S',
          firmware: '6.2.1',
          serial: 'CZPX2024X004K67890',
        },
      ],
    });
</script>

<Story name="Nothing waiting">
  <AppAt
    path="/printers/add"
    overrides={{ 'GET /api/v1/ui': ui, [LIST]: () => json(200, { items: [] }) }}
  />
</Story>

<Story name="Pending approval">
  <AppAt
    path="/printers/add"
    overrides={{ 'GET /api/v1/ui': ui, [LIST]: waiting }}
  />
</Story>

<Story name="Loading">
  <AppAt
    path="/printers/add"
    overrides={{ 'GET /api/v1/ui': ui, [LIST]: pending }}
  />
</Story>

<Story name="Failed">
  <AppAt
    path="/printers/add"
    overrides={{ 'GET /api/v1/ui': ui, [LIST]: failed }}
  />
</Story>

<Story name="Refused">
  <AppAt
    path="/printers/add"
    overrides={{ 'GET /api/v1/ui': ui, [LIST]: refused }}
  />
</Story>
