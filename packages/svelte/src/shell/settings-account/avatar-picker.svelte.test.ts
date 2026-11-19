import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import { expectNoViolations } from '../../test/axe';
import Screen from './settings-account-screen.svelte';

const props = {
  source: {
    principal: { id: 'user:alice', grants: [] },
    status: 'signed-in' as const,
  },
  identity: { displayName: 'Alice Doe', username: 'alice' },
  onChangePassword: () => undefined,
};
const image = (): File =>
  new File(
    [
      Uint8Array.from(
        atob('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'),
        (c) => c.charCodeAt(0),
      ),
    ],
    'portrait.gif',
    { type: 'image/gif' },
  );

afterEach(() => {
  vi.restoreAllMocks();
});

it('exposes the edit control to keyboard and opens the native file input', async () => {
  const { container } = await render(Screen, { props });
  const input =
    container.querySelector<HTMLInputElement>('input[type="file"]')!;
  const click = vi.spyOn(input, 'click').mockImplementation(() => undefined);
  const button = container.querySelector<HTMLButtonElement>(
    '[data-slot="avatar-edit"]',
  )!;
  const overlay = container.querySelector<HTMLElement>(
    '[data-slot="avatar-edit-overlay"]',
  )!;
  const pencil = overlay.querySelector('img')!;
  expect(button.closest('[aria-hidden="true"]')).toBeNull();
  expect(button.getBoundingClientRect().width).toBe(96);
  expect(
    parseFloat(getComputedStyle(button).borderRadius),
  ).toBeGreaterThanOrEqual(48);
  expect(pencil.getBoundingClientRect().width).toBe(16);
  button.focus();
  await expect.poll(() => getComputedStyle(overlay).opacity).toBe('1');
  await userEvent.keyboard('{Enter}');
  expect(click).toHaveBeenCalledTimes(1);
  await expectNoViolations(container);
});

it('previews locally, allows a repeated selection, and releases blob URLs', async () => {
  const revoke = vi.spyOn(URL, 'revokeObjectURL');
  const { container, unmount, rerender } = await render(Screen, { props });
  const input =
    container.querySelector<HTMLInputElement>('input[type="file"]')!;
  await page.elementLocator(input).upload(image());
  await page.getByRole('button', { name: 'Use photo', exact: true }).click();
  await expect
    .element(page.getByRole('status'))
    .toHaveTextContent(
      'Preview only. This photo is not saved to your account.',
    );
  await expect
    .poll(
      () =>
        container.querySelector<HTMLImageElement>(
          '[data-slot="account-avatar-image"]',
        )?.naturalWidth,
    )
    .toBe(512);
  const first = container.querySelector('img')!.src;
  expect(first).toMatch(/^blob:/);
  expect(input.value).toBe('');
  await page.elementLocator(input).upload(image());
  await page.getByRole('button', { name: 'Use photo', exact: true }).click();
  await expect.poll(() => revoke).toHaveBeenCalledWith(first);
  await rerender({
    ...props,
    source: { ...props.source, principal: { id: 'user:bob', grants: [] } },
  });
  expect(container.querySelector('[role="status"]')).toBeNull();
  expect(container.querySelector('img')?.getAttribute('src') ?? '').not.toMatch(
    /^blob:/,
  );
  await unmount();
  expect(revoke.mock.calls.length).toBeGreaterThanOrEqual(2);
});

it('rejects a file that is not an image without creating a preview', async () => {
  const { container } = await render(Screen, { props });
  const input =
    container.querySelector<HTMLInputElement>('input[type="file"]')!;
  await page
    .elementLocator(input)
    .upload(new File(['plain'], 'note.txt', { type: 'text/plain' }));
  await expect
    .element(page.getByRole('alert'))
    .toHaveTextContent('Choose a JPEG, PNG, WebP or GIF image.');
  expect(
    container.querySelector('[data-slot="account-avatar-fallback"]')
      ?.textContent,
  ).toContain('AD');
});

it('reports an image that cannot be decoded after a successful selection', async () => {
  const { container } = await render(Screen, { props });
  const input =
    container.querySelector<HTMLInputElement>('input[type="file"]')!;
  await page.elementLocator(input).upload(image());
  await page.getByRole('button', { name: 'Use photo', exact: true }).click();
  await expect
    .poll(
      () =>
        container.querySelector<HTMLImageElement>(
          '[data-slot="account-avatar-image"]',
        )?.naturalWidth,
    )
    .toBe(512);
  await page
    .elementLocator(input)
    .upload(new File(['broken'], 'broken.png', { type: 'image/png' }));
  await expect
    .poll(() => document.querySelector('[role="alert"]')?.textContent.trim(), {
      timeout: 2000,
    })
    .toBe('This image could not be opened. Choose another photo.');
});
