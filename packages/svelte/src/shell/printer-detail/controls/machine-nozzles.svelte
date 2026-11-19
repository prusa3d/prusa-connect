<!-- Nozzles: what is fitted on each toolhead, one row per head. -->
<script lang="ts">
  import {
    type NozzleFeatures,
    toolArgument,
    type ToolReading,
  } from '../../../domain';
  import { formatUnit, t } from '../../../i18n';
  import { toolLabel } from '../../printers/tool-label';
  import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    ToggleGroup,
    ToggleGroupItem,
  } from '../../../primitives';
  import {
    blocked,
    groupReason,
    NOZZLE_DIAMETERS,
    READING_TILE,
    SECTION_TITLE,
    VERBS,
    type SectionProps,
  } from './machine-controls';

  let {
    resolve,
    busy,
    onCommand,
    heads = [],
  }: Omit<SectionProps, 'reading'> & {
    /** The heads in the printer's tool tree. Empty means the one head, 0. */
    heads?: readonly ToolReading[];
  } = $props();

  const diameter = $derived(resolve(VERBS.toolDiameter));
  const highFlow = $derived(resolve(VERBS.toolHighFlow));
  const hardened = $derived(resolve(VERBS.toolHardened));
  const drawn = $derived(
    [diameter, highFlow, hardened].some((entry) => entry.kind !== 'absent'),
  );
  const reason = $derived(groupReason([diameter, highFlow, hardened]));

  // Tools count from 0, and a single head is tool 0.
  const tools = $derived(
    heads.length > 0 ? heads.map((head) => head.index) : [0],
  );

  // A diameter the printer reports that the list does not name is added, so
  // the control can show it.
  const diameters = $derived(
    [
      ...new Set([
        ...NOZZLE_DIAMETERS,
        ...heads.flatMap((head) =>
          head.features.diameter === undefined ? [] : [head.features.diameter],
        ),
      ]),
    ]
      .sort((left, right) => left - right)
      .map((value) => ({
        value: String(value),
        label: formatUnit(value, 'mm'),
      })),
  );

  // A control shows what was last sent from here, and before that what the
  // printer reports under `features/` in its tool tree.
  let sent = $state<Record<string, string>>({});

  // The feature under `features/` each argument sets.
  const FEATURE: Readonly<Record<string, keyof NozzleFeatures>> = {
    nozzle_diameter: 'diameter',
    high_flow: 'highFlow',
    hardened: 'hardened',
  };

  const reported = (tool: number, argument: string): string => {
    const feature = FEATURE[argument];
    const value =
      feature === undefined
        ? undefined
        : heads.find((head) => head.index === tool)?.features[feature];

    return value === undefined ? '' : String(value);
  };

  const shown = (tool: number, argument: string): string =>
    sent[`${String(tool)}:${argument}`] ?? reported(tool, argument);

  const send = (
    verb: string,
    argument: string,
    tool: number,
    value: string,
  ) => {
    if (value === '') return;
    sent[`${String(tool)}:${argument}`] = value;
    void onCommand(verb, { [argument]: value, tool: toolArgument(tool) });
  };

  const nameOf = (tool: number): string =>
    tools.length > 1
      ? toolLabel(tool)
      : t('printerDetailControls.nozzle.label');
</script>

{#snippet onOff(
  verb: string,
  argument: string,
  tool: number,
  label: string,
  off: boolean,
)}
  <ToggleGroup
    type="single"
    variant="outline"
    tone="primary"
    size="sm"
    value={shown(tool, argument)}
    onValueChange={(next: string) => send(verb, argument, tool, next)}
    aria-label={label}
    class="flex-nowrap"
    disabled={off}
  >
    <ToggleGroupItem value="false">
      {t('printerDetailControls.nozzles.off')}
    </ToggleGroupItem>
    <ToggleGroupItem value="true">
      {t('printerDetailControls.nozzles.on')}
    </ToggleGroupItem>
  </ToggleGroup>
{/snippet}

{#if drawn}
  <section class="flex flex-col gap-3 {READING_TILE}">
    <h3 class={SECTION_TITLE}>
      {t('printerDetailControls.nozzles.heading')}
    </h3>

    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('printerDetailControls.nozzles.tool')}</TableHead>
          {#if diameter.kind !== 'absent'}
            <TableHead>{t('printerDetailControls.nozzles.diameter')}</TableHead>
          {/if}
          {#if highFlow.kind !== 'absent'}
            <TableHead class="whitespace-normal"
              >{t('printerDetailControls.nozzles.highFlow')}</TableHead
            >
          {/if}
          {#if hardened.kind !== 'absent'}
            <TableHead class="whitespace-normal"
              >{t('printerDetailControls.nozzles.hardened')}</TableHead
            >
          {/if}
        </TableRow>
      </TableHeader>
      <TableBody>
        {#each tools as tool (tool)}
          {@const name = nameOf(tool)}
          <TableRow>
            <TableHead scope="row" class="text-foreground">{name}</TableHead>
            {#if diameter.kind !== 'absent'}
              <TableCell>
                <Select
                  value={shown(tool, 'nozzle_diameter')}
                  onValueChange={(next: string) =>
                    send(VERBS.toolDiameter, 'nozzle_diameter', tool, next)}
                  items={diameters}
                  disabled={blocked(diameter, busy)}
                >
                  <SelectTrigger
                    size="sm"
                    class="min-w-32"
                    aria-label={t('printerDetailControls.nozzles.diameterOf', {
                      tool: name,
                    })}
                  >
                    <SelectValue
                      placeholder={t('printerDetailControls.noReading')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {#each diameters as option (option.value)}
                      <SelectItem value={option.value}
                        >{option.label}</SelectItem
                      >
                    {/each}
                  </SelectContent>
                </Select>
              </TableCell>
            {/if}
            {#if highFlow.kind !== 'absent'}
              <TableCell>
                {@render onOff(
                  VERBS.toolHighFlow,
                  'high_flow',
                  tool,
                  t('printerDetailControls.nozzles.highFlowOf', { tool: name }),
                  blocked(highFlow, busy),
                )}
              </TableCell>
            {/if}
            {#if hardened.kind !== 'absent'}
              <TableCell>
                {@render onOff(
                  VERBS.toolHardened,
                  'hardened',
                  tool,
                  t('printerDetailControls.nozzles.hardenedOf', { tool: name }),
                  blocked(hardened, busy),
                )}
              </TableCell>
            {/if}
          </TableRow>
        {/each}
      </TableBody>
    </Table>

    {#if reason}
      <p class="text-xs text-muted-foreground">{reason}</p>
    {/if}
  </section>
{/if}
