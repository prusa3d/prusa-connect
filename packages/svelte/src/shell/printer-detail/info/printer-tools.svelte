<!-- Tools: each head of the printer, the slots of the feeder on it, the filament loaded in each, and what a multi-material unit is doing, as the printer reported them. -->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { STATE_PATH, toolsOf, type Device } from '../../../domain';
  import { formatUnit, t } from '../../../i18n';
  import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    KeyValueList,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    type KeyValueRow,
  } from '../../../primitives';

  import NozzleEditor from '../controls/nozzle-editor.svelte';
  import {
    declaresArgument,
    resolverFor,
    textOf,
    VERBS,
  } from '../controls/machine-controls';
  import FilamentPositions from '../filament-positions.svelte';
  import { slotLabel, toolLabel } from '../../printers/tool-label';
  import { compareIds, toolRowsOf } from '../tool-rows';
  import type { CommandPort, TelemetrySource } from '../types';

  type Tool = NonNullable<NonNullable<Device['tools']>['tools'][string]>;

  let {
    tools,
    buildVolume,
    live,
    material,
  }: {
    tools: Device['tools'];
    buildVolume?: Device['build_volume'];
    /**
     * What the rows read and send. Without commands the temperature column is
     * a reading; without the whole of it the Material column is too.
     */
    live?:
      | {
          device: Device;
          telemetry: TelemetrySource;
          commands?: CommandPort | undefined;
        }
      | undefined;
    /**
     * Draws the material of a tool the reader may assign, handed its id and
     * the chip to fall back on. Absent, the cell is that chip.
     */
    material?: Snippet<[string, Snippet]> | undefined;
  } = $props();

  const rows = $derived(toolRowsOf(tools));

  // The nozzle chip of the header sets the same heads under the same rules: a
  // printer that is not reporting offers nothing to set, and a verb with no
  // tool argument heats the one head only.
  const settable = $derived.by(() => {
    if (live?.commands === undefined || !live.device.connected)
      return undefined;
    const { device, telemetry, commands } = live;

    return {
      resolve: resolverFor(
        device,
        commands,
        textOf(telemetry, device, STATE_PATH),
      ),
      busy: commands.sending === true,
      onCommand: commands.onCommand,
      perTool: declaresArgument(device, VERBS.nozzle, 'tool'),
      readings: toolsOf(telemetry, device),
    };
  });

  const flag = (value: boolean | undefined): string => {
    if (value === undefined) return t('printerDetailInfo.tools.unknown');

    return value
      ? t('printerDetailInfo.tools.yes')
      : t('printerDetailInfo.tools.no');
  };

  // A null material package instance is a reading: the tool has nothing
  // loaded. An absent one is a printer that did not say. A head with a feeder
  // holds no material; its slots do.
  const materialText = (tool: Tool): string => {
    const slots = tool.feeder?.slot_count ?? 0;
    const model = tool.feeder?.model;
    if (slots > 0 && model !== undefined && model !== '')
      return t('printerDetailInfo.tools.feederModel', { model, n: slots });
    if (slots > 0) return t('printerDetailInfo.tools.feeder', { n: slots });

    const loaded = tool.material_package_instance;
    if (loaded === undefined) return t('printerDetailInfo.tools.unknown');

    return loaded === null
      ? t('printerDetailInfo.tools.noMaterial')
      : loaded.package.material.abbreviation;
  };

  // A head with a feeder holds no material, and a head the printer switched off
  // holds none to assign; every other row can be given one.
  const assignable = (tool: Tool | null): tool is Tool =>
    tool !== null && !tool.feeder?.slot_count;

  // The commands of the multi-material unit, as the one character its
  // firmware names each by. Any other command is shown by its character.
  const MMU_COMMANDS: Record<string, string> = {
    L: 'printerDetailInfo.tools.mmuLoad',
    U: 'printerDetailInfo.tools.mmuUnload',
    T: 'printerDetailInfo.tools.mmuToolChange',
    K: 'printerDetailInfo.tools.mmuCut',
    E: 'printerDetailInfo.tools.mmuEject',
    H: 'printerDetailInfo.tools.mmuHome',
  };

  // What the unit is doing: the command it runs and the step it is at by its
  // progress code, or idle when it runs none and its last step is done.
  const unitText = (feeder: NonNullable<Tool['feeder']>): string => {
    const code = feeder.mmu_progress_code ?? 0;
    const command = feeder.mmu_command_in_progress;
    if (command === undefined || command === '')
      return code === 0
        ? t('printerDetailInfo.tools.mmuIdle')
        : t('printerDetailInfo.tools.mmuStep', { code });

    const key = MMU_COMMANDS[command];

    return t('printerDetailInfo.tools.mmuBusy', {
      command:
        key === undefined
          ? t('printerDetailInfo.tools.mmuCommand', { command })
          : t(key),
      code,
    });
  };

  // One row for each head whose feeder reports what its unit is doing. A
  // printer with one head says MMU; one with more names the head.
  const unitRows = $derived<KeyValueRow[]>(
    tools === undefined
      ? []
      : Object.keys(tools.tools)
          .filter((id) => !id.includes('.'))
          .sort(compareIds)
          .flatMap((id) => {
            const feeder = tools.tools[id]?.feeder;
            if (feeder?.mmu_progress_code === undefined) return [];

            return [
              {
                id: `mmu-${id}`,
                label:
                  rows.filter((row) => !row.slot).length > 1
                    ? t('printerDetailInfo.tools.mmuOnHead', {
                        n: Number(id) + 1,
                      })
                    : t('printerDetailInfo.tools.mmu'),
                value: unitText(feeder),
              },
            ];
          }),
  );

  const volumeText = $derived(
    buildVolume === undefined
      ? undefined
      : t('printerDetailInfo.tools.volumeSize', {
          x: formatUnit(buildVolume.x_mm, 'mm', { maximumFractionDigits: 0 }),
          y: formatUnit(buildVolume.y_mm, 'mm', { maximumFractionDigits: 0 }),
          z: formatUnit(buildVolume.z_mm, 'mm', { maximumFractionDigits: 0 }),
        }),
  );

  const summary = $derived<KeyValueRow[]>(
    tools === undefined
      ? []
      : [
          {
            id: 'count',
            label: t('printerDetailInfo.tools.count'),
            value:
              tools.tool_count === undefined
                ? undefined
                : String(tools.tool_count),
          },
          {
            id: 'volume',
            label: t('printerDetailInfo.tools.volume'),
            value: volumeText,
          },
          ...unitRows,
        ],
  );
