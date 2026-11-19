// The Overview tab: one line per part of the server, each leading to the
// tab that says more.
import { expect, test } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { PAIRING_NEEDED_FAULT } from '../../data';
import { expectNoViolations } from '../../test/axe';
import { overviewOf } from './overview-fixtures';
import ServerOverview from './server-overview.svelte';

test('says how each part stands and leads to its tab', async () => {
  const { container } = await render(ServerOverview, {
    props: overviewOf(),
  });

  await expect.element(page.getByText('Healthy')).toBeVisible();
  await expect.element(page.getByText('2 of 3 connected')).toBeVisible();
  await expect.element(page.getByText('1 away')).toBeVisible();
  await expect.element(page.getByText('1.3.0 available')).toBeVisible();
  await expect
    .element(page.getByText('workshop-server.local:8080'))
    .toBeVisible();
  await expect
    .element(container.querySelector<HTMLElement>('[data-part="network"] a'))
    .toHaveAttribute('href', '/settings/server/network');
  await expect
    .element(container.querySelector<HTMLElement>('[data-part="backup"] a'))
    .toHaveAttribute('href', '/settings/server/backups');
  await expectNoViolations(container);
});

test('speaks of the update under way rather than the next release', async () => {
  const props = overviewOf();
  await render(ServerOverview, {
    props: {
      ...props,
      maintenance: {
        ...props.maintenance,
        upgrade: {
          ...({} as NonNullable<typeof props.maintenance.upgrade>),
          state: 'prepared',
        },
      },
    },
  });

  await expect
    .element(page.getByText('Update in progress: prepared'))
    .toBeVisible();
  await expect
    .element(page.getByText('1.3.0 available'))
    .not.toBeInTheDocument();
});

test('says so when no printer has connected and no backup exists', async () => {
  const props = overviewOf();
  await render(ServerOverview, {
    props: {
      ...props,
      health: { ...props.health, connectors: [] },
      maintenance: { ...props.maintenance, backups: [], releases: [] },
    },
  });

  await expect.element(page.getByText('No printers yet')).toBeVisible();
  await expect.element(page.getByText('No backup yet')).toBeVisible();
  await expect.element(page.getByText('No newer release')).toBeVisible();
});

test('does not claim no newer release when the list could not be read', async () => {
  const props = overviewOf();
  await render(ServerOverview, {
    props: {
      ...props,
      maintenance: {
        ...props.maintenance,
        releases: [],
        releaseError: {
          code: PAIRING_NEEDED_FAULT,
          message: 'pairing needed',
          params: {},
          status: 409,
          traceId: 't',
        },
      },
    },
  });

  await expect
    .element(page.getByText('Not checked, pair with the Beta Portal'))
    .toBeVisible();
  await expect
    .element(page.getByText('No newer release'))
    .not.toBeInTheDocument();
});

test('does not claim no newer release while the list is still read', async () => {
  const props = overviewOf();
  await render(ServerOverview, {
    props: {
      ...props,
      maintenance: { ...props.maintenance, releases: [], releaseLoading: true },
    },
  });

  await expect.element(page.getByText('1.2.0')).toBeVisible();
  await expect
    .element(page.getByText('No newer release'))
    .not.toBeInTheDocument();
});

test('names no address when the server announces nothing', async () => {
  await render(ServerOverview, {
    props: overviewOf({
      network: {
        announcement: 'off',
        described: { hidden: false, announcement: 'off', bridged: false },
        loaded: true,
        error: undefined,
      },
    }),
  });

  await expect.element(page.getByText('Announcing off')).toBeVisible();
  await expect
    .element(page.getByText('Switched off in the configuration file'))
    .toBeVisible();
  await expect.element(page.getByText(/\.local/)).not.toBeInTheDocument();
});

test('says the announcement stays inside a bridged container', async () => {
  await render(ServerOverview, {
    props: overviewOf({
      network: {
        announcement: 'announcing',
        described: {
          hidden: false,
          announcement: 'announcing',
          host: '19f634755a5c.local',
          port: 8080,
          bridged: true,
        },
        loaded: true,
        error: undefined,
      },
    }),
  });

  await expect
    .element(page.getByText('Inside the container only'))
    .toBeVisible();
  await expect.element(page.getByText(/\.local/)).not.toBeInTheDocument();
});

test('names the version alone where the server prepares no upgrade', async () => {
  const { container } = await render(ServerOverview, {
    props: { ...overviewOf(), updatesHref: undefined, backupsHref: undefined },
  });

  await expect.element(page.getByText('1.2.0')).toBeVisible();
  await expect
    .element(
      page.getByText(
        'Updates switched off by core.upgrade.enabled in the configuration file',
      ),
    )
    .toBeVisible();
  const version = container.querySelector('[data-part="version"]');
  expect(version?.querySelector('a')).toBeNull();
  expect(container.textContent).not.toContain('1.3.0 available');
  expect(container.querySelector('[data-part="backup"]')).toBeNull();
  await expectNoViolations(container);
});
