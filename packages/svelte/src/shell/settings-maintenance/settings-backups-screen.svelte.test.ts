import { expect, test } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { maintenanceSource } from './story-source';
import SettingsBackupsScreen from './settings-backups-screen.svelte';

const POSTGRES_ACK =
  'I confirm that the PostgreSQL databases outside this backup are backed up.';

test('asks for no PostgreSQL acknowledgement where the backup covers every database', async () => {
  await render(SettingsBackupsScreen, {
    props: { source: maintenanceSource() },
  });

  await expect
    .element(page.getByRole('button', { name: 'Create backup' }))
    .toBeEnabled();
  expect(page.getByRole('checkbox', { name: POSTGRES_ACK }).query()).toBeNull();
});

test('asks for the PostgreSQL acknowledgement where a database lies outside the backup', async () => {
  const base = maintenanceSource();
  await render(SettingsBackupsScreen, {
    props: {
      source: maintenanceSource({
        version: { ...base.version!, postgres_backup_ack_required: true },
      }),
    },
  });

  await expect
    .element(page.getByRole('checkbox', { name: POSTGRES_ACK }))
    .toBeVisible();
});

test('shows only the version that made a backup that has no update target', async () => {
  await render(SettingsBackupsScreen, {
    props: {
      source: maintenanceSource({
        backups: [
          {
            id: '20261123T104715Z-1.2.0',
            created_at: '2026-11-23T10:47:15Z',
            format: 1,
            from_version: '1.2.0',
            files: [],
          },
          {
            id: '20261123T104800Z-1.2.0-to-1.3.0',
            created_at: '2026-11-23T10:48:00Z',
            from_version: '1.2.0',
            to_version: '1.3.0',
            format: 1,
            files: [],
          },
        ],
      }),
    },
  });

  const backups = page.getByRole('listitem');
  await expect
    .element(backups.nth(0))
    .toMatchTextContent('files · 1.2.0 Restore');
  await expect.element(backups.nth(0)).not.toMatchTextContent('→');
  await expect
    .element(backups.nth(1))
    .toMatchTextContent('files · 1.2.0 → 1.3.0 Restore');
});
