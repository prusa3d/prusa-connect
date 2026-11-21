// Which screen answers which path.
import type { Component } from 'svelte';

import type {
  Accounts,
  AppPairingReview,
  AppSettings,
  Cameras,
  Credentials,
  Dashboards,
  DeviceIssues,
  DeviceModels,
  DeviceStates,
  DiscoveryVisibility,
  AllowedOrigins,
  TimeServerSwitch,
  Fleet,
  FleetListing,
  JobDetail as JobRecord,
  Maintenance,
  Modules,
  NearbyPrinters,
  Preferences,
  PrinterStorages,
  RunningTransfers,
  ServerHealth,
  ServerRestart,
  Session,
  Storage,
  Transfers,
} from '@prusa/connect-ui/data';
import { FEATURES, type PrintAction } from '@prusa/connect-ui/domain';
import type { LeaveCheck, RouteParams } from '@prusa/connect-ui/router';
import type { ModuleLoader, SlotRegistry } from '@prusa/connect-ui/slots';

import { type AppPath, PATHS } from './routes';
import type { AppVersion } from './stores/app-version.svelte';
import type { Jobs } from './stores/jobs.svelte';
import type { LeftDatabases } from './stores/left-databases.svelte';
import type { ModuleCatalogue } from './stores/module-catalogue.svelte';
import type { ModuleOperations } from './stores/module-operations.svelte';
import type { Registrations } from './stores/registrations.svelte';
import type { FractalShares } from './stores/fractal-shares.svelte';
import type { FractalStores } from './stores/fractal-stores.svelte';
import type { Sends } from './stores/sends.svelte';
import Account from './screens/account.svelte';
import AddPrinter from './screens/add-printer.svelte';
import DashboardPage from './screens/dashboard.svelte';
import Appearance from './screens/appearance.svelte';
import ApiCredentials from './screens/credentials.svelte';
import AppPairingPage from './screens/app-pairing.svelte';
import ExtensionDetail from './screens/extension-detail.svelte';
import PersonAccess from './screens/person-access.svelte';
import Diagnostics from './screens/diagnostics.svelte';
import Extensions from './screens/extensions.svelte';
import FileDetail from './screens/file-detail.svelte';
import Home from './screens/home.svelte';
import JobDetail from './screens/job-detail.svelte';
import JobsPage from './screens/jobs.svelte';
import MaintenancePage from './screens/maintenance.svelte';
import OldNetworkAddress from './screens/old-network-address.svelte';
import Server from './screens/server.svelte';
import ServerAddress from './screens/server-address.svelte';
import ModulePageScreen from './screens/module-page.svelte';
import People from './screens/people.svelte';
import NotFound from './screens/not-found.svelte';
import OldFileAddress from './screens/old-file-address.svelte';
import OldPeopleAddress from './screens/old-people-address.svelte';
import PrinterDetail from './screens/printer-detail.svelte';
import Printers from './screens/printers.svelte';
import StorageFiles from './screens/storage.svelte';
import Tags from './screens/tags.svelte';

export interface AppContext {
  fleet: Fleet;
  /** Address of a preview for a file on the printer. */
  deviceThumbnail?:
    ((deviceId: string, path: string) => string | undefined) | undefined;
  /** The states each device kind's adapter declared. */
  deviceStates: DeviceStates;
  /** What each reason a printer can carry an issue means. */
  deviceIssues: DeviceIssues;
  /** Which models have separate toolheads. */
  deviceModels: DeviceModels;
  /** What this server has installed, and the pages each module declares. */
  modules: Modules;
  /** The dashboards this person sees, and every write to them (ADR-20261008-sww). */
  dashboards: Dashboards;
  /** What draws the modules, which a dashboard's cards are drawn through. */
  slots: { registry: SlotRegistry; loader: ModuleLoader };
  /** What this server can install, and the installs and removals started here. */
  moduleCatalogue: ModuleCatalogue;
  /** What a module instance did lately: its memory history and log. */
  moduleOperations: ModuleOperations;
  /** The databases extensions left behind, and their purge (ADR-20261021-bqc). */
  leftDatabases: LeftDatabases;
  registrations: Registrations;
  /** How this deployment's page looks, and the address printers dial. */
  settings: AppSettings;
  /** The build this server runs, as the footer names it. */
  appVersion: AppVersion;
  /** Who this browser is signed in as, and the password change. */
  session: Session;
  /** The people who can sign in to this server. */
  accounts: Accounts;
  /** The API credentials the signed-in person holds. */
  credentials: Credentials;
  /** The pairing an app asked for, which the signed-in person decides. */
  appPairing: AppPairingReview;
  /** Every camera this server serves, and the address of each one's frame. */
  cameras: Cameras;
  /** The printers this server hears on its network. */
  nearby: NearbyPrinters;
  /** Whether this server hides itself from the printers on its network. */
  discovery: DiscoveryVisibility;
  /** Whether this server answers the time on its network. */
  timeServer: TimeServerSwitch;
  allowedOrigins: AllowedOrigins;
  /** Whether this server is ready, and how the printers behind each connector stand. */
  health: ServerHealth;
  /** Signed core releases, the current update transaction and protected backups. */
  maintenance: Maintenance;
  /** Restarting the server, by hand, into safe mode or out of it. */
  restart: ServerRestart;
  /** What every printer on this server has printed. */
  jobs: Jobs;
  /** Builds the store of one print, found by its id. */
  jobDetail: (jobId: string) => JobRecord;
  /** Builds the store of one page of the fleet, as the server filters and orders it. */
  fleetListing: () => FleetListing;
  /** This server's own files: the storages it declares and what is in them. */
  storage: Storage;
  /** The folders this server shares with the Fractal servers below it. */
  fractalShares: FractalShares;
  /** The folders the Fractal servers above and below share with this one. */
  fractalStores: FractalStores;
  /** The one store of a printer's file transfers, shared by every page. */
  transfers: (deviceId: string) => Transfers;
  /** What each printer said its storages are, for the print picker. */
  printerStorages: PrinterStorages;
  /** Every file the server is sending to a printer the person may read. */
  running: RunningTransfers;
  /** The files this tab is putting on printers, on every page. */
  sends: Sends;
  preferences: Preferences;
  /** Starts a print, from a storage of this server or from the printer's own files. */
  print: PrintAction;
  /** The parts the matched pattern named. */
  params: RouteParams;
  path: string;
  /** The query of the address, `?` included, or empty. */
  search: string;
  /** The fragment of the address, `#` included, or empty. */
  hash: string;
  /**
   * Goes somewhere this application serves. A path it does not serve is
   * refused. `replace` takes the current history entry instead of adding one.
   */
  go: (path: string, options?: { replace?: boolean }) => void;
  /**
   * Asks `check` before a link or the back button leaves the screen, such as
   * one holding changes nobody saved. Returns the call that stops asking.
   */
  checkLeave: (check: LeaveCheck) => () => void;
}

