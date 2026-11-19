<!--
  The settings of each printer setup of a 3MF project: the profiles, the
  nozzles, and the few print settings a person checks before printing, read
  into the same words whichever slicer wrote them.

  A setting the file does not hold is left out rather than drawn empty. A file
  with no settings at all, which is every plain 3MF, says so.
-->
<script lang="ts">
  import { formatList, formatNumber, formatUnit, t } from '../i18n';
  import {
    KeyValueList,
    type KeyValueRow,
    MaterialSwatch,
  } from '@prusa/connect-ui/primitives';
  import { millimetres } from './labels';
  import type { Project, ProjectSetup } from './model';

  let { project }: { project: Project } = $props();

  const SUPPORTS_KEY: Readonly<Record<string, string>> = {
    none: 'projectView.supports.none',
    everywhere: 'projectView.supports.everywhere',
    build_plate_only: 'projectView.supports.build_plate_only',
    enforcers_only: 'projectView.supports.enforcers_only',
  };

  const yesNo = (value: boolean | undefined): string => {
    if (value === undefined) return '';

    return value ? t('projectView.settings.yes') : t('projectView.settings.no');
  };

  const length = (value: number | undefined): string =>
    value === undefined || value === 0
      ? ''
      : formatUnit(value, 'mm', { maximumFractionDigits: 2 });

  const brimText = (width: number | undefined): string => {
    if (width === undefined) return '';

    return width === 0 ? t('projectView.settings.noBrim') : millimetres(width);
  };

  const rowsOf = (setup: ProjectSetup): KeyValueRow[] => {
    const settings = setup.settings;
    const infill =
      settings.infill_percent === undefined
        ? ''
        : formatNumber(settings.infill_percent / 100, {
            style: 'percent',
            maximumFractionDigits: 1,
          });
    const brim = brimText(settings.brim_width_mm);

    return (
      [
        {
          id: 'printerProfile',
          label: t('projectView.settings.printerProfile'),
          value: setup.printer_profile ?? '',
        },
        {
          id: 'printProfile',
          label: t('projectView.settings.printProfile'),
          value: setup.print_profile ?? '',
        },
        {
          id: 'materials',
          label: t('projectView.settings.materials'),
          value:
            setup.material_profiles.length === 0
              ? ''
              : formatList(setup.material_profiles),
        },
        {
          id: 'nozzles',
          label: t('projectView.settings.nozzles'),
          value:
            setup.nozzles.length === 0
              ? ''
              : formatList(
                  setup.nozzles.map((nozzle) => {
                    const diameter = formatUnit(nozzle.diameter_mm, 'mm', {
                      maximumFractionDigits: 2,
                    });

                    return nozzle.high_flow === true
                      ? t('projectView.settings.highFlow', { diameter })
                      : diameter;
                  }),
                ),
        },
        {
          id: 'layerHeight',
          label: t('projectView.settings.layerHeight'),
          value: length(settings.layer_height_mm),
        },
        {
          id: 'firstLayer',
          label: t('projectView.settings.firstLayer'),
          value: length(settings.first_layer_height_mm),
        },
        {
          id: 'infill',
          label: t('projectView.settings.infill'),
          value:
            infill !== '' && (settings.infill_pattern ?? '') !== ''
              ? t('projectView.settings.infillPattern', {
                  percent: infill,
                  pattern: settings.infill_pattern ?? '',
                })
              : infill,
        },
        {
          id: 'perimeters',
          label: t('projectView.settings.perimeters'),
          value:
            settings.perimeters === undefined
              ? ''
              : formatNumber(settings.perimeters),
        },
        {
          id: 'supports',
          label: t('projectView.settings.supports'),
          value:
            settings.supports === undefined
              ? ''
              : t(SUPPORTS_KEY[settings.supports] ?? settings.supports),
        },
        {
          id: 'brim',
          label: t('projectView.settings.brim'),
          value: brim,
        },
        {
          id: 'wipeTower',
          label: t('projectView.settings.wipeTower'),
          value: yesNo(settings.wipe_tower),
        },
        {
          id: 'maxHeight',
          label: t('projectView.settings.maxHeight'),
          value: length(setup.max_print_height_mm),
        },
        {
          id: 'labelObjects',
          label: t('projectView.settings.labelObjects'),
          value:
            setup.label_objects === undefined
              ? ''
              : yesNo(setup.label_objects === 'firmware'),
        },
      ] satisfies KeyValueRow[]
    ).filter((row) => row.value !== '');
  };

  const setups = $derived(
    project.setups.map((setup, index) => ({
      index,
      heading:
        project.setups.length > 1
          ? [
              t('projectView.settings.setup', {
                number: formatNumber(index + 1),
              }),
              setup.printer_model,
            ]
              .filter((part) => part !== undefined && part !== '')
              .join(' · ')
          : '',
      rows: rowsOf(setup),
      // One setup's filaments are in the overview above already.
      filaments: project.setups.length > 1 ? setup.settings.filaments : [],
    })),
  );

  // A long list reads in two columns on a wide page, the first column the
  // longer one.
  const halves = (rows: KeyValueRow[]): KeyValueRow[][] => {
    const cut = Math.ceil(rows.length / 2);

    return [rows.slice(0, cut), rows.slice(cut)].filter(
      (half) => half.length > 0,
    );
  };

  const empty = $derived(
    setups.every(
      (setup) => setup.rows.length === 0 && setup.filaments.length === 0,
    ),
  );
</script>

{#if empty}
  <p class="text-sm text-muted-foreground">{t('projectView.settings.none')}</p>
{:else}
  <div
    class="@container/settings grid grid-cols-1 content-start gap-6"
    data-slot="project-settings"
  >
    {#each setups as setup (setup.index)}
      <section class="grid grid-cols-1 content-start gap-3">
        {#if setup.heading !== ''}
          <h3 class="text-sm font-medium">{setup.heading}</h3>
        {/if}
        <div
          class="grid grid-cols-1 gap-x-10 gap-y-0 @3xl/settings:grid-cols-2"
        >
          {#each halves(setup.rows) as half, at (at)}
            <KeyValueList rows={half} labelWidth="14rem" />
          {/each}
        </div>
        {#if setup.filaments.length > 0}
          <div class="grid grid-cols-1 content-start gap-2">
            <span class="text-xs text-muted-foreground">
              {t('projectView.settings.filaments')}
            </span>
            <ul class="flex flex-wrap gap-x-4 gap-y-2">
              {#each setup.filaments as filament, at (at)}
                <li class="inline-flex items-center gap-2 text-sm">
                  <MaterialSwatch
                    class="size-4"
                    material={{
                      name: filament.type ?? '',
                      colorRgba: filament.colour,
                      colorName: filament.colour,
                    }}
                  />
                  <span class="font-mono text-xs text-muted-foreground">
                    {t('projectView.toolNumber', { n: at })}
                  </span>
                  <span>{filament.type ?? ''}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </section>
    {/each}
  </div>
{/if}
