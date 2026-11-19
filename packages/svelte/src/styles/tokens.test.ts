import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { compile } from '@tailwindcss/node';
import { beforeAll, describe, expect, it } from 'vitest';

// The record of what the theme says. Ported from the React design system's own
// `styles/tokens.test.ts` on the `legacy/main` branch, whose colour scales this
// theme copies value for value.

const stylesDir = dirname(fileURLToPath(import.meta.url));

const css = readFileSync(join(stylesDir, 'tokens.css'), 'utf8');

// tokens.css carries no Tailwind import; that line is the consumer's. The
// compiler needs one, so the compile tests wrap the file the way a consumer
// does, the same way contrast.test.ts beside this file does.
const compilable = `@import 'tailwindcss' source(none);\n${css}`;

// Matches braces so a nested at-rule cannot end the slice early. `from` starts
// the search for the marker further into the file.
const sliceBlock = (marker: string, from = 0): string => {
  const start = css.indexOf(marker, from);
  if (start === -1) throw new Error(`marker not found: ${marker}`);
  const open = css.indexOf('{', start);
  if (open === -1) throw new Error(`no block opens after ${marker}`);
  let depth = 0;
  for (let i = open; i < css.length; i += 1) {
    if (css[i] === '{') depth += 1;
    else if (css[i] === '}') {
      depth -= 1;
      if (depth === 0) return css.slice(open + 1, i);
    }
  }
  throw new Error(`unbalanced braces after ${marker}`);
};

const declares = (block: string, token: string, value: string): boolean =>
  new RegExp(
    `^\\s*${token}:\\s*${value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')};\\s*$`,
    'm',
  ).test(block);

const theme = sliceBlock('@theme inline');

const THEMES = ['light', 'dark'] as const;
type Theme = (typeof THEMES)[number];

const tokenBlock = (mode: Theme): string => sliceBlock(`/* @tokens ${mode} */`);

/**
 * The blocks one theme resolves a token through, most specific first: the
 * cascade the browser walks, spelled out. `.dark` sits on the root element
 * with `:root`, scores the same and comes later, so it wins where it declares.
 */
const cascade = (mode: Theme): string[] =>
  mode === 'light'
    ? [tokenBlock('light')]
    : [tokenBlock('dark'), tokenBlock('light')];