export type AppScreen = Component<{ ctx: AppContext }>;

export interface ScreenRoute {
  /** Typed against `PATHS`, so a screen cannot answer a path the matcher never learned. */
  pattern: AppPath;
  component: AppScreen;
}

export const SCREENS: ScreenRoute[] = [
  // The root is a decision rather than a page.
  { pattern: PATHS.home, component: Home },
  { pattern: PATHS.dashboards, component: DashboardPage },
  { pattern: PATHS.dashboard, component: DashboardPage },
  { pattern: PATHS.printers, component: Printers },
  { pattern: PATHS.printersAdd, component: AddPrinter },
  { pattern: PATHS.printerDetail, component: PrinterDetail },
  { pattern: PATHS.printerDetailTab, component: PrinterDetail },
  { pattern: PATHS.jobs, component: JobsPage },
  { pattern: PATHS.printerJobs, component: JobsPage },
  { pattern: PATHS.jobDetail, component: JobDetail },
  { pattern: PATHS.storage, component: StorageFiles },
  { pattern: PATHS.storageOldFile, component: OldFileAddress },
  { pattern: PATHS.storageRoot, component: StorageFiles },
  { pattern: PATHS.storageFolder, component: StorageFiles },
  { pattern: PATHS.storageFile, component: FileDetail },
  // ADR-20260930-rq4.
  ...(FEATURES.tags ? [{ pattern: PATHS.tags, component: Tags }] : []),
  { pattern: PATHS.settings, component: Account },
  { pattern: PATHS.settingsAccount, component: Account },
  { pattern: PATHS.settingsAppearance, component: Appearance },
  { pattern: PATHS.settingsTokens, component: ApiCredentials },
  { pattern: PATHS.pair, component: AppPairingPage },
  { pattern: PATHS.settingsPeople, component: People },
  { pattern: PATHS.personAccess, component: PersonAccess },
  { pattern: PATHS.settingsOldPeople, component: OldPeopleAddress },
  { pattern: PATHS.settingsExtensions, component: Extensions },
  { pattern: PATHS.extensionDetail, component: ExtensionDetail },
  { pattern: PATHS.extensionDetailTab, component: ExtensionDetail },
  { pattern: PATHS.settingsOldNetwork, component: OldNetworkAddress },
  { pattern: PATHS.settingsServer, component: ServerAddress },
  { pattern: PATHS.settingsServerOverview, component: Server },
  { pattern: PATHS.settingsServerHealth, component: Server },
  { pattern: PATHS.settingsServerUpdates, component: Server },
  { pattern: PATHS.settingsServerBackups, component: Server },
  { pattern: PATHS.settingsServerNetwork, component: Server },
  { pattern: PATHS.settingsMaintenance, component: MaintenancePage },
  ...(FEATURES.diagnostics
    ? [{ pattern: PATHS.settingsDiagnostics, component: Diagnostics }]
    : []),
  { pattern: PATHS.modulePage, component: ModulePageScreen },
];

/** The screen for a pattern, or the one that says this build answers none. */
export const screenFor = (pattern: string | undefined): AppScreen =>
  SCREENS.find((route) => route.pattern === pattern)?.component ?? NotFound;
