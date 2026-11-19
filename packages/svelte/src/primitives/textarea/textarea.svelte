<!--
  A box for text over several lines.

  The default box grows with its text. A resizable box uses its rows as the
  starting height and exposes the browser’s vertical resize handle.

  A disabled box greys its text instead of fading the whole element, and
  `pointer-events-none` keeps a click from putting a caret in it. An enabled
  box takes the secondary fill and a disabled one stays transparent.

  It paints nothing for an invalid value. `Field` adds the `aria-invalid:*`
  classes when it has an error to show, so a control wired by hand and a
  control inside a field cannot drift apart.

  Keyboard focus draws no ring: it turns the border `primary`, unless the value
  is invalid, when the border `Field` sets for the error stays.
-->
<script lang="ts">
  import type { TextareaProps } from './types';

  let {
    value = $bindable(),
    resizable = false,
    class: className,
    ...rest
  }: TextareaProps = $props();
</script>

<textarea
  bind:value
  data-slot="textarea"
  class={[
    'flex min-h-textarea-min w-full rounded-sm border border-input bg-transparent px-control-inline py-2 text-base transition-colors outline-none not-aria-invalid:border-secondary placeholder:text-muted-foreground enabled:bg-secondary disabled:pointer-events-none disabled:text-disabled-foreground md:text-sm',
    resizable
      ? 'field-sizing-fixed resize-y'
      : 'field-sizing-content resize-none',
    'focus-visible:ring-ring focus-visible:ring-offset-background',
    'focus-visible:ring-0 focus-visible:ring-offset-0 not-aria-invalid:focus-visible:border-primary',
    className,
  ]}
  {...rest}></textarea>
