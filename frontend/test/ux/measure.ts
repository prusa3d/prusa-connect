// What a real Chromium reports about a page: how far it moved, what a press
// changed outside the control that was pressed, and every text pair's
// contrast. Earlier research measured all three by hand; this file is the same
// measure, run by a test.
//
// Everything in `install()` runs inside the page, so it names no import and
// carries no type from this module.
import type { Browser, BrowserContext, Page } from 'playwright';
import { chromium } from 'playwright';

/** The size earlier research measured at, so the numbers are comparable. */
export const VIEWPORT = { width: 1280, height: 900 };

/**
 * The time every page reads from `Date`: when `fixtures/api.json` was last
 * recorded. Every date in the fixture lies before it. A screen that compares
 * a fixture date with the time, such as the badge on a credential that expires
 * within a fortnight, then draws the same thing on every day the lane runs.
 * Timers keep running, so a screen still settles.
 */
const RECORDED = new Date('2026-11-19T15:20:00Z');

export interface Pair {
  readonly color: string;
  readonly background: string;
  readonly ratio: number;
  readonly size: number;
  readonly bold: boolean;
  readonly sample: string;
  readonly where: string;
}

export interface Scope {
  /** Nodes added outside the region, by what they are. */
  readonly added: number;
  readonly removed: number;
  /** The first few, so a failure names what moved. */
  readonly examples: readonly string[];
}

/** What the page keeps under `__ux`, read back by the helpers below. */
interface Probe {
  cls: number;
  press: number;
  moved: string[];
  scope: { added: number; removed: number; examples: string[] };
  watching: MutationObserver | null;
  /** Requests to the core still waiting on an answer. */
  pending: number;
  /** When the document, the layout or a request last changed. */
  changed: number;
}

/** One shift, as the browser reports it. The DOM types do not declare it. */
interface Shift {
  value: number;
  hadRecentInput: boolean;
  sources?: readonly {
    node?: Node | null;
    previousRect: DOMRectReadOnly;
    currentRect: DOMRectReadOnly;
  }[];
}

// The probes the page keeps for the whole of its life. `buffered` is what
// makes the first shift countable: the observer is installed before the
// document runs, and the browser still replays what it recorded before.
//
// It is exported because a lane that signs in keeps one browser context for
// every page it opens, so it installs the probes itself instead of calling
// `open` below.
export const install = (): void => {
  const ux: Probe = {
    cls: 0,
    press: 0,
    moved: [],
    scope: { added: 0, removed: 0, examples: [] },
    watching: null,
    pending: 0,
    changed: performance.now(),
  };
  (globalThis as unknown as { __ux: Probe }).__ux = ux;

  // What `open` waits on. A screen has settled when nothing it asked the core
  // for is still on the way and nothing on it has changed for a while. The
  // event stream and the camera frames never stop, so they are not counted:
  // the one is a stream that is never answered, the other a feed.
  const touched = (): void => {
    ux.changed = performance.now();
  };
  const ongoing = (input: RequestInfo | URL): boolean => {
    const href =
      input instanceof Request ? input.url : new URL(input, location.href).href;
    const path = new URL(href).pathname;

    return path === '/api/v1/events' || path.endsWith('/snapshot');
  };
  const sent = globalThis.fetch.bind(globalThis);
  globalThis.fetch = async (input, init) => {
    if (ongoing(input)) return sent(input, init);
    ux.pending += 1;
    touched();
    try {
      return await sent(input, init);
    } finally {
      ux.pending -= 1;
      touched();
    }
  };
  new MutationObserver(touched).observe(document, {
    subtree: true,
    childList: true,
    attributes: true,
    characterData: true,
  });

  // What the browser names as the source of a shift, so a failure says which
  // element moved and by how far rather than only that the page moved.
  const named = (shift: Shift): void => {
    for (const source of shift.sources ?? []) {
      const node = source.node;
      const tag =
        node instanceof Element
          ? `${node.tagName.toLowerCase()}.${node.className.split(' ').slice(0, 3).join('.')}`
          : 'unknown';
      // Where it moved: on the page, or on a surface over it. A press that
      // opens a dialog may move the dialog and must not move the page.
      const page = document.querySelector('main');
      const side =
        node !== null && node !== undefined && page?.contains(node) === true
          ? 'main'
          : 'over';
      const from = Math.round(source.previousRect.top);
      const to = Math.round(source.currentRect.top);
      ux.moved.push(`${side} ${tag} ${String(from)} to ${String(to)}`);
    }
  };

  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      const shift = entry as PerformanceEntry & Shift;
      ux.press += shift.value;
      if (!shift.hadRecentInput) ux.cls += shift.value;
      touched();
      named(shift);
    }
  }).observe({ type: 'layout-shift', buffered: true });
};

