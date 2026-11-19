import type { Snippet } from 'svelte';
import type { ClassValue, HTMLAttributes } from 'svelte/elements';

/**
 * One value beside the row's name. A word, or a snippet for a badge or a
 * link.
 *
 * `false`, `null` and `undefined` are dropped, so a caller writes
 * `file.status && statusBadge` and the row closes the gap.
 */
export type DataRowValue = string | Snippet | false | null | undefined;

export interface DataRowProps extends Omit<
  HTMLAttributes<HTMLDivElement>,
  'class' | 'children'
> {
  /** The row's own value: a file name, a job name. Truncates on overflow. */
  primary: string;
  /**
   * Draws `primary` in the value face. For a string a reader compares
   * character by character, such as a file name, a path or an address. Never
   * for a name a person chose.
   */
  mono?: boolean;
  /**
   * Makes the whole row activatable and renders `primary` as a link.
   * Mutually exclusive with `onActivate`: a row either navigates or activates
   * in place, never both.
   */
  href?: string | undefined;
  /**
   * Makes the whole row activatable without navigating, such as a folder's
   * inline expand or a file's selection toggle. Renders `primary` as a
   * `<button>` rather than the `<a>` `href` renders. Nothing here is a route,
   * so nothing here carries link semantics: a screen reader announcing
   * "link", a browser's own "open in new tab".
   */
  onActivate?: () => void;
  /**
   * Marks the `onActivate` button as a disclosure, such as a folder row
   * toggling inline expansion, and carries its current state. Leave it out
   * for an `onActivate` that does something else, such as a selection toggle:
   * a control that is not a disclosure must not claim `aria-expanded`, and
   * leaving this out renders no such attribute at all.
   */
  activateExpanded?: boolean;
  /**
   * Leading-edge content: a disclosure button, a type glyph, a thumbnail. It
   * sits above the `href` or `onActivate` overlay, so a button placed here
   * stays clickable.
   */
  leading?: Snippet;
  /**
   * Priority-ordered, highest first. The first two always render. A third
   * onward renders only once the row's own width has room, or when
   * `wrapValues` is set.
   */
  secondaryValues?: DataRowValue[];
  /**
   * Lets the secondary values wrap onto further lines instead of dropping all
   * but the first two once the row is under 40rem. For a row whose later
   * values matter as much as its first, such as a job's start time and
   * duration, where hiding them leaves a row that says only what and where,
   * never when.
   */
  wrapValues?: boolean;
  /** 16px of block padding in place of 8px, for a row led by a print preview. */
  roomy?: boolean;
  /** Trailing-edge content, typically an actions menu trigger. */
  actions?: Snippet;
  class?: ClassValue | undefined;
}
