import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import IconMenuProbe from '../../test/icon-menu-probe.svelte';
import Button from './button.svelte';
import type {
  ButtonIconSize,
  ButtonProps,
  ButtonSize,
  ButtonVariant,
} from './types';
import { base, radii, sizes, variants } from './variants';

const label = (text: string) =>
  createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const VARIANTS: ButtonVariant[] = [
  'default',
  'secondary',
  'ghost',
  'destructive',
];

/**
 * The fill the compiled stylesheet paints for one utility.
 *
 * `getComputedStyle` cannot answer this. `:active` needs a real pointer press,
 * and a synthetic event never puts the element in that state. Reading the
 * declared value proves what actually breaks: a utility naming a token the
 * theme never registered compiles to nothing and disappears without a word.
 */
const declaredFill = (utility: string): string | undefined => {
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

  // Tailwind writes a variant as a nested rule, so the fill can sit one level
  // under the rule that names the class.
  const inside = (rules: CSSRuleList): string | undefined => {
    for (const rule of Array.from(rules)) {
      if (rule instanceof CSSStyleRule) {
        const own = rule.style.getPropertyValue('background-color');
        if (own) return own;
      }
      if (rule instanceof CSSGroupingRule) {
        const found = inside(rule.cssRules);
        if (found) return found;
      }
    }
    return undefined;
  };

  const walk = (rules: CSSRuleList): string | undefined => {
    for (const rule of Array.from(rules)) {
      const named = rule instanceof CSSStyleRule && names(rule.selectorText);
      if (named) {
        const own = rule.style.getPropertyValue('background-color');
        if (own) return own;
      }
      if (rule instanceof CSSGroupingRule) {
        const found = named ? inside(rule.cssRules) : walk(rule.cssRules);
        if (found) return found;
      }
    }
    return undefined;
  };

  for (const sheet of Array.from(document.styleSheets)) {
    try {
      const fill = walk(sheet.cssRules);
      if (fill) return fill;
    } catch {
      // A cross-origin sheet refuses `cssRules`. The theme is not in one.
      continue;
    }
  }
  return undefined;
};

const isIcon = (size: ButtonSize): size is ButtonIconSize =>
  size.startsWith('icon-');

const tooltip = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[role="tooltip"]');

const utility = (variant: ButtonVariant, prefix: string): string | undefined =>
  variants[variant].split(' ').find((name) => name.startsWith(prefix));

