<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import fixtures from '../../../test/ux/fixtures/api.json';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({ title: 'Screens/Settings/Server' });

  const READINESS = 'GET /api/v1/readiness';
  const CONNECTORS = 'GET /api/v1/health/connectors';

  const ready = {
    app: 'prusa-connect',
    blocked: null,
    declared: 4,
    instances: [],
    ready: true,
    serving: 4,
  };

  const ui = JSON.parse(fixtures['GET /api/v1/ui'].body) as Record<
    string,
    unknown
  >;
  const safeUi = {
    ...ui,
    modules: false,
    safe_mode: { reason: 'restart', since: '2026-10-30T09:12:00Z' },
  };
  // core.upgrade.enabled off: the server answers 404 for every upgrade and
  // backup read, so the screen reads the version alone.
  const upgradesOffUi = { ...ui, upgrades: false };

  // What the restart panel waits for: the maintenance read answers, and the
  // person is an administrator.
  const VERSION = 'GET /api/v1/server/version';
  const maintenance = {
    [VERSION]: () =>
      json(200, {
        build_profile: 'server',
        contract: '1.0.0',
        goarch: 'arm64',
        goos: 'linux',
        host_schema_compat: 10,
        host_schema_version: 12,
        managed_externally: false,
        version: '1.2.0',
      }),
    'GET /api/v1/server/upgrade': () => json(404, {}),
    'GET /api/v1/server/upgrade/releases': () => json(200, []),
    'GET /api/v1/server/upgrade/history': () => json(200, []),
    'GET /api/v1/server/backups': () => json(200, []),
  };

  const connector = (over: Record<string, unknown>) => ({
    connector: 'prusalink',
    counts: { connected: 3 },
    degraded: null,
    degraded_truncated: 0,
    status: 'pass',
    total: 3,
    ...over,
  });
</script>

<!--
  States the screen branches on: ready, not ready (the core answers 503 with
  the report), a degraded connector, loading and failed.
  Updates Off is a server with core.upgrade.enabled off: no Updates and no
  Backups tab, a version card that is not a link, and the restart still
  offered. Updates Off Address is a bookmark of the Updates tab there, which
  answers not found.
  The Overview tab's restart panel is Refused for a person who is not an
  administrator: the maintenance read answers 403 and the panel offers no
  restart. The health tab refuses no signed-in person: the readiness check
  needs no identity, and the connector health counts only the printers the
  person may see.
-->

<Story name="Overview">
  <AppAt
    path="/settings/server/overview"
    overrides={{
      ...maintenance,
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () =>
        json(200, {
          items: [
            connector({
              counts: { connected: 2, disconnected: 1 },
              status: 'degraded',
            }),
          ],
        }),
    }}
  />
</Story>

<Story name="Restart Allowed">
  <AppAt
    path="/settings/server/overview"
    overrides={{
      ...maintenance,
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Safe Mode">
  <AppAt
    path="/settings/server/overview"
    overrides={{
      ...maintenance,
      'GET /api/v1/ui': () => json(200, safeUi),
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Updates Off">
  <AppAt
    path="/settings/server/overview"
    overrides={{
      [VERSION]: maintenance[VERSION],
      'GET /api/v1/ui': () => json(200, upgradesOffUi),
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Updates Off Address">
  <AppAt
    path="/settings/server/updates"
    overrides={{
      'GET /api/v1/ui': () => json(200, upgradesOffUi),
    }}
  />
</Story>

<Story name="Refused">
  <AppAt
    path="/settings/server/overview"
    overrides={{
      ...maintenance,
      [VERSION]: refused,
      'GET /api/v1/ui': () => json(200, safeUi),
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Healthy">
  <AppAt
    path="/settings/server/health"
    overrides={{
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Not Ready">
  <AppAt
    path="/settings/server/health"
    overrides={{
      [READINESS]: () =>
        json(503, {
          ...ready,
          ready: false,
          serving: 2,
          blocked: ['module/fleet-stats', 'connector/prusalink'],
        }),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
    }}
  />
</Story>

<Story name="Degraded Connector">
  <AppAt
    path="/settings/server/health"
    overrides={{
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () =>
        json(200, {
          items: [
            connector({
              counts: { connected: 5, error: 2, disconnected: 1 },
              degraded: [
                {
                  id: 'printer-3',
                  attempts: 4,
                  reason: 'connection refused',
                  since: '2026-10-22T09:00:00.000Z',
                  state: 'error',
                },
              ],
              status: 'degraded',
              total: 8,
            }),
          ],
        }),
    }}
  />
</Story>

<Story name="Loading">
  <AppAt
    path="/settings/server/health"
    overrides={{ [READINESS]: pending, [CONNECTORS]: pending }}
  />
</Story>

<Story name="Failed">
  <AppAt
    path="/settings/server/health"
    overrides={{ [READINESS]: failed, [CONNECTORS]: failed }}
  />
</Story>

<Story name="Network">
  <AppAt
    path="/settings/server/network"
    overrides={{
      [READINESS]: () => json(200, ready),
      [CONNECTORS]: () => json(200, { items: [connector({})] }),
      'GET /api/v1/discovery/visibility': () =>
        json(200, {
          hidden: false,
          announcement: 'announcing',
          name: 'Workshop',
          host: 'workshop-server.local',
          port: 8080,
          services: ['_prusalink._tcp', '_prusa-connect._tcp'],
          addresses: ['192.168.1.10'],
          bridged: false,
        }),
      'GET /api/v1/time-server': () =>
        json(200, { enabled: true, state: 'serving', port: 123 }),
    }}
  />
</Story>
