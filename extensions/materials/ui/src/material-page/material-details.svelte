<!--
  What a material asks of the printer: the temperatures, the drying, the
  density, the hardness and the colour, as a grid of small labelled values
  under the brand and type. A catalogue material and a custom one share the
  fields they both have; a field neither gives is left out. A catalogue
  material's tags close it.

  A grid of its own rather than the host's KeyValueList: inside a module, the
  module's own utilities are scoped and outrank the host's responsive ones, so
  the list's rows never turned into rows (Core moduleBuild scoping).

  The add dialog shows it too. The columns fill whatever width they get, the
  page's or the dialog's.
-->
<script lang="ts">
  import {
    degreesFor,
    formatDuration,
    formatNumber,
    formatUnit,
  } from '@prusa/connect-ui/i18n';
  import { MaterialSwatch } from '@prusa/connect-ui/primitives';

  import type { Detail } from '../api';
  import MaterialIcon from '../material-icon/material-icon.svelte';
  import { heldHost } from '../host';

  let {
    detail,
    bare = false,
    wrapValues = false,
  }: {
    detail: Detail;
    /** Only the specs and tags: the add dialog names the material above them. */
    bare?: boolean;
    wrapValues?: boolean;
  } = $props();

  const t = heldHost().t;

  // The catalogue's fields and the custom one's, as one bag: a material has one
  // of the two, and both name a field the same way.
  const fields = $derived({ ...detail.custom, ...detail.material });

  // In the unit the reader chose, Celsius or Fahrenheit.
  const temp = (value?: number): string | undefined =>
    value === undefined ? undefined : formatUnit(value, '°C');

  const range = (min?: number, max?: number): string | undefined => {
    if (min !== undefined && max !== undefined && min !== max) {
      return t('materials.details.range', {
        min: formatNumber(degreesFor(min)),
        max: formatUnit(max, '°C'),
      });
    }

    return temp(min ?? max);
  };

  const drying = $derived.by(() => {
    const { drying_temp: dt, drying_time_min: dm } = fields;
    if (dt !== undefined && dm !== undefined) {
      return t('materials.details.drying', {
        temp: formatUnit(dt, '°C'),
        time: formatDuration(dm * 60),
      });
    }

    return temp(dt);
  });

  const rows = $derived([
    {
      id: 'nozzle',
      label: t('materials.details.nozzle'),
      value: range(fields.min_print_temp, fields.max_print_temp),
    },
    {
      id: 'bed',
      label: t('materials.details.bed'),
      value: range(fields.min_bed_temp, fields.max_bed_temp),
    },
    {
      id: 'chamber',
      label: t('materials.details.chamber'),
      value:
        temp(fields.chamber_temp) ??
        range(fields.min_chamber_temp, fields.max_chamber_temp),
    },
    {
      id: 'preheat',
      label: t('materials.details.preheat'),
      value: temp(fields.preheat_temp),
    },
    { id: 'drying', label: t('materials.details.dryingLabel'), value: drying },
    {
      id: 'density',
      label: t('materials.details.density'),
      value:
        fields.density === undefined
          ? undefined
          : t('materials.details.densityValue', { density: fields.density }),
    },
    {
      id: 'hardness',
      label: t('materials.details.hardness'),
      value:
        fields.hardness_shore_d === undefined
          ? undefined
          : t('materials.details.hardnessValue', {
              hardness: fields.hardness_shore_d,
            }),
    },
    {
      id: 'colour',
      label: t('materials.details.colour'),
      value: detail.color_rgba === '' ? undefined : detail.color_rgba,
      mono: true,
    },
  ]);

  const shown = $derived(rows.filter((row) => row.value !== undefined));

  // The catalogue's tags are its own identifiers, such as contains_carbon_fiber.
  // A known one reads from the catalogue of texts; one the catalogue added
  // later reads as its words: Contains carbon fiber.
  const tags = $derived(
    (fields.tags ?? []).map((tag) => {
      const key = `materials.tag.${tag}`;
      const line = t(key);
      if (line !== key) return line;
      const words = tag.replaceAll('_', ' ');

      return words.charAt(0).toUpperCase() + words.slice(1);
    }),
  );
</script>

<div data-part="material-details" class="flex items-start gap-4">
  {#if !bare}
    <MaterialIcon
      class="size-28"
      materialClass={detail.class}
      colorRgba={detail.color_rgba}
    />
  {/if}
  <div class="flex min-w-0 flex-1 flex-col gap-4">
    {#if !bare}
      <p class="text-sm text-muted-foreground">
        {[detail.brand_name, detail.type, detail.class]
          .filter((s) => s !== '')
          .join(' · ')}
      </p>
    {/if}
    {#if shown.length > 0}
      <dl class={['specs grid gap-x-8 gap-y-4', { 'wrap-values': wrapValues }]}>
        {#each shown as row (row.id)}
          <div class="flex min-w-0 flex-col gap-1">
            <dt class="text-xs tracking-wide text-muted-foreground uppercase">
              {row.label}
            </dt>
            <dd
              class={[
                'flex min-w-0 items-center gap-2 text-sm font-medium',
                row.mono ? 'font-mono' : 'tabular-nums',
              ]}
            >
              {#if row.id === 'colour'}
                <MaterialSwatch
                  class="size-4 shrink-0"
                  material={{ name: detail.name, colorRgba: detail.color_rgba }}
                />
              {/if}
              <span class="min-w-0 break-words">{row.value}</span>
            </dd>
          </div>
        {/each}
      </dl>
    {/if}
    {#if tags.length > 0}
      <div class="flex flex-col gap-2">
        <p class="text-xs tracking-wide text-muted-foreground uppercase">
          {t('materials.details.tags')}
        </p>
        <ul
          class="flex flex-wrap gap-2"
          aria-label={t('materials.details.tags')}
        >
          {#each tags as tag (tag)}
            <li class="rounded-full border border-border px-2 py-1 text-xs">
              {tag}
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>
</div>

<style>
  .specs.wrap-values {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .specs {
    grid-template-columns: repeat(
      auto-fill,
      minmax(calc(var(--spacing) * 36), 1fr)
    );
  }
</style>