export interface UxPage {
  readonly page: Page;
  close(): Promise<void>;
}

export interface OpenOptions {
  /** The theme the page paints in. Dark is the stored preference plus the machine's. */
  readonly dark?: boolean;
  /** The longest the screen is given to settle before a measure is read. */
  readonly settle?: number;
  /** The window's width, where a measure is about a phone or a tablet. */
  readonly width?: number;
  /** The window's height, where a measure is about what fits on one screen. */
  readonly height?: number;
  /** Runs before the page opens, such as to change what the core answers. */
  readonly prepare?: (sheet: Page) => Promise<void>;
}

export const launch = (): Promise<Browser> => chromium.launch();

/** How long nothing may change before a screen counts as settled. */
const QUIET_MS = 750;

/**
 * Waits until the screen has settled, and no longer than `most`. A screen that
 * keeps changing past it, a clock that ticks or a list that keeps loading, is
 * measured where it stands then, as it was when every screen waited the whole
 * of that time.
 */
export const settled = async (sheet: Page, most: number): Promise<void> => {
  try {
    await sheet.waitForFunction(
      (quiet) => {
        const ux = (globalThis as unknown as { __ux: Probe }).__ux;

        return ux.pending === 0 && performance.now() - ux.changed >= quiet;
      },
      QUIET_MS,
      { timeout: most, polling: 100 },
    );
  } catch {
    // Still changing at `most`: measured as it stands.
  }
};

/** A page with the probes installed, at a route, settled. */
export const open = async (
  browser: Browser,
  url: string,
  path: string,
  options: OpenOptions = {},
): Promise<UxPage> => {
  const context: BrowserContext = await browser.newContext({
    viewport: {
      width: options.width ?? VIEWPORT.width,
      height: options.height ?? VIEWPORT.height,
    },
    colorScheme: options.dark === true ? 'dark' : 'light',
    // A frame every two seconds is the demo's cadence, and a browser that
    // throttles a background tab would report it as four.
    reducedMotion: 'no-preference',
  });
  await context.clock.setFixedTime(RECORDED);

  const sheet = await context.newPage();
  await sheet.addInitScript(install);
  if (options.dark === true) {
    await sheet.addInitScript(() => {
      // The script runs on every navigation, `about:blank` included, and a
      // page with no origin refuses to store anything.
      try {
        globalThis.localStorage.setItem('theme', 'dark');
      } catch {
        /* no origin yet */
      }
    });
  }

  await options.prepare?.(sheet);
  await sheet.goto(`${url}${path}`, { waitUntil: 'domcontentloaded' });
  // The page the application drew, and then the time it takes to fill. A
  // measure taken before anything is drawn measures nothing. It waits on
  // `main` rather than a section inside it, because merge 41a1efb7 removed the
  // screens that drew those sections.
  await sheet.waitForSelector('main', { timeout: 30_000 });
  await settled(sheet, options.settle ?? 4000);

  return { page: sheet, close: () => context.close() };
};

/** Layout shift from the first paint to now, the way Core Web Vitals counts it. */
export const cls = (sheet: Page): Promise<number> =>
  sheet.evaluate(() => (globalThis as unknown as { __ux: Probe }).__ux.cls);

/**
 * The shift an action caused, with the browser's 500 ms excuse for anything
 * after a press removed. A person sees those too.
 */
