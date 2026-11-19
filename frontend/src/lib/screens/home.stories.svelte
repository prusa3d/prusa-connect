<!--
  Where the app opens. States it branches on: the dashboard, which is the
  default home page, with its list loading, failed and refused, and a server that leaves dashboards out, where the page
  steps aside for the printers list, or for the one printer's page when the
  fleet holds one (switch the printers toolbar). Skipped: the person's own
  choice of home page, which lives in this browser's storage and not in an
  answer the story core gives; and the blank page while the settings are read,
  which the frame never shows because it waits for the same answer.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({
    title: 'Screens/Home',
    parameters: {
      docs: {
        description: {
          component:
            'The cards are the core four. No module card is placed: fleet-stats, the extension Storybook serves, adds a page, a line on each printer card and a printer section, and no dashboard card. Its page is under Screens/Module page.',
        },
      },
    },
  });

  const LIST = 'GET /api/v1/dashboards';
</script>

<Story name="Dashboard">
  <AppAt path="/" />
</Story>

<Story name="Dashboards off">
  <AppAt
    path="/"
    overrides={{
      'GET /api/v1/ui': () =>
        json(200, {
          enabled: true,
          title: 'Prusa Connect',
          default_language: 'en',
          theme: 'system',
          dashboards: false,
          diagnostics: true,
        }),
    }}
  />
</Story>

<Story name="Loading">
  <AppAt path="/" overrides={{ [LIST]: pending }} />
</Story>

<Story name="Failed">
  <AppAt path="/" overrides={{ [LIST]: failed }} />
</Story>

<Story name="Refused">
  <AppAt path="/" overrides={{ [LIST]: refused }} />
</Story>
