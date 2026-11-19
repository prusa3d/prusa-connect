// A dashboard as the page reads it, the rules the core holds it to, and the
// dashboard the page writes from the fleet when nobody has made one.
//
// The rules are the core's (ADR-20261008-sww rules 1 to 6). The page checks them
// before anything is sent, so a person learns about a name that is too long
// in the dialog and not as a refusal at Save. The core checks them again.
import { clampTile, freeCell } from './dashboard-layout';

/** Who sees a dashboard (ADR-20261008-sww rule 8). */
export const DASHBOARD_VISIBILITIES = [
  'everyone',
  'operators',
  'admins',
] as const;
export type DashboardVisibility = (typeof DASHBOARD_VISIBILITIES)[number];

/** How tall one row of a grid is. */
export const DASHBOARD_DENSITIES = [
  'compact',
  'comfortable',
  'spacious',
] as const;
export type DashboardDensity = (typeof DASHBOARD_DENSITIES)[number];

/** The settings of one card: an object its type owns (ADR-20261008-sww rule 3). */
export type CardConfig = Readonly<Record<string, unknown>>;

/** One card on a dashboard's grid. */
export interface DashboardCard {
  readonly id: string;
  /** `<namespace>/<id>`, such as `core/printer` (ADR-20261008-sww rule 17). */
  readonly type: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly title: string;
  readonly config: CardConfig;
}

/** Everything a person changes about a dashboard before saving it. */
export interface DashboardDraft {
  readonly name: string;
  readonly visibility: DashboardVisibility;
  readonly columns: number;
  readonly density: DashboardDensity;
  readonly cards: readonly DashboardCard[];
}

/** A size in cells of a 12-column grid. */
export interface CellSize {
  readonly w: number;
  readonly h: number;
}

export const MAX_DASHBOARD_NAME_BYTES = 80;
export const MAX_CARD_TITLE_BYTES = 120;
export const MAX_CARDS = 64;
export const MAX_CARD_CONFIG_BYTES = 4096;

/** The grid every card size in a declaration is written for. */
export const BASE_COLUMNS = 12;

/** How many bytes a text takes as UTF-8, which is how the core counts. */
export const bytesOf = (value: string): number =>
  new TextEncoder().encode(value).length;

/**
 * What a one-line field may not hold, as the core refuses it: a control
 * character such as a line break, and an invisible formatting character such
 * as the right-to-left override, which would let one name read as another.
 */
const UNSEEN = /[\p{Cc}\p{Cf}\u2028\u2029]/u;

const validLine = (
  value: string,
  limit: number,
  required: boolean,
): boolean => {
  const held = value.trim();

  return (
    (!required || held !== '') && bytesOf(held) <= limit && !UNSEEN.test(held)
  );
};

/** True for a name the core keeps. */
export const validDashboardName = (name: string): boolean =>
  validLine(name, MAX_DASHBOARD_NAME_BYTES, true);

/** True for a heading over a card the core keeps. */
export const validCardTitle = (title: string): boolean =>
  validLine(title, MAX_CARD_TITLE_BYTES, false);

/** True for settings the core keeps: an object no longer than its limit. */
export const fitsCardConfig = (config: CardConfig): boolean =>
  bytesOf(JSON.stringify(config)) <= MAX_CARD_CONFIG_BYTES;

/** A size written for 12 columns, scaled to a grid of any width. */
export const scaledSize = (size: CellSize, columns: number): CellSize => ({
  w: Math.min(
    columns,
    Math.max(1, Math.round((size.w * columns) / BASE_COLUMNS)),
  ),
  h: size.h,
});

/** A card id no other card of the dashboard has. */
export const freshCardId = (taken: readonly DashboardCard[]): string => {
  const used = new Set(taken.map((card) => card.id));
  for (;;) {
    const id = Array.from(crypto.getRandomValues(new Uint8Array(6)), (byte) =>
      (byte % 36).toString(36),
    ).join('');
    if (!used.has(id)) return id;
  }
};

/** A card of one type placed where it first fits, with the settings given. */
export const placeCard = (
  cards: readonly DashboardCard[],
  type: string,
  size: CellSize,
  columns: number,
  config: CardConfig = {},
): DashboardCard => {
  const { w, h } = scaledSize(size, columns);
  const { x, y } = freeCell(cards, w, h, columns);

  return clampTile(
    {
      id: freshCardId(cards),
      type,
      x,
      y,
      w,
      h,
      title: '',
      config: { ...config },
    },
    columns,
  );
};

