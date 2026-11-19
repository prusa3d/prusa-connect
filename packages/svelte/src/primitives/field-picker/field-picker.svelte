<!--
  Which of a listing's fields it shows, as the reader's choice and not only
  the screen's.

  A jobs table carries eight columns because eight facts matter to someone. A
  farm watching throughput reads Started and Duration and never looks at
  Progress; a person chasing one failure reads Printer and State and wants the
  rest out of the way. Rather than pick for them, the listing states a
  sensible default and lets the reader put a field away.

  What may never be hidden: the field the row is identified by, and the
  controls that act on it. Hiding those leaves rows nobody can tell apart or
  act on. A caller marks them `required`, and they are offered ticked and
  unpickable rather than dropped from the list, because a reader looking for
  File should find it and see why it cannot go.

  The trigger is an icon at the view toggle's own size and not a select. This
  control belongs beside the view switch, since both answer how the list is
  drawn, and as a 120px labelled box it out-weighed the filters it stood next
  to. The count rides the glyph as a superscript rather than as words: a
  reader changes this to get rid of something, and how much is put away is the
  only thing they need back at a glance.

  Nothing is remembered between mounts. Where this belongs once the product
  has a real preferences store is beside the theme and the page size, keyed
  per listing.
-->
<script lang="ts">
  import SlidersHorizontalIcon from '@lucide/svelte/icons/sliders-horizontal';

  import { formatNumber, t } from '../../i18n';
  import type { ButtonIconSize } from '../button/types';
  import Button from '../button/button.svelte';
  import FacetRow from '../facet-row/facet-row.svelte';
  import Popover from '../popover/popover.svelte';
  import PopoverContent from '../popover/popover-content.svelte';
  import PopoverTrigger from '../popover/popover-trigger.svelte';
  import type { FieldPickerProps } from './types';

  // `state` is renamed, because a top-level binding of that name in a
  // component shadows the runtime helper `$state` compiles to.
  let {
    fields,
    state: fieldState,
    label,
    size = 'sm',
    class: className,
  }: FieldPickerProps = $props();

  const triggerSize = $derived<ButtonIconSize>(`icon-${size}`);

  const shows = (id: string): boolean => fieldState.visible.includes(id);

  // A required field is ticked whatever the listing holds, so it never counts
  // as put away.
  const hiddenCount = $derived(
    fields.filter((field) => !shows(field.id) && field.required !== true)
      .length,
  );

  const defaults = $derived(
    fields
      .filter((field) => field.required === true || !field.hiddenByDefault)
      .map((field) => field.id),
  );

  // What decides whether there is a way back to offer. Only the fields
  // offered here count: a listing may hold an id it does not offer.
  const isDefault = $derived(
    fields.every((field) => shows(field.id) === defaults.includes(field.id)),
  );

  const name = $derived(label ?? t('primitives.fieldPicker.label'));
  const triggerName = $derived(
    hiddenCount === 0
      ? name
      : t('primitives.fieldPicker.hidden', {
          label: name,
          count: hiddenCount,
        }),
  );
</script>

<Popover>
  <PopoverTrigger>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="secondary"
        size={triggerSize}
        label={triggerName}
        class={[
          'relative h-input-height w-input-height p-0 [&_svg]:size-icon-min',
          className,
        ]}
      >
        <SlidersHorizontalIcon aria-hidden="true" />
        {#if hiddenCount > 0}
          <span
            aria-hidden="true"
            class="absolute -top-1 -right-1 grid size-4 place-items-center rounded-full bg-primary font-mono text-xs leading-none text-primary-foreground tabular-nums"
          >
            {formatNumber(hiddenCount)}
          </span>
        {/if}
      </Button>
    {/snippet}
  </PopoverTrigger>
  <!-- The panel is named for a reader who reaches it by another route: what
       opened is the list of fields this listing can draw. -->
  <PopoverContent
    aria-label={t('primitives.fieldPicker.options', { label: name })}
    align="end"
    sideOffset={6}
    collisionPadding={12}
    class="w-60 p-2"
  >
    {#each fields as field (field.id)}
      <FacetRow
        checked={shows(field.id)}
        disabled={field.required}
        hint={field.required === true
          ? t('primitives.fieldPicker.alwaysShown')
          : undefined}
        onToggle={() => {
          fieldState.onToggle(field.id);
        }}
      >
        {#snippet label()}{field.label}{/snippet}
      </FacetRow>
    {/each}
    {#if !isDefault}
      <button
        type="button"
        onclick={() => {
          fieldState.onReset();
        }}
        class="mt-1 w-full cursor-pointer rounded-md border-t border-border px-1.5 py-2 text-left text-xs outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        {t('primitives.fieldPicker.reset')}
      </button>
    {/if}
  </PopoverContent>
</Popover>