/** What the theme pins: `[token, light, dark]`. */
const CONTRACT: [string, string, string][] = [
  ['--background', 'oklch(93.402524% 0 0)', '#0f0f0f'],
  ['--foreground', 'oklch(22.212874% 0 0)', '#fff'],
  ['--card', 'oklch(99.999999% 0 0)', 'oklch(22.212874% 0 0)'],
  ['--card-foreground', 'oklch(22.212874% 0 0)', '#fff'],
  ['--popover', 'oklch(99.999999% 0 0)', 'oklch(22.212874% 0 0)'],
  ['--popover-foreground', 'oklch(22.212874% 0 0)', '#fff'],
  ['--primary', 'oklch(64% 0.2194 37.8)', '#fa6831'],
  ['--primary-foreground', 'oklch(22.212874% 0 0)', '#0f0f0f'],
  ['--primary-hovered', 'var(--color-orange-light-8)', '#ff7b4a'],
  ['--primary-pressed', 'var(--color-orange-light-7)', '#e05a26'],
  ['--muted', 'oklch(93.402524% 0 0)', '#1b1b1b'],
  ['--muted-foreground', 'oklch(52.080655% 0 0)', '#b8b8b8'],
  ['--accent', 'oklch(93.402524% 0 0)', 'oklch(28.50174% 0 0)'],
  ['--border', 'oklch(78.260371% 0 0)', 'oklch(28.50174% 0 0)'],
  ['--selected', 'oklch(89.758125% 0.050058 257.676519)', '#364976'],
  ['--selected-foreground', 'oklch(22.212874% 0 0)', '#fff'],
  ['--selected-hovered', 'oklch(85.403746% 0.057175 257.45333)', '#425890'],
  ['--control-on', 'oklch(62.964222% 0.186235 264.278496)', '#4e81f8'],
  ['--control-on-foreground', 'oklch(22.212874% 0 0)', '#0f0f0f'],
  ['--progress', 'oklch(59.842125% 0.147407 264.655225)', '#7397ec'],
  ['--destructive', 'var(--color-red-light-11)', 'var(--color-red-dark-9)'],
  [
    '--destructive-surface',
    'var(--color-red-light-3)',
    'var(--color-red-dark-3)',
  ],
  [
    '--destructive-foreground',
    'var(--color-red-light-1)',
    'var(--color-red-dark-1)',
  ],
  [
    '--destructive-hovered',
    'var(--color-red-light-10)',
    'var(--color-red-dark-11)',
  ],
  [
    '--destructive-pressed',
    'var(--color-red-light-12)',
    'var(--color-red-dark-12)',
  ],
  [
    '--destructive-text',
    'var(--color-red-light-12)',
    'var(--color-red-dark-11)',
  ],
  ['--ring', 'oklch(59.842125% 0.147407 264.655225)', '#fa6831'],
  ['--disabled', 'oklch(93.402524% 0 0)', '#1b1b1b'],
  ['--disabled-foreground', 'var(--color-gray-light-10)', '#808080'],
  ['--success', 'var(--color-green-light-9)', 'var(--color-green-dark-9)'],
  [
    '--success-surface',
    'var(--color-green-light-3)',
    'var(--color-green-dark-3)',
  ],
  ['--success-foreground', '#000', '#000'],
  [
    '--success-text',
    'var(--color-green-light-12)',
    'var(--color-green-dark-11)',
  ],
  ['--warning', 'var(--color-yellow-light-9)', 'var(--color-yellow-dark-9)'],
  [
    '--warning-surface',
    'var(--color-yellow-light-3)',
    'var(--color-yellow-dark-3)',
  ],
  ['--warning-foreground', '#000', '#000'],
  [
    '--warning-text',
    'var(--color-yellow-light-12)',
    'var(--color-yellow-dark-11)',
  ],
  ['--surface', 'oklch(96.415349% 0 0)', 'oklch(20.019309% 0 0)'],
  ['--info', 'var(--color-blue-light-9)', 'var(--color-blue-dark-9)'],
  ['--info-surface', 'var(--color-blue-light-3)', 'var(--color-blue-dark-3)'],
  ['--info-foreground', '#000', '#000'],
  ['--info-text', 'var(--color-blue-light-12)', 'var(--color-blue-dark-11)'],
  ['--state-unknown', '#d3d7de', '#424549'],
  ['--state-unknown-edge', '#9daabd', '#9daabd'],
  ['--state-offline', '#d8d8d8', '#454545'],
  ['--state-offline-edge', '#adadad', '#adadad'],
  ['--state-ready', '#c4e8da', '#34493f'],
  ['--state-ready-edge', '#55c29c', '#55c29c'],
  ['--state-busy', '#d9dbf3', '#3d3f5a'],
  ['--state-busy-edge', '#8f95db', '#8f95db'],
  ['--state-printing', '#c5d4e7', '#3b444e'],
  ['--state-printing-edge', '#7da7d9', '#7da7d9'],
  ['--state-paused', '#e0d0e0', '#4a404a'],
  ['--state-paused-edge', '#c396c4', '#c396c4'],
  ['--state-stopped', '#e2dbd4', '#47423d'],
  ['--state-stopped-edge', '#a89d92', '#a89d92'],
  ['--state-finished', '#d4eec3', '#435238'],
  ['--state-finished-edge', '#a1ea70', '#a1ea70'],
  ['--state-error', '#e9c5c6', '#4f3b3b'],
  ['--state-error-edge', '#dd7b7d', '#dd7b7d'],
  ['--state-moving', '#cfe6f3', '#34475a'],
  ['--state-moving-edge', '#5fb4dd', '#5fb4dd'],
  ['--secondary', 'oklch(96.415349% 0 0)', '#2a2a2a'],
  ['--secondary-foreground', 'oklch(22.212874% 0 0)', '#fff'],
  ['--secondary-hovered', 'oklch(90.365199% 0 0)', '#323232'],
  ['--input', 'var(--color-gray-light-10)', 'var(--color-gray-dark-10)'],
];

/** Every token name the contract pins: what @theme inline has to register. */
const REGISTERED = CONTRACT.map(([token]) => token);

/** The largest corner radius the theme may use. */
const RADIUS_CAP = 5;

const declaredTokens = (block: string): string[] =>
  [...block.matchAll(/^\s*(--[a-z0-9-]+):/gm)].map((match) => match[1]!);