export const shiftAfter = async (
  sheet: Page,
  action: () => Promise<void>,
): Promise<number> => {
  await sheet.evaluate(() => {
    const ux = (globalThis as unknown as { __ux: Probe }).__ux;
    ux.press = 0;
    ux.moved = [];
  });
  await action();
  await sheet.waitForTimeout(900);

  return sheet.evaluate(
    () => (globalThis as unknown as { __ux: Probe }).__ux.press,
  );
};

/** Whether the page is painting in the dark theme. */
export const isDark = (sheet: Page): Promise<boolean> =>
  sheet.evaluate(() => document.documentElement.classList.contains('dark'));

/** How many nodes sit directly under `body` right now, comments and text included. */
export const bodyNodes = (sheet: Page): Promise<number> =>
  sheet.evaluate(() => document.body.childNodes.length);

/**
 * What an action added or removed inside the app's own tree and outside
 * `region`, which is the screen region the pressed control belongs to.
 *
 * What a floating surface does at the end of `body` is not counted here: a
 * menu, a listbox and a dialog belong there and nowhere else, and what they
 * leave behind once closed is what `bodyNodes` measures instead.
 */
export const changesOutside = async (
  sheet: Page,
  region: string,
  action: () => Promise<void>,
): Promise<Scope> => {
  await sheet.evaluate((selector) => {
    const ux = (globalThis as unknown as { __ux: Probe }).__ux;
    ux.scope = { added: 0, removed: 0, examples: [] };
    const inside = document.querySelector(selector);

    // The app's own tree: the shell around the page. Everything else directly
    // under `body` is where a floating surface is drawn.
    const shell = document.querySelector('main')?.parentElement ?? null;

    const name = (node: Node): string => {
      const value = node.nodeValue ?? '';
      if (node instanceof Element) {
        const classes = node.className.slice(0, 40);

        return `<${node.tagName.toLowerCase()} class="${classes}">`;
      }

      return node.nodeType === Node.COMMENT_NODE
        ? `comment(${value.slice(0, 20)})`
        : `text(${value.trim().slice(0, 20)})`;
    };

    const watcher = new MutationObserver((records) => {
      for (const record of records) {
        if (shell !== null && !shell.contains(record.target)) continue;
        if (inside?.contains(record.target) === true) continue;

        for (const node of record.addedNodes) {
          ux.scope.added += 1;
          if (ux.scope.examples.length < 6)
            ux.scope.examples.push(`+ ${name(node)}`);
        }
        for (const node of record.removedNodes) {
          ux.scope.removed += 1;
          if (ux.scope.examples.length < 6)
            ux.scope.examples.push(`- ${name(node)}`);
        }
      }
    });
    watcher.observe(document.body, { childList: true, subtree: true });
    ux.watching = watcher;
  }, region);

  await action();
  await sheet.waitForTimeout(600);

  return sheet.evaluate(() => {
    const ux = (globalThis as unknown as { __ux: Probe }).__ux;
    ux.watching?.disconnect();
    ux.watching = null;

    return ux.scope;
  });
};

/**
 * Every visible text node on the page, its colour against the first opaque
 * background above it, and the ratio between the two. The colour lane reads
 * the same list. The text of a disabled control is left out, which WCAG
 * 1.4.3 exempts.
 */
