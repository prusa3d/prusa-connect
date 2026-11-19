// The shape a dialog with more content than a screen takes.
//
// A panel with no cap grows past the top and the bottom of the page, and the
// footer moves out of view with it: on a phone the button a person came to
// press is off screen and nothing scrolls it back. `guided-dialog.svelte` is
// written against the same rules, and they are named here so a panel outside
// this folder uses them rather than writing them again.

/**
 * The cap on the panel, with the row template that gives the body the space
 * left over. The header and the footer take what they need, and the middle row
 * is the one that shrinks.
 */
export const DIALOG_SCROLLS = 'max-h-dialog-height grid-rows-dialog';

/**
 * The one part that scrolls. `min-h-0` is what lets it shrink inside the grid;
 * without it the row takes its content's height and the cap above does nothing.
 */
export const DIALOG_BODY = 'min-h-0 overflow-y-auto';
