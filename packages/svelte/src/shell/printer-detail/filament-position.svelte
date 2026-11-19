<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import InfoIcon from '@lucide/svelte/icons/info';
  import type { Snippet } from 'svelte';
  import SearchIcon from '@lucide/svelte/icons/search';
  import XIcon from '@lucide/svelte/icons/x';

  import { PREHEAT_PRESETS } from '../../domain';
  import { formatUnit, t } from '../../i18n';
  import {
    Button,
    CONTROL_FILL,
    CONTROL_RING,
    CONTROL_TRIGGER,
    Popover,
    PopoverContent,
    PopoverTrigger,
    SpoolIcon,
  } from '../../primitives';
  import {
    FILAMENT_COLORS,
    filamentColorOf,
    filamentPreview,
    setFilamentColor,
    type FilamentColor,
  } from '../printers/filament-colors';
  import {
    materialOverrideOf,
    setMaterialOverride,
    useFilamentFavorites,
    type FavoriteFilament,
  } from '../printers/filament-preferences';
  import editIcon from '../settings-account/assets/profile-edit.svg';
  import deleteIcon from './assets/favorite-delete.svg';
  import FilamentColorWheel from './filament-color-wheel.svelte';

  let {
    deviceId,
    toolId,
    position,
    bare = false,
    reported,
    empty = false,
    assignedName,
    assignedColor,
    remainingGrams,
    disabledReason,
    note,
    panel,
  }: {
    deviceId: string;
    toolId: string;
    position: string;
    /** Leaves the position out, where the row around the chip already names it. */
    bare?: boolean;
    reported: string | undefined;
    empty?: boolean;
    assignedName?: string | undefined;
    assignedColor?: string | undefined;
    remainingGrams?: number | undefined;
    disabledReason?: string | undefined;
    note?: string | undefined;
    /** Draws the picker itself, under a title bar with the close. */
    panel?:
      | Snippet<
          [
            close: () => void,
            setPanelTitle: (title?: string) => void,
            setPanelBusy: (busy: boolean) => void,
          ]
        >
      | undefined;
  } = $props();

  const preview = filamentPreview();
  const current = $derived(materialOverrideOf(deviceId, toolId));
  const currentColor = $derived(filamentColorOf(deviceId, toolId, preview));
  const label = $derived(
    current?.material ??
      reported ??
      t(
        empty
          ? 'printerDetailInfo.tools.noMaterial'
          : 'printerDetailInfo.filament.unknown',
      ),
  );

  let open = $state(false);
  let panelTitle = $state<string>();
  let panelBusy = $state(false);
  const setPanelBusy = (busy: boolean): void => {
    panelBusy = busy;
  };
  const setPanelTitle = (title?: string) => {
    panelTitle = title;
  };
  let tab = $state<'material' | 'favorites'>('material');
  let query = $state('');
  let filter = $state('');
  let colorFilter = $state<FilamentColor[0] | undefined>(undefined);
  let material = $state('');
  let brand = $state('');
  let brandInput = $state<HTMLInputElement>();
  let color = $state<FilamentColor | undefined>(undefined);
  let editing = $state<string | undefined>(undefined);
  let favoriteName = $state('');
  const uid = $props.id();
  const FIELD =
    'h-9 w-full min-w-0 rounded-sm bg-secondary px-2 text-sm text-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring';

  const PICKER_FIELD =
    'h-picker-field w-full min-w-0 rounded-sm bg-secondary ps-3 pe-9 text-sm text-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring';

  const swatch = (
    id: FilamentColor[0] | undefined,
  ): FilamentColor | undefined => FILAMENT_COLORS.find(([key]) => key === id);

  const { read: filamentFavorites, save: saveFilamentFavorites } =
    useFilamentFavorites();
  const favorites = $derived(filamentFavorites());
  const matching = $derived(
    favorites.filter(
      (item) =>
        (filter === '' || item.material === filter) &&
        (colorFilter === undefined || item.color === colorFilter) &&
        [item.name, item.material, item.brand].some((word) =>
          word.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
        ),
    ),
  );
  const materials = $derived(
    [
      ...new Set([
        ...PREHEAT_PRESETS.map((item) => item.material),
        reported,
        material,
      ]),
    ].filter((item): item is string => Boolean(item)),
  );
  const matchingMaterials = $derived(
    materials.filter(
      (item) =>
        item === material ||
        item.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
    ),
  );
  const favoriteMaterials = $derived([
    ...new Set(favorites.map((item) => item.material)),
  ]);
  const favoriteColors = $derived(
    FILAMENT_COLORS.filter(([id]) =>
      favorites.some((item) => item.color === id),
    ),
  );
  const alreadySaved = $derived(
    favorites.some(
      (item) =>
        item.material === material &&
        item.brand === brand.trim() &&
        item.color === color?.[0],
    ),
  );

  const begin = (next: boolean): void => {
    if (next ? disabledReason : panelBusy) return;
    open = next;
    if (!next) return;
    panelTitle = undefined;
    tab = 'material';
    query = '';
    filter = '';
    colorFilter = undefined;
    editing = undefined;
    if (panel) return;
    material = current?.material ?? reported ?? '';
    brand = current?.brand ?? '';
    color = currentColor;
  };

  const chooseFavorite = (item: FavoriteFilament): void => {
    material = item.material;
    brand = item.brand;
    color = swatch(item.color);
    query = '';
    tab = 'material';
  };

  const addFavorite = (): void => {
    if (material === '' || alreadySaved) return;
    const maker = brand.trim();
    saveFilamentFavorites([
      ...favorites,
      {
        id: Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
          byte.toString(16).padStart(2, '0'),
        ).join(''),
        name:
          maker === ''
            ? material
            : t('printerDetailInfo.filament.profileName', {
                material,
                brand: maker,
              }),
        material,
        brand: maker,
        ...(color === undefined ? {} : { color: color[0] }),
      },
    ]);
  };

  const saveName = (item: FavoriteFilament): void => {
    const name = favoriteName.trim();
    if (name === '') return;
    saveFilamentFavorites(
      favorites.map((entry) =>
        entry.id === item.id ? { ...entry, name } : entry,
      ),
    );
    editing = undefined;
  };

  const removeFavorite = (item: FavoriteFilament): void => {
    saveFilamentFavorites(favorites.filter(({ id }) => id !== item.id));
    if (editing === item.id) editing = undefined;
    filter = '';
    colorFilter = undefined;
  };

  const close = (): void => {
    open = false;
  };

  const confirm = (): void => {
    const maker = brand.trim();
    setMaterialOverride(
      deviceId,
      toolId,
      material === '' || (material === reported && maker === '')
        ? undefined
        : { material, brand: maker },
    );
    if (color !== currentColor) setFilamentColor(deviceId, toolId, color);
    open = false;
  };

  const switchTab = (next: typeof tab): void => {
    tab = next;
    query = '';
    editing = undefined;
  };
