<!--
  The dashboards over the story core, which keeps one: "Print farm", four core
  cards. States it branches on: the home dashboard at `/dashboards`, one by its
  id, the wall view, the editor opened on a new dashboard, no dashboard yet,
  the list loading, failing and refused, an id that is not one, and a server
  that leaves dashboards out. Skipped: the page that says the dashboard code
  did not load, which needs a `load` the app never hands the screen.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({
    title: 'Screens/Dashboard',
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

<Story name="Home dashboard">
  <AppAt path="/dashboards" />
</Story>

<Story name="By id">
  <AppAt path="/dashboards/1" />
</Story>

<Story name="Wall view">
  <AppAt path="/dashboards/1?kiosk=1" />
</Story>

<Story name="Editing">
  <AppAt path="/dashboards/1?edit=1" />
</Story>

<Story name="None yet">
  <AppAt
    path="/dashboards"
    overrides={{
      [LIST]: () => json(200, { items: [], total: 0, default_id: null }),
    }}
  />
</Story>

<Story name="Loading">
  <AppAt path="/dashboards" overrides={{ [LIST]: pending }} />
</Story>

<Story name="Failed">
  <AppAt path="/dashboards" overrides={{ [LIST]: failed }} />
</Story>

<Story name="Refused">
  <AppAt path="/dashboards" overrides={{ [LIST]: refused }} />
</Story>

<Story name="Not an id">
  <AppAt path="/dashboards/first" />
</Story>

<Story name="Dashboards off">
  <AppAt
    path="/dashboards/1"
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
