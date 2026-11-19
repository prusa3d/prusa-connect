<!--
  A label, one control, a description and an error, wired together.

  How the field reaches its control. React clones the element it was handed and
  writes an id, an `aria-describedby` and the invalid classes onto the clone.
  Svelte has no `cloneElement`, and a parent cannot reach into markup a caller
  wrote to add an attribute to it, so a snippet would leave the field asking the
  caller to spread the wiring and hoping they did. The control arrives as a
  component in `control` with its own props in `controlProps`, and the field
  renders it. That is what gives the field the three powers React's clone has:
  the wiring cannot be forgotten, the `aria-describedby` the caller already set
  is merged rather than dropped, and `rovingTabindexRoot` can be read off the
  control to refuse a composite the field cannot wire. A field exists only to
  guarantee that wiring, so a shape that makes it optional is not a field.

  What that costs. There is no `bind:` through the field and no children inside
  the control. Pass a value and a handler in `controlProps`, the way the React
  call sites do. An attachment passes through `controlProps` as well, which is
  how a caller focuses the control the way React's `ref` does.

  The invalid classes go on the control and not on this box, because the red
  edge belongs to the thing that holds the wrong value. The `aria-invalid`
  selector outscores the control's resting `border-input`, so the paint lands
  without `!important`.
-->
<script lang="ts">
  import type { Component } from 'svelte';

  import Label from '../label/label.svelte';
  import type { FieldControlProps, FieldProps } from './types';

  let {
    label,
    description,
    error,
    control: Control,
    controlProps,
    layout = 'stack',
    class: className,
    ...rest
  }: FieldProps = $props();

  // Tab moves focus straight to the checked item of a roving-tabindex
  // composite, never to the composite's own root, so the id and the
  // description below would sit on a node no assistive technology lands on.
  // Refusing is louder than mis-wiring it. The control is read once, when the
  // field is created, which is when React throws the same refusal.
  // svelte-ignore state_referenced_locally
  if (Control.rovingTabindexRoot !== undefined) {
    throw new Error(
      `Field cannot wrap ${Control.rovingTabindexRoot}. Its root never takes keyboard focus, so the id, the description and the invalid state a field writes would land on a node no assistive technology reaches. Give ${Control.rovingTabindexRoot} a fieldset and a legend, or an id its own items point at.`,
    );
  }

  // `FieldControl` is `Component<never>` so any control fits; the field writes
  // only the four props `FieldControlProps` names, which a control accepts.
  const Wrapped = $derived(Control as Component<FieldControlProps>);

  const invalidControl =
    'aria-invalid:border-destructive aria-invalid:focus-visible:ring-destructive';

  const id = $props.id();
  // A caller that named its control keeps that name, so an id it points at
  // from elsewhere on the page still reaches the control.
  const controlId = $derived(controlProps?.id ?? `${id}-control`);
  const descriptionId = `${id}-description`;
  const errorId = `${id}-error`;

  // What the control is described by already comes first, so a hint the caller
  // wired stays and the field's own two are added to it.
  const describes = $derived(
    [
      controlProps?.['aria-describedby'] ?? undefined,
      description ? descriptionId : undefined,
      error ? errorId : undefined,
    ]
      .filter((name) => name !== undefined)
      .join(' '),
  );
</script>

<div
  data-slot="field"
  class={[
    layout === 'stack' && 'flex flex-col gap-2',
    layout === 'row' &&
      'grid gap-form-note @lg/pane:grid-cols-settings-form @lg/pane:items-center @lg/pane:gap-0',
    className,
  ]}
  {...rest}
>
  <Label for={controlId}>{label}</Label>
  <Wrapped
    {...controlProps}
    id={controlId}
    aria-describedby={describes === '' ? undefined : describes}
    aria-invalid={error ? true : undefined}
    class={[controlProps?.class, error && invalidControl]}
  />
  {#if description}
    <p id={descriptionId} class="col-span-full text-sm text-muted-foreground">
      {description}
    </p>
  {/if}
  {#if error}
    <!-- eslint-disable no-restricted-syntax -- the field announces its own error; tests query it by role -->
    <p
      id={errorId}
      role="alert"
      class="col-span-full text-sm text-destructive-text"
    >
      {error}
    </p>
    <!-- eslint-enable no-restricted-syntax -->
  {/if}
</div>
