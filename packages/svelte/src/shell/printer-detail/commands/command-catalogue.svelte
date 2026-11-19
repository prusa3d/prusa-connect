<!-- Every verb this printer accepts, refused ones included. -->
<script lang="ts">
  import ActivityIcon from '@lucide/svelte/icons/activity';
  import SearchXIcon from '@lucide/svelte/icons/search-x';
  import SlidersHorizontalIcon from '@lucide/svelte/icons/sliders-horizontal';

  import { formatList, formatUnit, t } from '../../../i18n';
  import {
    PRINTER_STATE_LABEL_KEY,
    PRINTER_STATE_PROGRESS_TONE,
  } from '../../../domain';
  import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    ClippedName,
    EmptyState,
    Field,
    Input,
    Progress,
    TemperatureIcon,
  } from '../../../primitives';
  import CommandRow from './command-row.svelte';
  import type { CommandCatalogueProps, CommandListing } from './types';
  import {
    COMMAND_GROUPS,
    GROUP_LABEL_KEY,
    groupOf,
    rankOf,
    verbText,
  } from './vocabulary';

  let {
    rows,
    marked,
    sending,
    pending,
    readingDrive,
    onSend,
    printerState,
    activeJob,
    tuning,
    motion,
    embedded = false,
  }: CommandCatalogueProps = $props();

  let query = $state('');

  const trimmed = $derived(query.trim());

  // Searches the name a person reads as well as the wire token.
  const visible = $derived(
    trimmed === ''
      ? rows
      : rows.filter((listing) =>
          `${verbText(listing.entry.verb, true)} ${listing.entry.verb}`
            .toLowerCase()
            .includes(trimmed.toLowerCase()),
        ),
  );

  const groups = $derived(
    COMMAND_GROUPS.map((group) => ({
      group,
      listings: visible
        .filter((listing) => groupOf(listing.entry.verb) === group)
        .sort(
          (one, other) => rankOf(one.entry.verb) - rankOf(other.entry.verb),
        ),
    })).filter((section) => section.listings.length > 0),
  );
  const printGroup = $derived(groups.find(({ group }) => group === 'print'));
  const movementGroup = $derived(
    groups.find(({ group }) => group === 'movement'),
  );
  const otherGroups = $derived(
    groups.filter(
      ({ group }) => group !== 'print' && (embedded || group !== 'movement'),
    ),
  );

  const jobName = $derived(
    activeJob?.name ?? activeJob?.path?.split('/').at(-1),
  );
  const jobProgress = $derived.by(() => {
    const progress = activeJob?.progress;
    return typeof progress === 'number' && progress >= 0 && progress <= 100
      ? progress
      : null;
  });
  const playback = new Set([
    'core.command.pause_print',
    'core.command.resume_print',
    'core.command.stop_print',
  ]);

  function refusals(listings: readonly CommandListing[]) {
    return [
      ...new Set(
        listings.flatMap(({ projection }) => {
          if (projection.kind !== 'disabled') return [];
          const { key, states } = projection.reason;
          return [
            t(key, states ? { states: formatList([...states]) } : undefined),
          ];
        }),
      ),
    ];
  }
</script>

<Card
  radius="md"
  data-part={embedded ? 'embedded-command-catalogue' : undefined}
