<!-- The commands with no reading of their own. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../../../i18n';
  import {
    Button,
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
  } from '../../../primitives';
  import {
    blocked,
    groupReason,
    VERBS,
    type SectionProps,
  } from './machine-controls';
  import MachineHostname from './machine-hostname.svelte';

  let {
    resolve,
    busy,
    onCommand,
    ready,
    motionActions,
    motionReason,
    resetAction,
  }: Omit<SectionProps, 'reading'> & {
    ready: boolean;
    motionActions?: Snippet | undefined;
    motionReason?: string | undefined;
    resetAction?: Snippet | undefined;
  } = $props();

  const readyVerb = $derived(
    ready ? VERBS.cancelPrinterReady : VERBS.setPrinterReady,
  );
  const readyProjection = $derived(resolve(readyVerb));
  const idle = $derived(resolve(VERBS.setIdle));
  const beep = $derived(resolve(VERBS.beep));
  const hostname = $derived(resolve(VERBS.hostname));
  const systemDrawn = $derived(
    beep.kind !== 'absent' ||
      hostname.kind !== 'absent' ||
      resetAction !== undefined,
  );

  const motionDrawn = $derived(
    motionActions !== undefined || motionReason !== undefined,
  );
  const stateDrawn = $derived(
    readyProjection.kind !== 'absent' || idle.kind !== 'absent',
  );

  const stateReason = $derived(groupReason([readyProjection, idle]));
  const systemReason = $derived(
    groupReason([beep, hostname]) ??
      (resetAction ? groupReason([resolve(VERBS.resetPrinter)]) : undefined),
  );

  const uid = $props.id();
</script>

{#snippet subLabel(id: string, text: string)}
  <span
    {id}
    class="font-mono text-xs font-medium tracking-wider text-muted-foreground uppercase"
    >{text}</span
  >
{/snippet}

{#snippet stateActions()}
  <div role="group" aria-labelledby="{uid}-state" class="flex flex-wrap gap-2">
    {#if readyProjection.kind !== 'absent'}
      <Button
        variant="secondary"
        size="compact"
        disabled={blocked(readyProjection, busy)}
        onclick={() => onCommand(readyVerb)}
      >
        {ready
          ? t('printerDetailControls.unmarkReady')
          : t('printerDetailControls.markReady')}
      </Button>
    {/if}
    {#if idle.kind !== 'absent'}
      <Button
        variant="secondary"
        size="compact"
        disabled={blocked(idle, busy)}
        onclick={() => onCommand(VERBS.setIdle)}
      >
        {t('printerDetailControls.exitIdle')}
      </Button>
    {/if}
  </div>
{/snippet}

{#snippet systemActions()}
  {#if beep.kind !== 'absent'}
    <Button
      variant="secondary"
      size="md"
      disabled={blocked(beep, busy)}
      onclick={() => onCommand(VERBS.beep)}
    >
      {t('printerDetailControls.beep')}
    </Button>
  {/if}
  {#if hostname.kind !== 'absent'}
    <MachineHostname projection={hostname} {busy} {onCommand} />
  {/if}
{/snippet}

{#if motionDrawn || stateDrawn || systemDrawn}
  <div class="divide-y divide-border rounded-sm bg-surface px-3">
    {#if motionDrawn}
      <div class="flex flex-col gap-2 py-3">
        {#if motionActions}
          <div class="flex flex-wrap items-center gap-x-1.5 gap-y-2">
            {@render subLabel(
              `${uid}-axes`,
              t('printerDetailControls.utilities.axes'),
            )}
            <div
              role="group"
              aria-labelledby="{uid}-axes"
              class="ms-auto flex flex-wrap justify-end gap-x-1 gap-y-2"
            >
              {@render motionActions()}
            </div>
          </div>
        {/if}
        {#if motionReason}
          <p class="text-xs text-muted-foreground">{motionReason}</p>
        {/if}
      </div>
    {/if}

    {#if stateDrawn}
      <div class="flex flex-col gap-2 py-3">
        <div
          class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2"
        >
          {@render subLabel(
            `${uid}-state`,
            t('printerDetailControls.utilities.printerState'),
          )}
          {@render stateActions()}
        </div>
        {#if stateReason}
          <p class="text-xs text-muted-foreground">{stateReason}</p>
        {/if}
      </div>
    {/if}

    {#if systemDrawn}
      <Collapsible class="py-1">
        <CollapsibleTrigger variant="inline">
          {t('printerDetailControls.utilities.maintenance')}
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-2 pb-3">
          <div class="flex flex-wrap gap-2">
            {@render systemActions()}
            {#if resetAction}
              <div class="ms-auto">
                {@render resetAction()}
              </div>
            {/if}
          </div>
          {#if systemReason}
            <p class="mt-2 text-xs text-muted-foreground">{systemReason}</p>
          {/if}
        </CollapsibleContent>
      </Collapsible>
    {/if}
  </div>
{/if}
