<!--
  Step four: the printer dialled in. It names each printer that joined while
  the screen waited on a success panel, with the way to its page as the
  primary action and the way to add the next one below.
-->
<script lang="ts">
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
  import PlusIcon from '@lucide/svelte/icons/plus';

  import { t } from '../../i18n';
  import { Button, PrinterIcon } from '../../primitives';
  import {
    printerModelOf,
    printerModelLabelOf,
  } from '../printers/presentation';
  import type { JoinedPrinter } from './types';

  let {
    joined,
    onAnother,
  }: {
    joined: readonly JoinedPrinter[];
    onAnother: () => void;
  } = $props();
</script>

<div class="flex flex-col gap-6" data-part="step-connected" aria-live="polite">
  <ul class="flex flex-col gap-3">
    {#each joined as printer (printer.deviceId)}
      <li
        class="@container/joined rounded-md border border-border bg-success-surface p-4"
        data-part="joined"
        data-device={printer.deviceId}
      >
        <div
          class="flex flex-col gap-4 @md/joined:flex-row @md/joined:items-center"
        >
          <div class="flex min-w-0 flex-1 items-center gap-4">
            <span class="relative shrink-0">
              <PrinterIcon
                model={printerModelOf(printer.icon ?? printer.model ?? '')}
                reported={printer.reported}
                class="size-16"
              />
              <CircleCheckIcon
                aria-hidden="true"
                class="absolute -end-1 -bottom-1 size-6 rounded-full bg-card text-success-text"
              />
            </span>
            <div class="flex min-w-0 flex-col gap-0.5">
              <h3 class="text-base font-semibold">
                {t('addPrinter.connected.title', { printer: printer.name })}
              </h3>
              {#if printer.model !== undefined && printer.model !== printer.name}
                <p class="truncate text-sm text-muted-foreground">
                  {printerModelLabelOf(printer.model)}
                </p>
              {/if}
              <p class="text-sm text-muted-foreground">
                {t('addPrinter.connected.note')}
              </p>
            </div>
          </div>
          <!-- A link drawn as the button, because opening a printer goes
               somewhere. -->
          <Button size="lg" class="shrink-0">
            {#snippet child({ props })}
              <a {...props} href={printer.href}>
                {t('addPrinter.connected.open')}
                <ArrowRightIcon aria-hidden="true" />
              </a>
            {/snippet}
          </Button>
        </div>
      </li>
    {/each}
  </ul>

  <div class="border-t border-border pt-4">
    <Button variant="secondary" size="lg" onclick={onAnother}>
      <PlusIcon aria-hidden="true" />
      {t('addPrinter.connected.another')}
    </Button>
  </div>
</div>
