<!--
  Failed and cancelled jobs, newest first: the file, and when and how it
  ended, with the printer when the list spans more than one.
-->
<script lang="ts">
  import { formatDateTime } from '@prusa/connect-ui/i18n';

  import type { SettledJob } from '../domain/job-stats';
  import { heldHost } from '../host';

  interface Props {
    jobs: readonly SettledJob[];
    /** The printer's name, given when the list spans more than one printer. */
    printerName?: ((deviceId: string) => string) | undefined;
  }

  let { jobs, printerName }: Props = $props();
  const t = heldHost().t;
</script>

<ul class="flex flex-col gap-2 text-sm" data-part="fleet-stats-failures">
  {#each jobs as job (`${job.deviceId}:${job.jobId}:${String(job.ended)}`)}
    <li class="flex min-w-0 flex-col">
      <span class="truncate" title={job.name}
        >{job.name === '' ? t('fleet-stats.files.unnamed') : job.name}</span
      >
      <span class="text-xs text-muted-foreground">
        {printerName === undefined
          ? t('fleet-stats.section.recentLine', {
              outcome: t(`fleet-stats.section.outcome.${job.outcome}`),
              when: formatDateTime(job.ended),
            })
          : t('fleet-stats.failures.line', {
              printer: printerName(job.deviceId),
              outcome: t(`fleet-stats.section.outcome.${job.outcome}`),
              when: formatDateTime(job.ended),
            })}
      </span>
    </li>
  {/each}
</ul>
