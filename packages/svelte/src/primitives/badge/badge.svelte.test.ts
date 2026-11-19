import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Badge from './badge.svelte';
import type { BadgeVariant } from './types';
import { base, sizes, variants } from './variants';

const label = (text: string) =>
  createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const link = createRawSnippet<[{ props: Record<string, unknown> }]>(() => ({
  render: () =>
    `<a href="#printers" data-slot="badge" class="${base} ${variants.default}">Printers</a>`,
}));

const GENERAL: BadgeVariant[] = [
  'default',
  'secondary',
  'outline',
  'success',
  'warning',
  'info',
  'destructive',
];

const STATES: BadgeVariant[] = [
  'stateActive',
  'stateBusy',
  'stateMoving',
  'stateReady',
  'stateFinished',
  'stateStopped',
  'statePaused',
  'stateIdle',
  'stateAttention',
  'stateFault',
  'stateQuiet',
];

/**
 * The compiled rule for one utility, with `:hover` taken off it.
 *
 * An element can then be matched against the rule. `:hover` itself needs real
 * pointer input, and a synthetic event never satisfies it. The escaped
 * `\:hover\:` inside the class name is left alone, so only the pseudo-class
 * goes.
 */
const restingSelector = (utility: string): string | undefined => {
  const needle = `.${CSS.escape(utility)}`;

  // `.bg-primary` is a substring of `.bg-primary-hovered`, so a plain
  // `includes` reads the wrong rule. The class name has to end where the
  // needle does.
  const names = (selector: string): boolean => {
    for (let at = selector.indexOf(needle); at !== -1;) {
      if (!/[\w\\-]/.test(selector[at + needle.length] ?? '')) return true;
      at = selector.indexOf(needle, at + 1);
    }
    return false;
  };

  const walk = (rules: CSSRuleList): string | undefined => {
    for (const rule of Array.from(rules)) {
      if (rule instanceof CSSStyleRule && names(rule.selectorText)) {
        return rule.selectorText.replace(/(?<!\\):hover/g, '');
      }
      if (rule instanceof CSSGroupingRule) {
        const found = walk(rule.cssRules);
        if (found) return found;
      }
    }
    return undefined;
  };

  for (const sheet of Array.from(document.styleSheets)) {
    try {
      const found = walk(sheet.cssRules);
      if (found) return found;
    } catch {
      // A cross-origin sheet refuses `cssRules`. The theme is not in one.
      continue;
    }
  }
  return undefined;
};

