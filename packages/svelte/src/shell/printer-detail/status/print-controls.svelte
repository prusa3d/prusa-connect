<!-- The acts offered on the print. -->
<script lang="ts">
  import PauseIcon from '@lucide/svelte/icons/pause';
  import PlayIcon from '@lucide/svelte/icons/play';
  import SquareIcon from '@lucide/svelte/icons/square';

  import {
    capabilityFor,
    LIFECYCLE_VERBS,
    type Device,
    type LifecycleVerb,
  } from '../../../domain';
  import { t } from '../../../i18n';
  import { Button, ConfirmDialog } from '../../../primitives';
  import type { CommandPort } from '../types';
  import { commandProjection } from './presentation';

  let {
    device,
    commands,
    state,
    path,
    held,
    largePrintAgain = false,
  }: {
    device: Device;
    commands: CommandPort;
    /** The machine's own word for what it is doing, as the wire spells it. */
    state: string;
    /** The file to print again, when the machine has named one. */
    path: string | undefined;
    /** Whether the printer still holds that file, or undefined until known. */
    largePrintAgain?: boolean;
    held?: boolean | undefined;
  } = $props();

  const offered = $derived(
    LIFECYCLE_VERBS.filter(
      (verb) =>
        commandProjection(device, commands, state, verb).kind === 'available',
    ),
  );

  // Starting a print names a file.
  const printVerb = $derived(
    offered.find(
      (verb) =>
        verb !== 'stop_print' && (verb !== 'start_print' || path !== undefined),
    ),
  );
  // A file the printer no longer holds is said so in place of Print again.
  const gone = $derived(printVerb === 'start_print' && held === false);
  const stoppable = $derived(offered.includes('stop_print'));

  // A verb sent while the printer reads its drive is answered once the read ends.
  const waiting = $derived(
    commands.readingDrive === true &&
      offered.some((verb) => commands.pending === capabilityFor(verb)),
  );

  const label: Record<LifecycleVerb, string> = $derived({
    pause_print: t('printerDetailStatus.command.pause'),
    resume_print: t('printerDetailStatus.command.resume'),
    start_print: t('printerDetailStatus.command.printAgain'),
    stop_print: t('printerDetailStatus.command.stop'),
  });

  const send = (verb: LifecycleVerb): void => {
    void commands.onCommand(
      capabilityFor(verb),
      verb === 'start_print' && path !== undefined ? { path } : undefined,
    );
  };
</script>

{#snippet stopTrigger({ props }: { props: Record<string, unknown> })}
  <Button
    {...props}
    size="sm"
    variant="destructive"
    disabled={commands.sending}
    pending={commands.pending === capabilityFor('stop_print')}
  >
    {#if commands.pending !== capabilityFor('stop_print')}
      <SquareIcon aria-hidden="true" />
    {/if}
    {label.stop_print}
  </Button>
{/snippet}

{#if printVerb !== undefined || stoppable}
  <div class="flex shrink-0 flex-wrap items-center gap-2">
    {#if gone}
      <span class="text-sm text-muted-foreground">
        {t('printerDetailStatus.command.fileGone')}
      </span>
    {:else if printVerb}
      <Button
        size={largePrintAgain && printVerb === 'start_print' ? 'lg' : 'sm'}
        variant={printVerb === 'pause_print' ? 'secondary' : 'default'}
        disabled={commands.sending}
        pending={commands.pending === capabilityFor(printVerb)}
        onclick={() => {
          send(printVerb);
        }}
      >
        {#if commands.pending === capabilityFor(printVerb)}
          <!-- The button draws its own spinner in the icon's place. -->
        {:else if printVerb === 'pause_print'}
          <PauseIcon aria-hidden="true" />
        {:else}
          <PlayIcon aria-hidden="true" />
        {/if}
        {label[printVerb]}
      </Button>
    {/if}

    {#if stoppable}
      <ConfirmDialog
        trigger={stopTrigger}
        title={t('printerDetailStatus.stop.title')}
        description={t('printerDetailStatus.stop.detail')}
        confirmLabel={t('printerDetailStatus.stop.confirm')}
        onConfirm={() => {
          send('stop_print');
        }}
      />
    {/if}

    {#if waiting}
      <span class="basis-full text-xs text-muted-foreground">
        {t('printerDetail.readingDriveHint')}
      </span>
    {/if}
  </div>
{/if}
