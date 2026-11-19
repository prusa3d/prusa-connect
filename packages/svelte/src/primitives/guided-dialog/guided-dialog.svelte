<script lang="ts">
  // The shape every flow with more than one question uses: a dialog, a compact
  // stepper in its header, Back and one primary button, and the three ends a
  // flow reaches.
  //
  // The step index is the caller's, not this component's. Which step follows
  // depends on the answer, and a component that counted for itself would have
  // to be told to skip one anyway.
  import { t } from '../../i18n';
  import Button from '../button/button.svelte';
  import Dialog from '../dialog/dialog.svelte';
  import DialogContent from '../dialog/dialog-content.svelte';
  import DialogDescription from '../dialog/dialog-description.svelte';
  import DialogFooter from '../dialog/dialog-footer.svelte';
  import DialogHeader from '../dialog/dialog-header.svelte';
  import DialogTitle from '../dialog/dialog-title.svelte';
  import Progress from '../progress/progress.svelte';
  import Stepper from '../stepper/stepper.svelte';
  import GuidedEnd from './guided-end.svelte';
  import type { GuidedDialogProps } from './types';

  let {
    open = $bindable(false),
    title,
    description,
    steps,
    current,
    status = 'editing',
    canContinue = true,
    busy = false,
    waitingMessage,
    endTitle,
    endMessage,
    backLabel,
    nextLabel,
    finishLabel,
    doneLabel,
    onback,
    onnext,
    onclose,
    children,
    help,
    endActions,
    class: className,
  }: GuidedDialogProps = $props();

  const lastStep = $derived(current >= steps.length - 1);
  const ended = $derived(status === 'success' || status === 'failure');

  // A caller may name its own words. The default is read from the catalogue
  // when the component draws, so a change of language rewrites it
  // (frontend-i18n.md rule 1).
  const backText = $derived(backLabel ?? t('primitives.back'));
  const nextText = $derived(nextLabel ?? t('primitives.next'));
  const finishText = $derived(finishLabel ?? t('primitives.finish'));
  const doneText = $derived(doneLabel ?? t('primitives.done'));

  const primaryLabel = $derived(lastStep ? finishText : nextText);

  const close = () => {
    open = false;
    onclose?.();
  };

  // bits-ui reports a close it decided on itself, such as Escape or a press on
  // the scrim, through this one callback, so the caller hears every close and
  // not only the one its own button made.
  const onOpenChange = (next: boolean) => {
    if (!next) onclose?.();
  };
</script>

<Dialog bind:open {onOpenChange}>
  <!-- The panel is capped at the height of the page so a long step scrolls
       inside the body instead of running off the top. -->
  <DialogContent class={['max-h-dialog-height', className]}>
    <DialogHeader>
      <DialogTitle>{title}</DialogTitle>
      {#if description}
        <DialogDescription>{description}</DialogDescription>
      {/if}
      {#if !ended}
        <Stepper
          {steps}
          {current}
          compact
          label={t('primitives.stepper.of', { title })}
        />
      {/if}
    </DialogHeader>

    <!-- The only part that scrolls. The header and the footer stay where they
         are, so a long step never pushes its own buttons off the page. -->
    <div data-slot="dialog-body" class="min-h-0 overflow-y-auto">
      {#if status === 'waiting'}
        <!-- One indeterminate bar and no spinner (earlier research recommendation
             5): the bar is the tree's only waiting glyph, and reduced motion
             already stops it. `role="status"` reads the sentence out when it
             arrives, without taking focus off whatever the person is on. -->
        <div class="flex flex-col gap-3" role="status">
          <Progress tone="primary" />
          {#if waitingMessage}
            <p class="text-body text-muted-foreground">{waitingMessage}</p>
          {/if}
          {@render help?.()}
        </div>
      {:else if ended}
        <GuidedEnd
          status={status === 'success' ? 'success' : 'failure'}
          title={endTitle}
          message={endMessage}
          {help}
        />
      {:else}
        {@render children?.()}
      {/if}
    </div>

    <!-- A wait with nothing to press draws no band at all. An empty tinted
         strip under the message reads as a control that failed to arrive. -->
    {#if status !== 'waiting' || endActions !== undefined}
      <DialogFooter>
        {#if ended}
          {#if endActions}
            {@render endActions()}
          {:else}
            <Button onclick={close}>{doneText}</Button>
          {/if}
        {:else if status === 'waiting'}
          {@render endActions?.()}
        {:else}
          <Button
            variant="secondary"
            disabled={current <= 0}
            onclick={() => onback?.()}>{backText}</Button
          >
          <!-- Busy is not `disabled`: the button keeps focus while the answer
               is in flight, so it reports the state and swallows the press
               itself. -->
          <Button
            disabled={!canContinue}
            aria-busy={busy || undefined}
            aria-disabled={busy || undefined}
            onclick={() => {
              if (busy) return;
              onnext?.();
            }}>{primaryLabel}</Button
          >
        {/if}
      </DialogFooter>
    {/if}
  </DialogContent>
</Dialog>
