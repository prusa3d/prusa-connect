<!--
  The spools of one material.

  The uuid comes from the search of the address, because a module page is one
  path segment. The material is drawn from the row the module answers, which
  is there whether the catalogue knows the material, the person made it, or
  neither: a material built from a spool's copy still lists its spools.

  A person who may not write sees the material and its spools and no control
  the module would refuse.

  The printers the spools are on are read after the spools, to name them,
  and so are their tools, for what each row offers as the Tools tab does. A
  failed read draws the rows without names or without those actions.
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
    Card,
    CardContent,
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    EmptyState,
    ListingSkeleton,
    PageHeader,
    PageTrail,
    Selection,
    Toggle,
    toast,
  } from '@prusa/connect-ui/primitives';
  import { STATE_PATH, textAt } from '@prusa/connect-ui/domain';
  import { onMount, untrack } from 'svelte';

  import AddDialog from '../add-dialog/add-dialog.svelte';
  import CustomMaterialForm from '../add-dialog/custom-material-form.svelte';
  import AssignPrinterDialog from './assign-printer-dialog.svelte';
  import MaterialDetails from './material-details.svelte';
  import MergeSpoolDialog from './merge-spool-dialog.svelte';
  import {
    type Detail,
    type MaterialFields,
    materialsOn,
    type Printer,
    type Spool,
    type SpoolPatch,
    type SpoolsDraft,
  } from '../api';
  import { heldHost } from '../host';
  import { follow, type SpoolChange } from '../live';
  import { OWN_MODULE } from '../own-module';
  import { heldList } from '../list.svelte';
  import FaultNote from '../materials-page/fault-note.svelte';
  import { holdPrinterMaterials } from '../printer-material/printer-material-state.svelte';
  import { unloadFilament } from '../printer-material/tool-offer';
  import EditSpoolDialog from './edit-spool-dialog.svelte';
  import SpoolTable from './spool-table.svelte';

  const host = heldHost();
  const t = host.t;
  const service = materialsOn(host);
  const list = heldList();

  const uuid = new URLSearchParams(location.search).get('uuid') ?? '';

  let detail = $state.raw<Detail | undefined>(undefined);
  let spools = $state.raw<readonly Spool[]>([]);
  let printers = $state.raw<readonly Printer[]>([]);
  // Where the next page of spools starts, or undefined on the last one.
  let next = $state<number | undefined>(undefined);
  let loaded = $state(false);
  // The material is not there: no uuid, a 404, or it was just deleted.
  let gone = $state(uuid === '');
  let fault = $state.raw<unknown>(undefined);
  let busy = $state(false);

  let adding = $state(false);
  let editing = $state.raw<Spool | undefined>(undefined);
  let assigning = $state.raw<Spool | undefined>(undefined);
  let merging = $state.raw<Spool | undefined>(undefined);
  let editingMaterial = $state(false);
  /** What the confirmation will do, and what it says. */
  let confirming = $state.raw<
    { title: string; run: () => Promise<void> } | undefined
  >(undefined);
  // A refusal inside a dialog is drawn in that dialog and not under the page.
  let dialogFault = $state.raw<unknown>(undefined);

  const mayEditMaterial = $derived(
    list.mayWrite && detail?.custom !== undefined,
  );
  // A material only spools still know (a re-import dropped it, or it is a
  // colleague's custom one) takes no new spool: the module refuses it. It may
  // still be starred, being a row of the list.
  const known = $derived(
    detail?.material !== undefined || detail?.custom !== undefined,
  );

  let nameVersion = 0;

  /**
   * Reads the printers the listed spools are on. Each printer answered costs
   * the module a question to the host, so the page asks for no others.
   */
  const name = (): void => {
    const version = ++nameVersion;
    const ids = new Set(
      spools.flatMap((s) => (s.tool ? [s.tool.device_id] : [])),
    );
    if (ids.size === 0) return;
    service.printers([...ids]).then(
      (found) => {
        if (version === nameVersion) printers = found;
      },
      () => undefined,
    );
  };

  /**
   * Each printer still holding a listed spool's tool, held as its Tools tab
   * holds it: read on mount, again a moment after one of its tools changes,
   * and every few seconds while the page is in view. A printer the person
   * may not observe has no id here.
   */
  const heldIds = $derived(
    JSON.stringify(
      [
        ...new Set(
          spools.flatMap((s) =>
            s.tool && !s.tool.gone && s.tool.device_id !== ''
              ? [s.tool.device_id]
              : [],
          ),
        ),
      ].sort(),
    ),
  );
  // Not reactive: the holds are taken and released by the effect below.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const holds = new Map<string, ReturnType<typeof holdPrinterMaterials>>();
  let held = $state.raw<
    ReadonlyMap<string, ReturnType<typeof holdPrinterMaterials>['state']>
  >(new Map());
  $effect(() => {
    const ids = JSON.parse(heldIds) as string[];
    untrack(() => {
      for (const [id, hold] of holds) {
        if (!ids.includes(id)) {
          hold.release();
          holds.delete(id);
        }
      }
      for (const id of ids) {
        if (!holds.has(id))
          holds.set(id, holdPrinterMaterials(host, service, id));
      }
      held = new Map([...holds].map(([id, hold]) => [id, hold.state]));
    });
  });
  onMount(() => () => {
    for (const hold of holds.values()) hold.release();
    holds.clear();
  });

  /** The tools of each held printer, once read. */
  const tools = $derived(
    new Map(
      [...held].flatMap(([id, state]) =>
        state.printer === undefined ? [] : [[id, state.printer]],
      ),
    ),
  );

  /**
   * The state word and link of each held printer, as the host's fleet keeps
   * them live. A print that starts or ends changes what the rules allow, and
   * the module tells of no such change, so the Tools tab's read follows it.
   */
  const live = $derived(
    new Map(
      [...held.keys()].map((id) => {
        const device = host.fleet.device(id);
        return [
          id,
          device === undefined
            ? ''
            : `${String(device.connected)} ${textAt(device, STATE_PATH) ?? ''}`,
        ];
      }),
    ),
  );
  /** What each held printer's last read already answers for. */
  let seen = new Map<string, string>();
  $effect(() => {
    const now = live;
    untrack(() => {
      for (const [id, state] of now) {
        if (seen.has(id) && seen.get(id) !== state) held.get(id)?.soon();
      }
      seen = now;
    });
  });

  let readVersion = 0;

  const read = async (
    keepPages = false,
    signal?: AbortSignal,
  ): Promise<void> => {
    const version = ++readVersion;
    fault = undefined;
    try {
      const [found, first] = await Promise.all([
        service.material(uuid, signal),
        service.spools(uuid, undefined, signal),
      ]);
      const rows = [...first.spools];
      let offset = first.next;
      while (
        keepPages &&
        rows.length < spools.length &&
        offset !== undefined &&
        version === readVersion
      ) {
        const page = await service.spools(uuid, offset, signal);
        rows.push(...page.spools);
        offset = page.next;
      }
      if (version !== readVersion || signal?.aborted) return;
      detail = found;
      spools = rows;
      next = offset;
      name();
      if (keepPages) {
        selection.set(
          rows.filter((spool) => spool.tool != null).map((spool) => spool.id),
          false,
        );
      } else {
        selection.clear();
      }
    } catch (thrown) {
      if (version !== readVersion || signal?.aborted) return;
      if ((thrown as { status?: number }).status === 404) gone = true;
      else fault = thrown;
    } finally {
      if (version === readVersion && !signal?.aborted) loaded = true;
    }
  };

  const more = async (): Promise<void> => {
    if (next === undefined) return;
    const version = readVersion;
    fault = undefined;
    busy = true;
    try {
      const page = await service.spools(uuid, next);
      if (version !== readVersion) return;
      spools = [...spools, ...page.spools];
      next = page.next;
      name();
    } catch (thrown) {
      if (version === readVersion) fault = thrown;
    } finally {
      busy = false;
    }
  };

  /**
   * A spool of this material told of on the event stream, such as one losing
   * filament as it prints, takes its new weight in place: a read again would
   * otherwise reread the pages for every change in weight.
   */
  const weigh = (change: SpoolChange): void => {
    if (change.material_uuid !== uuid || change.deleted === true) return;
    spools = spools.map((s) =>
      s.id === change.spool_id
        ? { ...s, remaining_weight_g: change.remaining_weight_g }
        : s,
    );
  };

  onMount(() => {
    if (uuid === '') {
      loaded = true;

      return;
    }

    const following = new AbortController();
    const signal = AbortSignal.any([following.signal, host.signal]);
    let refresh: ReturnType<typeof setTimeout> | undefined;
    void read(false, signal);
    follow(host.client, signal, ['spool', 'tool'], (endpoint, change) => {
      if (change !== undefined && 'spool_uuid' in change) {
        weigh(change);
      } else if (endpoint === 'tool') {
        // The spool route applies the caller's permissions to the tool's location.
        clearTimeout(refresh);
        refresh = setTimeout(() => void read(true, signal), 300);
      }
    });

    return () => {
      clearTimeout(refresh);
      following.abort();
    };
  });

  /** Runs one write; a refusal is kept for the dialog or page that asked. */
  const attempt = async (work: () => Promise<void>): Promise<boolean> => {
    busy = true;
    dialogFault = undefined;
    try {
      await work();

      return true;
    } catch (thrown) {
      dialogFault = thrown;

      return false;
    } finally {
      busy = false;
    }
  };

  const saveSpool = async (patch: SpoolPatch): Promise<void> => {
    const target = editing;
    if (target === undefined) return;
    const saved = await attempt(async () => {
      const next = await service.updateSpool(target.id, patch);
      spools = spools.map((s) => (s.id === next.id ? next : s));
    });
    if (saved) editing = undefined;
  };

  const mergeSpool = async (into: number): Promise<void> => {
    const from = merging;
    if (from === undefined) return;
    const merged = await attempt(async () => {
      await service.merge(from.id, into);
    });
    if (!merged) return;
    merging = undefined;
    await read();
  };

  const saveMaterial = async (fields: MaterialFields): Promise<void> => {
    const saved = await attempt(async () => {
      await service.updateCustom(uuid, fields);
      await read();
    });
    if (saved) editingMaterial = false;
  };

  const confirmed = async (): Promise<void> => {
    const pending = confirming;
    confirming = undefined;
    if (pending === undefined) return;
    fault = undefined;
    // A refusal here is drawn under the page, as the dialog has closed.
    if (!(await attempt(pending.run))) {
      fault = dialogFault;
      dialogFault = undefined;
    }
  };

  // A copy is a new spool with the same package, weights, place and notes,
  // added at once; one copied by mistake goes with Delete.
  const copySpool = async (spool: Spool): Promise<void> => {
    fault = undefined;
    const copied = await attempt(async () => {
      // A field the spool lacks is left out of the draft, not set undefined.
      const kept = {
        material_uuid: spool.material_uuid,
        package_uuid: spool.package_uuid,
        full_weight_g: spool.full_weight_g,
        empty_weight_g: spool.empty_weight_g,
        length_mm: spool.length_mm,
        remaining_weight_g: spool.remaining_weight_g,
        notes: spool.notes,
        location_id: spool.location_id,
        quantity: 1,
      };
      await service.addSpools(
        Object.fromEntries(
          Object.entries(kept).filter(([, value]) => value !== undefined),
        ) as SpoolsDraft,
      );
    });
    if (copied) await read();
    else {
      fault = dialogFault;
      dialogFault = undefined;
    }
  };

  const unassignSpool = async (spool: Spool): Promise<void> => {
    const on = spool.tool;
    if (on == null) return;
    fault = undefined;
    const cleared = await attempt(async () => {
      await service.unassign(on.device_id, on.tool_id);
    });
    if (cleared) await read();
    else {
      fault = dialogFault;
      dialogFault = undefined;
    }
  };

  // The printer unloads the head it prints with, which is the one this row
  // offers it on.
  const unloadSpool = async (spool: Spool): Promise<void> => {
    const on = spool.tool;
    if (on == null) return;
    fault = undefined;
    if (await attempt(() => unloadFilament(host, on.device_id))) await read();
    else {
      fault = dialogFault;
      dialogFault = undefined;
    }
  };

  /** The ticked spools, which Delete selected deletes together. */
  const selection = new Selection<number>({
    rows: () => spools.map((s) => s.id),
  });

  // Deleted spools leave the list, which drops their ticks, and their places on the later pages.
  const removed = (ids: readonly number[]): void => {
    spools = spools.filter((s) => !ids.includes(s.id));
    if (next !== undefined) next -= ids.length;
  };

  const askDeleteSpool = (spool: Spool): void => {
    confirming = {
      title: t('materials.spools.deleteAsk', { id: spool.id }),
      run: async () => {
        await service.deleteSpool(spool.id);
        removed([spool.id]);
      },
    };
  };

  // The module deletes at most this many spools in one call.
  const BATCH = 100;

  // Each call is all or none: a refusal leaves its spools and the later
  // ones ticked and listed, and the earlier calls' spools gone.
  const askDeleteSelected = (): void => {
    const ids = [...selection.keys];
    confirming = {
      title: t('materials.spools.deleteSelectedAsk', { count: ids.length }),
      run: async () => {
        for (let at = 0; at < ids.length; at += BATCH) {
          const batch = ids.slice(at, at + BATCH);
          await service.deleteSpools(batch);
          removed(batch);
        }
      },
    };
  };

  const askDeleteMaterial = (): void => {
    confirming = {
      title: t('materials.material.deleteAsk', { name: detail?.name ?? '' }),
      run: async () => {
        await service.deleteCustom(uuid);
        toast.success(t('materials.material.deleted'));
        // The list shows it no more, so the next read drops it.
        void list.read();
        detail = undefined;
        gone = true;
      },
    };
  };

  const star = async (): Promise<void> => {
    if (detail === undefined) return;
    const on = !detail.is_favourite;
    fault = undefined;
    // The list keeps the refusal for its own page, so this page draws it too.
    if (await list.star(uuid, on)) detail = { ...detail, is_favourite: on };
    else fault = list.fault;
  };
