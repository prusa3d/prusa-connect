import type { Snippet } from 'svelte';
import type { ClassValue, HTMLButtonAttributes } from 'svelte/elements';

/**
 * The five looks. `default` is the brand fill, `secondary` the quiet fill,
 * `ghost` a button with no edge at rest, and `destructive` an outline that
 * fills only when it is touched. `link` is text with no box that underlines on
 * hover, and it inherits the surrounding font size.
 */
export type ButtonVariant =
  'default' | 'secondary' | 'ghost' | 'destructive' | 'link';

/** Text sizes. Compact gives more room than the small size. */
export type ButtonTextSize =
  'sm' | 'control' | 'compact' | 'md' | 'field' | 'lg';

/** The corner. `full` is a round button, such as an avatar that is pressed. */
export type ButtonRadius = 'sm' | 'full';

/**
 * A square size for each text size. A square button holds one glyph, or a
 * person's picture at `avatar`.
 */
export type ButtonIconSize = 'icon-sm' | 'icon-md' | 'icon-lg' | 'avatar';

/**
 * The size of a `link` button that carries an icon: it lays out as a flex row
 * with a gap, and wraps its text.
 */
export type ButtonLinkSize = 'inline-icon';

export type ButtonSize = ButtonTextSize | ButtonIconSize | ButtonLinkSize;

interface ButtonBaseProps extends Omit<
  HTMLButtonAttributes,
  'class' | 'children'
> {
  variant?: ButtonVariant;
  radius?: ButtonRadius;
  class?: ClassValue | undefined;
  children?: Snippet;
  /**
   * The act the button starts is under way. A spinner leads the content, the
   * button is `aria-busy` and cannot be pressed. Not drawn by a `child`.
   */
  pending?: boolean | undefined;
  /**
   * Draw the button as another element, an anchor or a menu item. The snippet
   * receives the attributes the `<button>` would have had and puts them where
   * it wants them. Nothing is merged for it: an attribute the snippet does not
   * spread is lost.
   */
  child?: Snippet<[{ props: Record<string, unknown> }]>;
}

/**
 * A square button shows no text, so it requires `label`. The label is the
 * button's accessible name and the text of the tooltip it shows on hover and
 * on keyboard focus. A text button has its name in its text and takes no
 * label.
 */
export type ButtonProps = ButtonBaseProps &
  (
    | { size?: ButtonTextSize | ButtonLinkSize; label?: never }
    | { size: ButtonIconSize; label: string }
  );
