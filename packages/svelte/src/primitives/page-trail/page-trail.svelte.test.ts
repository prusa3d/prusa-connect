import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import PageTrail from './page-trail.svelte';

const trail = [
  { label: 'USB Drive', href: '#/storage/usb' },
  { label: 'production', href: '#/storage/usb/folder/production' },
];

describe('PageTrail', () => {
  it('draws every ancestor as a link, outermost first', async () => {
    await render(PageTrail, {
      props: { trail, page: 'left-bracket-v4.bgcode' },
    });

    const links = [...document.querySelectorAll('a')].map((link) =>
      link.textContent.trim(),
    );
    expect(links).toEqual(['USB Drive', 'production']);
  });

  // A screen reader that reports the current page as a link offers a person a
  // press that goes nowhere.
  it('names the current page and does not link it', async () => {
    await render(PageTrail, {
      props: { trail, page: 'left-bracket-v4.bgcode' },
    });

    const marker = document.querySelector('[data-slot="breadcrumb-page"]');
    expect(marker?.getAttribute('aria-current')).toBe('page');
    expect(marker?.textContent.trim()).toBe('left-bracket-v4.bgcode');
    expect(
      page.getByRole('link', { name: 'left-bracket-v4.bgcode' }).elements(),
    ).toHaveLength(0);
  });

  // Going up is a state change on this screen, not a navigation, so the crumb
  // must not announce itself as a link or offer "open in new tab".
  it('draws a crumb that changes state as a button', async () => {
    const onSelect = vi.fn();
    await render(PageTrail, {
      props: {
        trail: [{ label: 'Search results', onSelect }],
        page: 'benchy.gcode',
      },
    });

    const crumb = page.getByRole('button', { name: 'Search results' });
    await expect
      .element(crumb)
      .toHaveClass('hover:text-foreground', { exact: false });
    await crumb.click();

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(document.querySelectorAll('a')).toHaveLength(0);
  });

  it('keeps the page in the sans face when it is a file name', async () => {
    const { unmount } = await render(PageTrail, {
      props: { trail, page: 'left-bracket-v4.bgcode', sansPage: true },
    });

    const marker = () =>
      document.querySelector('[data-slot="breadcrumb-page"]')?.className ?? '';
    expect(marker()).toContain('font-sans');
    await unmount();

    await render(PageTrail, { props: { trail, page: 'MK4S' } });
    expect(marker()).not.toContain('font-sans');
  });

  it('reaches every ancestor by keyboard, in the order of the trail', async () => {
    await render(PageTrail, {
      props: { trail, page: 'left-bracket-v4.bgcode' },
    });

    await userEvent.tab();
    await expect
      .element(page.getByRole('link', { name: 'USB Drive' }))
      .toHaveFocus();

    await userEvent.tab();
    await expect
      .element(page.getByRole('link', { name: 'production' }))
      .toHaveFocus();
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(PageTrail, {
      props: { trail, page: 'left-bracket-v4.bgcode' },
    });

    await expectNoViolations(container);
  });
});
