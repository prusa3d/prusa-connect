import type { Slider as SliderPrimitive } from 'bits-ui';

/**
 * One thumb or several. `type` is required: the headless layer decides the
 * shape of `value` from it, and will not read a lone number as a one thumb
 * slider on its own.
 */
export type SliderProps = SliderPrimitive.RootProps & {
  /**
   * The colour of the filled range and the thumb's edge. `default` is the
   * blue of a control's on state, for a value that only changes what the page
   * shows. `primary` is the orange of the primary button, for a value that
   * changes the machine, such as a print speed.
   */
  tone?: SliderTone;
};

export type SliderTone = 'default' | 'primary';