describe('token contract', () => {
  it.each(CONTRACT)('%s carries the value the theme pins', (token, l, d) => {
    expect(declares(tokenBlock('light'), token, l)).toBe(true);
    expect(declares(tokenBlock('dark'), token, d)).toBe(true);
  });

  // Undeclared in @theme inline emits no Tailwind utility, and nothing else in the toolchain reports it.
  it.each(REGISTERED)('%s is registered as a Tailwind colour', (token) => {
    const name = token.replace(/^--/, '');
    expect(
      new RegExp(`^\\s*--color-${name}:\\s*var\\(${token}\\);\\s*$`, 'm').test(
        theme,
      ),
    ).toBe(true);
  });
});

describe('raw palette', () => {
  // Guards against a short/missing palette copy, which would let every contract assertion above pass vacuously.
  it.each([
    '--color-gray-light-12',
    '--color-gray-dark-12',
    '--color-gray-dark-2',
    '--color-orange-light-9',
    '--color-orange-dark-9',
    '--color-orange-alpha-light-8',
    '--color-red-light-9',
    '--color-green-dark-9',
    '--color-yellow-dark-9',
    '--color-blue-light-11',
  ])('%s is declared', (token) => {
    expect(new RegExp(`^\\s*${token}:\\s*\\S`, 'm').test(css)).toBe(true);
  });

  it('carries the full 12-step gray ramp in both modes', () => {
    for (const mode of ['light', 'dark']) {
      for (let step = 1; step <= 12; step += 1) {
        expect(
          new RegExp(
            `^\\s*--color-gray-${mode}-${String(step)}:\\s*\\S`,
            'm',
          ).test(css),
        ).toBe(true);
      }
    }
  });
});

// A new --animate-* token added without a matching `none` line in the reduced-motion `@media` block animates for everybody, silently.
// --duration-progress is checked by name since it isn't an --animate-* token.
describe('reduced motion switches off every motion token', () => {
  const motion = sliceBlock('/* ── Motion');
  const reduced = sliceBlock('@media (prefers-reduced-motion: reduce)');
  const declared: string[] = [
    ...motion.matchAll(/^\s*(--animate-[a-z0-9-]+):/gm),
  ].map((match) => match[1]!);

  it('declares animation tokens to check', () => {
    expect(declared.length).toBeGreaterThan(12);
  });

  it.each(declared)('%s is none under reduced motion', (token) => {
    expect(new RegExp(`^\\s*${token}:\\s*none;`, 'm').test(reduced)).toBe(true);
  });

  it('takes the progress arc\u2019s travel to zero', () => {
    expect(/--duration-progress:\s*0s;/.test(reduced)).toBe(true);
  });

  it('stops the focus lift from rising', () => {
    expect(/--focus-lift-travel:\s*0px;/.test(reduced)).toBe(true);
  });
});

describe('visual language', () => {
  it('registers a monospace font family', () => {
    expect(theme.includes('--font-mono:')).toBe(true);
  });

  it('registers a heading font family', () => {
    expect(theme.includes('--font-heading:')).toBe(true);
  });

  it('lifts a focused control differently in light and dark', () => {
    const light = tokenBlock('light');
    const dark = tokenBlock('dark');
    // The shadow and the lit edge swap roles between themes; these substrings
    // are what tells the two apart rather than the exact wrapping of the value.
    expect(light).toContain('rgb(255 255 255 / 0.55)');
    expect(dark).toContain('rgb(255 255 255 / 0.28)');
  });

  // Radius and the type/spacing rhythm reach Tailwind through @theme inline
  // as variables, so a literal written back here would bypass the shape block
  // with nothing failing.
  it('routes every radius step through the corner tokens', () => {
    for (const step of ['xs', 'sm', 'md']) {
      expect(theme).toMatch(
        new RegExp(`^\\s*--radius-${step}: var\\(--corner-${step}\\);$`, 'm'),
      );
    }
  });

  it('scales the type steps and the spacing unit by the density', () => {
    const scaled = [
      ...theme.matchAll(/--(?:text-[a-z0-9]+|spacing):\s*([^;]+);/g),
    ].map((match) => match[1]);
    expect(scaled.length).toBeGreaterThan(5);
    for (const value of scaled) expect(value).toContain('var(--density)');
  });

  describe('shape', () => {
    const shape = sliceBlock('/* @shape */');

    it('stays within the corner cap', () => {
      const radii = [...shape.matchAll(/--corner-[a-z0-9]+:\s*(\d+)px;/g)].map(
        (match) => Number(match[1]),
      );
      expect(radii.length).toBe(3);
      expect(Math.max(...radii)).toBeLessThanOrEqual(RADIUS_CAP);
    });

    it('declares a density', () => {
      const density = /--density:\s*([\d.]+);/.exec(shape);
      expect(density).not.toBeNull();
      expect(Number(density![1])).toBeGreaterThan(0);
    });

    it.each([
      ['--corner-xs', '2px'],
      ['--corner-sm', '3px'],
      ['--corner-md', '5px'],
      ['--density', '0.73'],
      ['--text-size-factor', '1'],
    ])('%s is %s', (token, value) => {
      expect(declares(shape, token, value)).toBe(true);
    });

    it('sets the page in Inter', () => {
      expect(shape).toMatch(/--font-sans:\s*'Inter',/);
    });

    it.each([
      ['small', '0.9'],
      ['large', '1.125'],
    ])('scales the type for a %s text size by %s', (size, factor) => {
      expect(
        declares(
          sliceBlock(`:root[data-text-size='${size}']`),
          '--text-size-factor',
          factor,
        ),
      ).toBe(true);
    });
  });

  it('carries no legacy or bridge vocabulary', () => {
    expect(css).not.toMatch(/--legacy-/);
    expect(css).not.toMatch(/--ds-/);
  });
});

