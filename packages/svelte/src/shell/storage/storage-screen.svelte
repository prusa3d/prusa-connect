<script lang="ts">
  import { untrack } from 'svelte';

  import DownloadIcon from '@lucide/svelte/icons/download';
  import FolderInputIcon from '@lucide/svelte/icons/folder-input';
  import FolderPlusIcon from '@lucide/svelte/icons/folder-plus';
  import HardDriveIcon from '@lucide/svelte/icons/hard-drive';

  import { compareFiles } from '../../primitives/table/ordering';
  import { describeFault, t } from '../../i18n';
  import {
    BulkBar,
    Button,
    Card,
    CardContent,
    CardHeader,
    ConfirmDialog,
    EmptyState,
    FaultAlert,
    FileDropzone,
    FolderTrail,
    ListingSearch,
    ListingSearchSummary,
    ListingState,
    ListingToolbar,
    PageHeader,
    PromptDialog,
    Selection,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Skeleton,
    Slider,
    type ListField,
    toast,
    viewModes,
  } from '../../primitives';
  import { PrintDialog, type PrintableFile } from '../print-picker';
  import { browserStore } from '../printers/presentation';
  import {
    describeInTurn,
    isPrintFile,
    siblingPath,
    splitName,
    trailOf,
  } from './presentation';
  import rowsIcon from './assets/rows.svg';
  import tilesIcon from './assets/tiles.svg';
  import StorageListing from './storage-listing.svelte';
  import StorageFolderDialog from './storage-folder-dialog.svelte';
  import StorageUploads from './storage-uploads.svelte';
  import type {
    StorageFieldId,
    StoragePlace,
    StorageRow,
    StorageScreenProps,
    StorageUpload,
  } from './types';

  /** The split, at the one tier where both columns fit side by side. */
  let thumbnailSize = $state(0);

  const SPLIT_ROW =
    'flex flex-col @4xl/storage:flex-row @4xl/storage:items-stretch';

  const SPLIT_LEADING =
    'flex flex-col gap-0 border-b border-border pb-6 @4xl/storage:order-first @4xl/storage:w-(--file-upload-pane-width) @4xl/storage:min-w-0 @4xl/storage:max-w-2/5 @4xl/storage:border-b-0 @4xl/storage:pb-0 @4xl/storage:pe-panel-padding';

  const SPLIT_TRAILING =
    'border-border pt-6 @4xl/storage:border-s @4xl/storage:pt-0 @4xl/storage:ps-panel-padding';

  let {
    storage,
    address,
    onNavigate,
    fileHref,
    fileActions,
    sharing,
    store = browserStore(),
    devices = [],
    transitions,
    locked,
    printerStorages,
    onPrint,
    onPrinted,
  }: StorageScreenProps = $props();

  let pending = $state<StorageRow | undefined>(undefined);
  let printing = $state<StorageRow | undefined>(undefined);
  let query = $state('');
  let moving = $state(false);
  let creating = $state(false);
  let folderName = $state('');
  let renaming = $state<StorageRow | undefined>(undefined);
  let newName = $state('');
  // The folder waiting for the reader to confirm it is shared.
  let offering = $state<StorageRow | undefined>(undefined);

  const FIELDS: ListField[] = [
    { id: 'name', label: t('storage.column.name'), required: true },
    { id: 'size', label: t('storage.column.size') },
    { id: 'modified', label: t('storage.column.modified') },
  ];
  // The table draws Modified only where its pane has room, which the picker
  // cannot see from its popover, so the table offers no toggle for it.
  const TABLE_FIELDS = FIELDS.filter((field) => field.id !== 'modified');

  // Reads nothing reactive, so it runs once when the screen mounts. The store
  // hands back a read already in flight instead of starting a second one.
  $effect(() => {
    void storage.load();
  });

  // The folder the address names is opened, and the first storage when the
  // address names none and nothing is open yet. The store is read untracked,
  // so its own answers do not open the folder a second time.
  $effect(() => {
    const wanted = address;
    const first = storage.storages[0];
    untrack(() => {
      if (wanted !== undefined) {
        const open =
          storage.storage === wanted.storage &&
          storage.directory === wanted.directory;
        if (!open) {
          forget();
          void storage.open(wanted.storage, wanted.directory);
        }

        return;
      }
      if (storage.storage === '' && first !== undefined) {
        void storage.open(first.name);
      }
    });
  });

  const chosen = $derived(
    storage.storages.find((row) => row.name === storage.storage),
  );
  const root = $derived(chosen?.display_name ?? storage.storage);

  const trail = $derived(trailOf(root, storage.directory));
  const writable = $derived(chosen?.write === true);
  // An empty list is the server saying any extension is taken.
  const accept = $derived(
    chosen === undefined || chosen.allowed_extensions.length === 0
      ? undefined
      : chosen.allowed_extensions.join(','),
  );

  const fault = $derived(
    storage.error === undefined ? undefined : describeFault(storage.error),
  );
  const filesFault = $derived(
    storage.filesError === undefined
      ? undefined
      : describeFault(storage.filesError),
  );
  const emptyFolder = $derived(
    storage.filesLoaded && storage.entries.length === 0,
  );

  // The search is over the open folder and never over the storage.
  const needle = $derived(query.trim().toLowerCase());
  const searching = $derived(needle !== '');

  const matching = $derived(
    searching
      ? storage.entries.filter((entry) =>
          entry.name.toLowerCase().includes(needle),
        )
      : storage.entries,
  );

  const listing: ListingState<StorageRow> = new ListingState<StorageRow>({
    id: 'storage',
    viewSetting: 'files',
    fields: FIELDS,
    views: viewModes(['table', 'cards']),
    initialView: 'table',
    store: {
      read: (key) => store.read(key),
      write: (key, value) => {
        store.write(key, value);
      },
    },
    rows: () => matching,
    compare: (a, b) => compareFiles(a, b, a.name, b.name, listing.sort),
  });

  const offered = $derived(
    listing.view.value === 'table' ? TABLE_FIELDS : FIELDS,
  );

  const selection = new Selection<string>({
    rows: () => listing.rows.map((entry) => entry.path),
  });

  const shows = (field: StorageFieldId): boolean =>
    listing.fields.visible.includes(field);

  const under = (directory: string, name: string): string =>
    directory === '' ? name : `${directory}/${name}`;

  const receive = (files: File[]): void => {
    const target = storage.storage;
    for (const file of files) {
      void storage.upload(target, under(storage.directory, file.name), file);
    }
  };

  const cancelUpload = (upload: StorageUpload): void => {
    storage.cancelUpload(upload.storage, upload.path);
  };

  // A search is a new list, so it starts on its first page with nothing
  // ticked: a tick the person can no longer see is never acted on.
  const search = (next: string): void => {
    query = next;
    selection.clear();
    listing.pager?.onPageChange(0);
  };

  // A selection and a search belong to one folder, so walking to another
  // drops both, and the new folder opens on its first page.
  const forget = (): void => {
    search('');
  };

  const walk = (place: StoragePlace): void => {
    forget();
    if (onNavigate === undefined) {
      void storage.open(place.storage, place.directory);
    } else {
      onNavigate(place);
    }
  };

  const openFolder = (path: string): void => {
    walk({ storage: storage.storage, directory: path });
  };

  const openStorage = (name: string): void => {
    walk({ storage: name, directory: '' });
  };

  const rowHref = $derived(
    fileHref === undefined
      ? undefined
      : (row: StorageRow) => fileHref(storage.storage, row),
  );

  const pickedRows = $derived(
    storage.entries.filter((entry) => selection.has(entry.path)),
  );

  // A folder has no bytes to serve, so only the files are downloaded.
  const pickedFiles = $derived(pickedRows.filter((entry) => !entry.folder));

  const browsable = $derived(chosen?.browse === true);
  const movable = $derived(chosen?.move === true);

  // No request hands back several files at once, so each file is fetched by a
  // link of its own. They all go out on the one press, which is the gesture a
  // browser grants a multiple download on.
  const download = (): void => {
    for (const entry of pickedFiles) {
      const link = document.createElement('a');
      link.href = storage.contentUrl(storage.storage, entry.path);
      link.download = entry.name;
      link.rel = 'noopener';
      document.body.append(link);
      link.click();
      link.remove();
    }
  };

  const startMove = (): void => {
    moving = true;
    void storage.browseFolders(storage.storage, '');
  };

  // The rows are taken first, because the selection is cleared before the
  // first answer arrives.
  const moveTo = async (destination: string): Promise<void> => {
    const rows = [...pickedRows];
    moving = false;
    selection.clear();

    const results = await storage.moveAll(
      storage.storage,
      rows.map((entry) => ({
        from: entry.path,
        to: under(destination, entry.name),
      })),
    );

    // The moved rows leave the folder on screen, so the toast says they went
    // somewhere. Each refused row is named, and its error stays on the page.
    const moved = results.filter((row) => row.fault === undefined).length;
    if (moved > 0) toast.success(t('storage.move.done', { count: moved }));

    for (const row of results) {
      if (row.fault === undefined) continue;
      const name = rows.find((entry) => entry.path === row.from)?.name;
      toast.error(t('storage.move.failed', { name: name ?? row.from }), {
        description: describeFault(row.fault).title,
      });
    }
  };

  // A path segment and never a path.
  const folderFault = $derived(
    folderName.includes('/') ? t('storage.newFolder.invalid') : undefined,
  );

  const createFolder = (): void => {
    const segment = folderName.trim();
    creating = false;
    folderName = '';
    if (segment === '') return;

    void storage.createFolder(
      storage.storage,
      under(storage.directory, segment),
    );
  };

  const startRename = (entry: StorageRow): void => {
    newName = splitName(entry.name, entry.folder).stem;
    renaming = entry;
  };

  const renameFault = $derived(
    newName.includes('/') ? t('storage.rename.invalid') : undefined,
  );

  const rename = async (): Promise<void> => {
    const entry = renaming;
    const stem = newName.trim();
    renaming = undefined;
    if (entry === undefined) return;

    const { extension } = splitName(entry.name, entry.folder);
    const name = `${stem}${extension}`;
    if (stem === '' || name === entry.name) return;

    const [result] = await storage.moveAll(storage.storage, [
      { from: entry.path, to: siblingPath(entry.path, name) },
    ]);
    if (result?.fault === undefined) return;

    toast.error(t('storage.rename.failed', { name: entry.name }), {
      description: describeFault(result.fault).title,
    });
  };

  const changeShare = async (
    entry: StorageRow,
    shared: boolean,
  ): Promise<void> => {
    if (sharing === undefined) return;
    try {
      await sharing.share(storage.storage, entry.path, shared);
    } catch (thrown: unknown) {
      const key = shared ? 'storage.share.failed' : 'storage.unshare.failed';
      toast.error(t(key, { name: entry.name }), {
        description: describeFault(thrown).title,
      });
    }
  };

  // Sharing shows the folder's full path to every server below, so it is
  // confirmed first. Unsharing hides it at once and needs no question.
  const flipShare = (entry: StorageRow, shared: boolean): void => {
    if (shared) {
      offering = entry;
    } else {
      void changeShare(entry, false);
    }
  };

  const confirmShare = (): void => {
    const entry = offering;
    offering = undefined;
    if (entry !== undefined) void changeShare(entry, true);
  };

  const remove = (): void => {
    const entry = pending;
    pending = undefined;
    if (entry === undefined) return;

    void storage.remove(storage.storage, entry.path);
  };

  /** True where a row may be sent to a printer: a file, and a print call to make. */
  const printable = $derived(onPrint !== undefined && devices.length > 0);

  // The printers with no USB drive to put the file on, once they answered.
  const noStorage = $derived(
    devices
      .filter((device) => printerStorages?.noStorage(device.id) === true)
      .map((device) => device.id),
  );

  // This asks the server what is inside each print file on the open page.
  // Paging and sorting change the page, so the reads follow it, and the
  // controller stops the reads for a page the reader has left.
  $effect(() => {
    const target = storage.storage;
    const wanted = listing.page
      .filter((entry) => !entry.folder && isPrintFile(entry.name))
      .map((entry) => entry.path);
    if (wanted.length === 0) return;

    const batch = new AbortController();
    // The store reads its own record of described files before it asks. A
    // tracked read would restart the batch on every answer.
    untrack(() => {
      void describeInTurn(storage, target, wanted, batch.signal).catch(
        () => undefined,
      );
    });

    return () => {
      batch.abort();
    };
  });

  const describedOf = (entry: StorageRow) =>
    storage.describedFile(storage.storage, entry.path);

  const sheetFile = $derived.by((): PrintableFile | undefined => {
    const entry = printing;
    if (entry === undefined) return undefined;
    const described = describedOf(entry);

    return {
      details: described?.details,
      name: entry.name,
      source: entry.path,
      storage: storage.storage,
      thumbnailUrl: described?.thumbnailUrl,
    };
  });
