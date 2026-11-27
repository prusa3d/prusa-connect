<!--
  The settings of one card, drawn from the schema its declaration carries
  (ADR-20261008-sww rules 18 and 19). Every card's form looks alike, whichever module
  declared it, and a card never draws a form of its own unless its schema
  cannot say what it needs (rule 20).

  The values arrive already held to the schema, so a field always has a value
  of its kind. Every change hands back the whole settings object. A number, a
  line and a note are the library's Field, which ties the label, the control
  and its description together. A pick, a choice, a group of boxes and a
  switch draw their own label, because a field wraps one plain control.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import { Field, Input, Label, Switch, Textarea } from '../../../primitives';
  import { formatNumber, t } from '../../../i18n';
  import {
    type CardSetting,
    type CardSettings,
    keptNumber,
  } from '../../../slots';
  import { bytesOf, MAX_CARD_CONFIG_BYTES } from '../../../domain';
  import { dashboardContext } from '../context';
  import Checks from './checks.svelte';
  import Choice from './choice.svelte';
  import Pick from './pick.svelte';

  let {
    schema,
    config,
    label,
    onchange,
  }: {
    schema: readonly CardSetting[];
    config: CardSettings;
    /** The words of a label: a catalogue key for a core card, text for a module's. */
    label: (text: string) => string;
    onchange: (next: CardSettings) => void;
  } = $props();

  const { printers, cameras } = dashboardContext();
  const uid = $props.id();

  const set = (key: string, value: unknown): void => {
    onchange({ ...config, [key]: value });
  };

  /** The handler that writes one setting. */
  const setter =
    (key: string) =>
    (value: unknown): void => {
      set(key, value);
    };

  const text = (key: string): string => {
    const value = config[key];

    return typeof value === 'string' ? value : '';
  };

  const list = (key: string): string[] => {
    const value = config[key];

    return Array.isArray(value)
      ? value.filter((item): item is string => typeof item === 'string')
      : [];
  };

  const number = (key: string): number => {
    const value = config[key];

    return typeof value === 'number' ? value : 0;
  };

  /** The longest start of a text whose settings the core still keeps. */
  const fitted = (key: string, value: string, most: number): string => {
    const fits = (candidate: string): boolean =>
      bytesOf(candidate) <= most &&
      bytesOf(JSON.stringify({ ...config, [key]: candidate })) <=
        MAX_CARD_CONFIG_BYTES;
    if (fits(value)) return value;
    let low = 0;
    let high = value.length;
    while (low < high) {
      const middle = Math.ceil((low + high) / 2);
      if (fits(value.slice(0, middle))) low = middle;
      else high = middle - 1;
    }

    return value.slice(0, low);
  };

  /**
   * Writes the number a person typed, held to the field's bounds and steps,
   * and shows the kept number in the control, so the field never shows a
   * value the card does not use. An emptied field goes back to the number
   * it held.
   */
  const numberTyped =
    (field: Extract<CardSetting, { kind: 'number' }>) =>
    (event: Event & { currentTarget: HTMLInputElement }): void => {
      const typed = event.currentTarget.valueAsNumber;
      const held = number(field.key);
      const kept = Number.isFinite(typed) ? keptNumber(field, typed) : held;
      event.currentTarget.value = String(kept);
      if (kept !== held) set(field.key, kept);
    };

  /**
   * Writes text a person typed, cut to what the setting keeps. The control
   * shows the cut text too, so a person sees where it stops.
   */
  const textTyped =
    (key: string, most: number) =>
    (
      event: Event & {
        currentTarget: HTMLInputElement | HTMLTextAreaElement;
      },
    ): void => {
      const next = fitted(key, event.currentTarget.value, most);
      if (next !== event.currentTarget.value) event.currentTarget.value = next;
      set(key, next);
    };

  // A card's settings may turn on a camera picture, so the cameras are read
  // again each time the form opens. A camera added since the dashboard last
  // read the list is then offered without a reload of the page.
  onMount(() => {
    cameras.reload();
  });

  const optionsOf = (
    field: Extract<CardSetting, { options: unknown }>,
  ): { value: string; label: string }[] =>
    field.options.map((option) => ({
      value: option.value,
      label: label(option.label),
    }));
</script>

<div class="grid gap-4" data-part="card-settings">
  {#each schema as field (field.key)}
    {#if field.kind === 'printer'}
      <Pick
        label={label(field.label)}
        empty={t('dashboard.settings.printerNone')}
        value={text(field.key)}
        options={printers.printers}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'printers'}
      <Checks
        legend={label(field.label)}
        note={t('dashboard.settings.printersNote')}
        value={list(field.key)}
        options={printers.printers}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'tag'}
      <Pick
        label={label(field.label)}
        empty={t('dashboard.settings.tagNone')}
        note={t('dashboard.settings.tagNote')}
        value={text(field.key)}
        options={printers.tags}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'camera'}
      <Pick
        label={label(field.label)}
        empty={t('dashboard.settings.cameraNone')}
        note={cameras.loaded && !cameras.loading && cameras.items.length === 0
          ? t('dashboard.settings.cameraNoneNote')
          : undefined}
        value={text(field.key)}
        options={cameras.options(printers)}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'choice'}
      <Choice
        label={label(field.label)}
        value={text(field.key)}
        options={optionsOf(field)}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'choices'}
      <Checks
        legend={label(field.label)}
        value={list(field.key)}
        options={optionsOf(field)}
        onchange={setter(field.key)}
      />
    {:else if field.kind === 'number'}
      <Field
        label={label(field.label)}
        control={Input}
        controlProps={{
          type: 'number',
          inputmode: 'numeric',
          min: field.min,
          max: field.max,
          step: field.step,
          value: String(number(field.key)),
          onchange: numberTyped(field),
          onblur: numberTyped(field),
        }}
      />
    {:else if field.kind === 'text'}
      <Field
        label={label(field.label)}
        control={Input}
        controlProps={{
          autocomplete: 'off',
          value: text(field.key),
          oninput: textTyped(field.key, field.max_bytes),
        }}
      />
    {:else if field.kind === 'note'}
      {@const most = field.max_bytes}
      <div class="grid gap-2">
        <Field
          label={label(field.label)}
          control={Textarea}
          controlProps={{
            rows: 6,
            'aria-describedby': `${uid}-${field.key}-count`,
            value: text(field.key),
            oninput: textTyped(field.key, most),
          }}
        />
        <p
          id="{uid}-{field.key}-count"
          class="text-end text-xs text-muted-foreground tabular-nums"
        >
          {t('dashboard.settings.textCount', {
            used: formatNumber(bytesOf(text(field.key))),
            most: formatNumber(most),
          })}
        </p>
      </div>
    {:else}
      <div class="flex items-center justify-between gap-3">
        <Label for="{uid}-{field.key}">{label(field.label)}</Label>
        <Switch
          id="{uid}-{field.key}"
          checked={config[field.key] === true}
          onCheckedChange={setter(field.key)}
        />
      </div>
    {/if}
  {/each}
</div>