export const contrastPairs = (sheet: Page): Promise<Pair[]> =>
  sheet.evaluate(() => {
    // Every colour in this theme is written in oklch and computes to oklch, so
    // the sweep converts it itself. The browser has no API that hands a
    // computed colour back as sRGB: `getComputedStyle` keeps the colour space
    // it was written in, and a canvas hands back what it was given.
    // The numbers inside the brackets, in either the comma form or the space
    // form. A percentage is read as its fraction, which is what every slot
    // this theme writes means by one.
    const numbers = (value: string): number[] =>
      value
        .slice(value.indexOf('(') + 1, value.lastIndexOf(')'))
        .split(/[\s,/]+/)
        .filter((part) => part !== '')
        .map((part) =>
          part.endsWith('%')
            ? Number.parseFloat(part) / 100
            : Number.parseFloat(part),
        );

    const encode = (channel: number): number => {
      const held =
        channel <= 0.0031308
          ? 12.92 * channel
          : 1.055 * channel ** (1 / 2.4) - 0.055;

      return Math.min(255, Math.max(0, Math.round(held * 255)));
    };

    // Oklab to sRGB, the conversion in the CSS Color 4 specification.
    const fromOklab = (
      lightness: number,
      greenRed: number,
      blueYellow: number,
    ): [number, number, number] => {
      const long =
        (lightness + 0.3963377774 * greenRed + 0.2158037573 * blueYellow) ** 3;
      const medium =
        (lightness - 0.1055613458 * greenRed - 0.0638541728 * blueYellow) ** 3;
      const short =
        (lightness - 0.0894841775 * greenRed - 1.291485548 * blueYellow) ** 3;

      return [
        encode(
          4.0767416621 * long - 3.3077115913 * medium + 0.2309699292 * short,
        ),
        encode(
          -1.2684380046 * long + 2.6097574011 * medium - 0.3413193965 * short,
        ),
        encode(
          -0.0041960863 * long - 0.7034186147 * medium + 1.707614701 * short,
        ),
      ];
    };

    const rgba = (value: string): [number, number, number, number] => {
      const text = value.trim();
      if (text === '' || text === 'transparent') return [0, 0, 0, 0];

      if (text.startsWith('#')) {
        const hex = text.slice(1);
        const wide = hex.length >= 6;
        const at = (index: number): number => {
          const digit = hex[index] ?? '0';

          return wide
            ? parseInt(hex.slice(index * 2, index * 2 + 2), 16)
            : parseInt(`${digit}${digit}`, 16);
        };

        return [at(0), at(1), at(2), 1];
      }

      const parts = numbers(text);
      const alpha = (index: number): number => parts[index] ?? 1;

      if (text.startsWith('oklch')) {
        const chroma = parts[1] ?? 0;
        const hue = ((parts[2] ?? 0) * Math.PI) / 180;
        const [red, green, blue] = fromOklab(
          parts[0] ?? 0,
          chroma * Math.cos(hue),
          chroma * Math.sin(hue),
        );

        return [red, green, blue, alpha(3)];
      }

      if (text.startsWith('oklab')) {
        const [red, green, blue] = fromOklab(
          parts[0] ?? 0,
          parts[1] ?? 0,
          parts[2] ?? 0,
        );

        return [red, green, blue, alpha(3)];
      }

      // `color(srgb r g b / a)`, where each channel is already 0 to 1.
      if (text.startsWith('color(')) {
        const channels = parts.slice(1);

        return [
          Math.round((channels[0] ?? 0) * 255),
          Math.round((channels[1] ?? 0) * 255),
          Math.round((channels[2] ?? 0) * 255),
          channels[3] ?? 1,
        ];
      }

      // `rgb()` and `rgba()`, in either the comma form or the space form.
      return [parts[0] ?? 0, parts[1] ?? 0, parts[2] ?? 0, alpha(3)];
    };

    const over = (
      top: [number, number, number, number],
      under: [number, number, number, number],
    ): [number, number, number, number] => [
      top[3] * top[0] + (1 - top[3]) * under[0],
      top[3] * top[1] + (1 - top[3]) * under[1],
      top[3] * top[2] + (1 - top[3]) * under[2],
      1,
    ];

    const channel = (value: number): number => {
      const part = value / 255;

      return part <= 0.03928 ? part / 12.92 : ((part + 0.055) / 1.055) ** 2.4;
    };

    const luminance = (colour: [number, number, number, number]): number =>
      0.2126 * channel(colour[0]) +
      0.7152 * channel(colour[1]) +
      0.0722 * channel(colour[2]);

    const ratio = (
      one: [number, number, number, number],
      two: [number, number, number, number],
    ): number => {
      const first = luminance(one);
      const second = luminance(two);

      return (
        (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05)
      );
    };

    const where = (element: Element): string => {
      const parts: string[] = [];
      let held: Element | null = element;
      while (held !== null && parts.length < 3) {
        const id = held.id === '' ? '' : `#${held.id}`;
        parts.unshift(`${held.tagName.toLowerCase()}${id}`);
        held = held.parentElement;
      }

      return parts.join(' ');
    };

    // What the text really sits on: every background above it, composited
    // until one is opaque.
    const backdrop = (element: Element): [number, number, number, number] => {
      let held: Element | null = element;
      let stack: [number, number, number, number] = [0, 0, 0, 0];
      while (held !== null) {
        const paint = rgba(getComputedStyle(held).backgroundColor);
        if (paint[3] > 0) {
          stack = stack[3] === 0 ? paint : over(stack, paint);
          if (stack[3] >= 1) return stack;
        }
        held = held.parentElement;
      }

      return stack[3] >= 1 ? stack : [255, 255, 255, 1];
    };

    const walker = document.createTreeWalker(
      document.body,
      NodeFilter.SHOW_TEXT,
    );
    const pairs: Pair[] = [];
    const seen = new Set<string>();

    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const text = (node.nodeValue ?? '').trim();
      if (text === '') continue;
      const element = node.parentElement;
      if (element === null) continue;
      if (['SCRIPT', 'STYLE', 'TITLE'].includes(element.tagName)) continue;

      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      const style = getComputedStyle(element);
      if (style.visibility === 'hidden' || style.display === 'none') continue;

      // WCAG 1.4.3 exempts the text of an inactive control: a disabled
      // button is meant to read as unavailable, and holding it to 4.5:1
      // would make it read as available. bits-ui marks one with
      // `data-disabled` and a native control with the attribute.
      if (
        element.closest(
          '[disabled],[aria-disabled="true"],[data-disabled],fieldset[disabled]',
        ) !== null
      ) {
        continue;
      }

      // Every opacity above the text, because an ancestor set to a fraction
      // fades the text with it. That is where earlier research found the worst
      // pair on the page.
      let fade = 1;
      let held: Element | null = element;
      while (held !== null) {
        fade *= Number.parseFloat(getComputedStyle(held).opacity || '1');
        held = held.parentElement;
      }
      if (fade < 0.05) continue;

      const paper = backdrop(element);
      const ink = rgba(style.color);
      const drawn = over([ink[0], ink[1], ink[2], ink[3] * fade], paper);

      const size = Number.parseFloat(style.fontSize);
      const weight = Number.parseInt(style.fontWeight, 10);
      const key = [style.color, fade, paper.join(','), size, weight].join('|');
      if (seen.has(key)) continue;
      seen.add(key);

      pairs.push({
        color: style.color,
        background: getComputedStyle(element).backgroundColor,
        ratio: Math.round(ratio(drawn, paper) * 100) / 100,
        size,
        bold: weight >= 700,
        sample: text.slice(0, 40),
        where: where(element),
      });
    }

    return pairs;
  });

