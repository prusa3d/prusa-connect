// This product's own destinations.
import type { AnnouncedPage } from '@prusa/connect-ui/data';
import { FEATURES } from '@prusa/connect-ui/domain';
import { printerHref, type RailNavItem } from '@prusa/connect-ui/shell';
import {
  moduleIcon,
  type NavGroup,
  type NavItem,
} from '@prusa/connect-ui/slots';
import { translateFor } from '@prusa/connect-ui/i18n';
import BlocksIcon from '@lucide/svelte/icons/blocks';
import CircleUserIcon from '@lucide/svelte/icons/circle-user';
import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
import KeyRoundIcon from '@lucide/svelte/icons/key-round';
import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
import LifeBuoyIcon from '@lucide/svelte/icons/life-buoy';
import ListChecksIcon from '@lucide/svelte/icons/list-checks';
import PaletteIcon from '@lucide/svelte/icons/palette';
import { PrinterGlyphIcon } from '@prusa/connect-ui/primitives';
import ServerIcon from '@lucide/svelte/icons/server';
import SettingsIcon from '@lucide/svelte/icons/settings';
import TagIcon from '@lucide/svelte/icons/tag';
import UsersIcon from '@lucide/svelte/icons/users';

import { APP_NAMESPACE } from './i18n';
import type { SharedPlace } from './stores/fractal-stores.svelte';

const t = translateFor(APP_NAMESPACE);

/** The group a module's page is drawn under. */
export const EXTENSIONS = 'extensions';

/** Every path this product answers. */
export const PATHS = {
  home: '/',
  dashboards: '/dashboards',
  dashboard: '/dashboards/:id',
  printers: '/printers',
  printersAdd: '/printers/add',
  printerDetail: '/printers/:id',
  printerDetailTab: '/printers/:id/:tab',
  jobs: '/jobs',
  printerJobs: '/jobs/printer/:id',
  jobDetail: '/jobs/:id',
  storage: '/storage',
  // The old file address comes before the storage root, so the matcher
  // takes it first.
  storageOldFile: '/storage/file',
  storageRoot: '/storage/:storage',
  storageFolder: '/storage/:storage/folder/:path',
  storageFile: '/storage/:storage/file/:path',
  tags: '/tags',
  settings: '/settings',
  settingsAccount: '/settings/account',
  settingsAppearance: '/settings/appearance',
  settingsExtensions: '/settings/extensions',
  extensionDetail: '/settings/extensions/:namespace',
  extensionDetailTab: '/settings/extensions/:namespace/:tab',
  settingsDiagnostics: '/settings/diagnostics',
  // A bookmark to this address is sent on to the Network tab of the Server
  // room.
  settingsOldNetwork: '/settings/network',
  settingsServer: '/settings/server',
  settingsServerOverview: '/settings/server/overview',
  settingsServerHealth: '/settings/server/health',
  settingsServerUpdates: '/settings/server/updates',
  settingsServerBackups: '/settings/server/backups',
  settingsServerNetwork: '/settings/server/network',
  settingsMaintenance: '/settings/maintenance',
  settingsTokens: '/settings/tokens',
  // The page an app sends a person to, with its code in the query.
  pair: '/pair',
  settingsPeople: '/settings/people',
  personAccess: '/settings/people/:id',
  // A bookmark to this address is sent on to the People room.
  settingsOldPeople: '/settings/users',
  // Every page an installed module adds.
  modulePage: '/m/:namespace/:path',
} as const;

export type AppPath = (typeof PATHS)[keyof typeof PATHS];

/**
 * The storage page at one storage and one folder in it. With no storage it is
 * the storage page as the reader last left it. A path is one segment of the
 * address, so its slashes are encoded.
 */
export const storageHref = (storage?: string, folder = ''): string => {
  if (storage === undefined) return PATHS.storage;

  const root = `${PATHS.storage}/${encodeURIComponent(storage)}`;

  return folder === '' ? root : `${root}/folder/${encodeURIComponent(folder)}`;
};

/**
 * The folder another Fractal server shares that an address opens, from its
 * `node`, `store` and `folder` parameters, and undefined for the top.
 */
export const sharedPlaceOf = (search: string): SharedPlace | undefined => {
  const query = new URLSearchParams(search);
  const node = query.get('node');
  const instance = query.get('store');
  const path = query.get('folder');

  return node && instance && path ? { node, instance, path } : undefined;
};

/**
 * An address of the storage page with a shared folder in its query, so the
 * back button returns to it. The top of the shared folders adds nothing.
 */
export const withSharedPlace = (
  path: string,
  place: SharedPlace | undefined,
): string =>
  place === undefined
    ? path
    : `${path}?${new URLSearchParams({
        node: place.node,
        store: place.instance,
        folder: place.path,
      })}`;

/** The address of one dashboard (ADR-20261008-sww rule 14). */
export const dashboardHref = (id: number): string =>
  `${PATHS.dashboards}/${String(id)}`;

/** The page of one file on one storage of this server. */
export const storageFileHref = (storage: string, path: string): string =>
  `${PATHS.storage}/${encodeURIComponent(storage)}/file/${encodeURIComponent(path)}`;

/** One person's page, with their access to every printer. */
export const personHref = (id: string): string =>
  `${PATHS.settingsPeople}/${encodeURIComponent(id)}`;

/** The page of one installed module, at one of its tabs. */
export const extensionDetailHref = (namespace: string, tab?: string): string =>
  `${PATHS.settingsExtensions}/${encodeURIComponent(namespace)}${
    tab === undefined ? '' : `/${encodeURIComponent(tab)}`
  }`;

