import { afterEach, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { expectNoViolations } from '../../test/axe';
import Screen from './settings-account-screen.svelte';
import type { SettingsAccountScreenProps } from './types';

const props: SettingsAccountScreenProps = {
  source: { principal: { id: 'user:alice', grants: [] }, status: 'signed-in' },
  identity: {
    displayName: 'Alice Doe',
    username: 'alice',
    role: 'admin',
    createdAt: '2026-08-19T17:05:00Z',
  },
  onChangePassword: () => undefined,
};

afterEach(() => {
  document.documentElement.classList.remove('dark');
});

it.each([false, true])(
  'emphasizes identity values and keeps account metadata quiet (dark=%s)',
  async (dark) => {
    document.documentElement.classList.toggle('dark', dark);
    await render(Screen, { props });
    const style = (text: string): CSSStyleDeclaration =>
      getComputedStyle(page.getByText(text, { exact: true }).element());
    expect(parseFloat(style('Alice Doe').fontSize)).toBeCloseTo(16, 2);
    expect(style('Alice Doe').fontWeight).toBe('600');
    for (const value of ['alice', 'Administrator']) {
      expect(parseFloat(style(value).fontSize)).toBeCloseTo(13, 2);
      expect(style(value).fontWeight).toBe('600');
    }
    for (const label of ['Username', 'Role']) {
      expect(parseFloat(style(label).fontSize)).toBeCloseTo(12, 2);
      expect(style(label).fontWeight).toBe('400');
      expect(style(label).color).not.toBe(style('Administrator').color);
    }
    for (const metadata of ['Added', 'Signed in as', 'user:alice']) {
      expect(parseFloat(style(metadata).fontSize)).toBeCloseTo(12, 2);
      expect(style(metadata).fontWeight).toBe('400');
      expect(style(metadata).color).toBe(style('Username').color);
    }
  },
);

it.each([true, false])(
  'keeps related values close and separates metadata (creation date=%s)',
  async (withDate) => {
    const { container } = await render(Screen, {
      props: {
        ...props,
        identity: {
          ...props.identity,
          createdAt: withDate ? props.identity?.createdAt : undefined,
        },
      },
    });
    container.style.width = '770px';
    const row = (label: string): HTMLElement =>
      page
        .getByText(label, { exact: true })
        .element()
        .closest<HTMLElement>('[data-slot="settings-row"]')!;
    const username = row('Username');
    const labelBox = username.firstElementChild!.getBoundingClientRect();
    const valueBox = username.lastElementChild!.getBoundingClientRect();
    expect(labelBox.width).toBe(110);
    expect(valueBox.left - labelBox.right).toBe(12);
    const roleBox = row('Role').getBoundingClientRect();
    const subjectBox = row('Signed in as').getBoundingClientRect();
    const metadataBox = withDate
      ? row('Added').getBoundingClientRect()
      : subjectBox;
    expect(metadataBox.top - roleBox.bottom).toBeGreaterThanOrEqual(20);
    if (withDate) {
      expect(subjectBox.top - metadataBox.bottom).toBe(10);
      const date = row('Added').lastElementChild!.firstElementChild!;
      expect(parseFloat(getComputedStyle(date).fontSize)).toBeCloseTo(12, 2);
      expect(getComputedStyle(date).fontWeight).toBe('400');
    }
  },
);

it.each([320, 480, 512, 770])(
  'lays out identity without overflow at %ipx',
  async (width) => {
    const { container } = await render(Screen, { props });
    container.style.width = `${String(width)}px`;
    const pane = container.querySelector<HTMLElement>(
      '[data-slot="settings-pane"]',
    )!;
    const layout = container.querySelector<HTMLElement>(
      '[data-slot="account-identity-layout"]',
    )!;
    const avatar = container.querySelector<HTMLElement>(
      '[data-slot="account-avatar"]',
    )!;
    const details = layout.querySelector(
      '[data-slot="account-identity-details"]',
    )!;
    const imageBox = avatar.getBoundingClientRect();
    const detailsBox = details.getBoundingClientRect();
    expect(imageBox.width).toBe(96);
    expect(imageBox.height).toBe(96);
    expect(avatar.closest('[aria-hidden="true"]')).not.toBeNull();
    expect(pane.scrollWidth).toBeLessThanOrEqual(pane.clientWidth);
    if (width >= 512) expect(imageBox.right).toBeLessThan(detailsBox.left);
    else expect(imageBox.bottom).toBeLessThan(detailsBox.top);
    await expectNoViolations(container);
  },
);

it('uses name initials and recovers from a broken photo to a loaded photo', async () => {
  const { container, rerender } = await render(Screen, {
    props: {
      ...props,
      identity: {
        ...props.identity,
        avatarUrl: 'data:image/png;base64,invalid',
      },
    },
  });
  const fallback = (): HTMLElement =>
    container.querySelector('[data-slot="account-avatar-fallback"]')!;
  await expect.poll(() => fallback().textContent.trim()).toBe('AD');
  const photo =
    'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';
  await rerender({
    ...props,
    identity: { ...props.identity, avatarUrl: photo },
  });
  await expect
    .poll(
      () =>
        container.querySelector<HTMLImageElement>(
          '[data-slot="account-avatar-image"]',
        )?.naturalWidth,
    )
    .toBe(1);
  expect(container.querySelector('img')?.getAttribute('alt')).toBe('');
  await rerender({ ...props, identity: { username: 'marta.novakova' } });
  await expect.poll(() => fallback().textContent.trim()).toBe('MN');
});

it('shows a skeleton while identity is loading', async () => {
  const { container } = await render(Screen, {
    props: { ...props, identityLoading: true },
  });
  expect(container.querySelector('[data-slot="account-avatar"]')).toBeNull();
  expect(container.querySelectorAll('[role="status"]').length).toBe(1);
});