// Runs the real compiler: a source-text check can't see Tailwind's built-in default theme (a full palette, a radius scale to .75rem) unless reset.
describe('tailwind default theme reset', () => {
  let build: (candidates: string[]) => string;

  beforeAll(async () => {
    const compiled = await compile(compilable, {
      base: stylesDir,
      onDependency: () => undefined,
    });
    build = (candidates) => compiled.build(candidates);
  });

  it('emits nothing for stock-palette or oversized-radius utilities', () => {
    // None of these classes exist in src/next; they stand in for a future primitive copied from a component still carrying upstream's rounded-lg or an arbitrary Tailwind colour.
    // The reset has two deliberate exceptions, --color-white/--color-black, covered by keyword-colours.test.ts instead.
    const out = build([
      'bg-red-500',
      'text-blue-400',
      'rounded-lg',
      'rounded-xl',
    ]);
    expect(out).not.toMatch(/--color-red-500/);
    expect(out).not.toMatch(/--color-blue-400/);
    expect(out).not.toMatch(/--radius-lg/);
    expect(out).not.toMatch(/--radius-xl/);
  });

  // `p-5` rather than `p-6`: block 4 still pins Tailwind's own low steps onto
  // its nine-step scale, and a step it does not name is the one that shows the
  // density reaching the spacing unit.
  it('still resolves this project’s own tokens through the same compiler', () => {
    const out = build(['bg-primary', 'rounded-sm', 'p-5']);
    expect(out).toContain('.bg-primary');
    // Radius and spacing reach the rule as variables, which is what lets the
    // shape block and the text size setting move them without a second
    // utility.
    expect(
      /\.rounded-sm\s*\{\s*border-radius:\s*var\(--corner-sm\);?\s*\}/.test(
        out,
      ),
    ).toBe(true);
    expect(out).toContain('var(--density)');
  });

  // Guards the class strings the rail and the phone bar already write: absent
  // from @theme inline, each of these compiles to nothing and leaves the
  // current row with no fill at all.
  it('compiles the selected and control-on utilities', () => {
    const out = build([
      'bg-selected',
      'border-selected',
      'text-selected-foreground',
      'hover:bg-selected-hovered',
      'bg-control-on',
    ]);
    expect(out).toContain('.bg-selected');
    expect(out).toContain('.border-selected');
    expect(out).toContain('.text-selected-foreground');
    expect(out).toContain('bg-selected-hovered');
    expect(out).toContain('.bg-control-on');
  });
});

// @source's effect is which files Tailwind's scanner reads, invisible to a source-text check.
describe('tailwind source scoping', () => {
  it('disables automatic project-wide scanning', async () => {
    const { root } = await compile(compilable, {
      base: stylesDir,
      onDependency: () => undefined,
    });
    expect(root).toBe('none');
  });

  // A module imports this file for the theme and the `dark` variant, and emits
  // only the utilities it names itself (ADR-20260914-244 rules 10 to 13). A scan here
  // would put the whole library's in every module bundle.
  it('names no source of its own', async () => {
    const { sources } = await compile(compilable, {
      base: stylesDir,
      onDependency: () => undefined,
    });
    expect(sources).toEqual([]);
  });

  // app.css is the other half: the file an application imports, carrying the
  // scan that tokens.css must not. `@source` takes a path and never a package
  // name, so this is the only place the path can be written safely, with both
  // ends inside this package.
  it('names this package as the one source an application scans', async () => {
    const appCss = readFileSync(join(stylesDir, 'app.css'), 'utf8');
    const { sources } = await compile(
      `@import 'tailwindcss' source(none);\n${appCss}`,
      { base: stylesDir, onDependency: () => undefined },
    );

    expect(sources).toHaveLength(1);
    expect(sources[0]?.base).toBe(stylesDir);
    expect(sources[0]?.pattern).toBe('../');
  });
});

