<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({ title: 'Screens/Settings/Updates & Backups' });

  const VERSION = 'GET /api/v1/server/version';
  const UPGRADE = 'GET /api/v1/server/upgrade';
  const RELEASES = 'GET /api/v1/server/upgrade/releases';
  const HISTORY = 'GET /api/v1/server/upgrade/history';
  const BACKUPS = 'GET /api/v1/server/backups';

  const version = {
    build_profile: 'server',
    contract: '1.0.0',
    goarch: 'arm64',
    goos: 'linux',
    host_schema_compat: 10,
    host_schema_version: 12,
    managed_externally: false,
    version: '1.2.0',
  };

  const common = {
    [VERSION]: () => json(200, version),
    [UPGRADE]: () => json(404, {}),
    [RELEASES]: () => json(200, []),
    [HISTORY]: () => json(200, []),
    [BACKUPS]: () => json(200, []),
    'GET /api/v1/server/beta-portal-pairing': () =>
      json(200, { state: 'unpaired' }),
  };
</script>

<Story name="Available Release">
  <AppAt
    path="/settings/server/updates"
    overrides={{
      ...common,
      [RELEASES]: () =>
        json(200, [
          {
            binary: {
              name: 'prusa-connect_linux_arm64',
              size: 42000000,
              url: '/release/core',
            },
            name: 'Prusa Connect 1.3.0',
            notes: 'Reliability improvements and safer recovery.',
            prerelease: false,
            published_at: '2026-10-25T19:14:05Z',
            signature: {
              name: 'SHA256SUMS.sig',
              size: 96,
              url: '/release/signature',
            },
            sums: {
              name: 'SHA256SUMS',
              size: 64,
              url: '/release/sums',
            },
            version: '1.3.0',
          },
        ]),
      [BACKUPS]: () =>
        json(200, [
          {
            created_at: '2026-10-25T18:14:05Z',
            files: [],
            format: 1,
            from_version: '1.2.0',
            id: 'backup-20261025T181405Z',
          },
        ]),
    }}
  />
</Story>

<Story name="Loading">
  <AppAt
    path="/settings/server/updates"
    overrides={{
      [VERSION]: pending,
      [UPGRADE]: pending,
      [RELEASES]: pending,
      [HISTORY]: pending,
      [BACKUPS]: pending,
    }}
  />
</Story>

<Story name="Failed">
  <AppAt
    path="/settings/server/updates"
    overrides={{ ...common, [VERSION]: failed }}
  />
</Story>

<Story name="Refused">
  <AppAt
    path="/settings/server/updates"
    overrides={{ ...common, [VERSION]: refused }}
  />
</Story>
