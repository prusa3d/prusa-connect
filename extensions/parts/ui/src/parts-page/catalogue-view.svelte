<!--
  The catalogue with no part open: the folder tree beside the listing of the
  folder on screen.

  The header holds the act the screen exists for, a new part, put in the
  folder on screen. The folder column offers the rest: the dropzone at its
  head, the way the Storage screen has it, and a new folder beside the tree.
  Each is drawn only for a person who may write.

  The dialogs are held open by this view rather than hung off a trigger, so a
  row's own menu, the header and the empty state open the same one.

  Files dropped anywhere on the listing become parts of the folder on screen,
  and files dropped onto a folder of the tree become parts of that one. The
  upload in the header does the same as a drop onto the listing, for a person
  who cannot drag.
-->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import CloudUploadIcon from '@lucide/svelte/icons/cloud-upload';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import {
    Button,
    Card,
    CardContent,
    ConfirmDialog,
    PageHeader,
  } from '@prusa/connect-ui/primitives';

  import type { Folder } from '../parts';
  import { heldSession } from '../session';
  import DropTray from './drop-tray.svelte';
  import { fileDrop } from './file-drop';
  import type { FolderAct } from './folder-act';
  import FolderDialog from './folder-dialog.svelte';
  import FolderTree from './folder-tree.svelte';
  import FolderUpload from './folder-upload.svelte';
  import { SPLIT_LEADING, SPLIT_ROW, SPLIT_TRAILING } from './layout';
  import PartDialog from './part-dialog.svelte';
  import PartListing from './part-listing.svelte';
  import TagList from './tag-list.svelte';
  import UsageMeter from './usage-meter.svelte';

  const { catalogue, navigation, drop, host } = heldSession();
  const t = host.t;

  let folderAct = $state<FolderAct | undefined>(undefined);
  let creatingPart = $state(false);
  let deleting = $state<Folder | undefined>(undefined);

  const here = $derived(navigation.spot.folder);
  const mayWrite = $derived(catalogue.allowed.write);
  const hereName = $derived(
    catalogue.folder(here)?.name ?? t('parts.tree.all'),
  );

  // Stacked above the listing the tree is a screenful of its own, so it
  // starts shut there and shuts again once a folder is picked: it stays open
  // only in the folder it was opened in.
  let openIn = $state<number | undefined>(undefined);
  const treeOpen = $derived(openIn === here);

  const removeFolder = async (): Promise<void> => {
    const folder = deleting;
    deleting = undefined;
    if (folder === undefined) return;

    // A folder deleted from inside itself leaves the reader in its parent.
    if (await catalogue.deleteFolder(folder.id)) {
      if (here === folder.id) navigation.openFolder(folder.parent);
    }
  };
</script>

<PageHeader
  title={t('parts.title')}
  action={mayWrite
    ? {
        label: t('parts.action.newPart'),
        icon: PlusIcon,
        onclick: () => {
          creatingPart = true;
        },
      }
    : undefined}
/>

{#if catalogue.allowedRead && !mayWrite}
  <p data-part="viewer" class="text-sm text-muted-foreground">
    {t('parts.viewer')}
  </p>
{/if}

<Card>
  <CardContent class={SPLIT_ROW}>
    <aside class={SPLIT_LEADING} aria-label={t('parts.tree.label')}>
      <Button
        variant="secondary"
        size="sm"
        class="justify-between @4xl/parts:hidden"
        aria-expanded={treeOpen}
        data-part="tree-toggle"
        onclick={() => {
          openIn = treeOpen ? undefined : here;
        }}
      >
        <span class="truncate">{t('parts.tree.heading')}: {hereName}</span>
        <ChevronDownIcon
          aria-hidden="true"
          class={treeOpen ? 'rotate-180' : undefined}
        />
      </Button>
      <div
        class={[
          treeOpen ? 'grid' : 'hidden',
          'content-start gap-y-6 @4xl/parts:grid',
        ]}
      >
        <FolderUpload folder={here} />
        <FolderTree
          onNewFolder={() => {
            folderAct = { kind: 'create', parent: here };
          }}
        />
        <TagList />
        <UsageMeter />
      </div>
    </aside>

    <div
      class={[
        SPLIT_TRAILING,
        'group/drop relative grid min-w-0 content-start gap-y-4',
      ]}
      data-part="listing-drop"
      {@attach fileDrop((dropped) => {
        void drop.drop(here, dropped);
      }, drop.allowed)}
    >
      <DropTray />
      <PartListing
        onNewPart={() => {
          creatingPart = true;
        }}
        onChangeFolder={(folder: Folder) => {
          folderAct = { kind: 'change', folder };
        }}
        onDeleteFolder={(folder: Folder) => {
          deleting = folder;
        }}
      />
      <div
        class="pointer-events-none absolute inset-0 top-6 hidden place-content-center justify-items-center gap-y-2 rounded-md border-2 border-dashed border-primary bg-secondary p-6 text-center text-secondary-foreground group-data-dropping/drop:grid group-data-dropping/drop:animate-fade-in @4xl/parts:start-6 @4xl/parts:top-0"
        aria-hidden="true"
      >
        <CloudUploadIcon class="size-8 text-primary" />
        <p class="text-base font-medium">
          {t('parts.drop.hint', { folder: hereName })}
        </p>
        <p class="max-w-sm text-sm text-muted-foreground">
          {t('parts.drop.note')}
        </p>
      </div>
    </div>
  </CardContent>
</Card>

<FolderDialog
  act={folderAct}
  onClose={() => {
    folderAct = undefined;
  }}
/>

<PartDialog
  open={creatingPart}
  folder={here}
  onClose={() => {
    creatingPart = false;
  }}
/>

<ConfirmDialog
  open={deleting !== undefined}
  onOpenChange={(open: boolean) => {
    if (!open) deleting = undefined;
  }}
  title={t('parts.folder.delete.title', { name: deleting?.name ?? '' })}
  description={t('parts.folder.delete.note')}
  confirmLabel={t('parts.action.delete')}
  onConfirm={() => void removeFolder()}
/>