// Ratios cited in comments above were computed by hand and can't recompute themselves, so this suite re-measures every pair that meets on screen.
// OKLCH → sRGB (CSS Color 4) then WCAG 2.1 luminance/contrast, implemented rather than imported for forty lines of arithmetic that won't change.
type Srgb = readonly [number, number, number];

const encode = (v: number): number => {
  const x = Math.min(1, Math.max(0, v));
  return x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055;
};

const oklchToSrgb = (L: number, C: number, hDeg: number): Srgb => {
  const h = (hDeg * Math.PI) / 180;
  const a = C * Math.cos(h);
  const bb = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * bb) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * bb) ** 3;
  const sc = (L - 0.0894841775 * a - 1.291485548 * bb) ** 3;
  return [
    encode(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * sc),
    encode(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * sc),
    encode(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * sc),
  ];
};

const decode = (c: number): number =>
  c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;

const relativeLuminance = ([r, g, b]: Srgb): number =>
  0.2126 * decode(r) + 0.7152 * decode(g) + 0.0722 * decode(b);

const contrast = (fg: Srgb, bg: Srgb): number => {
  const a = relativeLuminance(fg);
  const b = relativeLuminance(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
};

// Two notations appear here: `oklch()` for most of the palette, hex for carried-over brand values and `#fff`/`#000`.
// Anything else returns null so a caller fails loudly instead of measuring a guess.
const parseColour = (value: string): Srgb | null => {
  const oklch = /^oklch\(([\d.]+)%\s+([\d.]+)\s+([\d.]+)\)$/.exec(value);
  if (oklch) {
    return oklchToSrgb(
      Number(oklch[1]) / 100,
      Number(oklch[2]),
      Number(oklch[3]),
    );
  }
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(value);
  if (!hex) return null;
  const digits = hex[1]!;
  const pairs =
    digits.length === 3
      ? (digits.match(/./g) ?? []).map((digit) => digit + digit)
      : [digits.slice(0, 2), digits.slice(2, 4), digits.slice(4, 6)];
  const channel = (pair: string): number => parseInt(pair, 16) / 255;
  return [channel(pairs[0]!), channel(pairs[1]!), channel(pairs[2]!)];
};

/** The raw palette, declared once in `:root` above the two theme blocks. */
const palette = new Map<string, Srgb>();
for (const match of css.matchAll(/(--color-[a-z0-9-]+):\s*([^;]+);/g)) {
  const srgb = parseColour(match[2]!.trim());
  if (srgb) palette.set(match[1]!, srgb);
}

// Throws rather than returning a default: an unresolved token would otherwise be silently skipped by the assertions below.
const resolveToken = (blocks: string[], token: string): Srgb => {
  const match = blocks
    .map((block) => new RegExp(`^\\s*${token}:\\s*([^;]+);`, 'm').exec(block))
    .find((hit) => hit !== null);
  if (!match)
    throw new Error(
      `${token} is not declared anywhere in this theme's cascade`,
    );
  const value = match[1]!.trim();
  const reference = /^var\((--color-[a-z0-9-]+)\)$/.exec(value);
  if (reference) {
    const named = reference[1]!;
    const srgb = palette.get(named);
    if (!srgb)
      throw new Error(
        `${token} references ${named}, which the palette does not declare in a notation this test parses`,
      );
    return srgb;
  }
  const literal = parseColour(value);
  if (!literal)
    throw new Error(
      `${token} is \`${value}\`, which this test cannot parse as a colour`,
    );
  return literal;
};

// Pairs that meet on screen; AA's 4.5:1 bar for normal text (each carries `text-sm`, 14px, somewhere in `components/ui`).
// `--disabled`/`--disabled-foreground` is absent: WCAG exempts inactive controls.
const TEXT_PAIRS: [fg: string, bg: string][] = [
  ['--foreground', '--background'],
  ['--card-foreground', '--card'],
  ['--popover-foreground', '--popover'],
  // The brand fills and the text on them are measured in contrast.test.ts.
  ['--secondary-foreground', '--secondary'],
  ['--secondary-foreground', '--secondary-hovered'],
  ['--muted-foreground', '--muted'],
  ['--muted-foreground', '--background'],
  // Not implied by the --background/--muted rows above: --surface is a step away from both. Measured 14.44:1/13.59:1 and 5.22:1/7.60:1.
  ['--foreground', '--surface'],
  ['--foreground', '--state-unknown'],
  ['--foreground', '--state-ready'],
  ['--foreground', '--state-busy'],
  ['--foreground', '--state-moving'],
  ['--foreground', '--state-paused'],
  ['--foreground', '--state-stopped'],
  ['--foreground', '--state-finished'],
  ['--state-idle-text', '--state-idle'],
  ['--muted-foreground', '--surface'],
  ['--destructive-foreground', '--destructive'],
  ['--destructive-foreground', '--destructive-hovered'],
  ['--destructive-text', '--background'],
  // Not implied by the --background pair: --card/--popover sit one step lighter than --background in dark, the gap the prior token fell into at 4.48:1.
  ['--destructive-text', '--card'],
  ['--destructive-text', '--popover'],
  ['--success-foreground', '--success'],
  ['--success-text', '--background'],
  ['--warning-foreground', '--warning'],
  ['--warning-text', '--background'],
  ['--info-foreground', '--info'],
  ['--info-text', '--background'],
];

describe('text contrast clears WCAG AA in both themes', () => {
  it('declares a palette to measure against', () => {
    // An empty palette would make every assertion below pass vacuously.
    expect(palette.size).toBeGreaterThan(40);
  });

  for (const mode of THEMES) {
    describe(mode, () => {
      for (const [fg, bg] of TEXT_PAIRS) {
        it(`${fg} on ${bg}`, () => {
          const blocks = cascade(mode);
          const ratio = contrast(
            resolveToken(blocks, fg),
            resolveToken(blocks, bg),
          );
          expect(
            Number(ratio.toFixed(2)),
            `${fg} on ${bg} in the ${mode} theme`,
          ).toBeGreaterThanOrEqual(4.5);
        });
      }
    });
  }
});

describe('one theme', () => {
  it('declares the dark block after the light one', () => {
    expect(css.indexOf('/* @tokens dark */')).toBeGreaterThan(
      css.indexOf('/* @tokens light */'),
    );
  });

  // A `:root` block after `.dark` scores the same and would win in dark too.
  it('redeclares no dark name in a later :root block', () => {
    const dark = new Set(declaredTokens(tokenBlock('dark')));
    const from = css.indexOf('/* @tokens dark */');
    const later = [...css.slice(from).matchAll(/^:root \{/gm)].map((match) =>
      sliceBlock(':root {', from + match.index),
    );
    expect(later.length).toBeGreaterThan(0);
    expect(
      later.flatMap(declaredTokens).filter((token) => dark.has(token)),
    ).toEqual([]);
  });

  it('declares no skin block and no skin variant', () => {
    expect(css).not.toMatch(/data-skin/);
    expect(css).not.toMatch(/skin-ember/);
  });

  // The names the dark half of the theme sets in place of the light ones.
  // `:root` and `.dark` score alike, so each holds in dark only because
  // `.dark` declares it and comes later in the file.
  it('keeps the dark values under plain .dark', () => {
    const dark = new Set(declaredTokens(tokenBlock('dark')));
    expect(
      [
        '--background',
        '--foreground',
        '--card',
        '--card-foreground',
        '--popover',
        '--popover-foreground',
        '--primary',
        '--primary-fill',
        '--link',
        '--primary-foreground',
        '--primary-hovered',
        '--primary-pressed',
        '--selected',
        '--selected-foreground',
        '--control-on',
        '--control-on-foreground',
        '--progress',
        '--selected-hovered',
        '--secondary',
        '--secondary-foreground',
        '--secondary-hovered',
        '--muted',
        '--muted-foreground',
        '--accent',
        '--surface',
        '--border',
        '--input',
        '--ring',
        '--disabled',
        '--disabled-foreground',
        '--destructive-text',
        '--success-text',
        '--info-text',
        '--state-idle',
        '--state-idle-text',
        '--state-idle-edge',
      ].filter((token) => !dark.has(token)),
    ).toEqual([]);
  });
});