</script>

<Card>
  <CardHeader>
    <CardTitle>
      <h2 class="contents">{t('printerDetailInfo.tools.heading')}</h2>
    </CardTitle>
  </CardHeader>
  <CardContent class="flex flex-col gap-4">
    {#if tools === undefined}
      <p class="text-sm text-muted-foreground">
        {t('printerDetailInfo.tools.none')}
      </p>
    {:else}
      <KeyValueList rows={summary} />
      {#if rows.length > 0}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('printerDetailInfo.tools.tool')}</TableHead>
              <TableHead>{t('printerDetailInfo.tools.nozzle')}</TableHead>
              <TableHead>{t('printerDetailInfo.tools.highFlow')}</TableHead>
              <TableHead>{t('printerDetailInfo.tools.hardened')}</TableHead>
              <TableHead>{t('printerDetailInfo.tools.material')}</TableHead>
              <TableHead>{t('printerDetailInfo.tools.temperature')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {#each rows as row (row.id)}
              <TableRow data-tool={row.id}>
                <TableHead
                  scope="row"
                  class={row.slot
                    ? 'ps-6 font-normal text-muted-foreground'
                    : 'text-foreground'}
                >
                  {row.slot
                    ? slotLabel(row.id.slice(row.id.lastIndexOf('.') + 1))
                    : toolLabel(row.id)}
                </TableHead>
                {#if row.tool === null}
                  <TableCell colspan={5} class="text-muted-foreground">
                    {t('printerDetailInfo.tools.off')}
                  </TableCell>
                {:else if row.slot}
                  <TableCell colspan={3}></TableCell>
                  <TableCell>{@render materialCell(row.id, row.tool)}</TableCell
                  >
                  <TableCell></TableCell>
                {:else}
                  <TableCell>
                    {row.tool.features?.nozzle_diameter === undefined
                      ? t('printerDetailInfo.tools.unknown')
                      : formatUnit(row.tool.features.nozzle_diameter, 'mm', {
                          maximumFractionDigits: 2,
                        })}
                  </TableCell>
                  <TableCell
                    >{flag(row.tool.features?.nozzle_high_flow)}</TableCell
                  >
                  <TableCell
                    >{flag(row.tool.features?.nozzle_hardened)}</TableCell
                  >
                  <TableCell>{@render materialCell(row.id, row.tool)}</TableCell
                  >
                  <TableCell>
                    <span class="inline-flex items-center gap-1">
                      {row.tool.nozzle_temp === undefined
                        ? t('printerDetailInfo.tools.unknown')
                        : formatUnit(row.tool.nozzle_temp, '°C', {
                            maximumFractionDigits: 0,
                          })}
                      {#if settable && (settable.perTool || rows.filter((entry) => !entry.slot).length === 1)}
                        {@const reading = settable.readings.find(
                          (head) => head.index === Number(row.id),
                        )}
                        <NozzleEditor
                          resolve={settable.resolve}
                          busy={settable.busy}
                          onCommand={settable.onCommand}
                          perTool={settable.perTool}
                          temperature={reading?.temperature}
                          target={reading?.target}
                          tool={Number(row.id)}
                        />
                      {/if}
                    </span>
                  </TableCell>
                {/if}
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      {/if}
    {/if}
  </CardContent>
</Card>

{#snippet materialCell(id: string, tool: Tool)}
  <!-- Without an extension that assigns materials the chip is the one the
       header draws on its own, which keeps a material on this browser. -->
  {#snippet reading()}
    {#if live && assignable(tool)}
      <FilamentPositions
        device={live.device}
        telemetry={live.telemetry}
        only={id}
      />
    {:else}
      {materialText(tool)}
    {/if}
  {/snippet}
  {#if material && assignable(tool)}
    {@render material(id, reading)}
  {:else}
    {@render reading()}
  {/if}
{/snippet}