/**
 * The Printers page narrowed to the printers that carry every one of these
 * tags. Each tag is its own `tag` parameter, so a tag needs no escaping of
 * its own beyond the query's.
 */
export const printersTaggedHref = (tags: readonly string[]): string => {
  const query = new URLSearchParams(tags.map((tag) => ['tag', tag]));

  return tags.length === 0 ? PATHS.printers : `${PATHS.printers}?${query}`;
};

/**
 * The tags an address chooses. Each `tag` parameter is one tag, and a comma
 * inside one separates several, as the device listing reads them.
 */
export const tagsOf = (search: string): string[] =>
  new URLSearchParams(search)
    .getAll('tag')
    .flatMap((value) => value.split(','))
    .map((tag) => tag.trim())
    .filter((tag) => tag !== '');

/** Where a print lands the reader: the printer it went to, or the fleet when it went to several. */
export const printedHref = (printers: readonly string[]): string =>
  printers.length === 1 && printers[0] !== undefined
    ? printerHref(printers[0])
    : PATHS.printers;

/** The Jobs page with the prints of one printer only. */
export const jobsOfPrinterHref = (printer: string): string =>
  `${PATHS.jobs}/printer/${encodeURIComponent(printer)}`;

/** The rail. `dashboards` is false on a server that leaves them out. */
export const routes = (dashboards = true): NavItem[] => [
  ...(dashboards
    ? [
        {
          id: 'dashboards',
          label: t('app.nav.dashboards'),
          href: PATHS.dashboards,
          icon: LayoutDashboardIcon,
        },
      ]
    : []),
  {
    id: 'printers',
    label: t('app.nav.printers'),
    href: PATHS.printers,
    icon: PrinterGlyphIcon,
  },
  {
    id: 'jobs',
    label: t('app.nav.jobs'),
    href: PATHS.jobs,
    icon: ListChecksIcon,
  },
  {
    id: 'storage',
    label: t('app.nav.storage'),
    href: PATHS.storage,
    icon: HardDriveIcon,
  },
  // ADR-20260930-rq4.
  ...(FEATURES.tags
    ? [
        {
          id: 'tags',
          label: t('app.nav.tags'),
          href: PATHS.tags,
          icon: TagIcon,
        },
      ]
    : []),
];

const moduleEntry = (page: AnnouncedPage): NavItem => ({
  id: `module:${page.namespace}/${page.path}`,
  label: page.title,
  href: page.href,
  group: EXTENSIONS,
  icon: moduleIcon(page.icon),
});

/**
 * One rail entry per page an installed module declares. A module showing
 * several pages gets one row under its own title, so its pages read as one
 * product among the other modules. In the open rail the row folds its pages
 * under it, and a collapsed rail lists them in a menu. The row's own address
 * is the first page's, as Settings' is its first section's.
 */
export const moduleNav = (pages: readonly AnnouncedPage[]): RailNavItem[] => {
  const byModule = new Map<string, AnnouncedPage[]>();
  for (const page of pages.filter((one) => !one.hidden)) {
    const own = byModule.get(page.namespace);
    if (own === undefined) byModule.set(page.namespace, [page]);
    else own.push(page);
  }
  return [...byModule.values()].flatMap(([first, ...rest]) => {
    if (first === undefined) return [];
    if (rest.length === 0) return [moduleEntry(first)];
    return [
      {
        ...moduleEntry(first),
        id: `module:${first.namespace}`,
        label: first.module ?? first.title,
        children: [first, ...rest].map((page) => ({
          ...moduleEntry(page),
          group: undefined,
        })),
      },
    ];
  });
};

/**
 * Settings is not one of the destinations above. `diagnostics` is false on a
 * server that leaves the bundle out.
 */
export const settingsRoute = (diagnostics = true): RailNavItem => ({
  id: 'settings',
  label: t('app.nav.settings'),
  // The first section and not the group's own path: `/settings` renders the
  // same screen, but reaches it with no section of the group marked current.
  href: PATHS.settingsAccount,
  icon: SettingsIcon,
  children: [
    {
      id: 'settings-account',
      label: t('app.nav.settings.account'),
      href: PATHS.settingsAccount,
      icon: CircleUserIcon,
    },
    {
      id: 'settings-appearance',
      label: t('app.nav.settings.appearance'),
      href: PATHS.settingsAppearance,
      icon: PaletteIcon,
    },
    {
      id: 'settings-extensions',
      label: t('app.nav.settings.extensions'),
      href: PATHS.settingsExtensions,
      icon: BlocksIcon,
    },
    {
      id: 'settings-tokens',
      label: t('app.nav.settings.tokens'),
      href: PATHS.settingsTokens,
      icon: KeyRoundIcon,
    },
    {
      id: 'settings-people',
      label: t('app.nav.settings.people'),
      href: PATHS.settingsPeople,
      icon: UsersIcon,
    },
    {
      id: 'settings-server',
      label: t('app.nav.settings.server'),
      href: PATHS.settingsServer,
      icon: ServerIcon,
    },
    ...(FEATURES.diagnostics && diagnostics
      ? [
          {
            id: 'settings-diagnostics',
            label: t('app.nav.settings.diagnostics'),
            href: PATHS.settingsDiagnostics,
            icon: LifeBuoyIcon,
          },
        ]
      : []),
  ],
});

export const navGroups = (): NavGroup[] => [
  { id: EXTENSIONS, label: t('app.nav.extensions') },
];

/** Every pattern the matcher answers, a module's page among them. */
export const PATTERNS: string[] = Object.values(PATHS);
