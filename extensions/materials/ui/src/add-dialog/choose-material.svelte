<!--
  Step 1: find the material. On the left, the search, a manufacturer, a type
  and a colour narrow `GET /materials` to 20 rows, with **+ Create custom
  material** above them; on the right, the picked material and its details, or
  a note saying they will show there. Continue takes the picked one to step 2.
  The custom path swaps the whole step for the custom-material form, which
  saves the material and goes on to step 2 with it.

  With no catalogue loaded the list holds only the person's own materials, so
  the filters are left away and a hint points at the catalogue settings.

  The list is a `MaterialList` of its own, so narrowing it here leaves the
  page's list behind the dialog as it was.
-->
<script lang="ts">
  import {
    Button,
    Card,
    CardContent,
    DialogFooter,
    ListingSkeleton,
    MaterialSwatch,
    ListingSearch,
  } from '@prusa/connect-ui/primitives';
  import { FilamentColorWheel } from '@prusa/connect-ui/shell';
  import { moduleTabId } from '@prusa/connect-ui/slots';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import { onMount, untrack } from 'svelte';

  import type { Detail, MaterialFields, MaterialsService } from '../api';
  import { heldCatalogue } from '../catalogue.svelte';
  import { heldHost } from '../host';
  import { MaterialList } from '../list.svelte';
  import MaterialDetails from '../material-page/material-details.svelte';
  import FaultNote from '../materials-page/fault-note.svelte';
  import { WHEEL, WHEEL_OUTER } from '../colour-palette';
  import ColourSwatches from '../colour-swatches.svelte';
  import SearchSelect from '../search-select.svelte';
  import CustomMaterialForm from './custom-material-form.svelte';

  let {
    service,
    chosen,
    colourWheel = false,
    onchoose,
    oncancel,
  }: {
    service: MaterialsService;
    /** The material step 2 had, when the person came back to change it. */
    chosen?: Detail | undefined;
    colourWheel?: boolean;
    onchoose: (detail: Detail) => void;
    /** Closes the dialog. */
    oncancel: () => void;
  } = $props();

  const host = heldHost();
  const t = host.t;
  const families = WHEEL;
  const colorLabel = (color: (typeof families)[number]) => t(color[2]);
  const catalogue = heldCatalogue();

  /** The dialog asks the core nothing: only a person who may write opens it. */
  const list = new MaterialList(
    untrack(() => service),
    Promise.resolve(true),
  );

  /** How long the search waits for the person to stop typing. */
  const DEBOUNCE_MS = 250;
  const LIMIT = 20;

  const brandOptions = $derived(
    list.brands.map((brand) => ({ value: brand.slug, label: brand.name })),
  );
  const typeOptions = $derived(
    list.types.map((type) => ({ value: type, label: type })),
  );

  // The host writes /m/<namespace>/<path>, which is how a module learns the
  // namespace the catalogue settings tab is under.
  const namespace = host.pageHref('').split('/')[2] ?? '';
  const settingsHref = `/settings/extensions/${namespace}/${moduleTabId(namespace, 'catalogue')}`;

  const noCatalogue = $derived(catalogue.status?.loaded === false);

  let typed = $state('');
  let timer: ReturnType<typeof setTimeout> | undefined;
  let picked = $state.raw<Detail | undefined>(untrack(() => chosen));
  /** The uuid whose details are being read, so a later pick wins. */
  let picking = $state<string | undefined>(undefined);
  let fault = $state.raw<unknown>(undefined);
  let creating = $state(false);
  let saving = $state(false);

  onMount(() => {
    void (async () => {
      await catalogue.read();
      const custom = catalogue.status?.loaded === false;
      await list.narrow({
        limit: LIMIT,
        custom: custom || undefined,
      });
      if (!custom) await list.readFacets();
    })();

    return () => {
      clearTimeout(timer);
    };
  });

  // One request once the person stops typing, not one per letter.
  const search = (text: string): void => {
    typed = text;
    clearTimeout(timer);
    timer = setTimeout(() => {
      const q = typed.trim();
      void list.narrow({ q: q === '' ? undefined : q });
    }, DEBOUNCE_MS);
  };

  const clear = (): void => {
    typed = '';
    clearTimeout(timer);
    void list.narrow({
      q: undefined,
      brand: undefined,
      type: undefined,
      color: undefined,
    });
  };

  const narrowed = $derived(
    typed !== '' ||
      ['brand', 'type', 'color', 'q'].some((key) => key in list.query),
  );

  const pick = async (uuid: string): Promise<void> => {
    picking = uuid;
    fault = undefined;
    try {
      const detail = await service.material(uuid);
      if (picking === uuid) picked = detail;
    } catch (thrown) {
      if (picking === uuid) fault = thrown;
    } finally {
      if (picking === uuid) picking = undefined;
    }
  };

  const create = async (fields: MaterialFields): Promise<void> => {
    saving = true;
    fault = undefined;
    try {
      const uuid = await service.createCustom(fields);
      onchoose(await service.material(uuid));
    } catch (thrown) {
      fault = thrown;
    } finally {
      saving = false;
    }
  };

  const meta = (brand: string, type: string): string =>
    [brand, type].filter((s) => s !== '').join(' · ');
