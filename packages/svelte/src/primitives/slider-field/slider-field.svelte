<!--
  A slider with the number beside it, typed as well as dragged. It sets a value
  on the machine, so the slider takes the primary tone.
-->
<script lang="ts">
  import Input from '../input/input.svelte';
  import Slider from '../slider/slider.svelte';
  import type { SliderFieldProps } from './types';

  /** A value being worked on, and the reported value it was made against. */
  interface Draft {
    since: number;
    value: number;
    /** What stands in the field, once it says something the draft does not. */
    typed: string | null;
  }

  let {
    value,
    onCommit,
    min,
    max,
    step,
    unit,
    label,
    disabled,
    class: className,
  }: SliderFieldProps = $props();

  const clamp = (next: number): number => Math.min(max, Math.max(min, next));

  let edit = $state<Draft | undefined>();
  // A fresh reading ends the draft.
  const current = $derived(edit?.since === value ? edit : undefined);
  const draft = $derived(current?.value ?? value);
  const text = $derived(current?.typed ?? String(draft));

  const commitTyped = (): void => {
    const typed = current?.typed;
    if (typed === undefined || typed === null) return;
    const parsed = Number.parseFloat(typed);
    const snapped =
      step === undefined
        ? parsed
        : Number(
            (min + Math.round((parsed - min) / step) * step).toPrecision(12),
          );
    const next = Number.isFinite(snapped) ? clamp(snapped) : value;
    edit = { since: value, value: next, typed: null };
    if (next !== value) onCommit(next);
  };
</script>

<div data-slot="slider-field" class={['flex items-center gap-3', className]}>
  <Slider
    type="single"
    value={draft}
    onValueChange={(next: number) => {
      edit = { since: value, value: next, typed: null };
    }}
    onValueCommit={(next: number) => {
      onCommit(next);
    }}
    {min}
    {max}
    step={step ?? 1}
    disabled={disabled ?? false}
    aria-label={label}
    tone="primary"
    class="min-w-24 flex-1"
  />
  <!-- `inputmode="numeric"` and `type="number"`. -->
  <div class="flex shrink-0 items-center gap-1">
    <div class="w-32">
      <Input
        type="number"
        inputmode="numeric"
        value={text}
        {min}
        {max}
        {step}
        {disabled}
        aria-label={`${label}, ${unit}`}
        oninput={(event: Event & { currentTarget: HTMLInputElement }) => {
          edit = {
            since: value,
            value: draft,
            typed: event.currentTarget.value,
          };
        }}
        onblur={commitTyped}
        onkeydown={(event: KeyboardEvent) => {
          if (event.key !== 'Enter') return;
          event.preventDefault();
          commitTyped();
        }}
        class="text-right font-mono tabular-nums"
      />
    </div>
    <span aria-hidden="true" class="text-sm text-muted-foreground">{unit}</span>
  </div>
</div>