</script>

<section class="@container flex flex-col gap-4">
  <!-- The way back to the list, as a printer's page has it: above the
       heading, ending in this material's name. -->
  <PageTrail
    trail={[
      { label: t('materials.page.title'), href: host.pageHref('materials') },
    ]}
    page={detail?.name ?? t('materials.material.page.title')}
  />

  {#if gone}
    <EmptyState
      headingLevel="h2"
      heading={t('materials.material.gone.heading')}
      description={t('materials.material.gone.description')}
    >
      {#snippet action()}
        <a
          href={host.pageHref('materials')}
          class="underline underline-offset-4"
        >
          {t('materials.material.back')}
        </a>
      {/snippet}
    </EmptyState>
  {:else}
    <PageHeader title={detail?.name ?? t('materials.material.page.title')}>
      {#snippet actions()}
        {#if detail !== undefined && (known || detail.spools > 0)}
          <Toggle
            size="md"
            bind:pressed={
              () => detail?.is_favourite ?? false, () => void star()
            }
            aria-label={t('materials.row.star', { name: detail.name })}
            disabled={!list.mayWrite || list.busy}
          >
            <span
              class={[
                'text-base',
                detail.is_favourite &&
                  'text-(--color-orange-light-9) dark:text-(--color-orange-dark-9)',
              ]}
              aria-hidden="true"
            >
              {detail.is_favourite ? '★' : '☆'}
            </span>
          </Toggle>
        {/if}
        {#if mayEditMaterial}
          <Button
            variant="secondary"
            size="md"
            onclick={() => {
              dialogFault = undefined;
              editingMaterial = true;
            }}
          >
            {t('materials.material.edit')}
          </Button>
          <Button variant="destructive" size="md" onclick={askDeleteMaterial}>
            {t('materials.material.delete')}
          </Button>
        {/if}
      {/snippet}
    </PageHeader>

    {#if list.mayOnlyRead}
      <p data-part="viewer" class="text-sm text-muted-foreground">
        {t('materials.page.viewer')}
      </p>
    {/if}

    <FaultNote {fault} />

    {#if !loaded}
      <ListingSkeleton label={t('materials.page.reading')} rows={4} />
    {:else if detail !== undefined}
      <Card>
        <CardContent>
          <MaterialDetails {detail} />
        </CardContent>
      </Card>

      <Card>
        <CardContent class="flex flex-col gap-4">
          <div class="flex items-center justify-between gap-3">
            <h2 id="spools" class="text-lg font-medium">
              {t('materials.spools.title')}
            </h2>
            <div class="flex gap-2">
              {#if list.mayWrite && selection.count > 0}
                <Button
                  variant="destructive"
                  size="sm"
                  onclick={askDeleteSelected}
                >
                  {t('materials.spools.deleteSelected', {
                    count: selection.count,
                  })}
                </Button>
              {/if}
              {#if list.mayWrite && known}
                <Button size="sm" onclick={() => (adding = true)}>
                  {t('materials.spools.add')}
                </Button>
              {/if}
            </div>
          </div>
          {#if !known}
            <p data-part="unknown" class="text-sm text-muted-foreground">
              {t('materials.material.unknown')}
            </p>
          {/if}
          {#if spools.length === 0}
            <EmptyState
              headingLevel="h3"
              heading={t('materials.spools.empty.heading')}
              description={t('materials.spools.empty.description')}
            />
          {:else}
            <SpoolTable
              {spools}
              packages={detail.packages}
              mayWrite={list.mayWrite}
              {busy}
              {printers}
              {tools}
              mergeable={next !== undefined ||
                spools.some((s) => s.source === 'printer')}
              onassign={(spool: Spool) => (assigning = spool)}
              onunassign={(spool: Spool) => void unassignSpool(spool)}
              onunload={(spool: Spool) => void unloadSpool(spool)}
              onmerge={(spool: Spool) => {
                dialogFault = undefined;
                merging = spool;
              }}
              onedit={(spool: Spool) => {
                dialogFault = undefined;
                editing = spool;
              }}
              oncopy={(spool: Spool) => void copySpool(spool)}
              ondelete={askDeleteSpool}
              {selection}
            />
            {#if next !== undefined}
              <Button
                variant="secondary"
                class="self-start"
                disabled={busy}
                onclick={() => void more()}
              >
                {t('materials.page.more')}
              </Button>
            {/if}
          {/if}
        </CardContent>
      </Card>
    {/if}
  {/if}
</section>

<AddDialog
  open={adding}
  material={uuid}
  onclose={() => (adding = false)}
  onadded={() => void read()}
/>

<EditSpoolDialog
  spool={editing}
  {busy}
  fault={dialogFault}
  onsave={(patch: SpoolPatch) => void saveSpool(patch)}
  onclose={() => (editing = undefined)}
/>

<AssignPrinterDialog
  {service}
  spool={assigning}
  onclose={() => (assigning = undefined)}
  onassigned={() => void read()}
/>

<MergeSpoolDialog
  {service}
  spool={merging}
  loaded={spools}
  {next}
  {busy}
  fault={dialogFault}
  onmerge={(into: number) => void mergeSpool(into)}
  onclose={() => (merging = undefined)}
/>

<Dialog
  open={editingMaterial}
  onOpenChange={(next: boolean) => {
    if (!next) editingMaterial = false;
  }}
>
  <!-- Portalled, so the panel carries the module's name for the utilities
       inside it. A scoped rule never matches the scope's root, so the
       panel's own cap is the host's. -->
  <DialogContent
    data-module={OWN_MODULE}
    size="xl"
    class="max-h-dialog-height overflow-y-auto"
  >
    <DialogHeader>
      <DialogTitle>{t('materials.material.editTitle')}</DialogTitle>
    </DialogHeader>
    {#if editingMaterial && detail?.custom}
      <CustomMaterialForm
        initial={detail.custom}
        submitLabel={t('materials.material.save')}
        {busy}
        fault={dialogFault}
        onsave={(fields: MaterialFields) => void saveMaterial(fields)}
        oncancel={() => (editingMaterial = false)}
      />
    {/if}
  </DialogContent>
</Dialog>

<AlertDialog
  open={confirming !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) confirming = undefined;
  }}
>
  <AlertDialogContent data-module={OWN_MODULE}>
    <AlertDialogHeader>
      <AlertDialogTitle>{confirming?.title ?? ''}</AlertDialogTitle>
      <AlertDialogDescription>
        {t('materials.confirm.note')}
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>{t('materials.add.cancel')}</AlertDialogCancel>
      <AlertDialogAction variant="destructive" onclick={() => void confirmed()}>
        {t('materials.confirm.delete')}
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