describe('Badge', () => {
  // Compared without the printer states: `secondary` and `stateAttention`
  // read the same orange tint pair out of tokens.css, and a printer state
  // never renders beside a secondary badge.
  it('renders the general variants with their own attribute and their own colours', async () => {
    const looks = new Set<string>();

    for (const variant of GENERAL) {
      const { container, unmount } = await render(Badge, {
        props: { variant, children: label(variant) },
      });
      const badge = container.querySelector('[data-slot="badge"]');
      expect(badge?.getAttribute('data-variant')).toBe(variant);
      const style = getComputedStyle(badge!);
      looks.add(`${style.backgroundColor}/${style.color}`);
      await unmount();
    }

    // One rendering per variant. An entry that falls back to another's
    // utilities collapses this set.
    expect(looks.size).toBe(GENERAL.length);
  });

  it('renders each printer state with its own attribute and a fill', async () => {
    for (const variant of STATES) {
      const { container, unmount } = await render(Badge, {
        props: { variant, children: label(variant) },
      });
      const badge = container.querySelector('[data-slot="badge"]');
      expect(badge?.getAttribute('data-variant')).toBe(variant);
      expect(getComputedStyle(badge!).backgroundColor).not.toBe(
        'rgba(0, 0, 0, 0)',
      );
      await unmount();
    }
  });

  // Offline is the one state drawn as an absence: no fill, a dashed outline.
  it('draws stateOffline hollow and dashed', async () => {
    const { container } = await render(Badge, {
      props: { variant: 'stateOffline', children: label('Offline') },
    });
    const style = getComputedStyle(
      container.querySelector('[data-slot="badge"]')!,
    );
    expect(style.backgroundColor).toBe('rgba(0, 0, 0, 0)');
    expect(style.borderTopStyle).toBe('dashed');
  });

  it('names every variant the map declares', () => {
    expect([...GENERAL, ...STATES, 'stateOffline'].sort()).toEqual(
      Object.keys(variants).sort(),
    );
  });

  it('gives a printer state a 4px edge in its own colour', async () => {
    for (const variant of STATES) {
      const { container, unmount } = await render(Badge, {
        props: { variant, children: label('Printing') },
      });
      const style = getComputedStyle(
        container.querySelector('[data-slot="badge"]')!,
      );

      // The edge is what tells one state from another at a glance in a row of
      // them, and it makes the box 3px wider than a success or warning badge.
      expect(style.borderLeftWidth).toBe('4px');
      expect(style.borderTopWidth).toBe('1px');
      expect(style.borderLeftColor).not.toBe(style.borderTopColor);
      await unmount();
    }
  });

  it('keeps the hover fill for the link form alone', async () => {
    const selector = restingSelector('[a&]:hover:bg-primary-hovered');
    expect(selector).toBeDefined();

    const plain = await render(Badge, { props: { children: label('Ready') } });
    const span = plain.container.querySelector('[data-slot="badge"]');
    expect(span?.matches(selector!)).toBe(false);
    await plain.unmount();

    const linked = await render(Badge, { props: { child: link } });
    const anchor = linked.container.querySelector('a');
    expect(anchor?.matches(selector!)).toBe(true);
  });

  it('takes focus from the keyboard in its link form and paints a ring', async () => {
    await render(Badge, { props: { child: link } });

    const anchor = page.getByRole('link', { name: 'Printers' });
    await userEvent.tab();
    await expect.element(anchor).toHaveFocus();
    expect(anchor.element().matches(':focus-visible')).toBe(true);
    // The ring is a box shadow, so a resting badge has none.
    expect(getComputedStyle(anchor.element()).boxShadow).not.toBe('none');
  });

  it('hands its attributes to the child snippet, which draws its own element', async () => {
    let handed: Record<string, unknown> | undefined;
    const probe = createRawSnippet<[{ props: Record<string, unknown> }]>(
      (argument) => {
        handed = argument().props;
        return { render: () => '<a href="#printers">Printers</a>' };
      },
    );

    const { container } = await render(Badge, {
      props: { child: probe, variant: 'success' },
    });

    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toBeInTheDocument();
    expect(container.querySelector('span')).toBeNull();
    expect(handed?.['data-slot']).toBe('badge');
    expect(handed?.['data-variant']).toBe('success');
    expect(handed?.class).toEqual([
      base,
      variants.success,
      sizes.sm,
      false,
      false,
      undefined,
    ]);
  });

  it('has no accessibility violation', async () => {
    const { container } = await render(Badge, {
      props: { children: label('Ready') },
    });

    await expectNoViolations(container);
  });

  it('is mono for a state variant, plain otherwise, and mono follows the prop', async () => {
    const badge = (text: string) =>
      page.getByText(text).element().closest('[data-slot="badge"]');

    await render(Badge, {
      props: { variant: 'stateActive', children: label('Printing') },
    });
    expect(badge('Printing')).toHaveClass('font-mono', 'uppercase');
    await render(Badge, { props: { children: label('Ready') } });
    expect(badge('Ready')).not.toHaveClass('font-mono');
    await render(Badge, {
      props: {
        variant: 'stateIdle',
        mono: false,
        size: 'md',
        children: label('Idle'),
      },
    });
    expect(badge('Idle')).not.toHaveClass('font-mono');
    expect(badge('Idle')).toHaveClass('py-1');
  });
});

describe('Badge size and numeric', () => {
  it('draws lg in the larger type', async () => {
    await render(Badge, { props: { size: 'lg', children: label('Bench') } });
    const badge = document.querySelector<HTMLElement>('[data-slot="badge"]')!;
    expect(parseFloat(getComputedStyle(badge).fontSize)).toBeCloseTo(14, 0);
  });

  it('draws sm and md in the small type', async () => {
    await render(Badge, { props: { size: 'sm', children: label('Small') } });
    await render(Badge, { props: { size: 'md', children: label('Medium') } });
    for (const badge of document.querySelectorAll<HTMLElement>(
      '[data-slot="badge"]',
    )) {
      expect(parseFloat(getComputedStyle(badge).fontSize)).toBeCloseTo(12, 0);
    }
  });

  it('keeps the case of a numeric reading and sets it in mono', async () => {
    await render(Badge, {
      props: { numeric: true, children: label('1 mm/s') },
    });
    const style = getComputedStyle(
      document.querySelector<HTMLElement>('[data-slot="badge"]')!,
    );
    expect(style.textTransform).toBe('none');
    expect(style.fontVariantNumeric).toContain('tabular-nums');
    expect(style.fontFamily).toContain('mono');
  });
});
