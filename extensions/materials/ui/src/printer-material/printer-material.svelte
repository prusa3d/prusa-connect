<!--
  The Tools tab of the printer page: a row per tool, and an MMU's slots under
  one header. The Tools table beside it already shows nozzles and materials,
  so a row keeps what the table cannot: what is left of the spool, Change,
  Unload and Unassign, and a link to the material. Assign and Change open the
  same picker as the header chip and the table.

  A control the rules refuse is disabled, and an info mark says why. A person
  who may not operate the printer sees the controls disabled for that reason
  first. A printer the person may not observe is not there for them, so the
  tab draws nothing.
-->
<script lang="ts">
  import { STATE_PATH, textAt } from '@prusa/connect-ui/domain';
  import {
    Button,
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DIALOG_SCROLLS,
    ListingSkeleton,
  } from '@prusa/connect-ui/primitives';
  import { FilamentPosition } from '@prusa/connect-ui/shell';
  import { onMount, untrack, type Snippet } from 'svelte';

  import { materialsOn, type PrinterTools, type Tool } from '../api';
  import { heldHost } from '../host';
  import { heldList } from '../list.svelte';
  import FaultNote from '../materials-page/fault-note.svelte';
  import TapTooltip from '../tap-tooltip.svelte';
  import MaterialPicker from './material-picker.svelte';
  import MmuGroup from './mmu-group.svelte';
  import { OWN_MODULE } from '../own-module';
  import { holdPrinterMaterials } from './printer-material-state.svelte';
  import { reasonText, refusalText } from './reasons';
  import ToolActions from './tool-actions.svelte';
  import ToolIcon from './tool-icon.svelte';
  import { toolOffer, unloadFilament } from './tool-offer';
  import ToolTile from './tool-tile.svelte';
  import { type ToolView, toolView } from './tool-view';

  let {
    deviceId,
    compact = false,
    toolId,
    fallback,
  }: {
    deviceId: string;
    compact?: boolean;
    /** With `compact`, the one tool to draw, so a table cell can hold its chip. */
    toolId?: string;
    fallback?: Snippet;
  } = $props();

  const host = heldHost();
  const t = host.t;
  const service = materialsOn(host);
  const inventory = heldList();

  /** The tool a change is in flight on. */
  let working = $state<string | undefined>(undefined);
  /** A change the module refused, and on which tool. */
  let refused = $state.raw<
    | { tool: string; thrown: unknown; action: 'unassign' | 'add' | 'unload' }
    | undefined
  >(undefined);
  /** The tool the picker dialog is open on. */
  let assigning = $state.raw<Tool | undefined>(undefined);
  /** The title of the form the picker swapped itself for. */
  let pickerTitle = $state<string>();
  let pickerBusy = $state(false);

  let materials =
    $state.raw<ReturnType<typeof holdPrinterMaterials>['state']>();
  const printer = $derived(materials?.printer);
  const fault = $derived(materials?.fault);
  const gone = $derived(materials?.gone ?? false);

  function compactRefusal(
    mayOperate: boolean,
    connected: boolean,
    state: NonNullable<typeof printer>['state'],
    tool: Tool,
    reported: string,
  ): string | undefined {
    if (!mayOperate) return t('materials.printer.notOperator');
    if (!inventory.mayWrite)
      return t(
        inventory.mayOnlyRead
          ? 'materials.header.readOnly'
          : 'materials.header.checkingAccess',
      );
    if (!connected) return t('materials.printer.offline');
    return reasonText(t, tool.assign, 'assign', state, reported);
  }

  onMount(() => {
    const held = holdPrinterMaterials(host, service, deviceId);
    materials = held.state;
    return () => held.release();
  });

  /** The printer's state word and link, as the host's fleet keeps them live. */
  const live = $derived.by(() => {
    const device = host.fleet.device(deviceId);
    if (device === undefined) return undefined;
    return `${String(device.connected)} ${textAt(device, STATE_PATH) ?? ''}`;
  });
  /** The value the mount read already answers for. */
  let seen: string | undefined;

  // A print that starts or a filament change changes what the rules allow, so
  // the tools are read again once the state settles.
  $effect(() => {
    const now = live;
    untrack(() => {
      if (now === undefined || now === seen) return;
      const first = seen === undefined;
      seen = now;
      if (!first) materials?.soon();
    });
  });

  /** Runs one change on a tool, then reads the tools again. */
  const change = async (
    tool: Tool,
    action: 'unassign' | 'add' | 'unload',
  ): Promise<void> => {
    if (working !== undefined) return;
    working = tool.id;
    refused = undefined;
    try {
      if (action === 'add') await service.addReported(deviceId, tool.id);
      else if (action === 'unassign') await service.unassign(deviceId, tool.id);
      else await unloadFilament(host, deviceId);
      await materials?.read();
    } catch (thrown) {
      refused = { tool: tool.id, thrown, action };
    } finally {
      working = undefined;
    }
  };

  /** The tools drawn: all of them, or the one a table cell asked for. */
  const shown = $derived(
    (printer?.tools ?? []).filter(
      (x) => toolId === undefined || x.id === toolId,
    ),
  );

  const printing = $derived(printer?.state === 'PRINTING');
  const slots = $derived(printer?.tools.filter((x) => x.kind === 'slot') ?? []);
  const heads = $derived(printer?.tools.filter((x) => x.kind === 'head') ?? []);
  /** One print head and no MMU: the page describes that tool in full. */
  const single = $derived(heads.length === 1 && slots.length === 0);
  const viewOf = (tool: Tool): ToolView => {
    const id = tool.assignment?.spool_id;
    const spool = id === undefined ? undefined : materials?.spools.get(id);
    return toolView(t, tool, spool, single, printing);
  };
  const headViews = $derived(heads.map(viewOf));

  /** Why the MMU's Unload filament is refused, undefined when it is not. */
  const mmuWhy = (
    slot: Tool,
    loaded: Tool | undefined,
    operate: boolean,
  ): string | undefined => {
    if (!operate) return t('materials.printer.notOperator');
    if (slot.unassign === 'printing')
      return t('materials.tools.unloadPrinting');
    if (slot.unassign === 'busy') return t('materials.printer.busy');
    if (slot.unassign !== '') return t('error.materials/TOOL_REFUSED');
    if (loaded === undefined) return t('materials.tools.nothingLoaded');
    return undefined;
  };

  /** The MMUs, each the slots of one head, with what its header needs. */
  const mmus = $derived(
    [...new Set(slots.map((s) => s.id.split('.')[0] ?? ''))].flatMap((head) => {
      const group = slots.filter((s) => s.id.startsWith(`${head}.`));
      const [first] = group;
      if (first === undefined) return [];
      const loaded = group.find((s) => s.active);
      return [
        {
          head,
          first,
          views: group.map(viewOf),
          // The printer unloads through the head whatever slot fed it.
          unloads: loaded ?? first,
          why: mmuWhy(first, loaded, printer?.may_operate ?? false),
        },
      ];
    }),
  );
