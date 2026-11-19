<!--
  A single line of text a person types into.

  The height, the inset and the type size come from the control scale in
  `control-trigger`, the same scale a select and a facet stand on. A search
  field and the facets beside it in a listing toolbar are then one control at
  one height. `size` shadows the native attribute of that name, which counts
  characters and no caller here wants. The md input uses a separate height
  token to match the settings reference.

  It paints nothing for an invalid value. `Field` adds the `aria-invalid:*`
  classes when it has an error to show, so a control wired by hand and a
  control inside a field cannot drift apart.

  Keyboard focus draws no ring: it turns the border `primary`, unless the value
  is invalid, when the border `Field` sets for the error stays.
-->
<script lang="ts">
  import { CONTROL_SIZES } from '../control-trigger/control-trigger';
  import type { InputProps } from './types';

  let { size = 'md', class: className, ...rest }: InputProps = $props();
</script>

<input
  data-slot="input"
  data-size={size}
  class={[
    'w-full min-w-0 rounded-sm border border-input bg-transparent transition-colors outline-none not-aria-invalid:border-secondary selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground enabled:bg-secondary disabled:pointer-events-none disabled:text-disabled-foreground dark:not-aria-invalid:not-focus-visible:hover:border-input',
    '@max-3xl/app-shell:min-h-touch-target pointer-coarse:min-h-touch-target',
    CONTROL_SIZES[size],
    size === 'md' && 'h-input-height',
    'focus-visible:ring-ring focus-visible:ring-offset-background',
    'focus-visible:ring-0 focus-visible:ring-offset-0 not-aria-invalid:focus-visible:border-primary',
    className,
  ]}
  {...rest}
/>

<style>
  /* A number field draws no native spinner. The browser paints it inside the
     field's own inset, over the last digits of a narrow field, and the arrow
     keys still step the value. Firefox hides it only through `textfield`. */
  input[type='number'] {
    appearance: textfield;
  }

  input[type='number']::-webkit-inner-spin-button,
  input[type='number']::-webkit-outer-spin-button {
    appearance: none;
    margin: 0;
  }
</style>
