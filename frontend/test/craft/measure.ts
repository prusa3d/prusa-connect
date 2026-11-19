// What a rendered route is made of, counted: how many radii, how many text
// sizes, how many greys, how many borders, how many primary buttons, how many
// icons without a name, how many regions punctuate two ways, and how many
// figures in a table are not tabular.
//
// Earlier research names each of these as a count a test takes over the page. The
// theme decides what the count is allowed to be (`ceilings.ts`), and
// `baseline.json` records what the screens carry today, so a craft task lowers
// a line and nothing raises one by accident.
//
// Everything inside `count()` runs in the page, so it names no import and
// carries no type from this module.
import type { Page } from 'playwright';

/** One route's craft counts. Every field is a number a test can pin. */
export interface CraftCount {
  /** Distinct non-zero border radii drawn on the page. */
  readonly radii: number;
  /** Distinct font sizes on visible text. */
  readonly sizes: number;
  /** Distinct grey text colours. A colour with chroma is not counted here. */
  readonly greys: number;
  /** Distinct colours of a border somebody can actually see. */
  readonly borders: number;
  /** Controls filled with the accent: the thing to press. */
  readonly primaryButtons: number;
  /** Every visible `svg` on the page. */
  readonly icons: number;
  /** Icons that are neither hidden from a screen reader nor given a name. */
  readonly namelessIcons: number;
  /** Regions where some sentences end with a stop and others do not. */
  readonly mixedPunctuation: number;
  /** Table cells holding a figure that is not set in tabular figures. */
  readonly looseFigures: number;
}

/** The order the counts are reported and compared in. */
export const METRICS = [
  'radii',
  'sizes',
  'greys',
  'borders',
  'primaryButtons',
  'icons',
  'namelessIcons',
  'mixedPunctuation',
  'looseFigures',
] as const satisfies readonly (keyof CraftCount)[];

export type Metric = (typeof METRICS)[number];

