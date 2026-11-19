import type { Component, Snippet } from 'svelte';
import type { ClassValue } from 'svelte/elements';

export interface DetailTab {
  id: string;
  label: string;
  /** A decorative icon beside the label in a segmented strip. */
  icon?: Component | undefined;
  /** A number drawn in a badge after the label, such as the jobs running. */
  count?: number | undefined;
  /**
   * What the tab shows. Absent draws the strip's `fallback`, so a screen ships
   * its frame before its tabs and is never broken in between.
   */
  panel?: Snippet | undefined;
}

export interface DetailTabsProps {
  tabs: readonly DetailTab[];
  /**
   * Drawn for a tab with no panel. Left out, the strip draws a skeleton, which
   * is right while a panel is on its way and wrong where this build draws
   * none: a skeleton that never resolves reads as a broken page.
   */
  fallback?: Snippet | undefined;
  /** The open tab. Which one that is belongs to the screen, not to the part. */
  active: string;
  onChange: (id: string) => void;
  /** Equal-width tabs that wrap inside the strip. */
  segmented?: boolean | undefined;
  class?: ClassValue | undefined;
}
