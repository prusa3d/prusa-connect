import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import SettingsAccountScreen from './settings-account-screen.svelte';

it('measures the account form', async () => {
  const { container } = await render(SettingsAccountScreen, {
    props: {
      source: {
        principal: { id: 'user:alice', grants: [] },
        status: 'signed-in',
      },
      onChangePassword: () => undefined,
    },
  });
  const pane = container.querySelector<HTMLElement>(
    '[data-slot="settings-pane"]',
  )!;
  const input = container.querySelector<HTMLInputElement>(
    '[data-slot="input"]',
  )!;
  const label = container.querySelector<HTMLElement>('label')!;
  const header = pane.firstElementChild as HTMLElement;
  expect(input.getBoundingClientRect().height).toBe(36);
  expect(getComputedStyle(input).fontSize).toBe('13.9994px');
  expect(getComputedStyle(input).borderRadius).toBe('3px');
  expect(getComputedStyle(label).fontSize).toBe('13.9994px');
  expect(header.getBoundingClientRect().height).toBe(36);
  expect(getComputedStyle(input.closest('form')!).gap).toBe('20px');
  expect(getComputedStyle(input).fontFamily).toContain('Inter');
});

it.each([320, 770])('keeps the fields readable at %ipx', async (width) => {
  const { container } = await render(SettingsAccountScreen, {
    props: {
      source: {
        principal: { id: `user:${'a'.repeat(180)}`, grants: [] },
        status: 'signed-in',
      },
      identity: {
        username: 'a'.repeat(180),
        displayName: 'Long name '.repeat(10),
        role: 'admin',
        createdAt: '2026-08-19T17:05:00Z',
      },
      onChangePassword: () => undefined,
    },
  });
  container.style.width = `${String(width)}px`;
  await new Promise<void>((resolve) =>
    requestAnimationFrame(() => {
      resolve();
    }),
  );
  const pane = container.querySelector<HTMLElement>(
    '[data-slot="settings-pane"]',
  )!;
  const inputs = [
    ...container.querySelectorAll<HTMLInputElement>('[data-slot="input"]'),
  ];
  const submit = container.querySelector<HTMLButtonElement>(
    'button[type="submit"]',
  )!;
  expect(submit.getBoundingClientRect().right).toBe(
    inputs[0]!.getBoundingClientRect().right,
  );
  expect(pane.getBoundingClientRect().width).toBe(width);
  expect(pane.scrollWidth).toBeLessThanOrEqual(pane.clientWidth);
  for (const [index, input] of inputs.entries()) {
    const box = input.getBoundingClientRect();
    expect(box.height).toBe(36);
    expect(box.width).toBeGreaterThanOrEqual(width === 320 ? 260 : 280);
    expect(box.width).toBeLessThanOrEqual(310);
    if (index > 0)
      expect(box.y).toBeGreaterThan(
        inputs[index - 1]!.getBoundingClientRect().bottom,
      );
  }
  await userEvent.click(page.getByLabelText('New password'));
  await userEvent.keyboard('short');
  const field = inputs[1]!.closest('[data-slot="field"]')!;
  const error = field.querySelector<HTMLElement>('[role="alert"]')!;
  await expect.element(page.getByRole('alert')).toBeVisible();
  expect(error.getBoundingClientRect().top).toBeGreaterThanOrEqual(
    inputs[1]!.getBoundingClientRect().bottom,
  );
  expect(pane.scrollWidth).toBeLessThanOrEqual(pane.clientWidth);
});

it('uses the flat Figma fill in dark and preserves keyboard focus', async () => {
  document.documentElement.classList.add('dark');
  try {
    const { container } = await render(SettingsAccountScreen, {
      props: {
        source: {
          principal: { id: 'user:alice', grants: [] },
          status: 'signed-in',
        },
        onChangePassword: () => undefined,
      },
    });
    const input = container.querySelector<HTMLInputElement>(
      '[data-slot="input"]',
    )!;
    const style = getComputedStyle(input);
    expect(style.backgroundColor).toBe('rgb(42, 42, 42)');
    expect(style.borderColor).toBe(style.backgroundColor);
    expect(style.borderRadius).toBe('3px');
    expect(input.getBoundingClientRect().height).toBe(36);
    input.focus();
    expect(input.matches(':focus-visible')).toBe(true);
    expect(getComputedStyle(input).boxShadow).not.toBe('none');
    await userEvent.click(page.getByLabelText('New password'));
    await userEvent.keyboard('short');
    const invalid = container.querySelector<HTMLInputElement>(
      '[aria-invalid="true"]',
    )!;
    await userEvent.hover(page.getByLabelText('New password'));
    await expect
      .poll(() => getComputedStyle(invalid).borderColor)
      .toBe('rgb(229, 72, 77)');
  } finally {
    document.documentElement.classList.remove('dark');
  }
});

it.each([320, 800, 960, 1440])(
  'wraps the account cards at %ipx',
  async (width) => {
    const { container } = await render(SettingsAccountScreen, {
      props: {
        source: {
          principal: { id: 'user:alice', grants: [] },
          status: 'signed-in',
        },
        identity: {
          displayName: 'Alice',
          username: 'alice',
          role: 'admin',
          createdAt: '2026-08-19T17:05:00Z',
        },
        onChangePassword: () => undefined,
      },
    });
    container.style.width = `${String(width)}px`;
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => {
        resolve();
      }),
    );
    const panels = container.querySelector('[data-slot="account-panels"]')!;
    const cards = [...panels.children];
    expect(cards).toHaveLength(2);
    const first = cards[0]!.getBoundingClientRect();
    const second = cards[1]!.getBoundingClientRect();
    if (width < 916) {
      expect(second.top - first.bottom).toBeCloseTo(10, 1);
      for (const card of cards) {
        const contentHeight = [...card.children].reduce(
          (sum, child) => sum + child.getBoundingClientRect().height,
          0,
        );
        expect(card.getBoundingClientRect().height).toBeCloseTo(
          contentHeight,
          1,
        );
      }
    } else {
      expect(second.top).toBe(first.top);
      expect(second.height).toBe(first.height);
      expect(second.left - first.right).toBeCloseTo(20, 1);
    }
    for (const card of cards)
      expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
  },
);