</script>

<div class="@container/storage flex flex-col gap-4">
  <PageHeader title={t('storage.title')} />

  {#if storage.loading && !storage.loaded}
    <div role="status" aria-label={t('storage.loading')}>
      <Card>
        <CardHeader><Skeleton class="h-4 w-40" /></CardHeader>
        <CardContent class="flex flex-col gap-3">
          {#each { length: 5 }, row (row)}
            <Skeleton class="h-6 w-full" />
          {/each}
        </CardContent>
      </Card>
    </div>
  {:else if storage.storages.length === 0}
    {#if fault}
      <FaultAlert {fault} />
    {:else}
      <Card>
        <CardContent>
          <EmptyState
            icon={HardDriveIcon}
            heading={t('storage.unavailable.heading')}
            headingLevel="h2"
            description={t('storage.unavailable.note')}
          />
        </CardContent>
      </Card>
    {/if}
  {:else}
    <FaultAlert {fault} />

    <Card density="panel">
      <CardContent
        class={[SPLIT_ROW, 'pt-section-block pb-panel-padding-mobile']}
      >
        <div class={SPLIT_LEADING}>
          {#if writable}
            <h2 class="mb-3 text-sm font-medium">
              {t('storage.upload.heading', {
                name: chosen?.display_name ?? storage.storage,
              })}
            </h2>
            <FileDropzone {accept} onFiles={receive} />
          {/if}

          <StorageUploads
            uploads={storage.uploads}
            onClear={() => {
              storage.clearUploads();
            }}
            onCancel={cancelUpload}
          />
        </div>

        <div
          class={[
            SPLIT_TRAILING,
            '@container/storage-files flex min-w-0 flex-1 flex-col gap-section-inline',
          ]}
        >
          <BulkBar
            count={pickedRows.length}
            label={t('storage.bulk.selected', { count: pickedRows.length })}
            selectAllLabel={selection.count < selection.total
              ? t('storage.bulk.selectAll', { count: selection.total })
              : undefined}
            onSelectAll={() => {
              selection.selectAll();
            }}
            clearLabel={t('storage.bulk.clear')}
            onClear={() => {
              selection.clear();
            }}
            {actions}
            {idle}
          />

          <div class="flex flex-wrap items-center justify-between gap-3">
            <FolderTrail
              crumbs={trail}
              label={t('storage.trail.label')}
              onOpen={openFolder}
            />
            <!-- Same width as the one that hides a list preview. -->
            <div
              class={[
                'ms-auto flex items-center gap-3',
                listing.view.value === 'table' &&
                  '@max-sm/storage-files:hidden',
              ]}
            >
              <span class="text-xs text-muted-foreground"
                >{t('storage.thumbnail.size')}</span
              >
              <div class="w-28 shrink-0">
                <Slider
                  type="single"
                  bind:value={thumbnailSize}
                  min={0}
                  max={3}
                  step={1}
                  aria-label={t('storage.thumbnail.size')}
                  class="h-8 [&_[data-slot=slider-track]]:bg-input"
                />
              </div>
            </div>
          </div>

          {#if searching}
            <ListingSearchSummary
              class="justify-between"
              summary={t('storage.search.results', { count: listing.total })}
              clearLabel={t('storage.search.clear')}
              onClear={() => {
                search('');
              }}
            />
          {/if}

          <StorageListing
            {listing}
            {thumbnailSize}
            {filesFault}
            loading={storage.filesLoading}
            {emptyFolder}
            {searching}
            {query}
            {writable}
            {printable}
            {selection}
            {shows}
            thumbnailOf={(entry: StorageRow) =>
              describedOf(entry)?.thumbnailUrl}
            fileHref={rowHref}
            storageName={storage.storage}
            {fileActions}
            {sharing}
            onOpen={openFolder}
            onPrint={(entry: StorageRow) => {
              printing = entry;
              // Only a printer that is here can be asked.
              void printerStorages?.check(
                devices
                  .filter((device) => device.connected)
                  .map((device) => device.id),
              );
            }}
            onRename={startRename}
            onDelete={(entry: StorageRow) => {
              pending = entry;
            }}
            onClearSearch={() => {
              search('');
            }}
            onShare={flipShare}
          />
        </div>
      </CardContent>
    </Card>
  {/if}
</div>

{#if onPrint}
  <PrintDialog
    open={printing !== undefined}
    onOpenChange={(next) => {
      if (!next) printing = undefined;
    }}
    file={sheetFile}
    {devices}
    {transitions}
    {locked}
    {noStorage}
    {onPrint}
    {onPrinted}
  />
{/if}

<ConfirmDialog
  open={pending !== undefined}
  onOpenChange={(next) => {
    if (!next) pending = undefined;
  }}
  title={t('storage.delete.title', { name: pending?.name ?? '' })}
  description={pending?.folder === true
    ? t('storage.delete.folderNote')
    : t('storage.delete.fileNote')}
  confirmLabel={t('storage.action.delete')}
  onConfirm={remove}
/>

<ConfirmDialog
  open={offering !== undefined}
  onOpenChange={(next) => {
    if (!next) offering = undefined;
  }}
  title={t('storage.share.title', { name: offering?.name ?? '' })}
  description={t('storage.share.note', {
    // The storage by the name the servers below see, not its display name.
    path: `${storage.storage} · /${offering?.path ?? ''}`,
  })}
  confirmLabel={t('storage.share.confirm')}
  variant="confirm"
  onConfirm={confirmShare}
/>

<PromptDialog
  open={creating}
  onOpenChange={(open: boolean) => {
    creating = open;
    if (!open) folderName = '';
  }}
  title={t('storage.newFolder.title')}
  description={t('storage.newFolder.note')}
  label={t('storage.newFolder.label')}
  value={folderName}
  onValueChange={(next: string) => {
    folderName = next;
  }}
  error={folderFault}
  confirmLabel={t('storage.action.create')}
  onConfirm={createFolder}
/>

<PromptDialog
  open={renaming !== undefined}
  onOpenChange={(open: boolean) => {
    if (!open) renaming = undefined;
  }}
  title={t('storage.rename.title', { name: renaming?.name ?? '' })}
  description={t('storage.rename.note')}
  label={t('storage.rename.label')}
  value={newName}
  onValueChange={(next: string) => {
    newName = next;
  }}
  error={renameFault}
  confirmLabel={t('storage.action.rename')}
  onConfirm={() => void rename()}
/>

{#snippet actions()}
  {#if browsable && pickedFiles.length > 0}
    <Button variant="secondary" size="sm" class="shrink-0" onclick={download}>
      <DownloadIcon aria-hidden="true" />
      {t('storage.bulk.download', { count: pickedFiles.length })}
    </Button>
  {/if}
  {#if movable}
    <Button variant="secondary" size="sm" class="shrink-0" onclick={startMove}>
      <FolderInputIcon aria-hidden="true" />
      {t('storage.action.moveTo')}
    </Button>
  {/if}
{/snippet}

{#snippet idle()}
  {@const listed = storage.filesLoaded && filesFault === undefined}
  <ListingToolbar
    class="w-full"
    fields={listed ? offered : undefined}
    fieldState={listed ? listing.fields : undefined}
    view={listed
      ? {
          ...listing.view,
          options: listing.view.options.map((option) => ({
            ...option,
            iconSrc: option.value === 'table' ? rowsIcon : tilesIcon,
          })),
        }
      : undefined}
    afterFacets={narrowing}
    {trailing}
  />
{/snippet}

{#snippet narrowing()}
  {@render afterFacets()}
  <div class="min-w-0 grow basis-28">
    {@render searchBox()}
  </div>
{/snippet}

{#snippet searchBox()}
  <!-- The placeholder says the search covers only the open folder. -->
  <ListingSearch
    value={query}
    onValue={search}
    label={t('storage.search.label')}
    placeholder={t('storage.search.placeholder')}
  />
{/snippet}

{#snippet afterFacets()}
  {#if storage.storages.length > 1}
    <!-- The list gets the name too, because axe flags an open list that has
           no name of its own. -->
    <Select
      value={storage.storage}
      onValueChange={openStorage}
      items={storage.storages.map((row) => ({
        value: row.name,
        label: row.display_name,
        disabled: !row.browse,
      }))}
    >
      <SelectTrigger
        size="sm"
        aria-label={t('storage.choice.label')}
        class="w-fit"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent aria-label={t('storage.choice.label')}>
        {#each storage.storages as row (row.name)}
          <SelectItem value={row.name} disabled={!row.browse}>
            {row.display_name}
          </SelectItem>
        {/each}
      </SelectContent>
    </Select>
  {/if}
{/snippet}

{#snippet trailing()}
  {#if writable}
    <Button
      variant="secondary"
      size="field"
      class="shrink-0"
      onclick={() => {
        creating = true;
      }}
    >
      <FolderPlusIcon aria-hidden="true" />
      {t('storage.action.newFolder')}
    </Button>
  {/if}
{/snippet}

<!-- Moving an entry to the folder it is already in is what the server
     refuses, so that folder cannot be taken. -->
<StorageFolderDialog
  open={moving}
  onOpenChange={(next: boolean) => {
    moving = next;
  }}
  title={t('storage.move.title', { count: pickedRows.length })}
  description={t('storage.move.note')}
  confirmLabel={t('storage.action.moveHere')}
  {root}
  current={storage.directory}
  at={storage.pickerPath}
  folders={storage.pickerFolders}
  fault={storage.pickerError}
  onWalk={(path: string) => void storage.browseFolders(storage.storage, path)}
  onConfirm={(destination: string) => void moveTo(destination)}
/>