</script>

<Popover bind:open={() => open, begin}>
  <PopoverTrigger
    data-part="filament-position"
    data-tool={toolId}
    aria-disabled={disabledReason ? true : undefined}
    title={disabledReason ?? note}
    onclick={(event: MouseEvent) => {
      if (disabledReason) event.preventDefault();
    }}
    class={[
      CONTROL_TRIGGER,
      'min-h-8 max-w-full gap-1 px-1.5 text-start text-xs whitespace-normal',
      bare && 'w-36! lg:w-60!',
      disabledReason
        ? 'border-dashed bg-transparent text-muted-foreground aria-disabled:cursor-not-allowed'
        : [CONTROL_FILL, CONTROL_RING],
    ]}
  >
    <SpoolIcon
      aria-hidden="true"
      filamentColor={panel ? assignedColor : currentColor?.[1]}
    />
    {#if !bare}
      <span class="shrink-0 text-muted-foreground">{position}</span>
    {/if}
    <span
      class={['min-w-0 font-medium', bare ? 'flex-1 truncate' : 'break-words']}
      >{assignedName ?? label}</span
    >
    {#if remainingGrams !== undefined}
      <span
        class={[
          'shrink-0 text-muted-foreground tabular-nums',
          bare && 'max-lg:hidden',
        ]}
      >
        {formatUnit(remainingGrams, 'g', { maximumFractionDigits: 0 })}
      </span>
    {:else if !panel && currentColor}
      <span class="sr-only">{t(currentColor[2])}</span>
    {/if}
    {#if disabledReason}
      <InfoIcon class="size-3.5 shrink-0" aria-hidden="true" />
      <span class="sr-only">{disabledReason}</span>
    {:else}
      <ChevronDownIcon class="size-3.5 shrink-0" aria-hidden="true" />
      {#if note}<span class="sr-only">{note}</span>{/if}
    {/if}
  </PopoverTrigger>
  <PopoverContent
    data-part="filament-position-picker"
    align="start"
    sideOffset={6}
    sticky="always"
    collisionPadding={8}
    class="filament-picker flex max-h-filament-picker flex-col overflow-hidden bg-card"
  >
    <div class="flex shrink-0 items-stretch bg-secondary pe-1">
      {#if panel}
        <h3 class="min-w-0 flex-1 px-3 py-2.5 text-sm font-semibold">
          {panelTitle ?? t('printerDetailInfo.tools.material')}
        </h3>
      {:else}
        <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- picker tab filled with the card colour when pressed; no Button or ToggleGroup look matches -->
        <button
          type="button"
          class="min-h-picker-control min-w-0 flex-1 px-2 text-sm hover:text-foreground focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring aria-pressed:bg-card"
          aria-pressed={tab === 'material'}
          onclick={() => switchTab('material')}
        >
          {t('printerDetailInfo.tools.material')}
        </button>
        <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- picker tab filled with the card colour when pressed; no Button or ToggleGroup look matches -->
        <button
          type="button"
          class="min-h-picker-control min-w-0 flex-1 px-2 text-sm hover:text-foreground focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring aria-pressed:bg-card"
          aria-pressed={tab === 'favorites'}
          onclick={() => switchTab('favorites')}
        >
          {t('printerDetailInfo.filament.favorites')}
        </button>
      {/if}
      <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- 40px full-height close in the picker bar; no Button size matches -->
      <button
        type="button"
        class="grid w-picker-control shrink-0 place-items-center rounded-sm hover:bg-accent focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
        disabled={panelBusy}
        aria-label={t('primitives.close')}
        onclick={() => {
          open = false;
        }}
      >
        <XIcon class="size-5" aria-hidden="true" />
      </button>
    </div>

    {#if panel}
      {#if open}
        {@render panel(close, setPanelTitle, setPanelBusy)}
      {/if}
    {:else}
      <div class="min-h-0 overflow-y-auto">
        {@render searchBox()}

        {#if tab === 'material'}
          <div
            class="grid grid-cols-label-field items-center gap-x-4 gap-y-2 border-b border-border p-4 text-sm"
          >
            <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- grid label at the text-sm line height; Label sets leading-none -->
            <label for={`${uid}-material`}
              >{t('printerDetailInfo.tools.material')}</label
            >
            <div class="relative min-w-0">
              <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- native select with an overlaid chevron; Select is a popover listbox -->
              <select
                id={`${uid}-material`}
                class={`appearance-none ${PICKER_FIELD}`}
                bind:value={material}
              >
                <option value=""
                  >{t('printerDetailInfo.filament.chooseMaterial')}</option
                >
                {#each matchingMaterials as item (item)}
                  <option value={item}>{item}</option>
                {/each}
              </select>
              <ChevronDownIcon
                class="pointer-events-none absolute end-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
            </div>
            <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- grid label at the text-sm line height; Label sets leading-none -->
            <label for={`${uid}-brand`}
              >{t('printerDetailInfo.filament.brand')}</label
            >
            <div class="relative min-w-0">
              <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- borderless native datalist input with showPicker; Input has a border and other padding -->
              <input
                class={`brand-picker ${PICKER_FIELD}`}
                id={`${uid}-brand`}
                maxlength={60}
                list={`${uid}-brands`}
                bind:this={brandInput}
                bind:value={brand}
              />
              <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- full-height chevron laid over the brand input; no Button size matches -->
              <button
                type="button"
                class="absolute end-0 top-0 grid h-full w-10 place-items-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
                aria-label={t('printerDetailInfo.filament.brandOptions')}
                onclick={() => {
                  brandInput?.focus();
                  // Safari before 16 has no showPicker.
                  if (brandInput && 'showPicker' in brandInput) {
                    brandInput.showPicker();
                  }
                }}
              >
                <ChevronDownIcon class="size-4" aria-hidden="true" />
              </button>
            </div>
            <datalist id={`${uid}-brands`}>
              <option value="Prusament"></option>
              {#each [...new Set(favorites.map((item) => item.brand))].filter((maker) => maker !== '' && maker !== 'Prusament') as maker (maker)}
                <option value={maker}></option>
              {/each}
            </datalist>
          </div>
          <div class="grid gap-2 p-4">
            <div class="flex items-center justify-between gap-2">
              <h3 class="text-sm font-semibold">
                {t('printerDetailInfo.filament.wheel')}
              </h3>
              <Button
                size="sm"
                variant="secondary"
                disabled={material === '' || alreadySaved}
                onclick={addFavorite}
              >
                {t(
                  alreadySaved
                    ? 'printerDetailInfo.filament.favoriteSaved'
                    : 'printerDetailInfo.filament.addFavorite',
                )}
              </Button>
            </div>
            <FilamentColorWheel
              colors={FILAMENT_COLORS}
              outerCount={12}
              selected={color}
              onChoose={(next: FilamentColor) => {
                color = next;
              }}
            />
          </div>
          {#if current}
            <div class="flex justify-center border-t border-border px-4 py-3">
              <Button
                size="sm"
                variant="ghost"
                onclick={() => {
                  material = reported ?? '';
                  brand = '';
                }}
              >
                {t('printerDetailInfo.filament.useReported')}
              </Button>
            </div>
          {/if}
        {:else}
          <div class="grid gap-3 p-4">
            {#if favorites.length > 0}
              <div class="grid gap-2 border-b border-border pb-3">
                <div class="flex flex-wrap items-center justify-center gap-1">
                  <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- round colour filter around an inset dot; SwatchPicker draws filled squares -->
                  <button
                    type="button"
                    class="color-filter"
                    aria-label={t('printerDetailInfo.filament.allColors')}
                    title={t('printerDetailInfo.filament.allColors')}
                    aria-pressed={colorFilter === undefined}
                    onclick={() => {
                      colorFilter = undefined;
                    }}
                  >
                    <span class="color-dot all-colors" aria-hidden="true"
                    ></span>
                  </button>
                  {#each favoriteColors as item (item[0])}
                    <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- round colour filter around an inset dot; SwatchPicker draws filled squares -->
                    <button
                      type="button"
                      class="color-filter"
                      aria-label={t(item[2])}
                      title={t(item[2])}
                      aria-pressed={colorFilter === item[0]}
                      onclick={() => {
                        colorFilter =
                          colorFilter === item[0] ? undefined : item[0];
                      }}
                    >
                      <span
                        class="color-dot"
                        style:background-color={item[1]}
                        aria-hidden="true"
                      ></span>
                    </button>
                  {/each}
                </div>
                <div class="flex flex-wrap justify-center gap-1.5">
                  {#each favoriteMaterials as item (item)}
                    <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- rounded-full filled material pill; Chip is rounded-xs and bordered -->
                    <button
                      type="button"
                      class="min-h-7 rounded-full bg-secondary px-3 py-1 text-xs hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring aria-pressed:bg-control-on aria-pressed:text-control-on-foreground"
                      aria-pressed={filter === item}
                      onclick={() => {
                        filter = filter === item ? '' : item;
                      }}
                    >
                      {item}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}
            {#each matching as item (item.id)}
              {@const favoriteColor = swatch(item.color)}
              <div
                class="grid grid-cols-content-action items-center gap-1 rounded-sm bg-secondary p-2"
              >
                <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- borderless favourite row; Tile is bordered and stacked -->
                <button
                  type="button"
                  class="flex w-full items-center gap-2 rounded-sm px-2 py-1 text-start hover:bg-accent"
                  onclick={() => chooseFavorite(item)}
                >
                  <SpoolIcon
                    aria-hidden="true"
                    filamentColor={favoriteColor?.[1]}
                  />
                  <span class="grid min-w-0 text-sm">
                    <span class="font-medium break-words">{item.name}</span>
                    <span class="break-words text-muted-foreground">
                      {item.brand === ''
                        ? item.material
                        : t('printerDetailInfo.filament.profileDetail', {
                            material: item.material,
                            brand: item.brand,
                          })}
                      {#if favoriteColor}<span class="sr-only"
                          >{t(favoriteColor[2])}</span
                        >{/if}
                    </span>
                  </span>
                </button>
                <div class="flex items-center gap-1">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    label={t('printerDetailInfo.filament.rename', {
                      name: item.name,
                    })}
                    title={t('printerDetailInfo.filament.rename', {
                      name: item.name,
                    })}
                    aria-expanded={editing === item.id}
                    onclick={() => {
                      editing = item.id;
                      favoriteName = item.name;
                    }}
                  >
                    <img
                      src={editIcon}
                      alt=""
                      width="16"
                      height="16"
                      class="shrink-0 invert dark:invert-0"
                    />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    label={t('printerDetailInfo.filament.removeFavorite', {
                      name: item.name,
                    })}
                    title={t('printerDetailInfo.filament.removeFavorite', {
                      name: item.name,
                    })}
                    onclick={() => removeFavorite(item)}
                  >
                    <img
                      src={deleteIcon}
                      alt=""
                      width="16"
                      height="16"
                      class="shrink-0 invert dark:invert-0"
                    />
                  </Button>
                </div>
                {#if editing === item.id}
                  <div class="col-span-full flex flex-wrap gap-1 px-1">
                    <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- borderless rename field with a ring focus; Input has a border and 10px padding -->
                    <input
                      class={FIELD}
                      aria-label={t('printerDetailInfo.filament.favoriteName')}
                      maxlength={60}
                      bind:value={favoriteName}
                    />
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={favoriteName.trim() === ''}
                      onclick={() => saveName(item)}
                      >{t('printerDetailInfo.filament.saveName')}</Button
                    >
                    <Button
                      size="sm"
                      variant="ghost"
                      onclick={() => {
                        editing = undefined;
                      }}>{t('primitives.cancel')}</Button
                    >
                  </div>
                {/if}
              </div>
            {:else}
              <p class="text-sm text-muted-foreground">
                {t(
                  favorites.length === 0
                    ? 'printerDetailInfo.filament.noFavorites'
                    : 'printerDetailInfo.filament.noResults',
                )}
              </p>
            {/each}
          </div>
        {/if}
      </div>

      <div
        class="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border p-3"
      >
        <Button
          size="sm"
          variant="ghost"
          onclick={() => {
            open = false;
          }}>{t('primitives.cancel')}</Button
        >
        <Button size="sm" onclick={confirm}
          >{t('printerDetailInfo.filament.confirm')}</Button
        >
      </div>
    {/if}
  </PopoverContent>
</Popover>

{#snippet searchBox()}
  <div class="flex items-center gap-2 border-b border-border p-4">
    <div class="relative min-w-0 flex-1">
      <SearchIcon
        class="pointer-events-none absolute start-3 top-1/2 size-picker-search-icon -translate-y-1/2 text-foreground"
        strokeWidth={1.5}
        aria-hidden="true"
      />
      <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- no primitive draws the borderless 44px picker search -->
      <input
        class="h-picker-search w-full min-w-0 rounded-sm border-0 bg-transparent ps-picker-control pe-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring"
        type="search"
        aria-label={t('printerDetailInfo.filament.search')}
        placeholder={t('printerDetailInfo.filament.search')}
        bind:value={query}
      />
    </div>
  </div>
{/snippet}

<style>
  :global(.filament-picker) {
    inline-size: min(340px, calc(100vw - 1rem));
  }

  /* Two classes outrank the content's own `p-4` without `!important`. */
  :global(.filament-picker[data-part='filament-position-picker']) {
    padding: 0;
  }

  :global(.filament-picker:has([data-material-picker])) {
    inline-size: min(420px, calc(100vw - 1rem));
  }

  @media (min-width: 44rem) {
    :global(.filament-picker:has([data-picker-expanded])) {
      inline-size: min(680px, calc(100vw - 1rem));
      max-block-size: min(
        calc(100dvh - 1rem),
        var(--bits-popover-content-available-height, 80dvh)
      );
    }

    :global(
      .filament-picker:has([data-material-picker][data-picker-expanded])
    ) {
      inline-size: min(740px, calc(100vw - 1rem));
    }
  }

  @media (width < 44rem) {
    /* The bottom bar of a narrow screen lies over the page, and the room
       bits-ui reports does not count it. */
    :global(.filament-picker[data-part='filament-position-picker']) {
      max-block-size: min(
        43rem,
        calc(100dvh - 4.5rem),
        calc(var(--bits-popover-content-available-height, 80dvh) - 3.75rem)
      );
    }

    /* The stacked panel can use the viewport above the bottom bar when neither
       side of its trigger fits. bits-ui writes the wrapper's position as an inline style,
       which only `!important` outranks. */
    :global(
      [data-bits-floating-content-wrapper]:has(
        > .filament-picker [data-picker-expanded]
      )
    ) {
      position: fixed !important;
      inset: calc(50% - 1.75rem) auto auto 50% !important;
      transform: translate(-50%, -50%) !important;
    }

    :global(.filament-picker:has([data-picker-expanded])) {
      max-block-size: calc(100dvh - 4.5rem);
    }
  }

  .color-filter {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
  }

  .color-filter[aria-pressed='true'],
  .color-filter:focus-visible {
    outline: 2px solid var(--color-control-on);
    outline-offset: -1px;
  }

  .color-dot {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 1px solid var(--color-border);
  }

  .all-colors {
    background: var(--filament-all-colors);
  }

  /*
    The fourth `!important` of this file, beside the three on the floating
    wrapper: Chromium writes `display: list-item` inline on a datalist input's
    indicator, and only the flag outranks an inline style.
  */
  .brand-picker::-webkit-calendar-picker-indicator {
    display: none !important;
  }
</style>