>
  {#if !embedded}
    <CardHeader>
      <CardTitle>
        <h2 class="contents">{t('printerDetailCommands.catalogue.heading')}</h2>
      </CardTitle>
    </CardHeader>
  {/if}
  <CardContent class="flex flex-col gap-4">
    {#if rows.length === 0}
      <EmptyState
        icon={SlidersHorizontalIcon}
        heading={t('printerDetailCommands.none.heading')}
        headingLevel="h3"
        description={t('printerDetailCommands.none.detail')}
      />
    {:else}
      <Field
        class="max-w-sm"
        label={t('printerDetail.commands.search.label')}
        control={Input}
        controlProps={{
          type: 'text',
          placeholder: embedded
            ? undefined
            : t('printerDetail.commands.search.placeholder'),
          value: query,
          oninput: (event: Event) => {
            const field = event.currentTarget;
            if (field instanceof HTMLInputElement) query = field.value;
          },
        }}
      />
      <!-- The filtered list is feedback enough for a sighted reader. -->
      <div aria-live="polite" class="sr-only">
        {trimmed === ''
          ? t('printerDetail.commands.search.available', { count: rows.length })
          : t('printerDetail.commands.search.matching', {
              count: visible.length,
              query: trimmed,
              total: rows.length,
            })}
      </div>

      {#if visible.length === 0}
        <EmptyState
          icon={SearchXIcon}
          heading={t('printerDetail.commands.noMatch.heading')}
          headingLevel="h3"
          description={t('printerDetail.commands.noMatch.detail', {
            query: trimmed,
          })}
        />
      {/if}
      {#if printGroup}
        {@render groupCard(printGroup)}
      {/if}
      <div class="grid items-start gap-3 @2xl/commands:grid-cols-2">
        {#if !embedded && (movementGroup !== undefined || motion !== undefined)}
          <div class:hidden={!movementGroup}>
            {@render groupCard(
              movementGroup ?? { group: 'movement', listings: [] },
            )}
          </div>
        {/if}
        {#if otherGroups.length}
          <div
            class={[
              'grid gap-3',
              (embedded || !movementGroup) &&
                '@2xl/commands:col-span-2 @2xl/commands:grid-cols-2',
            ]}
          >
            {#each otherGroups as section (section.group)}
              {@render groupCard(section)}
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </CardContent>
</Card>

{#snippet groupCard(section: (typeof groups)[number])}
  {@const isPrint = section.group === 'print'}
  {@const showMotion = section.group === 'movement' && motion !== undefined}
  {@const printButtons = isPrint
    ? section.listings.filter(({ entry }) => playback.has(entry.verb))
    : []}
  {@const other = printButtons.length
    ? section.listings.filter(({ entry }) => !playback.has(entry.verb))
    : section.listings}
  {@const reasons = refusals(other)}
  <section
    class="flex min-w-0 flex-col gap-3 rounded-sm border border-border p-3"
  >
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h3
        class="flex items-center gap-2 font-mono text-xs font-medium tracking-wider text-muted-foreground uppercase"
      >
        {#if section.group === 'state'}
          <ActivityIcon aria-hidden="true" class="size-4 shrink-0" />
        {:else if section.group === 'heat'}
          <TemperatureIcon aria-hidden="true" class="size-4 shrink-0" />
        {/if}
        {t(GROUP_LABEL_KEY[section.group])}
      </h3>
      {#if isPrint && printerState}
        <span class="font-mono text-xs text-muted-foreground"
          >{t(PRINTER_STATE_LABEL_KEY[printerState])}</span
        >
      {/if}
    </div>
    {#if printButtons.length || (isPrint && activeJob)}
      {@const stop = printButtons.find(
        ({ entry }) => entry.verb === 'core.command.stop_print',
      )}
      {@const transport = printButtons.filter(
        ({ entry }) => entry.verb !== 'core.command.stop_print',
      )}
      <div class="flex flex-wrap items-center gap-3 rounded-sm bg-surface p-2">
        {#if transport.length}
          <div class="flex flex-wrap gap-2">
            {#each transport as listing (listing.entry.verb)}
              {@render command(listing)}
            {/each}
          </div>
        {/if}
        {#if isPrint && activeJob}
          <div
            class="order-first flex min-w-0 grow basis-full flex-col gap-1 @md/commands:order-none @md/commands:basis-48"
          >
            <div class="flex min-w-0 items-baseline justify-between gap-2">
              {#if jobName}
                <ClippedName name={jobName} class="min-w-0 font-mono text-sm" />
              {:else}
                <span class="text-sm">{t('printers.jobNameUnavailable')}</span>
              {/if}
              {#if jobProgress !== null}
                <span class="shrink-0 font-mono text-xs tabular-nums"
                  >{formatUnit(jobProgress, '%')}</span
                >
              {/if}
            </div>
            <Progress
              value={jobProgress}
              tone={printerState
                ? PRINTER_STATE_PROGRESS_TONE[printerState]
                : 'muted'}
              aria-label={jobName
                ? t('printerDetailStatus.progress.labelFor', { name: jobName })
                : t('printerDetailStatus.progress.label')}
              size="sm"
            />
          </div>
        {/if}
        {#if stop}
          <div class="ms-auto">{@render command(stop)}</div>
        {/if}
      </div>
    {/if}
    {#if showMotion}
      <div class="max-w-2xl" hidden={trimmed !== ''}>{@render motion()}</div>
    {/if}
    {#if other.length}
      {#if showMotion && trimmed === ''}
        <h4 class="text-xs font-medium text-muted-foreground">
          {t('printerDetailCommands.motion.exact')}
        </h4>
      {/if}
      <div class="flex flex-wrap gap-2">
        {#each other as listing (listing.entry.verb)}
          {@render command(listing)}
        {/each}
      </div>
    {/if}
    {#if printButtons.length && printButtons.every(({ projection }) => projection.kind === 'disabled')}
      <p class="text-xs text-muted-foreground">
        {t('printerDetailCommands.playback.unavailable')}
      </p>
    {/if}
    {#if reasons.length}
      <p class="text-xs text-muted-foreground">{reasons.join(' ')}</p>
    {/if}
  </section>
{/snippet}

{#snippet command(listing: CommandListing)}
  <CommandRow
    entry={listing.entry}
    projection={listing.projection}
    marked={marked === listing.entry.verb}
    {tuning}
    {sending}
    {pending}
    {readingDrive}
    {onSend}
  />
{/snippet}
