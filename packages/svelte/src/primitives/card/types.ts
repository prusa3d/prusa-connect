import type { HTMLAttributes } from 'svelte/elements';

// Every part is a div. A card carries no role of its own: a caller that wants
// one names it, as the landmark story does with `role="region"`.

/**
 * `critical` and `attention` tint the border and fill with that severity.
 * `surface` is a fill with no border, for a card standing inside a panel.
 */
export type CardTone = 'default' | 'critical' | 'attention' | 'surface';

/**
 * The padding and gap between parts. `default` steps up with the card's own
 * width, `compact`, `snug` and `tight` stay fixed (`snug` is `compact` with
 * 8px between the parts), and `flush` has none for a card
 * whose content draws to the edge. `panel` is a page's own panel: the card
 * leaves the block padding to its content and insets it by the panel padding.
 */
export type CardDensity =
  'default' | 'compact' | 'snug' | 'tight' | 'flush' | 'panel';

/** The corner. `md` is the panel a screen stands its content in. */
export type CardRadius = 'sm' | 'md';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  tone?: CardTone;
  density?: CardDensity;
  radius?: CardRadius;
}

export type CardHeaderProps = HTMLAttributes<HTMLDivElement>;

export type CardTitleProps = HTMLAttributes<HTMLDivElement>;

export type CardDescriptionProps = HTMLAttributes<HTMLDivElement>;

export type CardActionProps = HTMLAttributes<HTMLDivElement>;

export type CardContentProps = HTMLAttributes<HTMLDivElement>;

export type CardFooterProps = HTMLAttributes<HTMLDivElement>;
