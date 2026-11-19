<!--
  Adds spools in two steps: choose the material, then say how many spools of
  it and what they weigh. The page's Add material opens it at step 1, where a
  custom material can be made as well. Given a `material`, the list row's + or
  the material page, it opens at step 2 with that material chosen.

  The draft is made new each time the dialog opens, so a dialog closed half way
  starts empty next time. A refusal is drawn in the dialog and the form is
  kept; the dialog closes only once the spools are added.
-->
<script lang="ts">
  import {
    Button,
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    ListingSkeleton,
    MaterialSwatch,
    toast,
  } from '@prusa/connect-ui/primitives';
  import { untrack } from 'svelte';

  import { type Detail, materialsOn } from '../api';
  import { heldHost } from '../host';
  import { OWN_MODULE } from '../own-module';
  import FaultNote from '../materials-page/fault-note.svelte';
  import ChooseMaterial from './choose-material.svelte';
  import { SpoolDraftForm } from './draft.svelte';
  import SpoolDetails from './spool-details.svelte';

  let {
    open,
    onclose,
    onadded,
    material,
  }: {
    open: boolean;
    onclose: () => void;
    /** Called with how many spools were added, before the dialog closes. */
    onadded: (count: number) => void;
    /** The material to add spools of; step 1 is skipped. */
    material?: string | undefined;
  } = $props();

  const host = heldHost();
  const t = host.t;
  const service = materialsOn(host);

  const STEPS = [
    'materials.add.stepChoose',
    'materials.add.stepSpools',
  ] as const;

  const draft = $derived(open ? new SpoolDraftForm() : undefined);
  let busy = $state(false);
  let fault = $state.raw<unknown>(undefined);

  // A dialog opened on one material reads it and starts at step 2.
  $effect(() => {
    const opened = draft;
    untrack(() => {
      fault = undefined;
      if (opened !== undefined && material !== undefined) {
        void preset(opened, material);
      }
    });
  });

  const preset = async (into: SpoolDraftForm, uuid: string): Promise<void> => {
    try {
      into.choose(await service.material(uuid));
    } catch (thrown) {
      fault = thrown;
    }
  };

  const submit = async (event: SubmitEvent): Promise<void> => {
    event.preventDefault();
    if (draft === undefined || !draft.valid || busy) return;
    busy = true;
    fault = undefined;
    try {
      const { added: count } = await service.addSpools(draft.body());
      toast.success(t('materials.add.created', { n: count }));
      onadded(count);
      onclose();
    } catch (thrown) {
      fault = thrown;
    } finally {
      busy = false;
    }
  };
</script>

<Dialog
  {open}
  onOpenChange={(next: boolean) => {
    if (!next) onclose();
  }}
>
  <!-- Portalled, so the panel carries the module's name for the utilities
       inside it. A scoped rule never matches the scope's root, so the
       panel's own cap is the host's. -->
  <DialogContent
    data-module={OWN_MODULE}
    size="xl"
    class="max-h-dialog-height overflow-y-auto"
  >
    <DialogHeader>
      <DialogTitle>
        {material === undefined
          ? t('materials.add.title')
          : t('materials.spools.add')}
      </DialogTitle>
      <DialogDescription class="sr-only">
        {draft?.step === 2
          ? t('materials.add.spoolNote')
          : t('materials.add.chooseNote')}
      </DialogDescription>
      <!-- The two steps in one line. The
           host's Stepper stacks each number over its label, so the dialog
           draws its own; the step it is on carries aria-current. -->
      <ol
        data-part="steps"
        aria-label={t('materials.add.steps')}
        class="flex items-center gap-3 text-sm"
      >
        {#each STEPS as label, index (label)}
          {@const at = (draft?.step ?? 1) - 1}
          {#if index > 0}
            <li aria-hidden="true" class="rule h-px bg-border"></li>
          {/if}
          <li
            class="flex items-center gap-2"
            aria-current={index === at ? 'step' : undefined}
          >
            <span
              aria-hidden="true"
              class={[
                'flex size-6 items-center justify-center rounded-full text-xs font-semibold',
                index <= at
                  ? 'bg-primary text-primary-foreground'
                  : 'border border-border text-muted-foreground',
              ]}
            >
              {index < at ? '✓' : index + 1}
            </span>
            <span
              class={['font-semibold', index > at && 'text-muted-foreground']}
            >
              {t(label)}
            </span>
          </li>
        {/each}
      </ol>
    </DialogHeader>

    {#if draft === undefined}
      <!-- Closed: nothing to draw. -->
    {:else if draft.step === 2 && draft.material}
      {@const chosen = draft.material}
      <form class="grid gap-4" onsubmit={(event) => void submit(event)}>
        <div
          data-part="chosen"
          class="flex items-center gap-3 rounded-sm border border-border p-3"
        >
          <MaterialSwatch
            class="size-6"
            material={{
              name: chosen.name,
              colorRgba:
                chosen.color_rgba === '' ? undefined : chosen.color_rgba,
            }}
          />
          <div class="min-w-0 flex-1">
            <p class="font-medium">{chosen.name}</p>
            <p class="text-sm text-muted-foreground">
              {[chosen.brand_name, chosen.type]
                .filter((s) => s !== '')
                .join(' · ')}
            </p>
          </div>
          <!-- Opened on one material, from its row or its page, the dialog is
               about that material and has nothing to change it to. -->
          {#if material === undefined}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onclick={() => {
                draft.step = 1;
                fault = undefined;
              }}
            >
              {t('materials.add.change')}
            </Button>
          {/if}
        </div>

        <SpoolDetails {draft} />
        <FaultNote {fault} />

        <DialogFooter>
          <Button type="button" variant="secondary" onclick={onclose}>
            {t('materials.add.cancel')}
          </Button>
          <Button type="submit" disabled={!draft.valid || busy}>
            {t('materials.add.submit')}
          </Button>
        </DialogFooter>
      </form>
    {:else if material !== undefined && draft.material === undefined}
      <!-- The given material is being read. -->
      <FaultNote {fault} />
      {#if fault === undefined}
        <ListingSkeleton label={t('materials.add.reading')} rows={3} />
      {/if}
    {:else}
      <ChooseMaterial
        {service}
        chosen={draft.material}
        onchoose={(detail: Detail) => {
          draft.choose(detail);
        }}
        oncancel={onclose}
      />
    {/if}
  </DialogContent>
</Dialog>

<style>
  .rule {
    flex: 0 1 calc(var(--spacing) * 40);
  }
</style>
