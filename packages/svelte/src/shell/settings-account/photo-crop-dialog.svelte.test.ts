import { afterEach, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { expectNoViolations } from '../../test/axe';
import Editor from './photo-crop-dialog.svelte';
import Screen from './settings-account-screen.svelte';

async function sample(): Promise<File> {
  const canvas = document.createElement('canvas');
  canvas.width = 400;
  canvas.height = 200;
  const ctx = canvas.getContext('2d')!;
  ctx.fillStyle = 'red';
  ctx.fillRect(0, 0, 200, 200);
  ctx.fillStyle = 'blue';
  ctx.fillRect(200, 0, 200, 200);
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((value) => {
      if (value === null) {
        reject(new Error('no PNG'));
        return;
      }
      resolve(value);
    });
  });
  return new File([blob], 'sample.png', { type: 'image/png' });
}
const props = {
  source: {
    principal: { id: 'user:alice', grants: [] },
    status: 'signed-in' as const,
  },
  identity: { displayName: 'Alice Doe' },
  onChangePassword: () => undefined,
};
afterEach(() => {
  vi.restoreAllMocks();
});

it('discards an unfinished crop when the account changes', async () => {
  const { container, rerender } = await render(Screen, { props });
  await page
    .elementLocator(
      container.querySelector<HTMLInputElement>('input[type=file]')!,
    )
    .upload(await sample());
  await expect.element(page.getByRole('dialog')).toBeVisible();
  await rerender({
    ...props,
    source: { ...props.source, principal: { id: 'user:bob', grants: [] } },
  });
  await expect.poll(() => document.querySelector('[role=dialog]')).toBeNull();
  await rerender(props);
  expect(document.querySelector('[role=dialog]')).toBeNull();
});

it('moves pixels with keyboard and pointer, zooms, and exports the confirmed crop', async () => {
  const confirm = vi.fn();
  await render(Editor, {
    props: {
      file: await sample(),
      onConfirm: confirm,
      onCancel: () => undefined,
    },
  });
  const viewport = document.querySelector<HTMLButtonElement>(
    '[data-slot="avatar-crop-viewport"]',
  )!;
  await expect.poll(() => viewport.disabled).toBe(false);
  const canvas = viewport.querySelector('canvas')!;
  const center = (): number[] =>
    Array.from(canvas.getContext('2d')!.getImageData(256, 256, 1, 1).data);
  viewport.focus();
  await userEvent.keyboard('{ArrowRight}');
  await expect.poll(() => center()[0]).toBe(255);
  expect(center()[2]).toBe(0);
  vi.spyOn(viewport, 'setPointerCapture').mockImplementation(() => undefined);
  viewport.dispatchEvent(
    new PointerEvent('pointerdown', {
      pointerId: 1,
      button: 0,
      clientX: 100,
      clientY: 100,
      bubbles: true,
    }),
  );
  viewport.dispatchEvent(
    new PointerEvent('pointermove', {
      pointerId: 1,
      buttons: 1,
      clientX: 250,
      clientY: 100,
      bubbles: true,
    }),
  );
  viewport.dispatchEvent(
    new PointerEvent('pointerup', { pointerId: 1, bubbles: true }),
  );
  document.querySelector<HTMLElement>('[role=slider]')!.focus();
  await userEvent.keyboard('{End}');
  expect(
    document.querySelector('[role=slider]')?.getAttribute('aria-valuenow'),
  ).toBe('3');
  await expect.poll(() => center()[0]).toBe(255);
  expect(canvas.getContext('2d')!.getImageData(0, 0, 1, 1).data[3]).toBe(0);
  await expectNoViolations(document.querySelector('[role=dialog]')!);
  await page.getByRole('button', { name: 'Use photo', exact: true }).click();
  await expect.poll(() => confirm.mock.calls.length).toBe(1);
  const file = confirm.mock.calls[0]![0] as File;
  const url = URL.createObjectURL(file);
  const output = new Image();
  output.src = url;
  await output.decode();
  expect(output.naturalWidth).toBe(512);
  expect(output.naturalHeight).toBe(512);
  const check = document.createElement('canvas');
  check.width = 512;
  check.height = 512;
  const ctx = check.getContext('2d')!;
  ctx.drawImage(output, 0, 0);
  expect(Array.from(ctx.getImageData(256, 256, 1, 1).data)).toEqual([
    255, 0, 0, 255,
  ]);
  URL.revokeObjectURL(url);
});

it('keeps the previous avatar on cancel and ignores a late export callback', async () => {
  const file = await sample();
  const { container } = await render(Screen, { props });
  const input = container.querySelector<HTMLInputElement>('input[type=file]')!;
  await page.elementLocator(input).upload(file);
  await expect.element(page.getByRole('dialog')).toBeVisible();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(container.querySelector('[role=status]')).toBeNull();
  await page.elementLocator(input).upload(file);
  const viewport = document.querySelector<HTMLButtonElement>(
    '[data-slot="avatar-crop-viewport"]',
  )!;
  await expect.poll(() => viewport.disabled).toBe(false);
  let exported: BlobCallback | undefined;
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(
    (callback) => {
      exported = callback;
    },
  );
  await page.getByRole('button', { name: 'Use photo', exact: true }).click();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  exported?.(new Blob(['later'], { type: 'image/png' }));
  expect(container.querySelector('[role=status]')).toBeNull();
  expect(
    container.querySelector('[data-slot="account-avatar-fallback"]')
      ?.textContent,
  ).toContain('AD');
});
