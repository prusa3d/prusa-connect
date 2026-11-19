<!--
  The jobs screen, all printers and one printer, over the story core.
  States: populated (every printer's jobs), one printer's jobs, no jobs
  anywhere, loading, failed, refused. The screen reads the fleet's jobs in one
  list, so each state answers that list.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { fleetOf } from '../../../.storybook/fleet';
  import {
    failed,
    type Overrides,
    pending,
    refused,
  } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import { json } from '../../test/stub';
  import { jobsOfPrinterHref, PATHS } from '../routes';

  const { Story } = defineMeta({ title: 'Screens/Jobs' });

  const everyJob = (answer: Overrides[string]): Overrides => ({
    'GET /api/v1/jobs': answer,
  });

  // An empty cursor is the last page; a list without one reads as cut short.
  const noJobs = () => json(200, { items: [], cursor: '', limit: 200 });

  const first = fleetOf('one')[0]?.id ?? '';
</script>

<Story name="All printers"><AppAt path={PATHS.jobs} /></Story>
<Story name="One printer">
  <AppAt path={jobsOfPrinterHref(first)} />
</Story>
<Story name="Empty">
  <AppAt path={PATHS.jobs} overrides={everyJob(noJobs)} />
</Story>
<Story name="Loading">
  <AppAt path={PATHS.jobs} overrides={everyJob(pending)} />
</Story>
<Story name="Failed">
  <AppAt path={PATHS.jobs} overrides={everyJob(failed)} />
</Story>
<Story name="Refused">
  <AppAt path={PATHS.jobs} overrides={everyJob(refused)} />
</Story>