/** Take every count over the page as it stands. */
export const count = (sheet: Page): Promise<CraftCount> =>
  sheet.evaluate(() => {
    // What the theme resolves a token to, measured rather than parsed: a probe
    // element is painted with the token and the browser hands back the colour
    // it chose. Reading the custom property itself would return the name it
    // points at and not a colour.
    const resolve = (value: string, property: string): string => {
      const probe = document.createElement('div');
      probe.style.position = 'absolute';
      probe.style.opacity = '0';
      probe.style.setProperty(property, value);
      document.body.append(probe);
      const painted = getComputedStyle(probe).getPropertyValue(property);
      probe.remove();

      return painted;
    };

    const visible = (element: Element): boolean => {
      const style = getComputedStyle(element);
      if (style.visibility === 'hidden' || style.display === 'none')
        return false;
      const box = element.getBoundingClientRect();

      return box.width > 0 && box.height > 0;
    };

    /** Whether a painted colour carries no hue a person could name. */
    const isGrey = (colour: string): boolean => {
      const numbers = colour
        .slice(colour.indexOf('(') + 1, colour.lastIndexOf(')'))
        .split(/[\s,/]+/)
        .filter((part) => part !== '')
        .map((part) => Number.parseFloat(part));
      if (colour.startsWith('oklch') || colour.startsWith('oklab')) {
        // oklch carries chroma outright; oklab's two axes are the same thing
        // written as a pair.
        const chroma = colour.startsWith('oklch')
          ? (numbers[1] ?? 0)
          : Math.hypot(numbers[1] ?? 0, numbers[2] ?? 0);

        return chroma < 0.02;
      }
      const channels = numbers.slice(0, 3);

      return Math.max(...channels) - Math.min(...channels) <= 6;
    };

    /**
     * The alpha a painted colour carries, 1 where it writes none. The browser
     * hands back `rgba(0, 0, 0, 0)` for a control with no fill and
     * `oklch(0 0 0 / 0)` for one written in the modern syntax, so both
     * spellings are read: the number after the slash, or the fourth of four
     * separated by commas.
     */
    const alphaOf = (colour: string): number => {
      const inside = colour.slice(
        colour.indexOf('(') + 1,
        colour.lastIndexOf(')'),
      );
      const slash = inside.indexOf('/');
      const written =
        slash === -1
          ? inside.split(',').map((part) => part.trim())[3]
          : inside.slice(slash + 1).trim();
      if (written === undefined || written === '') return 1;
      const number = Number.parseFloat(written);
      if (Number.isNaN(number)) return 1;

      return written.endsWith('%') ? number / 100 : number;
    };

    /** Whether the page draws anything at all in this colour. */
    const painted = (colour: string): boolean =>
      colour !== '' && colour !== 'transparent' && alphaOf(colour) > 0;

    const all = [...document.querySelectorAll('main *, header *, nav *')];

    const radii = new Set<string>();
    const borders = new Set<string>();
    const sizes = new Set<string>();
    const greys = new Set<string>();
    let icons = 0;
    let namelessIcons = 0;
    let primaryButtons = 0;

    const primaryFill = resolve('var(--primary-fill)', 'background-color');

    for (const element of all) {
      if (!visible(element)) continue;
      const style = getComputedStyle(element);

      // A radius the page draws. The four corners are read separately, because
      // a shape rounded on one side is still a radius this theme has to own.
      for (const corner of [
        style.borderTopLeftRadius,
        style.borderTopRightRadius,
        style.borderBottomLeftRadius,
        style.borderBottomRightRadius,
      ]) {
        if (corner !== '' && corner !== '0px') radii.add(corner);
      }

      // A border a person can see: it has width, it is not `none`, and its
      // colour is painted. Each side is read on its own for the same reason.
      for (const side of ['Top', 'Right', 'Bottom', 'Left'] as const) {
        const width = Number.parseFloat(
          style.getPropertyValue(`border-${side.toLowerCase()}-width`),
        );
        const kind = style.getPropertyValue(
          `border-${side.toLowerCase()}-style`,
        );
        const colour = style.getPropertyValue(
          `border-${side.toLowerCase()}-color`,
        );
        if (width > 0 && kind !== 'none' && painted(colour))
          borders.add(colour);
      }

      // The product logo is a wordmark drawn as an svg, not an icon.
      if (
        element.tagName === 'svg' &&
        element.getAttribute('data-slot') !== 'connect-logo'
      ) {
        icons += 1;
        // The library draws every glyph with aria-hidden, which is right for a
        // glyph beside a word and wrong for the only thing in a button: the
        // control is then a button nobody can hear. So the name is looked for
        // on the control the glyph sits in, and a glyph standing on its own has
        // to carry one itself.
        const control = element.closest(
          'button, a, [role="button"], [role="link"], [role="menuitem"], [role="tab"], [role="option"], summary',
        );
        const named = (node: Element): boolean =>
          (node.getAttribute('aria-label') ?? '').trim() !== '' ||
          node.hasAttribute('aria-labelledby') ||
          (node.getAttribute('title') ?? '').trim() !== '' ||
          node.querySelector('title') !== null;

        if (control === null) {
          const hidden = element.closest('[aria-hidden="true"]') !== null;
          if (!hidden && !named(element)) namelessIcons += 1;
        } else if (
          control.textContent.trim() === '' &&
          !named(control) &&
          !named(element)
        ) {
          namelessIcons += 1;
        }
      }

      if (
        painted(style.backgroundColor) &&
        style.backgroundColor === primaryFill
      ) {
        const control = element.closest('button, a, [role="button"]');
        if (control === element) primaryButtons += 1;
      }
    }

    // Text is read off the elements that actually hold a text node, so a
    // wrapper that only inherits a size is not counted as a size of its own.
    const walker = document.createTreeWalker(
      document.body,
      NodeFilter.SHOW_TEXT,
    );
    const regions = new Map<Element, { closed: number; open: number }>();
    const scope = (element: Element): Element =>
      element.closest(
        'section, dialog, form, article, aside, nav, header, footer',
      ) ?? document.body;

    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const text = (node.nodeValue ?? '').trim();
      if (text === '') continue;
      const element = node.parentElement;
      if (element === null) continue;
      if (['SCRIPT', 'STYLE', 'TITLE'].includes(element.tagName)) continue;
      if (!visible(element)) continue;

      const style = getComputedStyle(element);
      sizes.add(style.fontSize);
      if (isGrey(style.color)) greys.add(style.color);

      // A sentence: three words or more, opening with a capital. A label, a
      // name and a reading are none of those, and the period rule does not
      // reach them.
      const sentence = /^[A-Z][^]*\s[^]*\s/.test(text);
      if (sentence) {
        const region = scope(element);
        const tally = regions.get(region) ?? { closed: 0, open: 0 };
        if (/[.?!]$/.test(text)) tally.closed += 1;
        else tally.open += 1;
        regions.set(region, tally);
      }
    }

    let mixedPunctuation = 0;
    for (const tally of regions.values())
      if (tally.closed > 0 && tally.open > 0) mixedPunctuation += 1;

    // A figure in a column lines up only when the face gives every digit the
    // same width. Most rows here are drawn as a grid rather than a table, so
    // the scan is over every element holding text and nothing else, and what
    // counts as a reading is decided by the text: at most six words, and at
    // least three characters in ten a digit. That keeps `99%`, `25.0°C`,
    // `1.0 MB` and a date and leaves out a printer called `MK4S DEMO-05`.
    //
    // A mono face gives every digit the same width already, so an id or a
    // serial set in mono is not counted.
    let looseFigures = 0;
    for (const element of document.querySelectorAll('main *, header *')) {
      if (element.children.length > 0) continue;
      if (!visible(element)) continue;
      const text = element.textContent.trim();
      if (text === '') continue;
      const digits = (text.match(/\d/g) ?? []).length;
      if (digits * 10 < text.length * 3) continue;
      if (text.split(/\s+/).length > 6) continue;
      const style = getComputedStyle(element);
      if (/mono|consolas|menlo|courier/i.test(style.fontFamily)) continue;
      if (!style.fontVariantNumeric.includes('tabular-nums')) looseFigures += 1;
    }

    return {
      radii: radii.size,
      sizes: sizes.size,
      greys: greys.size,
      borders: borders.size,
      primaryButtons,
      icons,
      namelessIcons,
      mixedPunctuation,
      looseFigures,
    };
  });
