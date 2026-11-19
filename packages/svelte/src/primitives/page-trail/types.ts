export interface TrailCrumb {
  label: string;
  /**
   * Where the crumb leads. Leave it out and pass `onSelect` for a screen
   * whose parent is its own earlier state.
   */
  href?: string;
  /**
   * For a detail that is the same screen with its listing put away. Going up
   * then changes state instead of navigating. The crumb is drawn as a button
   * through the breadcrumb link's `child` snippet, so it reads like the crumb
   * beside it.
   */
  onSelect?: () => void;
}

export interface PageTrailProps {
  /**
   * The ancestors, outermost first. The current page is `page` and is never
   * in here.
   */
  trail: TrailCrumb[];
  /**
   * The page a person is on. It closes the trail and is never a link, which
   * is the W3C breadcrumb pattern's own rule and the way a reader can tell
   * where the trail ends.
   */
  page: string;
  /**
   * Sets the page's own crumb in the theme's sans face instead of the face it
   * inherits, for a page named by a file name.
   */
  sansPage?: boolean;
}
