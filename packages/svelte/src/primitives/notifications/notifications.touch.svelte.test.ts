import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import NotificationsLive from '../../test/notifications-live.svelte';

const panel = (): HTMLElement | null =>
  document.querySelector<HTMLElement>(
    '[data-slot="popover-content"], [data-slot="sheet-content"]',
  );

describe('Notifications under a finger', () => {
  it('stands Mark all read on the 44px floor under a finger', async () => {
    await render(NotificationsLive);
    await page.getByRole('button', { name: 'Notifications, 2 unread' }).click();
    await expect.poll(panel).not.toBe(null);
    const style = getComputedStyle(
      page.getByRole('button', { name: 'Mark all read' }).element(),
    );

    expect(parseFloat(style.height)).toBeGreaterThanOrEqual(44);
  });
});
