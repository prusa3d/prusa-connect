<!--
  The places spools are kept, which everyone shares: each with how many live
  spools are kept there, renamed in its row and deleted after a confirmation
  that says how many spools it leaves with no location. A new place is added
  at the top. A person who may only look sees the list without controls.

  A settings pane takes no props, and the host draws it with no heading, so
  the pane names itself (see the catalogue tab).
-->
<script lang="ts">
  import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
    Button,
    Input,
    ListingSkeleton,
    RowList,
    SettingsPane,
  } from '@prusa/connect-ui/primitives';
  import { onMount } from 'svelte';

  import { type Location, materialsOn } from '../api';
  import { heldHost } from '../host';
  import { OWN_MODULE } from '../own-module';
  import { heldList } from '../list.svelte';
  import FaultNote from '../materials-page/fault-note.svelte';

  const host = heldHost();
  const t = host.t;
  const service = materialsOn(host);
  const list = heldList();

  let locations = $state.raw<readonly Location[]>([]);
  let loaded = $state(false);
  let busy = $state(false);
  let fault = $state.raw<unknown>(undefined);
  let adding = $state('');
  /** The location being renamed, and the name typed for it. */
  let editing = $state.raw<{ id: number; name: string } | undefined>(undefined);
  /** The location a delete waits on a confirmation for. */
  let deleting = $state.raw<Location | undefined>(undefined);

  const read = async (): Promise<void> => {
    try {
      locations = await service.locations();
    } catch (thrown) {
      fault = thrown;
    } finally {
      loaded = true;
    }
  };

  onMount(() => {
    void read();
  });

  /** Runs one write and reads the list again; a refusal stays on the pane. */
  const write = async (work: () => Promise<unknown>): Promise<boolean> => {
    busy = true;
    fault = undefined;
    try {
      await work();
      await read();

      return true;
    } catch (thrown) {
      fault = thrown;

      return false;
    } finally {
      busy = false;
    }
  };

  const add = async (event: SubmitEvent): Promise<void> => {
    event.preventDefault();
    const name = adding.trim();
    if (name === '') return;
    if (await write(() => service.createLocation(name))) adding = '';
  };

  const rename = async (event: SubmitEvent): Promise<void> => {
    event.preventDefault();
    const target = editing;
    if (target === undefined) return;
    if (await write(() => service.renameLocation(target.id, target.name))) {
      editing = undefined;
    }
  };

  const remove = async (): Promise<void> => {
    const target = deleting;
    deleting = undefined;
    if (target) await write(() => service.deleteLocation(target.id));
  };
</script>

<SettingsPane title={t('materials.locations.title')} edged>
  <div
    class="flex max-w-3xl flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <p class="text-body text-muted-foreground">
      {t('materials.locations.note')}
    </p>

    {#if list.mayWrite}
      <form class="flex gap-2" onsubmit={(event) => void add(event)}>
        <Input
          name="name"
          autocomplete="off"
          aria-label={t('materials.locations.new')}
          placeholder={t('materials.locations.new')}
          value={adding}
          oninput={(e: Event) =>
            (adding = (e.currentTarget as HTMLInputElement).value)}
        />
        <Button type="submit" size="md" disabled={busy || adding.trim() === ''}>
          {t('materials.locations.add')}
        </Button>
      </form>
    {/if}

    <FaultNote {fault} />

    {#if !loaded}
      <ListingSkeleton label={t('materials.locations.reading')} rows={3} />
    {:else if locations.length === 0}
      <p class="text-muted-foreground">{t('materials.locations.empty')}</p>
    {:else}
      <RowList data-part="locations">
        {#each locations as location (location.id)}
          <div
            class="flex items-center gap-3 border-b border-border px-3 py-2 text-sm last:border-b-0"
            data-location={location.id}
          >
            {#if editing?.id === location.id}
              <form
                class="flex flex-1 items-center gap-2"
                onsubmit={(event) => void rename(event)}
              >
                <Input
                  name="name"
                  autocomplete="off"
                  aria-label={t('materials.locations.renameOf', {
                    name: location.name,
                  })}
                  value={editing.name}
                  oninput={(e: Event) =>
                    (editing = {
                      id: location.id,
                      name: (e.currentTarget as HTMLInputElement).value,
                    })}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={busy || editing.name.trim() === ''}
                >
                  {t('materials.locations.save')}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onclick={() => (editing = undefined)}
                >
                  {t('materials.add.cancel')}
                </Button>
              </form>
            {:else}
              <span class="min-w-0 flex-1 truncate font-medium"
                >{location.name}</span
              >
              <span class="text-muted-foreground">
                {t('materials.locations.spools', { n: location.spools })}
              </span>
              {#if list.mayWrite}
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('materials.locations.renameOf', {
                    name: location.name,
                  })}
                  disabled={busy}
                  onclick={() => {
                    fault = undefined;
                    editing = { id: location.id, name: location.name };
                  }}
                >
                  {t('materials.locations.rename')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('materials.locations.deleteOf', {
                    name: location.name,
                  })}
                  disabled={busy}
                  onclick={() => (deleting = location)}
                >
                  {t('materials.confirm.delete')}
                </Button>
              {/if}
            {/if}
          </div>
        {/each}
      </RowList>
    {/if}
  </div>
</SettingsPane>

<AlertDialog
  open={deleting !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) deleting = undefined;
  }}
>
  <AlertDialogContent data-module={OWN_MODULE}>
    <AlertDialogHeader>
      <AlertDialogTitle>
        {t('materials.locations.deleteAsk', { name: deleting?.name ?? '' })}
      </AlertDialogTitle>
      <AlertDialogDescription>
        {t('materials.locations.leftWithout', { n: deleting?.spools ?? 0 })}
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>{t('materials.add.cancel')}</AlertDialogCancel>
      <AlertDialogAction variant="destructive" onclick={() => void remove()}>
        {t('materials.confirm.delete')}
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