</script>

{#snippet heading(text: string)}
  <p class="text-sm font-semibold text-muted-foreground">{text}</p>
{/snippet}

{#if creating}
  <CustomMaterialForm
    submitLabel={t('materials.custom.create')}
    busy={saving}
    {fault}
    onsave={(fields: MaterialFields) => void create(fields)}
    oncancel={() => {
      creating = false;
      fault = undefined;
    }}
  />
{:else}
  <div data-part="choose-material" class="flex flex-col gap-4">
    <div class="flex flex-wrap items-start gap-6">
      <!-- Find material: the left, wider column. -->
      <div class="find flex min-w-0 flex-col gap-3" data-part="find">
        <div class="flex items-center justify-between gap-2">
          {@render heading(t('materials.add.find'))}
          <Button variant="ghost" size="sm" onclick={() => (creating = true)}>
            <PlusIcon aria-hidden="true" />
            {t('materials.add.create')}
          </Button>
        </div>

        {#if noCatalogue}
          <p data-part="no-catalogue" class="text-sm text-muted-foreground">
            {t('materials.add.noCatalogue')}
            <a href={settingsHref} class="underline underline-offset-4">
              {t('materials.add.settings')}
            </a>
          </p>
        {:else}
          <ListingSearch
            label={t('materials.filter.search')}
            placeholder={t('materials.add.searchPlaceholder')}
            value={typed}
            onValue={search}
          />

          <div class="flex flex-wrap items-center gap-2">
            <SearchSelect
              label={t('materials.filter.brand')}
              allLabel={t('materials.filter.anyBrand')}
              options={brandOptions}
              value={list.query.brand}
              onPick={(brand: string | undefined) =>
                void list.narrow({ brand })}
            />
            <SearchSelect
              label={t('materials.add.type')}
              allLabel={t('materials.add.anyType')}
              options={typeOptions}
              value={list.query.type}
              onPick={(type: string | undefined) => void list.narrow({ type })}
            />
            {#if narrowed}
              <Button variant="ghost" size="sm" onclick={clear}>
                {t('materials.add.clear')}
              </Button>
            {/if}
          </div>

          <div class="flex items-center gap-3">
            <span class="text-sm text-muted-foreground">
              {t('materials.add.colour')}
            </span>
            {#if colourWheel}
              <div class="w-32 shrink-0">
                <FilamentColorWheel
                  colors={families}
                  outerCount={WHEEL_OUTER}
                  selected={families.find(
                    ([family]) => family === list.query.color,
                  )}
                  labelOf={colorLabel}
                  onChoose={([color]: (typeof families)[number]) =>
                    void list.narrow({
                      color: list.query.color === color ? undefined : color,
                    })}
                />
              </div>
            {:else}
              <ColourSwatches
                value={list.query.color}
                onPick={(color: string | undefined) =>
                  void list.narrow({ color })}
              />
            {/if}
          </div>
        {/if}

        <FaultNote fault={fault ?? list.fault} />

        <div
          data-part="choices"
          class="h-88 overflow-y-auto rounded-md border border-border"
        >
          {#if !list.loaded}
            <div class="p-3">
              <ListingSkeleton label={t('materials.add.reading')} rows={5} />
            </div>
          {:else if list.rows.length === 0}
            <p class="p-3 text-sm text-muted-foreground">
              {t('materials.add.none')}
            </p>
          {:else}
            <ul class="flex flex-col">
              {#each list.rows as row (row.uuid)}
                {@const on = picked?.uuid === row.uuid}
                <li class="border-b border-border last:border-b-0">
                  <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- standing exception (ui-integration.md): a full-width pressable picker row no primitive draws -->
                  <button
                    type="button"
                    aria-pressed={on}
                    class={[
                      'flex w-full items-center gap-3 px-3 py-2 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring',
                      on
                        ? 'bg-primary text-primary-foreground'
                        : 'hover:bg-accent',
                    ]}
                    onclick={() => void pick(row.uuid)}
                  >
                    <MaterialSwatch
                      class="size-10 shrink-0"
                      material={{
                        name: row.name,
                        colorRgba:
                          row.color_rgba === '' ? undefined : row.color_rgba,
                      }}
                    />
                    <span class="flex min-w-0 flex-1 flex-col">
                      <span class="truncate font-medium">{row.name}</span>
                      <span
                        class={[
                          'truncate text-xs',
                          !on && 'text-muted-foreground',
                        ]}
                      >
                        {meta(row.brand_name, row.type)}
                      </span>
                    </span>
                    {#if row.is_favourite}
                      <span
                        aria-label={t('materials.add.favourite')}
                        class={[!on && 'text-primary']}
                      >
                        ★
                      </span>
                    {/if}
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        {#if list.loaded}
          <p class="text-xs text-muted-foreground" data-part="count">
            {t('materials.add.count', {
              shown: list.rows.length,
              total: list.total,
            })}
          </p>
        {/if}
      </div>

      <!-- Selected material: the right column, the same height whether a
           material is picked or not, so the dialog does not jump. -->
      <div class="picked flex min-w-0 flex-col gap-3" data-part="picked">
        {@render heading(t('materials.add.selected'))}
        {#if picked}
          <Card>
            <CardContent class="flex items-center gap-3">
              <MaterialSwatch
                class="size-12 shrink-0"
                material={{
                  name: picked.name,
                  colorRgba:
                    picked.color_rgba === '' ? undefined : picked.color_rgba,
                }}
              />
              <div class="min-w-0">
                <p class="truncate text-base font-semibold">{picked.name}</p>
                <p class="truncate text-sm text-muted-foreground">
                  {meta(picked.brand_name, picked.type)}
                </p>
              </div>
            </CardContent>
          </Card>
          {@render heading(t('materials.add.details'))}
          <Card>
            <CardContent>
              <MaterialDetails detail={picked} bare />
            </CardContent>
          </Card>
        {:else}
          <div
            data-part="pick-hint"
            class="flex min-h-40 items-center justify-center rounded-md border border-dashed border-border p-4 text-center text-sm text-muted-foreground"
          >
            {t('materials.add.pickHint')}
          </div>
        {/if}
      </div>
    </div>

    <DialogFooter>
      <Button variant="secondary" onclick={oncancel}>
        {t('materials.add.cancel')}
      </Button>
      <Button
        disabled={picked === undefined || picking !== undefined}
        onclick={() => {
          if (picked) onchoose(picked);
        }}
      >
        {t('materials.add.next')}
      </Button>
    </DialogFooter>
  </div>
{/if}

<style>
  .find {
    flex: 3 1 calc(var(--spacing) * 88);
  }
  .picked {
    flex: 2 1 calc(var(--spacing) * 64);
  }
</style>
