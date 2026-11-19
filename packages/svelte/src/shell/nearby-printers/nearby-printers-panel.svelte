<!--
  The printers this server hears on the local network that are not among the
  reader's printers here. Each row links to that printer's own page and to
  the guide that adds a printer. Hearing a printer grants it nothing.

  The panel is a hint beside the printers page, so it draws nothing until
  there is something to say: not while the first read is in flight, not when
  every printer it hears is already here, and not for a person who may not see
  the list at all.
-->
<script lang="ts">
  import PlusIcon from '@lucide/svelte/icons/plus';

  import { describeFault, t } from '../../i18n';
  import { Button, FaultAlert, RowList } from '../../primitives';
  import { printersToAdd } from './presentation';
  import type { NearbyPrintersPanelProps } from './types';

  let { source, addHref }: NearbyPrintersPanelProps = $props();

  const others = $derived(printersToAdd(source.items));

  // A person who may not see the printers nearby is not told about a list
  // they cannot read. Any other refusal is a fault worth showing.
  const fault = $derived(
    source.error === undefined || source.error.status === 403
      ? undefined
      : describeFault(source.error),
  );

  const headingId = $props.id();
</script>

{#if fault !== undefined || (source.loaded && others.length > 0)}
  <section
    class="flex flex-col gap-3"
    aria-labelledby={headingId}
    data-slot="nearby-printers"
  >
    <header class="flex flex-col gap-1">
      <h2 id={headingId} class="text-sm font-semibold">
        {t('nearbyPrinters.title')}
      </h2>
      <p class="text-sm text-muted-foreground">{t('nearbyPrinters.note')}</p>
    </header>
    <FaultAlert {fault} />
    {#if others.length > 0}
      <RowList>
        {#each others as printer, index (`${printer.name} ${printer.url}`)}
          <div
            class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 last:border-b-0"
          >
            <div class="flex min-w-0 flex-col gap-0.5">
              <a
                id={`${headingId}-${String(index)}`}
                class="rounded-sm text-sm font-medium underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
                href={printer.url}
              >
                {printer.name}
              </a>
              <span class="text-xs text-muted-foreground tabular-nums">
                <!-- A port is an identifier, so it is not grouped as a number. -->
                {t('nearbyPrinters.address', {
                  address: printer.address,
                  port: String(printer.port),
                })}
              </span>
            </div>
            {#if addHref !== undefined}
              <Button variant="default" size="sm">
                {#snippet child({ props })}
                  <a
                    {...props}
                    href={addHref}
                    aria-describedby={`${headingId}-${String(index)}`}
                  >
                    <PlusIcon aria-hidden="true" />
                    {t('nearbyPrinters.add')}
                  </a>
                {/snippet}
              </Button>
            {/if}
          </div>
        {/each}
      </RowList>
    {/if}
  </section>
{/if}
