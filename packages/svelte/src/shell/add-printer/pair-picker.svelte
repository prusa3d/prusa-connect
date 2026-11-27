<!--
  The printers this server hears on the local network and does not hold yet,
  to pick one to pair. A search box narrows the list by name or address. A
  printer the list does not show is added by its address in the field below
  the picker, which the form draws.

  mDNS gives a Buddy printer's name and address only. The model is shown when
  the name is still the one the firmware gives a new printer.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import {
    Button,
    EmptyState,
    ListingNoMatch,
    ListingSearch,
    RowList,
  } from '../../primitives';
  import { matchingCandidates, modelOfName } from './presentation';
  import type { NearbyCandidate } from './types';

  let {
    nearby,
    disabled = false,
    onChoose,
  }: {
    nearby: readonly NearbyCandidate[];
    disabled?: boolean;
    onChoose: (candidate: NearbyCandidate) => void;
  } = $props();

  const headingId = $props.id();

  let query = $state('');

  const shown = $derived(matchingCandidates(nearby, query));
</script>

<section
  class="flex flex-col gap-3"
  aria-labelledby={headingId}
  data-part="pair-picker"
>
  <div class="flex flex-col gap-1">
    <h3 id={headingId} class="text-sm font-semibold">
      {t('addPrinter.pair.nearby.heading')}
    </h3>
    <p class="text-sm text-muted-foreground">
      {t('addPrinter.pair.nearby.note')}
    </p>
  </div>

  {#if nearby.length === 0}
    <EmptyState
      size="sm"
      heading={t('addPrinter.pair.nearby.empty')}
      description={t('addPrinter.pair.nearby.emptyNote')}
    />
  {:else}
    <ListingSearch
      value={query}
      onValue={(next: string) => {
        query = next;
      }}
      label={t('addPrinter.pair.nearby.search')}
      placeholder={t('addPrinter.pair.nearby.searchPlaceholder')}
    />
    {#if shown.length === 0}
      <ListingNoMatch
        headingLevel="h4"
        heading={t('addPrinter.pair.nearby.noMatch')}
        clearLabel={t('addPrinter.pair.nearby.clear')}
        onClear={() => {
          query = '';
        }}
      />
    {:else}
      <RowList>
        {#each shown as candidate, index (`${candidate.name} ${candidate.address}`)}
          {@const model = modelOfName(candidate.name)}
          <div
            class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 last:border-b-0"
            data-part="pair-candidate"
          >
            <div class="flex min-w-0 flex-col gap-0.5">
              <span
                id={`${headingId}-${String(index)}`}
                class="truncate text-sm font-medium"
              >
                {candidate.name}
              </span>
              <span class="text-xs text-muted-foreground tabular-nums">
                {model === undefined
                  ? t('addPrinter.pair.nearby.address', {
                      address: candidate.address,
                      port: String(candidate.port),
                    })
                  : t('addPrinter.pair.nearby.modelAddress', {
                      model,
                      address: candidate.address,
                      port: String(candidate.port),
                    })}
              </span>
            </div>
            <Button
              size="sm"
              {disabled}
              aria-describedby={`${headingId}-${String(index)}`}
              onclick={() => {
                onChoose(candidate);
              }}
            >
              {t('addPrinter.pair.nearby.pair')}
            </Button>
          </div>
        {/each}
      </RowList>
      <p class="text-xs text-muted-foreground" aria-live="polite">
        {t('addPrinter.pair.nearby.count', { count: shown.length })}
      </p>
    {/if}
  {/if}
</section>
