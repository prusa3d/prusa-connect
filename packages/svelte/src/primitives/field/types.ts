import type { Component } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import type {
  AriaAttributes,
  ClassValue,
  HTMLAttributes,
} from 'svelte/elements';

/**
 * What a field writes onto the control it wraps. A control that takes these
 * four can be wrapped; every other prop is the control's own.
 */
export interface FieldControlProps {
  id?: string | null | undefined;
  class?: ClassValue | undefined;
  'aria-describedby'?: string | null | undefined;
  'aria-invalid'?: AriaAttributes['aria-invalid'];
  // Svelte declares a data attribute and an attachment on every element, so a
  // control's own props carry these two index signatures. The symbol one is
  // also what lets a caller put an attachment in `controlProps` and reach the
  // control the way React's `ref` does.
  [key: `data-${string}`]: unknown;
  [key: symbol]: Attachment | false | undefined | null;
}

/**
 * A control a field can wrap: any component. A component with props of its
 * own that are required, such as `Combobox`, cannot be typed as accepting only
 * the four a field writes, so the type is `Component<never>` and the field
 * trusts the control to take them. `controlProps` is the unchecked place for
 * everything else.
 *
 * `rovingTabindexRoot` names a composite whose root never takes focus, such as
 * a radio group or a toggle group. A field refuses to wrap one, because the id
 * and the description it writes would land on an element no screen reader ever
 * reaches. The name travels as a value rather than as a flag, because a
 * minified build does not keep the function's own name.
 */
export type FieldControl = Component<never> & {
  rovingTabindexRoot?: string;
};

export interface FieldProps extends Omit<
  HTMLAttributes<HTMLDivElement>,
  'class' | 'children'
> {
  /** The name of the control, drawn above it. */
  label: string;
  /** A sentence under the control that says what a good value looks like. */
  description?: string;
  /** What is wrong with the value. It also paints the control as invalid. */
  error?: string;
  /**
   * `stack` draws the label above the control. `row` sets the label beside
   * the control on a wide settings pane, in a settings row's columns.
   */
  layout?: 'stack' | 'row' | undefined;
  class?: ClassValue | undefined;
  /** The control itself, as a component the field renders. */
  control: FieldControl;
  /**
   * Everything the control needs that the field does not write itself. An
   * `aria-describedby` set here is kept: the description and the error are
   * added to it rather than written over it, and an `id` set here is the id
   * the label points at.
   *
   * The field knows only the four props above, so the rest are unchecked here.
   * A caller that wants its control's own props checked writes the control's
   * props object with that control's `Props` type and passes it in.
   */
  controlProps?: FieldControlProps & Record<string, unknown>;
}
