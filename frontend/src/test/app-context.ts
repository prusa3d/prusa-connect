// The application's own context, built over the fetch stub, so a screen test
// names the two or three fields its assertion is about.
//
// The stores are the real ones. A screen reads what a store answers, and a
// hand-written double of a store would prove the double works.
import {
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
  JobDetail,
  Maintenance,
  Modules,
  NearbyPrinters,
  Preferences,
  PrinterStorages,
  RunningTransfers,
  ServerHealth,
  ServerRestart,
  Session,
  startPrint,
  Storage,
} from '@prusa/connect-ui/data';
import { createModuleLoader, SlotRegistry } from '@prusa/connect-ui/slots';
import { bind } from 'fractal-client';

import type { AppContext } from '../lib/screens';
import { AppVersion } from '../lib/stores/app-version.svelte';
import { FractalShares } from '../lib/stores/fractal-shares.svelte';
import { FractalStores } from '../lib/stores/fractal-stores.svelte';
import { Jobs } from '../lib/stores/jobs.svelte';
import { LeftDatabases } from '../lib/stores/left-databases.svelte';
import { ModuleCatalogue } from '../lib/stores/module-catalogue.svelte';
import { ModuleOperations } from '../lib/stores/module-operations.svelte';
import { Registrations } from '../lib/stores/registrations.svelte';
import { Sends } from '../lib/stores/sends.svelte';
import { transferStores } from '../lib/stores/transfer-stores';
import { type Routes, stubClient } from './stub';

export const appContext = (
  routes: Routes = {},
  over: Partial<AppContext> = {},
): AppContext => {
  const { client } = stubClient(routes);
  const fleet = new Fleet(client);
  const registry = new SlotRegistry();
  const settings = new AppSettings(client);
  const storage = new Storage(client);
  const transfers = transferStores(client);
  const running = new RunningTransfers(client);

  return {
    fleet,
    deviceStates: new DeviceStates(client),
    deviceIssues: new DeviceIssues(client),
    deviceModels: new DeviceModels(client),
    jobs: new Jobs(client, fleet),
    jobDetail: (jobId) => new JobDetail(client, jobId),
    fleetListing: () => new FleetListing(client),
    session: new Session(client),
    accounts: new Accounts(client),
    cameras: new Cameras(client),
    nearby: new NearbyPrinters(client),
    discovery: new DiscoveryVisibility(client),
    timeServer: new TimeServerSwitch(client),
    allowedOrigins: new AllowedOrigins(client),
    health: new ServerHealth(client),
    maintenance: new Maintenance(client, undefined, {
      upgrades: () => settings.upgrades,
    }),
    restart: new ServerRestart(client),
    credentials: new Credentials(client),
    appPairing: new AppPairingReview(client),
    dashboards: new Dashboards(client),
    slots: {
      registry,
      loader: createModuleLoader({ client, registry, fleet }),
    },
    modules: new Modules(client),
    // The catalogue reads the key the way the app wires it, so a case
    // that answers GET /api/v1/ui with modules off sees what the page does.
    moduleCatalogue: new ModuleCatalogue(
      client,
      undefined,
      () => settings.modules,
      () => settings.safeMode !== undefined,
    ),
    moduleOperations: new ModuleOperations(client),
    leftDatabases: new LeftDatabases(client),
    registrations: new Registrations(client),
    settings,
    appVersion: new AppVersion(client),
    storage,
    fractalShares: new FractalShares(bind(client)),
    fractalStores: new FractalStores(bind(client)),
    transfers,
    printerStorages: new PrinterStorages(client),
    running,
    sends: new Sends(storage, transfers, running),
    preferences: new Preferences(),
    print: (request) => startPrint(client, request),
    params: {},
    path: '/',
    search: '',
    hash: '',
    go: () => undefined,
    checkLeave: () => () => undefined,
    ...over,
  };
};
