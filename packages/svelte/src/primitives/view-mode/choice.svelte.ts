/**
 * The reader's choice of view, and whether the listing can honour it.
 *
 * `minRem` names, per view, the container width that view needs. It is the
 * same measured number the screen's own container query uses, in the same rem
 * units, because a table's threshold is a question about that table's columns
 * rather than a constant any other screen shares.
 *
 * A choice that no longer fits is remembered and not overwritten: widen the
 * window and the table the reader asked for comes back on its own.
 */
export interface ViewModeFit {
  initial: string;
  /** Shown in place of a chosen view that needs more room. Must itself need none. */
  fallback: string;
  minRem?: Record<string, number>;
}

export class ViewModeState {
  /** What the reader asked for, whether or not it fits. Drives the toggle. */
  chosen = $state('');

  readonly #fallback: string;
  readonly #minRem: Record<string, number>;
  #width = $state<number | null>(null);

  constructor({ initial, fallback, minRem = {} }: ViewModeFit) {
    this.chosen = initial;
    this.#fallback = fallback;
    this.#minRem = minRem;
  }

  /**
   * Goes on the element the screen's own `@container` name sits on, as
   * `{@attach state.measure}`. The observer reports the first size before the
   * frame is painted, so a narrow listing never flashes the table on its way
   * to the rows.
   */
  measure = (node: HTMLElement): (() => void) => {
    const observer = new ResizeObserver(() => {
      this.#width = node.clientWidth;
    });
    observer.observe(node);

    return () => {
      observer.disconnect();
    };
  };

  /** Whether the container is wide enough for this view. */
  roomFor(view: string): boolean {
    const needed = this.#minRem[view];
    if (needed === undefined || this.#width === null) return true;

    // The thresholds are rem-valued, like the container queries they come
    // from, so the root's own font size is what converts them. A literal 16
    // would be wrong under any reader setting that moves it.
    const rem = Number.parseFloat(
      getComputedStyle(document.documentElement).fontSize,
    );

    return this.#width >= needed * (Number.isNaN(rem) ? 16 : rem);
  }

  /** What to render now. */
  get mode(): string {
    return this.roomFor(this.chosen) ? this.chosen : this.#fallback;
  }
}
