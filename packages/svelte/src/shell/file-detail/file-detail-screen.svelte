<script lang="ts">
  import BoxIcon from '@lucide/svelte/icons/box';
  import DownloadIcon from '@lucide/svelte/icons/download';
  import FileIcon from '@lucide/svelte/icons/file';
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import FileQuestionIcon from '@lucide/svelte/icons/file-question-mark';
  import HistoryIcon from '@lucide/svelte/icons/history';
  import LockIcon from '@lucide/svelte/icons/lock';
  import PencilLineIcon from '@lucide/svelte/icons/pencil-line';
  import PlayIcon from '@lucide/svelte/icons/play';
  import SearchXIcon from '@lucide/svelte/icons/search-x';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';

  import FileToolNeeds from './file-tool-needs.svelte';
  import {
    isPrintFile,
    PRINTER_STATE_LABEL_KEY,
    printStateOf,
  } from '../../domain';
  import {
    describeFault,
    formatBytes,
    formatDateTime,
    formatDuration,
    formatUnit,
    t,
  } from '../../i18n';
  import {
    ABOVE_ROW_LINK,
    Badge,
    Button,
    Card,
    CardContent,
    ConfirmDialog,
    DataRow,
    DetailHeader,
    DetailTabs,
    DropdownMenuItem,
    EmptyState,
    FaultAlert,
    FileThumbnail,
    KeyValueList,
    PageTrail,
    PrinterIcon,
    PromptDialog,
    RowActions,
    Skeleton,
    type DetailTab,
    type KeyValueRow,
    RowList,
  } from '../../primitives';
  import { printerModelOf, stateBadgeVariant } from '../printers/presentation';
  import type { FileDetailScreenProps } from './types';

  const SLICEABLE = /\.(?:3mf|stl|obj|step|stp|amf)$/i;

  let {
    path,
    entry,
    rootLabel,
    rootHref,
    folderHref,
    acts,
    prints,
    description,
    fault,
    loading = false,
    menu,
    contents,
    tab,
    onTabChange,
    moduleTabs,
  }: FileDetailScreenProps = $props();

  // The two dialogs are held open by the screen rather than hung off a trigger.
  let removing = $state(false);
  let renaming = $state(false);
  let draft = $state('');

  const segments = $derived(path.split('/').filter((part) => part !== ''));

  const name = $derived(
    entry?.display_name ?? entry?.name ?? segments.at(-1) ?? path,
  );

  const waiting = $derived(loading || entry === undefined);

  // The listing answered and holds no such file.
  const missing = $derived(
    !loading && entry === undefined && fault === undefined,
  );

  const addressedNames = $derived(
    entry === undefined ? [path] : [entry.name, entry.path],
  );
  const printable = $derived(addressedNames.some(isPrintFile));
  const sliceable = $derived(
    addressedNames.some((value) => SLICEABLE.test(value)),
  );

  const Glyph = $derived.by(() => {
    if (printable) return FileCode2Icon;

    return sliceable ? BoxIcon : FileIcon;
  });

  const refusal = $derived(
    fault === undefined ? undefined : describeFault(fault),
  );

  // Every folder above the file, the root first.
  const crumbs = $derived([
    { label: rootLabel, href: rootHref },
    ...segments.slice(0, -1).map((part, index) => ({
      label: part,
      href: folderHref(segments.slice(0, index + 1).join('/')),
    })),
  ]);

  const folder = $derived(segments.slice(0, -1).join('/'));

  // A fact the storage did not answer is dropped rather than drawn empty.
  const facts: KeyValueRow[] = $derived(
    entry === undefined
      ? []
      : (
          [
            {
              id: 'where',
              label: t('fileDetail.fact.where'),
              value: folder === '' ? rootLabel : `${rootLabel} / ${folder}`,
            },
            {
              id: 'size',
              label: t('fileDetail.fact.size'),
              value: formatBytes(entry.size),
            },
            {
              id: 'modified',
              label: t('fileDetail.fact.modified'),
              value:
                entry.modified === undefined
                  ? ''
                  : formatDateTime(entry.modified),
            },
            {
              id: 'checksum',
              label: t('fileDetail.fact.checksum'),
              value: entry.sha256 ?? '',
              mono: true,
              copyable: true,
            },
          ] satisfies KeyValueRow[]
        ).filter((row) => row.value !== ''),
  );

  const details = $derived(description?.details);

  // Millimetres of filament read as a wall of digits, so the spool's own unit.
  const METRES = 1000;

  const slicer: KeyValueRow[] = $derived(
    details === undefined
      ? []
      : (
          [
            {
              id: 'time',
              label: t('fileDetail.fact.time'),
              value:
                details.estimated_seconds === undefined
                  ? ''
                  : formatDuration(details.estimated_seconds),
            },
            {
              id: 'filamentLength',
              label: t('fileDetail.fact.filamentLength'),
              value:
                details.filament_mm === undefined
                  ? ''
                  : formatUnit(details.filament_mm / METRES, 'm', {
                      maximumFractionDigits: 2,
                    }),
            },
            {
              id: 'filamentWeight',
              label: t('fileDetail.fact.filamentWeight'),
              value:
                details.filament_g === undefined
                  ? ''
                  : formatUnit(details.filament_g, 'g', {
                      maximumFractionDigits: 1,
                    }),
            },
            {
              id: 'layerHeight',
              label: t('fileDetail.fact.layerHeight'),
              value:
                details.layer_height_mm === undefined
                  ? ''
                  : formatUnit(details.layer_height_mm, 'mm', {
                      maximumFractionDigits: 2,
                    }),
            },
            {
              id: 'nozzle',
              label: t('fileDetail.fact.nozzle'),
              value:
                details.nozzle_mm === undefined
                  ? ''
                  : formatUnit(details.nozzle_mm, 'mm', {
                      maximumFractionDigits: 2,
                    }),
            },
            {
              id: 'material',
              label: t('fileDetail.fact.material'),
              value: details.material ?? '',
            },
            {
              id: 'slicedFor',
              label: t('fileDetail.fact.slicedFor'),
              value: details.printer_model ?? '',
            },
          ] satisfies KeyValueRow[]
        ).filter((row) => row.value !== ''),
  );

  const canPrint = $derived(
    printable && acts?.print !== undefined && acts.sending !== true,
  );
  const writable = $derived(entry !== undefined && entry.read_only !== true);
  const canRemove = $derived(
    writable && acts?.remove !== undefined && acts.sending !== true,
  );
  const canRename = $derived(
    writable && acts?.rename !== undefined && acts.sending !== true,
  );
  // The menu is the file's overflow.
  const hasMenu = $derived(
    acts?.downloadHref !== undefined || canRename || canRemove,
  );

  // The page's own tabs, then a module's. Prints is a tab only where the app
  // lists them; a tab that says the list is missing tells a person nothing.
  const tabs: DetailTab[] = $derived([
    { id: 'contents', label: t('fileDetail.tab.contents'), panel: contentsTab },
    ...(prints === undefined
      ? []
      : [
          {
            id: 'prints',
            label: t('fileDetail.tab.prints'),
            panel: printsTab,
          },
        ]),
    { id: 'details', label: t('fileDetail.tab.details'), panel: detailsTab },
    ...(moduleTabs ?? []),
  ]);

  // A screen that does not keep the open tab keeps it here.
  let chosen = $state('contents');
  const asked = $derived(tab ?? chosen);
  // A tab that is not on this page, such as a module's for another file type,
  // opens the contents.
  const active = $derived(
    tabs.some((row) => row.id === asked) ? asked : 'contents',
  );
  const choose = (id: string): void => {
    chosen = id;
    onTabChange?.(id);
  };
