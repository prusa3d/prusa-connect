<script lang="ts">
  import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
  import PencilIcon from '@lucide/svelte/icons/pencil';

  import { formatNumber, t } from '../../i18n';
  import Button from '../button/button.svelte';
  import Field from '../field/field.svelte';
  import Input from '../input/input.svelte';
  import Popover from '../popover/popover.svelte';
  import PopoverContent from '../popover/popover-content.svelte';
  import PopoverHeader from '../popover/popover-header.svelte';
  import PopoverTitle from '../popover/popover-title.svelte';
  import PopoverTrigger from '../popover/popover-trigger.svelte';
  import Slider from '../slider/slider.svelte';
  import type { ValueChoice, ValueEditorProps, ValuePreset } from './types';

  let {
    label,
    subject,
    currentLabel,
    target,
    unit,
    min,
    max,
    step = 1,
    presets,
    disabled,
    reason,
    trigger,
    choices,
    choice,
    announceTarget,
    onCommit,
    class: className,
    ...rest
  }: ValueEditorProps = $props();

  const clamp = (next: number): number => Math.min(max, Math.max(min, next));

  let open = $state(false);
  // The draft starts at the target and goes back to it every time the surface
  // opens, so what is read here is only the value before the first session.
  // svelte-ignore state_referenced_locally
  let draft = $state(target);
  // Text, not a number: an empty field and a `-` before the first digit are
  // legitimate mid-edit states a number cannot hold.
  // svelte-ignore state_referenced_locally
  let inputText = $state(String(target));
  // Held apart from the draft because preset values are not unique.
  let presetLabel = $state('');
  let picked = $state<ValueChoice | undefined>();
  // The slider snaps what it is given to `step` and reports the snap back, which
  // would overwrite a figure typed between steps, so it gets a value already on one.
  const sliderValue = $derived(
    Math.min(max, min + Math.round((draft - min) / step) * step),
  );
  const listing = $derived(choices !== undefined && picked === undefined);

  const nameFor = (who: string | undefined): string =>
    who === undefined
      ? t('primitives.valueEditor.value', { label: label.toLowerCase() })
      : t('primitives.valueEditor.valueFor', {
          subject: who,
          label: label.toLowerCase(),
        });
  const valueName = $derived(nameFor(subject));
  const shownSubject = $derived(picked?.subject ?? subject);
  const triggerName = $derived(
    announceTarget
      ? t('primitives.valueEditor.triggerNow', {
          value: valueName,
          target: formatNumber(target),
          unit,
        })
      : t('primitives.valueEditor.trigger', { value: valueName }),
  );
  const title = $derived(
    shownSubject === undefined
      ? label
      : t('primitives.valueEditor.title', { subject: shownSubject, label }),
  );

  const startFrom = (value: number): void => {
    draft = value;
    inputText = String(value);
    presetLabel = '';
  };

  const startSession = (nextOpen: boolean): void => {
    if (!nextOpen) return;
    picked = undefined;
    startFrom(target);
  };

  const pick = (next: ValueChoice): void => {
    picked = next;
    startFrom(next.target);
  };

  const setDraftValue = (next: number): void => {
    const clamped = clamp(next);
    draft = clamped;
    inputText = String(clamped);
    presetLabel = '';
  };

  const choosePreset = (preset: ValuePreset): void => {
    // `setDraftValue` clears the label, so it has to run before this.
    setDraftValue(preset.value);
    presetLabel = preset.label;
  };

  const commit = (): void => {
    onCommit(draft, picked);
    open = false;
  };
</script>

<div data-slot="value-editor" class={['inline-flex', className]} {...rest}>
  <Popover bind:open onOpenChange={startSession}>
    <PopoverTrigger>
      <!-- Annotated because the trigger's attributes go into an object. -->
      {#snippet child({ props }: { props: Record<string, unknown> })}
        {#if trigger}
          {@render trigger({
            props: {
              ...props,
              'aria-label': triggerName,
              title: triggerName,
              disabled,
            },
          })}
        {:else}
          <Button
            {...props}
            variant="ghost"
            size="icon-md"
            label={triggerName}
            {disabled}
          >
            <PencilIcon />
          </Button>
        {/if}
      {/snippet}
    </PopoverTrigger>
    <PopoverContent align="end" class="w-80">
      <PopoverHeader class="flex-row items-center">
        {#if picked !== undefined}
          <Button
            variant="ghost"
            size="icon-sm"
            label={t('primitives.back')}
            onclick={() => {
              picked = undefined;
            }}
          >
            <ChevronLeftIcon />
          </Button>
        {/if}
        <PopoverTitle>{title}</PopoverTitle>
      </PopoverHeader>

      <div class="mt-4 flex flex-col gap-4">
        {#if listing}
          <ul class="flex flex-col gap-1" aria-label={title}>
            {#each choices ?? [] as entry (entry.key)}
              <li>
                <Button
                  variant="ghost"
                  size="sm"
                  class="w-full justify-start"
                  onclick={() => {
                    pick(entry);
                  }}
                >
                  {#if choice}
                    {@render choice(entry)}
                  {:else}
                    {entry.subject}
                  {/if}
                </Button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="font-mono text-sm text-muted-foreground tabular-nums">
            {t('primitives.valueEditor.now', {
              reading: picked?.currentLabel ?? currentLabel,
            })}
          </p>

          <!-- A value the printer will not take opens to say so. -->
          {#if reason}
            <p class="text-sm text-muted-foreground">{reason}</p>
          {:else}
            {#if presets && presets.length > 0}
              <!-- The row wraps, and every value is in it. -->
              <div class="flex flex-wrap gap-2">
                {#each presets as preset (preset.label)}
                  <Button
                    variant={presetLabel === preset.label
                      ? 'default'
                      : 'secondary'}
                    size="sm"
                    onclick={() => {
                      choosePreset(preset);
                    }}
                  >
                    {preset.label}
                  </Button>
                {/each}
              </div>
            {/if}

            <Slider
              type="single"
              value={sliderValue}
              {min}
              {max}
              {step}
              aria-label={nameFor(shownSubject)}
              onValueChange={setDraftValue}
            />

            <Field
              label={picked?.targetLabel ??
                t('primitives.valueEditor.target', { unit })}
              control={Input}
              controlProps={{
                type: 'number',
                inputmode: 'numeric',
                min,
                max,
                step,
                value: inputText,
                oninput: (
                  event: Event & { currentTarget: HTMLInputElement },
                ) => {
                  inputText = event.currentTarget.value;
                  // A hand typed figure came from nobody.
                  presetLabel = '';
                  const parsed = Number(event.currentTarget.value);
                  // An empty or unreadable field leaves the draft alone rather.
                  if (
                    event.currentTarget.value !== '' &&
                    !Number.isNaN(parsed)
                  ) {
                    draft = clamp(parsed);
                  }
                },
                onblur: () => {
                  setDraftValue(draft);
                },
              }}
            />

            <div class="flex justify-end gap-2">
              <Button
                variant="secondary"
                size="sm"
                onclick={() => {
                  open = false;
                }}
              >
                {t('primitives.cancel')}
              </Button>
              <Button
                size="sm"
                onclick={() => {
                  commit();
                }}
              >
                {t('primitives.valueEditor.set')}
              </Button>
            </div>
          {/if}
        {/if}
      </div>
    </PopoverContent>
  </Popover>
</div>
