import type { Snippet } from 'svelte';
import type { ClassValue, HTMLAttributes } from 'svelte/elements';

/**
 * The padding step. `list` is 12px by 8px, for a tile in a list of them, and
 * `flush` has none, for a tile whose content draws to the edge.
 */
export type TileSize = 'sm' | 'md' | 'list' | 'flush';

/** What a `child` snippet spreads onto its own press element. */
export interface TilePressProps {
  /** What the frame's `with-press-focus` selectors find. */
  'data-slot': 'tile-press';
  class: string;
  'aria-label': string | undefined;
  'aria-disabled': true | undefined;
}

export interface TileProps extends Omit<
  HTMLAttributes<HTMLDivElement>,
  'class' | 'children' | 'onclick'
> {
  /** The tile is a link. */
  href?: string | undefined;
  /** The tile is a button. */
  onclick?: ((event: MouseEvent) => void) | undefined;
  /** Sets `aria-pressed` on the button and draws the selected look. */
  pressed?: boolean | undefined;
  /** The selected look alone, for a tile picked with a checkbox in `top`. */
  selected?: boolean | undefined;
  disabled?: boolean | undefined;
  /** A dashed destructive border: the thing the tile stands for is switched off. */
  off?: boolean | undefined;
  /** A fainter border and hover, for a tile among many on a busy surface. */
  quiet?: boolean | undefined;
  size?: TileSize | undefined;
  /** The corner. `md` is a card of the file browsers. */
  radius?: 'sm' | 'md' | undefined;
  /** The press element's accessible name, when its content does not say it. */
  label?: string | undefined;
  /** Controls drawn above the press element's hit area: a checkbox, row actions. */
  top?: Snippet | undefined;
  /**
   * A press element of the caller's own, such as a link that navigates on the
   * client. It spreads `props` onto that element and renders the content inside.
   */
  child?: Snippet<[{ props: TilePressProps }]> | undefined;
  /** The content. Its layout, row or column, is the caller's. */
  children?: Snippet | undefined;
  class?: ClassValue | undefined;
}