</script>

<div class="mx-auto flex w-full max-w-7xl flex-col gap-6">
  <PageTrail trail={crumbs} page={name} sansPage />

  <FaultAlert fault={refusal} />

  {#if missing}
    <Card>
      <CardContent>
        <EmptyState
          icon={SearchXIcon}
          heading={t('fileDetail.missing.heading')}
          headingLevel="h1"
          description={t('fileDetail.missing.detail')}
        />
      </CardContent>
    </Card>
  {:else if entry !== undefined || fault === undefined}
    <DetailHeader
      title={name}
      loading={waiting}
      {media}
      {titleAside}
      {meta}
      {actions}
    />

    {#if waiting}
      <!-- The tabs' shape while the listing is on its way. -->
      <div class="flex flex-col gap-4">
        <Skeleton class="h-11 w-72 max-w-full" />
        <Skeleton class="h-40 w-full" />
      </div>
    {:else}
      <DetailTabs {tabs} {active} onChange={choose} />
    {/if}
  {/if}
</div>

<!-- A module's view draws its own cards, so the page wraps only its own part
     in one (ADR-20261029-dgv). -->
{#snippet contentsTab()}
  {#if contents}
    {@render contents(coreContents)}
  {:else}
    {@render coreContents()}
  {/if}
{/snippet}

{#snippet printsTab()}
  <Card>
    <CardContent class="flex flex-col gap-4">
      {#if prints === undefined || prints.length === 0}
        <EmptyState
          icon={HistoryIcon}
          heading={t('fileDetail.prints.none.heading')}
          headingLevel="h2"
          description={t('fileDetail.prints.none.detail')}
        />
      {:else}
        <RowList>
          {#each prints as print (print.id)}
            {@const printState = printStateOf({
              state: print.state,
              ended: print.ended !== undefined,
              outcome: print.outcome,
            })}
            {#snippet stateBadge()}
              <Badge variant={stateBadgeVariant(printState)}>
                {t(PRINTER_STATE_LABEL_KEY[printState])}
              </Badge>
            {/snippet}
            <!-- Above the row's own overlay. -->
            {#snippet printer()}
              <a
                href={print.printerHref}
                title={print.printerName}
                class={[
                  'inline-flex min-w-0 items-center gap-1.5 rounded-sm underline underline-offset-4 hover:text-link focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
                  ABOVE_ROW_LINK,
                ]}
              >
                <PrinterIcon
                  model={printerModelOf(print.printerModel ?? '')}
                  reported={print.printerModel}
                  class="size-5 shrink-0"
                />
                <span class="min-w-0 truncate">{print.printerName}</span>
              </a>
            {/snippet}
            <DataRow
              href={print.href}
              primary={formatDateTime(print.started)}
              mono
              wrapValues
              secondaryValues={[
                stateBadge,
                printer,
                print.durationSeconds === undefined
                  ? undefined
                  : formatDuration(print.durationSeconds),
              ]}
            />
          {/each}
        </RowList>
        <p class="text-xs text-muted-foreground">
          {t('fileDetail.prints.match')}
        </p>
      {/if}
    </CardContent>
  </Card>
{/snippet}

{#snippet detailsTab()}
  <Card data-settings-panel>
    <CardContent>
      <KeyValueList rows={facts} />
    </CardContent>
  </Card>
{/snippet}

{#snippet coreContents()}
  <Card data-settings-panel>
    <CardContent>
      <section class="flex flex-col gap-4">
        <h2 class="flex items-center gap-2 text-sm font-medium">
          <FileCode2Icon
            aria-hidden="true"
            class="size-4 text-muted-foreground"
          />
          {t('fileDetail.contents.heading')}
        </h2>
        {#if description === undefined}
          <!-- The server opens the file to answer, which outlasts the listing. -->
          <Skeleton class="h-20 w-full" />
        {:else if slicer.length === 0}
          <EmptyState
            icon={FileQuestionIcon}
            heading={t('fileDetail.contents.none.heading')}
            headingLevel="h3"
            description={t('fileDetail.contents.none.detail')}
          />
        {:else}
          <KeyValueList rows={slicer} />
          {#if details !== undefined}
            <FileToolNeeds {details} />
          {/if}
        {/if}
      </section>
    </CardContent>
  </Card>
{/snippet}

{#snippet media()}
  <FileThumbnail
    {name}
    thumbnailUrl={description?.thumbnailUrl}
    glyph={Glyph}
    class="size-24"
  />
{/snippet}

<!-- What a person reads first about the file, on one line. -->
{#snippet meta()}
  {#if entry}
    <p class="text-sm text-muted-foreground">
      {[
        formatBytes(entry.size),
        entry.modified === undefined ? '' : formatDateTime(entry.modified),
      ]
        .filter((part) => part !== '')
        .join(' · ')}
    </p>
  {/if}
{/snippet}

{#snippet titleAside()}
  {#if entry}
    {#if entry.read_only === true}
      <Badge variant="outline">
        <LockIcon aria-hidden="true" />
        {t('primitives.readOnly')}
      </Badge>
    {/if}
  {/if}
{/snippet}

{#snippet actions()}
  {#if canPrint}
    <Button size="lg" onclick={() => acts?.print?.()}>
      <PlayIcon aria-hidden="true" />
      {t('fileDetail.action.print')}
    </Button>
  {/if}
  {#if menu}
    {@render menu(hasMenu ? ownItems : undefined)}
  {:else if hasMenu}
    <RowActions {name}>
      {@render ownItems()}
    </RowActions>
  {/if}
{/snippet}

{#snippet ownItems()}
  {#if acts?.downloadHref}
    <DropdownMenuItem>
      {#snippet child({ props })}
        <a {...props} href={acts?.downloadHref} download={name}>
          <DownloadIcon aria-hidden="true" />
          {t('fileDetail.action.download')}
        </a>
      {/snippet}
    </DropdownMenuItem>
  {/if}
  {#if canRename}
    <DropdownMenuItem
      onSelect={() => {
        draft = name;
        renaming = true;
      }}
    >
      <PencilLineIcon aria-hidden="true" />
      {t('fileDetail.action.rename')}
    </DropdownMenuItem>
  {/if}
  {#if canRemove}
    <DropdownMenuItem
      variant="destructive"
      onSelect={() => {
        removing = true;
      }}
    >
      <Trash2Icon aria-hidden="true" />
      {t('fileDetail.action.delete')}
    </DropdownMenuItem>
  {/if}
{/snippet}

<ConfirmDialog
  open={removing}
  onOpenChange={(open: boolean) => {
    removing = open;
  }}
  title={t('fileDetail.delete.title', { name })}
  description={t('fileDetail.delete.note')}
  confirmLabel={t('fileDetail.action.delete')}
  onConfirm={() => acts?.remove?.()}
/>

<PromptDialog
  open={renaming}
  onOpenChange={(open: boolean) => {
    renaming = open;
  }}
  title={t('fileDetail.rename.title')}
  label={t('fileDetail.rename.label')}
  value={draft}
  onValueChange={(value: string) => {
    draft = value;
  }}
  confirmLabel={t('fileDetail.action.rename')}
  onConfirm={() => acts?.rename?.(draft)}
/>
