<script lang="ts">
  // The shape every settings tab uses: a heading, one line of description, the
  // rows, and a band at the foot holding what saves them (earlier research
  // recommendation 6).
  //
  // It is painted rather than outlined, and it takes the
  // card surface, because a screen that mixes panes and cards reads as one
  // surface at one height.
  import type { SettingsPaneProps } from './types';

  let {
    title,
    description,
    children,
    actions,
    headerEnd,
    message,
    messageIsFailure = false,
    edged = false,
    class: className,
  }: SettingsPaneProps = $props();

  // The heading names the section, so the section points at it and a screen
  // reader announces the pane by its own title.
  const headingId = $props.id();
</script>

<section
  data-slot="settings-pane"
  aria-labelledby={headingId}
  class={[
    'rounded-panel @container/pane relative flex flex-col overflow-clip rounded-md bg-card text-card-foreground shadow-settings-pane dark:shadow-none',
    edged && 'border border-border not-dark:border-panel-outline',
    className,
  ]}
>
  <div
    data-slot="settings-pane-header"
    class="flex min-h-settings-header flex-wrap items-center justify-between gap-x-4 gap-y-1 rounded-t-md bg-secondary px-settings-header-inline py-settings-header-inset"
  >
    <h2 id={headingId} class="text-lead font-normal">
      {title}
    </h2>
    {@render headerEnd?.()}
  </div>

  <div data-slot="settings-pane-body" class="flex flex-col px-0 pb-0">
    {#if description}
      <!-- The pane spans the page, so the text keeps a settings row's
           measure, set on its content box so both end at one edge. -->
      <p
        class="text-body box-content max-w-3xl px-panel-padding-mobile pt-section-gap text-muted-foreground @lg/pane:px-panel-padding"
      >
        {description}
      </p>
    {/if}
    {@render children?.()}
  </div>

  {#if message}
    <p
      data-slot="settings-pane-message"
      role={messageIsFailure ? 'alert' : 'status'}
      class={[
        'text-body px-panel-padding-mobile pb-section-gap @lg/pane:px-panel-padding',
        messageIsFailure ? 'text-destructive-text' : 'text-muted-foreground',
      ]}
    >
      {message}
    </p>
  {/if}

  {#if actions}
    <!-- The band sits back from the pane by a tone of its own, so the rule
         over it is the seam between two different paints and not a line drawn
         for its own sake. -->
    <div
      data-slot="settings-pane-actions"
      class="bg-tone-sunken text-tone-sunken-foreground flex flex-col-reverse gap-2 border-t border-border p-panel-padding-mobile @xs/pane:flex-row @xs/pane:items-center @xs/pane:justify-end @lg/pane:p-panel-padding"
    >
      {@render actions()}
    </div>
  {/if}
</section>
