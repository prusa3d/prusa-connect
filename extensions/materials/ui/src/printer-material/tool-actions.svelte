<!--
  The buttons of a tool. Assign, or Change when a material is
  assigned, while the printer is connected. Unload and Unassign as toolOffer
  decides, which a spool row on a material's page also uses. Add to Materials
  for an NFC spool the module does not hold yet.

  A button the rules refuse is disabled; the tile says why.
-->
<script lang="ts">
  import { Button } from '@prusa/connect-ui/primitives';

  import type { PrinterTools } from '../api';
  import { heldHost } from '../host';
  import { toolOffer } from './tool-offer';
  import type { ToolView } from './tool-view';

  let {
    view,
    printer,
    working,
    stacked,
    onassign,
    onunload,
    onunassign,
    onadd,
  }: {
    view: ToolView;
    printer: PrinterTools;
    /** A change is in flight on some tool. */
    working: boolean;
    /** Full-width buttons one under another, as on a tile. */
    stacked: boolean;
    onassign: () => void;
    onunload: () => void;
    onunassign: () => void;
    onadd: () => void;
  } = $props();

  const t = heldHost().t;

  const tool = $derived(view.tool);
  const operate = $derived(printer.may_operate);
  const offer = $derived(toolOffer(t, printer, tool, true));
  /** An NFC spool the printer read that Materials does not hold yet. */
  const showAdd = $derived(
    tool.from_nfc &&
      (tool.reported?.instance_uuid ?? '') !== '' &&
      tool.assignment?.spool_id === undefined,
  );
</script>

<div
  data-part="tool-actions"
  class={stacked
    ? 'flex w-full items-end gap-1'
    : 'flex flex-wrap items-center gap-2'}
>
  <div class={stacked ? 'flex flex-1 flex-col gap-1' : 'contents'}>
    {#if printer.connected}
      <Button
        size="sm"
        variant="secondary"
        disabled={!operate || tool.assign !== '' || working}
        onclick={onassign}
      >
        {view.name === undefined
          ? t('materials.printer.assign')
          : t('materials.printer.change')}
      </Button>
    {/if}
    {#if offer.unload}
      <Button
        size="sm"
        variant="secondary"
        disabled={offer.refused || working}
        onclick={onunload}
      >
        {t('materials.tools.unload')}
      </Button>
    {/if}
    {#if offer.unassign}
      <Button
        size="sm"
        variant="secondary"
        disabled={offer.refused || working}
        onclick={onunassign}
      >
        {t('materials.printer.unassign')}
      </Button>
    {/if}
    {#if showAdd}
      <Button
        size="sm"
        variant="secondary"
        disabled={!operate || working}
        onclick={onadd}
      >
        {t('materials.printer.addToMaterials')}
      </Button>
    {/if}
  </div>
</div>