</script>

{#snippet toolActions(on: PrinterTools, view: ToolView)}
  {@const failed = refused?.tool === view.tool.id ? refused : undefined}
  <div class="flex w-full flex-col gap-2">
    <ToolActions
      {view}
      printer={on}
      working={working !== undefined}
      stacked
      onassign={() => {
        refused = undefined;
        pickerTitle = undefined;
        assigning = view.tool;
      }}
      onunload={() => void change(view.tool, 'unload')}
      onunassign={() => void change(view.tool, 'unassign')}
      onadd={() => void change(view.tool, 'add')}
    />
    {#if failed}
      {@const text =
        failed.action === 'unassign'
          ? refusalText(
              t,
              failed.thrown,
              'unassign',
              view.tool.reported?.abbreviation ?? '',
            )
          : undefined}
      {#if text}
        <p data-part="refusal" class="text-sm text-destructive">{text}</p>
      {:else}
        <FaultNote fault={failed.thrown} />
      {/if}
    {/if}
  </div>
{/snippet}

{#snippet tile(on: PrinterTools, view: ToolView)}
  <ToolTile
    {view}
    open={assigning?.id === view.tool.id}
    reasons={toolOffer(t, on, view.tool, true).reasons}
  >
    {#snippet actions()}
      {@render toolActions(on, view)}
    {/snippet}
  </ToolTile>
{/snippet}

{#if compact && toolId !== undefined && printer === undefined && fault === undefined && !gone}
  <!-- A table cell stays empty while the tools are read, so the fallback does not flash. -->
{:else if compact && (gone || (toolId !== undefined && shown.length === 0))}
  {@render fallback?.()}
{:else if compact}
  <div
    data-part="materials-positions"
    role="group"
    class={[
      'flex flex-wrap items-center gap-x-2 gap-y-1.5',
      toolId === undefined && 'mt-1',
    ]}
    aria-label={t('materials.tools.title')}
  >
    <FaultNote {fault} />
    {#if printer === undefined}
      {#if fault === undefined}
        <span class="text-sm text-muted-foreground"
          >{t('materials.printer.reading')}</span
        >
      {/if}
    {:else if printer.tools.length === 0}
      <span class="text-sm text-muted-foreground"
        >{t('materials.tools.noTools')}</span
      >
    {:else}
      {#each shown as tool (tool.id)}
        {@const view = viewOf(tool)}
        {@const reported = tool.reported?.abbreviation ?? ''}
        {@const why = compactRefusal(
          printer.may_operate,
          printer.connected,
          printer.state,
          tool,
          reported,
        )}
        <FilamentPosition
          {deviceId}
          toolId={tool.id}
          position={view.label}
          bare={toolId !== undefined}
          {reported}
          assignedName={view.name ?? (reported || t('materials.printer.none'))}
          note={view.name === undefined && reported
            ? t('materials.tools.reportedOnly', { material: reported })
            : undefined}
          assignedColor={view.colorRgba}
          remainingGrams={view.leftG}
          disabledReason={why ??
            (tool.assign !== ''
              ? t('error.materials/TOOL_REFUSED')
              : undefined)}
        >
          {#snippet panel(
            close: () => void,
            setPanelTitle: (title?: string) => void,
            setPanelBusy: (busy: boolean) => void,
          )}
            <MaterialPicker
              {service}
              {deviceId}
              {tool}
              onclose={close}
              onassigned={() => {
                void materials?.read();
                close();
              }}
              ontitle={setPanelTitle}
              onbusy={setPanelBusy}
            />
          {/snippet}
        </FilamentPosition>
      {/each}
    {/if}
  </div>
{:else if !gone}
  <!-- The tab already says Tools, so the section repeats it only to a screen reader. -->
  <section
    data-part="printer-tools"
    aria-label={t('materials.tools.title')}
    class="@container flex flex-col gap-4"
  >
    <FaultNote {fault} />
    {#if printer === undefined}
      {#if fault === undefined}
        <ListingSkeleton label={t('materials.printer.reading')} rows={2} />
      {/if}
    {:else if printer.tools.length === 0}
      <p class="text-sm text-muted-foreground">
        {t('materials.tools.noTools')}
      </p>
    {:else}
      {@const on = printer}
      {#if headViews.length > 0}
        <ul
          class="tiles grid gap-2"
          style="--tools: {Math.min(headViews.length, 6)};"
        >
          {#each headViews as view (view.tool.id)}
            {@render tile(on, view)}
          {/each}
        </ul>
      {/if}
      {#each mmus as mmu (mmu.head)}
        <MmuGroup
          feeder={mmu.first.feeder ?? ''}
          nozzle={mmu.first.nozzle}
          unload={on.connected ? mmuUnload : undefined}
        >
          {#each mmu.views as view (view.tool.id)}
            {@render tile(on, view)}
          {/each}
        </MmuGroup>
        {#snippet mmuUnload()}
          <span class="flex items-center gap-2">
            {#if mmu.why !== undefined}
              <TapTooltip text={mmu.why}>
                <span data-part="reasons" class="inline-flex text-primary">
                  <ToolIcon name="info" class="size-4" />
                </span>
              </TapTooltip>
            {/if}
            <Button
              size="sm"
              variant="secondary"
              disabled={mmu.why !== undefined || working !== undefined}
              onclick={() => void change(mmu.unloads, 'unload')}
            >
              {t('materials.tools.unloadFilament')}
            </Button>
          </span>
        {/snippet}
      {/each}
    {/if}
  </section>
{/if}

{#if !compact}
  <!-- The same picker as the header chip and the Tools table. -->
  <Dialog
    open={assigning !== undefined}
    onOpenChange={(next: boolean) => {
      if (!next && !pickerBusy) assigning = undefined;
    }}
  >
    <!-- The panel is portalled, so it carries the module's name for its
         utilities. -->
    <DialogContent data-module={OWN_MODULE} size="md" class={DIALOG_SCROLLS}>
      <DialogHeader>
        <DialogTitle>
          {pickerTitle ??
            t('materials.printer.assignTo', {
              tool: assigning === undefined ? '' : viewOf(assigning).label,
            })}
        </DialogTitle>
      </DialogHeader>
      {#if assigning}
        <MaterialPicker
          {service}
          {deviceId}
          tool={assigning}
          onclose={() => (assigning = undefined)}
          onassigned={() => {
            void materials?.read();
            assigning = undefined;
          }}
          ontitle={(title?: string) => (pickerTitle = title)}
          onbusy={(busy: boolean) => (pickerBusy = busy)}
        />
      {/if}
    </DialogContent>
  </Dialog>
{/if}

<style>
  /* The tools share one row, up to six, each at most 20rem wide; a
     narrow screen fits as many as it can. */
  .tiles {
    grid-template-columns: repeat(var(--tools), minmax(0, 20rem));
  }

  @container (max-width: 52rem) {
    .tiles {
      grid-template-columns: repeat(
        auto-fill,
        minmax(calc(var(--spacing) * 36), 1fr)
      );
    }
  }

  /* On a phone three tiles leave each name two lines and a cramped circle. */
  @container (max-width: 30rem) {
    .tiles {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
