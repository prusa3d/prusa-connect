// The stores that hold what the signed-in person was allowed to read. Each
// one is read through the browser's cookie, and the server answers each
// person with what that person may see.
//
// They are built together so that `reset` reaches every one of them. It
// resets every store in the object below, so a store added there is reset
// with the rest.
import type { PrusaConnect } from '@prusa/connect';
import {
  AllowedOrigins,
  Accounts,
  AppPairingReview,
  Cameras,
  Credentials,
  Dashboards,
  DiscoveryVisibility,
  Fleet,
  Modules,
  NearbyPrinters,
  Notices,
  type NoticesOptions,
  PrinterStorages,
  RunningTransfers,
  ServerHealth,
  Storage,
  TimeServerSwitch,
} from '@prusa/connect-ui/data';
import { announceNotice } from '@prusa/connect-ui/shell';
import { bind } from 'fractal-client';

import { FractalShares } from './fractal-shares.svelte';
import { FractalStores } from './fractal-stores.svelte';
import { Jobs } from './jobs.svelte';
import { LeftDatabases } from './left-databases.svelte';
import { ModuleCatalogue } from './module-catalogue.svelte';
import { ModuleOperations } from './module-operations.svelte';
import { Registrations } from './registrations.svelte';
import { Sends } from './sends.svelte';
import { type TransferStores, transferStores } from './transfer-stores';

export interface PersonStores {
  /** The printers, fed by the event stream as it runs. */
  readonly fleet: Fleet;
  /** The print histories, gathered across the fleet. */
  readonly jobs: Jobs;
  /** The devices waiting to be let onto this server. */
  readonly registrations: Registrations;
  /**
   * This server's own files. One store for the app, so the storage page
   * keeps the folder a person was in after they leave the page.
   */
  readonly storage: Storage;
  /** The folders this server shares with the Fractal servers below it. */
  readonly fractalShares: FractalShares;
  /** The folders the Fractal servers above and below share with this one. */
  readonly fractalStores: FractalStores;
  /** Every printer's transfers, one store each, shared by the pages. */
  readonly transfers: TransferStores;
  /** What each printer said its storages are, read when a picker offers it. */
  readonly printerStorages: PrinterStorages;
  /**
   * Every file on its way to a printer this person may read, whoever started
   * it, kept current from the fleet's event stream.
   */
  readonly running: RunningTransfers;
  /** The files on their way to printers, this tab's own and everyone's, on every page. */
  readonly sends: Sends;
  /** The people who can sign in to this server. */
  readonly accounts: Accounts;
  /** One listing of every camera. */
  readonly cameras: Cameras;
  /** The API credentials the signed-in person holds. */
  readonly credentials: Credentials;
  /** The pairing an app asked for, which the signed-in person decides. */
  readonly appPairing: AppPairingReview;
  /** The dashboards this person sees, and the one on screen. */
  readonly dashboards: Dashboards;
  /** What this server has installed, as far as this person may see. */
  readonly modules: Modules;
  /** What this server can install, from its index and its marketplace. */
  readonly moduleCatalogue: ModuleCatalogue;
  /** What a module instance did lately: memory history and log. */
  readonly moduleOperations: ModuleOperations;
  /** The databases extensions left behind, and their purge. */
  readonly leftDatabases: LeftDatabases;
  /** The printers this server hears on its network. */
  readonly nearby: NearbyPrinters;
  /** Whether this server hides itself from the local network. */
  readonly discovery: DiscoveryVisibility;
  /** Whether this server answers the time on the local network. */
  readonly timeServer: TimeServerSwitch;
  /** The browser origins that may call this server. */
  readonly allowedOrigins: AllowedOrigins;
  /** Whether this server is ready, as far as this person may see. */
  readonly health: ServerHealth;
  /** What the bell lists while the app is open. */
  readonly notices: Notices;
  /** Forgets what every store here read, and drops the answers on their way. */
  reset(): void;
}

/**
 * `principal` answers who is signed in now. The stores that open an event
 * stream name that person to it. `notices` is how the bell reads what the
 * app knows besides the stream: how this server admits printers, and the
 * names of models. Its `onNotice` defaults to the library's toasts.
 */
export const personStores = (
  client: PrusaConnect,
  principal: () => string | undefined = () => undefined,
  // What follows an install, a removal or a write to a module: the app
  // passes the refresh that also reloads the slots.
  modulesChanged?: () => Promise<void> | void,
  notices: NoticesOptions = {},
  // Whether the server runs the modules it installed (ADR-20261019-h76). Off, the
  // module catalogue reads no index and no marketplace.
  modulesOn?: () => boolean,
  safeMode?: () => boolean,
): PersonStores => {
  const fleet = new Fleet(client);
  const modules = new Modules(client);
  const leftDatabases = new LeftDatabases(client);
  const storage = new Storage(client);
  const transfers = transferStores(client, principal);
  const running = new RunningTransfers(client);
  const fractal = bind(client);
  const stores = {
    fleet,
    jobs: new Jobs(client, fleet),
    registrations: new Registrations(client),
    storage,
    fractalShares: new FractalShares(fractal),
    fractalStores: new FractalStores(fractal),
    transfers,
    printerStorages: new PrinterStorages(client),
    running,
    sends: new Sends(storage, transfers, running),
    accounts: new Accounts(client),
    cameras: new Cameras(client),
    credentials: new Credentials(client),
    appPairing: new AppPairingReview(client),
    dashboards: new Dashboards(client),
    modules,
    moduleCatalogue: new ModuleCatalogue(
      client,
      // A removal, and an update that moves a database, leave one behind.
      async () => {
        await Promise.all([
          modulesChanged?.() ?? modules.load({ fresh: true }),
          leftDatabases.load(),
        ]);
      },
      modulesOn,
      safeMode,
    ),
    moduleOperations: new ModuleOperations(client),
    leftDatabases,
    nearby: new NearbyPrinters(client),
    discovery: new DiscoveryVisibility(client),
    timeServer: new TimeServerSwitch(client),
    allowedOrigins: new AllowedOrigins(client),
    health: new ServerHealth(client),
    notices: new Notices({ onNotice: announceNotice, ...notices }),
  };

  return {
    ...stores,
    reset: () => {
      for (const store of Object.values(stores)) store.reset();
    },
  };
};

/** What the app forgets when a person leaves, and where the next one starts. */
export interface Leaving {
  readonly stores: PersonStores;
  /** The module loader, which admitted modules for the person who left. */
  readonly loader: { reset(): void };
  /** The router, which still shows the address the person left open. */
  readonly router: { replace(path: string): void };
  /** Where the next person starts. */
  readonly home: string;
}

/**
 * Forgets what the person who left could read and the modules admitted for
 * them, and sends the page home. The address they left open may name what
 * only they could see.
 */
export const forgetThePerson = ({
  stores,
  loader,
  router,
  home,
}: Leaving): void => {
  stores.reset();
  loader.reset();
  router.replace(home);
};