/** True when two layouts put every card in the same place. */
export const samePlaces = (
  a: readonly DashboardCard[],
  b: readonly DashboardCard[],
): boolean =>
  a.length === b.length &&
  a.every((card, index) => {
    const other = b[index];

    return (
      other?.id === card.id &&
      other.x === card.x &&
      other.y === card.y &&
      other.w === card.w &&
      other.h === card.h
    );
  });

/** The templates a new dashboard can start from. */
export const DASHBOARD_TEMPLATES = [
  'blank',
  'farm',
  'printer',
  'wall',
] as const;
export type DashboardTemplate = (typeof DASHBOARD_TEMPLATES)[number];

/** One card of a template, in cells of a 12-column grid. */
export interface TemplateCard {
  readonly type: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly config?: CardConfig;
}

const TEMPLATE_CARDS: Readonly<
  Record<DashboardTemplate, readonly TemplateCard[]>
> = {
  blank: [],
  farm: [
    { type: 'core/summary', x: 0, y: 0, w: 12, h: 2 },
    {
      type: 'core/printers',
      x: 0,
      y: 2,
      w: 8,
      h: 8,
      config: { order: 'state' },
    },
    { type: 'core/finishing', x: 8, y: 2, w: 4, h: 4 },
    { type: 'core/issues', x: 8, y: 6, w: 4, h: 4 },
  ],
  printer: [
    { type: 'core/printer', x: 0, y: 0, w: 4, h: 6 },
    { type: 'core/camera', x: 4, y: 0, w: 8, h: 6 },
    { type: 'core/temperature', x: 0, y: 6, w: 12, h: 4 },
  ],
  wall: [
    { type: 'core/clock', x: 0, y: 0, w: 3, h: 2 },
    { type: 'core/summary', x: 3, y: 0, w: 9, h: 2 },
    {
      type: 'core/printers',
      x: 0,
      y: 2,
      w: 9,
      h: 8,
      config: { order: 'remaining' },
    },
    { type: 'core/finishing', x: 9, y: 2, w: 3, h: 8, config: { limit: 8 } },
  ],
};

/** The cards of a template, for the picture of it. */
export const templateCards = (
  template: DashboardTemplate,
): readonly TemplateCard[] => TEMPLATE_CARDS[template];

/** The card types that show one printer and are given one from the fleet. */
const ONE_PRINTER = new Set(['core/printer', 'core/temperature']);

/**
 * The dashboard a template writes. A card that shows one printer is given the
 * first printer of the fleet, and a camera card the first camera, so the
 * dashboard draws something at once.
 */
export const draftFromTemplate = (
  template: DashboardTemplate,
  name: string,
  firstPrinter: string | undefined,
  firstCamera: string | undefined,
): DashboardDraft => ({
  name: name.trim(),
  visibility: 'everyone',
  columns: BASE_COLUMNS,
  density: 'comfortable',
  cards: TEMPLATE_CARDS[template].map((spec, index) => {
    const config: Record<string, unknown> = { ...spec.config };
    if (ONE_PRINTER.has(spec.type) && firstPrinter !== undefined) {
      config.device = firstPrinter;
    }
    if (spec.type === 'core/camera' && firstCamera !== undefined) {
      config.camera = firstCamera;
    }

    return {
      id: `t${String(index + 1)}`,
      type: spec.type,
      x: spec.x,
      y: spec.y,
      w: spec.w,
      h: spec.h,
      title: '',
      config,
    };
  }),
});

/**
 * The dashboard the home page draws when the person sees none: a page of the
 * one printer, or a farm of many (ADR-20261008-sww rule 12). It is drawn and never
 * stored until a person saves it.
 */
export const dashboardForFleet = (
  printers: readonly string[],
  cameraOf: (printer: string) => string | undefined,
  name: string,
): DashboardDraft => {
  const [only] = printers;
  if (printers.length === 1 && only !== undefined) {
    return draftFromTemplate('printer', name, only, cameraOf(only));
  }

  return draftFromTemplate('farm', name, undefined, undefined);
};
