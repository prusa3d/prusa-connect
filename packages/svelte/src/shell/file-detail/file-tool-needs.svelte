<!-- What a print file needs of each filament, by the T number that selects it, and what its G-code asks the printer to check before it prints. -->
<script lang="ts">
  import type { PrintDetails } from '../../data';
  import { formatList, formatUnit, t } from '../../i18n';
  import { toolLabel } from '../printers/tool-label';
  import {
    KeyValueList,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    type KeyValueRow,
  } from '../../primitives';

  let { details }: { details: PrintDetails } = $props();

  const tools = $derived(details.filaments ?? []);
  // A file for one filament says nothing per filament that the rows above do
  // not.
  const perTool = $derived(tools.length > 1);

  const temperature = (value: number | undefined): string =>
    value === undefined
      ? ''
      : formatUnit(value, '°C', { maximumFractionDigits: 0 });

  const used = (value: boolean | undefined): string => {
    if (value === undefined) return t('fileDetail.tools.unknown');

    return value ? t('fileDetail.tools.used') : t('fileDetail.tools.unused');
  };

  const bed = $derived(details.bed);
  const checks = $derived(details.checks);
  const features = $derived(checks?.features ?? []);

  const rows = $derived<KeyValueRow[]>(
    (
      [
        {
          id: 'checkedModel',
          label: t('fileDetail.checks.model'),
          value: checks?.printer_model ?? '',
        },
        {
          id: 'features',
          label: t('fileDetail.checks.features'),
          value: features.length === 0 ? '' : formatList(features),
        },
        {
          id: 'minFirmware',
          label: t('fileDetail.checks.minFirmware'),
          value: checks?.min_firmware ?? '',
          mono: true,
        },
        {
          id: 'topLayer',
          label: t('fileDetail.checks.height'),
          value:
            details.max_layer_z_mm === undefined
              ? ''
              : formatUnit(details.max_layer_z_mm, 'mm', {
                  maximumFractionDigits: 2,
                }),
        },
        {
          id: 'bed',
          label: t('fileDetail.checks.bed'),
          value:
            bed?.width_mm === undefined || bed.depth_mm === undefined
              ? ''
              : t('fileDetail.checks.bedSize', {
                  width: formatUnit(bed.width_mm, 'mm', {
                    maximumFractionDigits: 0,
                  }),
                  depth: formatUnit(bed.depth_mm, 'mm', {
                    maximumFractionDigits: 0,
                  }),
                }),
        },
      ] satisfies KeyValueRow[]
    ).filter((row) => row.value !== ''),
  );
</script>

{#if perTool}
  <section class="flex flex-col gap-3">
    <h3 class="text-sm font-medium text-foreground">
      {t('fileDetail.tools.heading')}
    </h3>
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('fileDetail.tools.tool')}</TableHead>
          <TableHead>{t('fileDetail.tools.use')}</TableHead>
          <TableHead>{t('fileDetail.tools.material')}</TableHead>
          <TableHead>{t('fileDetail.tools.nozzle')}</TableHead>
          <TableHead>{t('fileDetail.tools.nozzleTemperature')}</TableHead>
          <TableHead>{t('fileDetail.tools.bedTemperature')}</TableHead>
          <TableHead>{t('fileDetail.tools.filament')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {#each tools as tool (tool.index)}
          <TableRow>
            <TableHead scope="row" class="text-foreground">
              {toolLabel(tool.index)}
            </TableHead>
            <TableCell>{used(tool.used)}</TableCell>
            <TableCell>{tool.material ?? ''}</TableCell>
            <TableCell>
              {tool.nozzle_mm === undefined
                ? ''
                : formatUnit(tool.nozzle_mm, 'mm', {
                    maximumFractionDigits: 2,
                  })}
            </TableCell>
            <TableCell>{temperature(tool.nozzle_temp_c)}</TableCell>
            <TableCell>{temperature(tool.bed_temp_c)}</TableCell>
            <TableCell>
              {tool.filament_g === undefined
                ? ''
                : formatUnit(tool.filament_g, 'g', {
                    maximumFractionDigits: 2,
                  })}
            </TableCell>
          </TableRow>
        {/each}
      </TableBody>
    </Table>
  </section>
{/if}

{#if rows.length > 0}
  <section class="flex flex-col gap-3">
    <h3 class="text-sm font-medium text-foreground">
      {t('fileDetail.checks.heading')}
    </h3>
    <KeyValueList {rows} />
  </section>
{/if}
