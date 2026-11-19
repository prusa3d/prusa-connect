<!--
  How far along a print is, as a bar in the colour of the printer's state.

  A printer that reports no progress has no print to measure, so it draws the
  empty track at the same height and no bar: the library's bar with no value
  pulses as "still loading", which would read as a print on its way. The
  core's printer cards draw a plain rule for the same reason.
-->
<script lang="ts">
  import { Progress } from '../../../primitives';

  import type { PrinterView } from '../printers';
  import { t } from '../../../i18n';
  import { progressTone } from './tones';

  let { view }: { view: PrinterView } = $props();
</script>

{#if view.progress === null}
  <div
    data-part="job-bar"
    aria-hidden="true"
    class="h-1.5 rounded-sm bg-border"
  ></div>
{:else}
  <Progress
    data-part="job-bar"
    data-printing={view.state === 'PRINTING' || undefined}
    value={view.progress}
    tone={progressTone(view.state)}
    aria-label={t('dashboard.printer.progressLabel', {
      name: view.jobName ?? view.name,
    })}
    class="h-1.5"
  />
{/if}

<style>
  :global(
    [data-slot='dashboard-card']
      [data-part='job-bar'][data-printing]
      [data-slot='progress-indicator']
  ) {
    background-color: var(--primary);
  }
</style>
