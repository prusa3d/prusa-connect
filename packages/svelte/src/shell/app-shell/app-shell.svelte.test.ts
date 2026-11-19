// Two primary navigation landmarks are in the markup at once and CSS picks
// one, so the test checks the accessibility tree rather than the DOM: exactly
// one of them is reachable at each width. The footer carries a third,
// unrelated `nav` for its own links, so the query is scoped to the name the
// rail and the phone bar share.
//
// The width is set on the shell's own container rather than the viewport,
// because the breakpoint is a container query.
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { t } from '../../i18n';
import type { NavItem } from '../../slots';
import AppShell from './app-shell.svelte';

const ITEMS: NavItem[] = [
  { id: 'printers', label: 'Printers', href: '/printers' },
];

const body = createRawSnippet(() => ({
  render: () => `<p>The fleet</p>`,
}));

const tall = createRawSnippet(() => ({
  render: () => `<div style="height: 300vh"></div>`,
}));

const mount = (width: string, children = body) => {
  const host = document.createElement('div');
  host.style.width = width;
  document.body.append(host);
  // `target` is a Svelte mount option, so it belongs beside `props` in the
  // second argument. The third argument only reaches `baseElement`, and a
  // `target` placed there is silently ignored, which mounts the shell into a
  // fresh element at document.body instead of one sized for this test.
  return render(AppShell, {
    target: host,
    props: {
      productName: 'Prusa Connect',
      navItems: ITEMS,
      currentPath: '/printers',
      connection: 'online',
      children,
    },
  });
};

describe('the frame', () => {
  it('offers a skip to the main landmark as the first Tab stop', async () => {
    await mount('80rem');
    const skip = page.getByRole('link', { name: t('shell.skipToContent') });
    // Above the screen until it has focus, inside it once it does.
    expect(skip.element().getBoundingClientRect().bottom).toBeLessThanOrEqual(
      0,
    );

    await userEvent.tab();
    await expect.element(skip).toHaveFocus();
    await expect
      .poll(() => skip.element().getBoundingClientRect().top)
      .toBeGreaterThanOrEqual(0);

    await userEvent.keyboard('{Enter}');
    await expect.element(page.getByRole('main')).toHaveFocus();
  });

  it('draws one navigation landmark on a wide container', async () => {
    await mount('80rem');

    const rails = page.getByRole('navigation', { name: t('shell.nav') }).all();

    expect(rails).toHaveLength(1);
  });

  it('draws one navigation landmark on a narrow container', async () => {
    await mount('30rem');

    const bars = page.getByRole('navigation', { name: t('shell.nav') }).all();

    expect(bars).toHaveLength(1);
  });

  it('measures the header once for one size', async () => {
    await mount('80rem');

    await expect
      .poll(() =>
        document.documentElement.style.getPropertyValue('--app-header-height'),
      )
      .not.toBe('');
  });

  it('keeps the phone bar on the bottom edge of a tall page', async () => {
    await mount('30rem', tall);
    window.scrollTo(0, 0);

    const bar = page
      .getByRole('navigation', { name: t('shell.nav') })
      .element()
      .getBoundingClientRect();

    expect(bar.bottom).toBe(window.innerHeight);
  });
});
