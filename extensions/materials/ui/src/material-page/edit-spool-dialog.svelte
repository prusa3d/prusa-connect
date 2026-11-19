<!--
  Changes one spool: the weight left, where it is kept, the notes, and whether
  it is archived, with an optional reason and note. Unticking Archived brings
  it back, and the module clears the reason and the note. It sends only the
  fields the person changed, because the module keeps whatever a PATCH leaves
  out.

  A spool on a printer tool takes notes and a location only: the printer
  counts its weight, and an archived spool goes on no tool.

  The fields are made new each time the dialog opens on a spool, so a dialog
  closed half way starts from the spool again.
-->
<script lang="ts">
  import {
    Button,
    Checkbox,
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    Field,
    Input,
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Textarea,
  } from '@prusa/connect-ui/primitives';
  import { untrack } from 'svelte';

  import type { Spool, SpoolPatch } from '../api';
  import { heldHost } from '../host';
  import { OWN_MODULE } from '../own-module';
  import LocationField from '../location-field.svelte';
  import FaultNote from '../materials-page/fault-note.svelte';
  import { typedWeight } from '../materials-page/weights';

  let {
    spool,
    busy,
    fault,
    onsave,
    onclose,
  }: {
    /** The spool being changed; the dialog is open while there is one. */
    spool: Spool | undefined;
    busy: boolean;
    fault: unknown;
    onsave: (patch: SpoolPatch) => void;
    onclose: () => void;
  } = $props();

  const t = heldHost().t;
  const uid = $props.id();

  /** A spool on a tool keeps its weight and its archive: the printer counts it. */
  const onTool = $derived(spool?.tool != null);

  let remaining = $state('');
  let locationId = $state<number | undefined>(undefined);
  let notes = $state('');
  const NONE = '*';
  let archived = $state(false);
  /** The archive reason, NONE for none. */
  let reason = $state(NONE);
  let reasonNote = $state('');

  const reasons = [
    'consumed',
    'defective',
    'lost',
    'expired',
    'damaged',
    'other',
  ] as const;
  const reasonItems = $derived([
    { value: NONE, label: t('materials.archive.noReason') },
    ...reasons.map((r) => ({
      value: r,
      label: t(`materials.archive.reason.${r}`),
    })),
  ]);

  $effect(() => {
    const opened = spool;
    untrack(() => {
      remaining = String(opened?.remaining_weight_g ?? '');
      locationId = opened?.location_id;
      notes = opened?.notes ?? '';
      archived = opened?.archived_at != null;
      reason = opened?.archive_reason ?? NONE;
      reasonNote = opened?.archive_reason_note ?? '';
    });
  });

  const weight = $derived(typedWeight(remaining) ?? NaN);
  const valid = $derived(Number.isFinite(weight) && weight >= 0);

  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    if (spool === undefined || !valid || busy) return;
    const patch: SpoolPatch = {};
    if (weight !== spool.remaining_weight_g) patch.remaining_weight_g = weight;
    // 0 takes the spool out of any location.
    if (locationId !== spool.location_id) patch.location_id = locationId ?? 0;
    if (notes !== spool.notes) patch.notes = notes;
    if (archived !== (spool.archived_at != null)) patch.archived = archived;
    // Restoring clears the reason and the note, so only an archived spool sends them.
    if (archived) {
      const chosen = reason === NONE ? '' : reason;
      if (chosen !== (spool.archive_reason ?? '')) {
        patch.archive_reason = chosen as NonNullable<
          SpoolPatch['archive_reason']
        >;
      }
      if (reasonNote !== (spool.archive_reason_note ?? '')) {
        patch.archive_reason_note = reasonNote;
      }
    }
    // Nothing changed: there is nothing to send.
    if (Object.keys(patch).length === 0) onclose();
    else onsave(patch);
  };
</script>

<Dialog
  open={spool !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) onclose();
  }}
>
  <!-- Portalled, so the panel carries the module's name for the utilities
       inside it. A scoped rule never matches the scope's root, so the
       panel's own cap is the host's. -->
  <DialogContent
    data-module={OWN_MODULE}
    class="max-h-dialog-height overflow-y-auto"
  >
    <DialogHeader>
      <DialogTitle
        >{t('materials.edit.title', { id: spool?.id ?? 0 })}</DialogTitle
      >
      <DialogDescription>{t('materials.edit.note')}</DialogDescription>
      {#if onTool}
        <p data-part="on-tool" class="text-sm text-muted-foreground">
          {t('materials.edit.onTool')}
        </p>
      {/if}
    </DialogHeader>
    <form class="grid gap-4" onsubmit={submit}>
      <Field
        label={t('materials.spool.remaining')}
        control={Input}
        controlProps={{
          name: 'remaining_weight_g',
          type: 'text',
          inputmode: 'decimal',
          required: true,
          disabled: onTool,
          value: remaining,
          oninput: (e: Event) =>
            (remaining = (e.currentTarget as HTMLInputElement).value),
        }}
      />
      <LocationField
        value={locationId}
        name={spool?.location}
        onpick={(id: number | undefined) => (locationId = id)}
      />
      <div class="flex flex-col gap-2">
        <Label for="{uid}-notes">{t('materials.spool.notes')}</Label>
        <Textarea id="{uid}-notes" rows={2} bind:value={notes} />
      </div>
      <div class="flex items-center gap-2">
        <Checkbox
          id="{uid}-archived"
          disabled={onTool}
          checked={archived}
          onCheckedChange={(on: boolean) => (archived = on)}
        />
        <Label for="{uid}-archived">{t('materials.archive.archived')}</Label>
      </div>
      {#if archived}
        <div class="flex flex-col gap-2">
          <Label for="{uid}-reason">{t('materials.archive.reason')}</Label>
          <Select
            value={reason}
            onValueChange={(chosen: string) => (reason = chosen)}
            items={reasonItems}
          >
            <SelectTrigger id="{uid}-reason" class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {#each reasonItems as item (item.value)}
                <SelectItem value={item.value} label={item.label}>
                  {item.label}
                </SelectItem>
              {/each}
            </SelectContent>
          </Select>
        </div>
        <div class="flex flex-col gap-2">
          <Label for="{uid}-reason-note">{t('materials.archive.note')}</Label>
          <Textarea
            id="{uid}-reason-note"
            rows={2}
            maxlength={1000}
            bind:value={reasonNote}
          />
        </div>
      {/if}
      <FaultNote {fault} />
      <DialogFooter>
        <Button type="button" variant="secondary" onclick={onclose}>
          {t('materials.add.cancel')}
        </Button>
        <Button type="submit" disabled={!valid || busy}>
          {t('materials.edit.save')}
        </Button>
      </DialogFooter>
    </form>
  </DialogContent>
</Dialog>
