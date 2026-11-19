<script lang="ts">
  import type { SettingsRowProps } from './types';

  let {
    label,
    labelHelp,
    description,
    control,
    id,
    layout = 'form',
    class: className,
  }: SettingsRowProps = $props();
</script>

<!-- The label is text and not a `<label>` element: the control in the slot
     owns its own labelling, and a second `<label>` around it would give a
     screen reader two names for one field. -->
<div
  data-slot="settings-row"
  class={[
    'flex min-h-control-height max-w-3xl flex-col gap-form-note @md/pane:flex-row @md/pane:items-center @md/pane:justify-between @lg/pane:grid',
    layout === 'form' && '@lg/pane:grid-cols-settings-form @lg/pane:gap-0',
    layout === 'switch' &&
      '@lg/pane:grid-cols-settings-switch @lg/pane:gap-section-gap',
    className,
  ]}
>
  <div class="flex min-w-0 flex-col gap-0.5">
    <!-- The id sits on the label text alone, so a control named by it does
         not read the help beside the label as part of its name. -->
    <span
      class={[
        'text-body font-normal',
        labelHelp && 'inline-flex items-center gap-1',
      ]}
      ><span id={id ? `${id}-label` : undefined}>{label}</span
      >{#if labelHelp}{@render labelHelp()}{/if}</span
    >
    {#if description}
      <span
        id={id ? `${id}-description` : undefined}
        data-slot="settings-row-description"
        class="text-caption text-muted-foreground"
      >
        {description}
      </span>
    {/if}
  </div>
  {#if control}
    <div class="flex min-w-0 shrink-0 items-center gap-2 break-words">
      {@render control()}
    </div>
  {/if}
</div>
