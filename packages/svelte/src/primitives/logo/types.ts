import type { ClassValue } from 'svelte/elements';

/**
 * How big the mark is set. `lg` reaches past the interface's type scale, which
 * stops at `--text-2xl` because no text needs to be bigger. A display wordmark
 * does, and it is derived from that token rather than declared in pixels, so
 * `--density` and the text size setting still reach it.
 */
export type LogoSize = 'sm' | 'md' | 'lg';

/**
 * `two` sets the name on one line, which is the width-matched shape. `three`
 * gives each word its own line for a narrow slot: a phone's brand cell has
 * height to spare and no width. `three` does not width-match, because `Local`
 * alone would have to be set larger than `PRUSA` to span it.
 */
export type LogoLines = 'two' | 'three';

export interface LogoProps {
  size?: LogoSize;
  lines?: LogoLines;
  /**
   * The line under the wordmark. Empty renders the bare `PRUSA` mark, which is
   * what a favicon-sized or very narrow slot takes. A name other than the
   * default keeps the size but loses the width match, which only holds for the
   * string it was measured against.
   */
  subtitle?: string;
  class?: ClassValue | undefined;
}