describe('Button', () => {
  it('is a plain button by default, so it does not submit a form by accident', async () => {
    await render(Button, { props: { children: label('Print') } });

    await expect
      .element(page.getByRole('button', { name: 'Print' }))
      .toHaveAttribute('type', 'button');
  });

  it('renders each variant with its own attribute and its own colours', async () => {
    const looks = new Set<string>();

    for (const variant of VARIANTS) {
      const { unmount } = await render(Button, {
        props: { variant, children: label(variant) },
      });
      const button = page.getByRole('button', { name: variant });
      await expect.element(button).toHaveAttribute('data-variant', variant);
      const style = getComputedStyle(button.element());
      looks.add(`${style.backgroundColor}/${style.color}`);
      await unmount();
    }

    // Four variants, four renderings. A variant map entry that falls back to
    // another's utilities collapses this set.
    expect(looks.size).toBe(VARIANTS.length);
  });

  // The craft lane counts primary buttons by `--primary-fill`, so a default
  // button painted in another colour hides every one of them from it.
  it('paints the default button in the primary fill', async () => {
    await render(Button, { props: { children: label('Print') } });
    const probe = document.createElement('div');
    probe.style.backgroundColor = 'var(--primary-fill)';
    document.body.append(probe);
    const fill = getComputedStyle(probe).backgroundColor;
    probe.remove();

    expect(
      getComputedStyle(page.getByRole('button', { name: 'Print' }).element())
        .backgroundColor,
    ).toBe(fill);
  });

  it('measures each size, and at least 44px for the touch size', async () => {
    const boxes = new Map<ButtonSize, DOMRect>();

    for (const size of Object.keys(sizes) as ButtonSize[]) {
      const props: ButtonProps = isIcon(size)
        ? { size, label: size, children: label('+') }
        : { size, 'aria-label': size, children: label('+') };
      const { unmount } = await render(Button, { props });
      const button = page.getByRole('button', { name: size });
      await expect.element(button).toHaveAttribute('data-size', size);
      boxes.set(size, button.element().getBoundingClientRect());
      await unmount();
    }

    expect(boxes.get('sm')!.height).toBeLessThan(boxes.get('md')!.height);
    expect(boxes.get('md')!.height).toBeLessThan(boxes.get('lg')!.height);
    expect(boxes.get('icon-md')!.width).toBe(boxes.get('icon-md')!.height);

    // 44px is the floor for a control operated standing, with a gloved hand,
    // on a wall-mounted tablet. `icon-lg` takes it under a finger (button.touch.svelte.test.ts).
    expect(boxes.get('lg')!.height).toBeGreaterThanOrEqual(44);
    expect(boxes.get('icon-lg')!.width).toBe(boxes.get('icon-lg')!.height);
    expect(boxes.get('icon-lg')!.height).toBeGreaterThanOrEqual(24);
  });

  it('keeps one radius at every size', async () => {
    const radii: string[] = [];
    for (const size of ['sm', 'lg'] as const) {
      const { unmount } = await render(Button, {
        props: { size, children: label('Print') },
      });
      const button = page.getByRole('button', { name: 'Print' });
      radii.push(getComputedStyle(button.element()).borderRadius);
      await unmount();
    }

    expect(radii[0]).toBe(radii[1]);
  });

  it('takes focus from the keyboard and paints a ring for it', async () => {
    await render(Button, { props: { children: label('Print') } });

    const button = page.getByRole('button', { name: 'Print' });
    await userEvent.tab();
    await expect.element(button).toHaveFocus();
    expect(button.element().matches(':focus-visible')).toBe(true);
    // The ring is a box shadow, so a resting button has none.
    expect(getComputedStyle(button.element()).boxShadow).not.toBe('none');
  });

  it('blocks the click while disabled', async () => {
    const onclick = vi.fn();
    await render(Button, {
      props: { onclick, disabled: true, children: label('Delete') },
    });

    const button = page.getByRole('button', { name: 'Delete' });
    await expect.element(button).toBeDisabled();
    await button.click({ force: true });
    expect(onclick).not.toHaveBeenCalled();
  });

  it('shows a pointer cursor, and no pointer once it is disabled', async () => {
    const { unmount } = await render(Button, {
      props: { children: label('Print') },
    });
    expect(getComputedStyle(page.getByRole('button').element()).cursor).toBe(
      'pointer',
    );
    await unmount();

    await render(Button, {
      props: { disabled: true, children: label('Print') },
    });
    expect(
      getComputedStyle(page.getByRole('button').element()).cursor,
    ).not.toBe('pointer');
  });

  it('answers a press in every variant, with a fill of its own', () => {
    const pressed = new Set<string>();

    for (const variant of VARIANTS) {
      const press = utility(variant, 'active:bg-');
      expect(press).toBeDefined();
      const fill = declaredFill(press!);
      expect(fill).toBeDefined();
      pressed.add(fill!);

      const resting = utility(variant, 'bg-');
      expect(declaredFill(resting!)).not.toBe(fill);
    }

    // A tablet has no hover, so a press is the only state left to acknowledge
    // a command with. A Print that looks the same for the second the command
    // takes gets tapped twice, and the second tap queues a second job.
    expect(pressed.size).toBe(VARIANTS.length);
  });

  it('gives ghost a resting colour of its own', async () => {
    const { container } = await render(Button, {
      props: { variant: 'ghost', children: label('Details') },
    });

    const reference = document.createElement('span');
    reference.className = 'text-muted-foreground';
    container.appendChild(reference);
    const colour = getComputedStyle(
      page.getByRole('button', { name: 'Details' }).element(),
    ).color;
    const muted = getComputedStyle(reference).color;
    const ambient = getComputedStyle(container).color;
    reference.remove();

    // Ambient text is coloured already, so a comparison against black or
    // transparent would miss a ghost that inherits the wrong colour.
    expect(colour).toBe(muted);
    expect(colour).not.toBe(ambient);
  });

  it('hands its attributes to the child snippet, which draws its own element', async () => {
    let handed: Record<string, unknown> | undefined;
    const link = createRawSnippet<[{ props: Record<string, unknown> }]>(
      (argument) => {
        handed = argument().props;
        return {
          render: () =>
            `<a href="#printers" class="${base} ${radii.sm} ${variants.default} ${sizes.md}">Printers</a>`,
        };
      },
    );

    const { container } = await render(Button, {
      props: { child: link, type: 'submit' },
    });

    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toBeInTheDocument();
    expect(container.querySelector('button')).toBeNull();
    expect(handed?.['data-slot']).toBe('button');
    expect(handed?.['data-variant']).toBe('default');
    expect((handed?.class as unknown[]).filter(Boolean)).toEqual([
      base,
      radii.sm,
      variants.default,
      sizes.md,
    ]);
    // The plain button defaults to `type="button"`. A snippet draws whatever
    // element it likes, so it gets the caller's `type` as it was given.
    expect(handed?.type).toBe('submit');
  });

  describe('a square button', () => {
    const pause = () =>
      render(Button, {
        props: {
          size: 'icon-md',
          label: 'Pause the print',
          children: label('⏸'),
        },
      });

    it('is named by its label and shows it on hover', async () => {
      await pause();

      const button = page.getByRole('button', { name: 'Pause the print' });
      await userEvent.hover(button);

      await expect.poll(tooltip).not.toBe(null);
      expect(tooltip()?.textContent).toContain('Pause the print');
      // The tooltip repeats the name, so the button is not described by it too.
      expect(button.element().hasAttribute('aria-describedby')).toBe(false);
    });

    it('shows the label on keyboard focus', async () => {
      await pause();

      await userEvent.tab();
      await expect
        .element(page.getByRole('button', { name: 'Pause the print' }))
        .toHaveFocus();
      await expect.poll(tooltip).not.toBe(null);
      expect(tooltip()?.textContent).toContain('Pause the print');
    });

    it('stays quiet when focus is handed back after a pointer press', async () => {
      await pause();

      const button = page.getByRole('button', { name: 'Pause the print' });
      await button.click();
      await userEvent.unhover(button);
      button.element().blur();
      await expect.poll(tooltip).toBe(null);

      // A dialog or a menu does this when it closes.
      button.element().focus();

      await expect.element(button).toHaveFocus();
      await new Promise((resolve) => setTimeout(resolve, 100));
      expect(tooltip()).toBe(null);
    });

    it('still opens the menu it is the trigger of', async () => {
      await render(IconMenuProbe);

      const button = page.getByRole('button', { name: 'More actions' });
      await button.click();

      await expect.element(page.getByRole('menu')).toBeInTheDocument();
      await expect.element(button).toHaveAttribute('data-state', 'open');
      await expect.element(button).toHaveAttribute('aria-expanded', 'true');
    });

    it('keeps the tooltip shut over its open menu, so focus reaches the items', async () => {
      await render(IconMenuProbe);

      const button = page.getByRole('button', { name: 'More actions' });
      button.element().focus();
      await userEvent.keyboard('{Enter}');
      await expect.element(page.getByRole('menuitem')).toHaveFocus();

      await userEvent.hover(button, { force: true });
      await new Promise((resolve) => setTimeout(resolve, 100));

      // Focus that arrived from the keyboard opened the tooltip, and the Enter
      // shut it again; its exit animation may still be on the page.
      expect(tooltip()?.dataset.state ?? 'closed').toBe('closed');
      await expect.element(page.getByRole('menuitem')).toHaveFocus();
      expect(button.element().hasAttribute('data-disabled')).toBe(false);
    });
  });

  it('has no accessibility violation', async () => {
    const { container } = await render(Button, {
      props: { children: label('Print') },
    });

    await expectNoViolations(container);
  });

  it('keeps an aria-disabled button focusable', async () => {
    await render(Button, {
      props: { 'aria-disabled': 'true', children: label('Restart') },
    });
    const button = page.getByRole('button', { name: 'Restart' });
    await userEvent.tab();
    await expect.element(button).toHaveFocus();
    expect(button.element().getAttribute('tabindex')).not.toBe('-1');
  });

  it('draws a link with no box and the surrounding font size', async () => {
    await render(Button, {
      props: { variant: 'link', children: label('Guide') },
    });
    const button = page.getByRole('button', { name: 'Guide' });
    await expect.element(button).toHaveAttribute('data-variant', 'link');
    const style = getComputedStyle(button.element());
    expect(style.paddingLeft).toBe('0px');
    expect(style.fontSize).toBe(
      getComputedStyle(button.element().parentElement!).fontSize,
    );
  });
});

describe('Button link with an icon', () => {
  it('lays the icon and the words out in a row with a gap', async () => {
    await render(Button, {
      props: { variant: 'link', size: 'inline-icon', children: label('Guide') },
    });
    const style = getComputedStyle(
      page.getByRole('button', { name: 'Guide' }).element(),
    );
    expect(style.display).toBe('inline-flex');
    expect(parseFloat(style.columnGap)).toBeGreaterThan(0);
    expect(style.paddingLeft).toBe('0px');
  });
});