/**
 * The brand fills that carry the Prusa Design System's own white text, as
 * `color|background` the way the browser computes them. Those pairs sit under
 * 4.5:1 by the design system's choice, and the component library's
 * `src/styles/contrast.test.ts` holds each one to its measured ratio.
 */
export const designSystemPairs = (sheet: Page): Promise<Set<string>> =>
  sheet
    .evaluate(() => {
      const PAIRS: [string, string][] = [
        ['--primary-foreground', '--primary'],
        ['--primary-foreground', '--primary-hovered'],
        ['--primary-foreground', '--primary-pressed'],
        ['--selected-foreground', '--selected'],
        ['--selected-foreground', '--selected-hovered'],
        ['--control-on-foreground', '--control-on'],
      ];
      const probe = document.createElement('span');
      document.body.append(probe);
      const keys = PAIRS.map(([fg, bg]) => {
        probe.style.color = `var(${fg})`;
        probe.style.backgroundColor = `var(${bg})`;
        const style = getComputedStyle(probe);

        return `${style.color}|${style.backgroundColor}`;
      });
      probe.remove();

      return keys;
    })
    .then((keys) => new Set(keys));

/** WCAG 1.4.3: 4.5:1, or 3:1 for text at 24 px, or 18.66 px and bold. */
export const floorFor = (pair: Pair): number =>
  pair.size >= 24 || (pair.bold && pair.size >= 18.66) ? 3 : 4.5;

/** What the browser named as the source of every shift so far. */
export const moved = (sheet: Page): Promise<string[]> =>
  sheet.evaluate(() => (globalThis as unknown as { __ux: Probe }).__ux.moved);
