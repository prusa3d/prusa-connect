<script lang="ts">
  import { untrack } from 'svelte';
  import { browserStorage, forgetRemembered, theme } from '@prusa/connect-ui';
  import { operationTable } from '@prusa/connect';
  import {
    AppSettings,
    DeviceIssues,
    deviceThumbnailUrlOf,
    DeviceModels,
    DeviceStates,
    downloadDiagnostics,
    FleetListing,
    JobDetail,
    Maintenance,
    type Notice,
    Preferences,
    ServerRestart,
    Session,
    SetupFlow,
    startPrint,
  } from '@prusa/connect-ui/data';
  import { translateFor } from '@prusa/connect-ui/i18n';
  import { Router, type RouterNavigation } from '@prusa/connect-ui/router';
  import { homeOn } from '@prusa/connect-ui/domain';
  import {
    Alert,
    AlertDescription,
    Button,
    setDeclaredModels,
  } from '@prusa/connect-ui/primitives';
  import {
    announceNotice,
    AppShell,
    jobHref,
    printerHref,
    SafeModeBanner,
    SignInScreen,
  } from '@prusa/connect-ui/shell';
  import {
    createModuleLoader,
    setSlotContext,
    SlotRegistry,
    withModuleNav,
  } from '@prusa/connect-ui/slots';

  import { client, onRefusal } from './lib/client';
  import FirstRun from './lib/first-run.svelte';
  import { saveBlob } from './lib/download';
  import { entries, log, navigate } from './lib/log';
  import { APP_NAMESPACE, chooseLocale } from './lib/i18n';
  import {
    extensionDetailHref,
    moduleNav,
    navGroups,
    PATHS,
    PATTERNS,
    routes,
    settingsRoute,
  } from './lib/routes';
  import PersonScreen from './lib/person-screen.svelte';
  import { readRestartAccess } from './lib/restart-access';
  import { AppVersion } from './lib/stores/app-version.svelte';
  import { type AppContext, type AppScreen, screenFor } from './lib/screens';
  import { LiveUpdates } from './lib/stores/live-updates.svelte';
  import { debounced, moduleRefresh } from './lib/stores/module-refresh';
  import { forgetThePerson, personStores } from './lib/stores/person-stores';
  import {
    addressAfterExpiry,
    addressAfterSignIn,
    rereadOnRefusal,
    searchBeforeSignIn,
    signInFailureOf,
    withoutSignInFailure,
  } from './lib/stores/session-expiry';
  import {
    forgetEachPerson,
    whileSignedIn,
  } from './lib/stores/signed-in.svelte';

  const t = translateFor(APP_NAMESPACE);
  const crashLog = log.scope('app-boundary');
  let downloadFailed = $state(false);
  const logCrash = (error: unknown): void => {
    crashLog.error(error instanceof Error ? error.message : String(error), {
      stack: error instanceof Error ? error.stack : undefined,
    });
  };

  const {
    navigation,
    useServerAppearance = false,
    deviceThumbnail = (deviceId, path) =>
      deviceThumbnailUrlOf(client, deviceId, path),
  }: {
    navigation?: RouterNavigation;
    useServerAppearance?: boolean;
    deviceThumbnail?: NonNullable<AppContext['deviceThumbnail']>;
  } = $props();
  const router = new Router({
    known: PATTERNS,
    navigation: untrack(() => navigation),
  });
  // The server sends a sign-in at the provider that did not finish back here
  // with its reason. The sign-in says it once, and a reload does not.
  const failureOnLoad = signInFailureOf(router.search);
  let signInFailure = $state(failureOnLoad);
  if (failureOnLoad !== undefined) {
    router.replace(withoutSignInFailure(router.path, router.search));
  }
  const Screen = $derived(screenFor(router.pattern));
  // The boundary around a screen that failed, kept so moving to another
  // address served by the same screen draws it again. A move to another
  // screen remounts the boundary instead, and this one is dropped.
  let crashed:
    { screen: AppScreen; path: string; reset: () => void } | undefined;
  const screenFailed = (error: unknown, reset: () => void): void => {
    logCrash(error);
    crashed = { screen: Screen, path: router.path, reset };
  };
  $effect(() => {
    const screen = Screen;
    const path = router.path;
    if (crashed === undefined || crashed.path === path) return;
    const { screen: failed, reset } = crashed;
    crashed = undefined;
    if (failed === screen) reset();
  });
  const settings = new AppSettings(client);

  // A skin an older build left in this browser's storage is deleted.
  forgetRemembered(browserStorage(), 'skin');

  $effect(() => {
    if (!useServerAppearance || !settings.asked) return;
    const preferredTheme = settings.theme;
    untrack(() => {
      theme.useDefault(preferredTheme);
    });
  });
  const appVersion = new AppVersion(client);
  const maintenance = new Maintenance(client, undefined, {
    upgrades: () => settings.upgrades,
  });
  // One store for the banner and for the Server room's panel, so a restart
  // either of them started shows in both.
  const restart = new ServerRestart(client);
  let restartMayRun = $state<boolean | undefined>(undefined);
  // What the adapters this server installed say a machine of each kind can do.
  const deviceStates = new DeviceStates(client);
  // What each reason a printer can carry an issue means.
  const deviceIssues = new DeviceIssues(client);
  // Which models change tools, so a printer's page draws a row per toolhead.
  const deviceModels = new DeviceModels(client);
  const preferences = new Preferences();
  // The root draws the dashboard when that is the home page, so the rail
  // marks Dashboards there.
  const railPath = $derived(
    router.path === PATHS.home &&
      homeOn(preferences.home, settings.dashboards) === 'dashboard'
      ? PATHS.dashboards
      : router.path,
  );
  const session = new Session(client);
  // The first-run wizard's conversation with the server (ADR-20261015-ejz).
  const setupFlow = new SetupFlow(client);
  // What the signed-in person was allowed to read, forgotten when the person
  // changes.
  const person = personStores(
    client,
    () => session.principal?.id,
    () => refreshModules(),
    {
      admitsOnArrival: () => settings.printerAdmission === 'automatic',
      modelName: (model) => deviceModels.entryOf(model)?.display_name,
      // The add-printer page already shows a printer asking, so a toast
      // there would say it twice.
      onNotice: (notice) => {
        if (
          notice.kind === 'printer-pending' &&
          router.path === PATHS.printersAdd
        ) {
          return;
        }
        announceNotice(notice);
      },
    },
    () => settings.modules,
    () => settings.safeMode !== undefined,
  );
  // Where a notice in the bell leads.
  const noticeHref = (notice: Notice): string | undefined => {
    if (
      notice.kind === 'printer-pending' ||
      notice.kind === 'printer-refused'
    ) {
      return PATHS.printersAdd;
    }
    if (notice.kind === 'module-switched-off') {
      const namespace = person.modules.items.find(
        (item) => item.name === notice.module,
      )?.namespace;

      return namespace === undefined
        ? undefined
        : extensionDetailHref(namespace);
    }
    if (
      (notice.kind === 'printer-connected' ||
        notice.kind === 'printer-error' ||
        notice.kind === 'printer-attention' ||
        notice.kind === 'printer-offline') &&
      notice.deviceId !== undefined
    ) {
      return printerHref(notice.deviceId);
    }
    if (
      (notice.kind === 'job-finished' || notice.kind === 'job-failed') &&
      notice.jobId !== undefined
    ) {
      return jobHref(notice.jobId);
    }

    return undefined;
  };
  // The session tells the live updates who a new cookie belongs to, so a
  // password change keeps the stream and another person's sign-in drops it.
  // LiveUpdates feeds events to the fleet store as they arrive.
  // A burst of transitions (stopping, stopped, starting, ready) is one read.
  const refreshSoon = debounced(() => {
    void refreshModules();
    // A listing's rows take each module's instance from the catalogue, so a
    // change made in another tab reaches its switch only through this read.
    if (person.moduleCatalogue.loaded) void person.moduleCatalogue.load();
  }, 250);
  const refreshMaintenanceSoon = debounced(() => {
    if (maintenance.loaded) void maintenance.load();
  }, 250);
  const live = new LiveUpdates(
    client,
    person.fleet,
    session,
    refreshSoon,
    refreshMaintenanceSoon,
    () => {
      void person.dashboards.refresh();
      void person.dashboards.reload();
    },
  );

  // What draws the modules this server has installed.
  const registry = new SlotRegistry();
  const loader = createModuleLoader({ client, registry, fleet: person.fleet });
  const refreshModules = moduleRefresh({
    modules: person.modules,
    loader,
    registry,
  });

  // The outlets find both here rather than in a module-level singleton.
  setSlotContext({ registry, loader });

  // A printer icon with no artwork of its own reads the vocabulary's name for
  // the model and the drawing a driver declared for it (ADR-20261016-qrp).
  setDeclaredModels((reported) => deviceModels.declared(reported));

  const host = $derived(window.location.host);

  const ctx: AppContext = {
    deviceThumbnail: (deviceId, path) => deviceThumbnail(deviceId, path),
    get fleet() {
      return person.fleet;
    },
    get deviceStates() {
      return deviceStates;
    },
    get deviceIssues() {
      return deviceIssues;
    },
    get deviceModels() {
      return deviceModels;
    },
    get modules() {
      return person.modules;
    },
    get dashboards() {
      return person.dashboards;
    },
    slots: { registry, loader },
    get moduleCatalogue() {
      return person.moduleCatalogue;
    },
    get moduleOperations() {
      return person.moduleOperations;
    },
    get leftDatabases() {
      return person.leftDatabases;
    },
    get registrations() {
      return person.registrations;
    },
    get appVersion() {
      return appVersion;
    },
    get settings() {
      return settings;
    },
    get session() {
      return session;
    },
    get accounts() {
      return person.accounts;
    },
    get credentials() {
      return person.credentials;
    },
    get appPairing() {
      return person.appPairing;
    },
    get cameras() {
      return person.cameras;
    },
    get nearby() {
      return person.nearby;
    },
    get discovery() {
      return person.discovery;
    },
    get timeServer() {
      return person.timeServer;
    },
    get health() {
      return person.health;
    },
    get maintenance() {
      return maintenance;
    },
    restart,
    get jobs() {
      return person.jobs;
    },
    jobDetail: (jobId) => new JobDetail(client, jobId),
    fleetListing: () => new FleetListing(client),
    get storage() {
      return person.storage;
    },
    get fractalShares() {
      return person.fractalShares;
    },
    get fractalStores() {
      return person.fractalStores;
    },
    transfers: person.transfers,
    get printerStorages() {
      return person.printerStorages;
    },
    get running() {
      return person.running;
    },
    get sends() {
      return person.sends;
    },
    get preferences() {
      return preferences;
    },
    print: (request) => startPrint(client, request),
    get params() {
      return router.params;
    },
    get path() {
      return router.path;
    },
    get search() {
      return router.search;
    },
    get hash() {
      return router.hash;
    },
    go: (path: string, options?: { replace?: boolean }) => {
      if (options?.replace) router.replace(path);
      else router.path = path;
    },
    checkLeave: (check) => router.checkLeave(check),
  };

  $effect(() => router.listen());
  // One entry per screen: only the template is read, so a move between two
  // printers' pages is not a second entry.
  $effect(() => {
    navigate(router.pattern);
  });

  // A 401 from any call asks the session who this browser is. The stream
  // ending with `auth.expired` reaches the same question through `live`.
  onRefusal(rereadOnRefusal(session));

  // A server with no decision recorded answers `core.auth/SETUP_PENDING`.
  const firstRun = $derived(session.setupState?.state === 'pending');

  $effect(() => {
    void session.readSetup().then(() => session.read());
  });

  // One person's data is never drawn for the next, and neither are the
  // modules admitted for them. This runs before the work below starts again
  // for the new person.
  forgetEachPerson(session, () => {
    forgetThePerson({
      stores: person,
      loader,
      router,
      home:
        session.ended === 'refused'
          ? addressAfterExpiry(PATHS.home, router.path, router.search)
          : PATHS.home,
    });
    maintenance.reset();
  });

  // The banner offers Restart normally to an administrator. One read behind
  // the restart's own gate says who that is, so a person who is not one costs
  // a single refusal rather than the Server room's five reads.
  $effect(() => {
    restartMayRun = undefined;
    if (session.status !== 'signed-in' || settings.safeMode === undefined) {
      return;
    }
    let current = true;
    void readRestartAccess(client).then((allowed) => {
      if (current) restartMayRun = allowed;
    });

    return () => {
      current = false;
    };
  });

  // A restart wait outlives the page that started it, since the banner
  // shows it on every page. It ends only with the app.
  $effect(() => () => {
    restart.cancel();
  });

  // Nothing is fetched until there is a session to fetch it with. When
  // another person signs in from another tab, this runs again for them.
  whileSignedIn(session, () => {
    void settings.read();
    void appVersion.load();
    // The bell names a printer asking to join by its model.
    void deviceModels.read();
    void deviceIssues.read();
    void person.modules.load();

    // The bell hears what the fleet applies, so it costs no second stream.
    // An issue a printer already had when the fleet was read is not news.
    const unwatch = person.notices.watch(person.fleet);
    // The printers asking to join are applied from the same stream, so the
    // add-printer page never needs a reload to see one.
    const unwatchRegistrations = person.registrations.watch(person.fleet);
    // A USB drive put in or pulled out is applied from the same stream.
    const unwatchStorages = person.printerStorages.watch(person.fleet);
    // Every file on its way to a printer this person may read, whoever sent
    // it, read once and kept current from the same stream.
    const unwatchRunning = person.running.watch(person.fleet);
    void person.running.load();
    void person.fleet.load().then(() => {
      person.notices.seed(person.fleet.devices);
    });
    const close = live.open();
    // A tab shown after a long sleep never draws an old list of waiting
    // printers.
    const onVisible = (): void => {
      if (document.visibilityState !== 'visible') return;
      if (person.registrations.loaded) void person.registrations.load();
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      unwatch();
      unwatchRegistrations();
      unwatchStorages();
      unwatchRunning();
      close();
      document.removeEventListener('visibilitychange', onVisible);
      refreshSoon.cancel();
    };
  });

  // The frame waits for the listing and for the server's settings instead of
  // drawing without them: the rail and the report link read the settings.
  const listed = $derived(
    (person.modules.loaded || person.modules.error !== undefined) &&
      settings.asked,
  );

  // The issuer sends the browser back to `/`, so the address the sign-in was
  // carrying waits in sessionStorage across the round trip.
  const RETURN_KEY = 'pcl.signin.return';

  const signInThroughProvider = (): void => {
    signInFailure = undefined;
    try {
      globalThis.sessionStorage.setItem(
        RETURN_KEY,
        searchBeforeSignIn(PATHS.home, router.path, router.search),
      );
    } catch {
      // Storage refused: the person lands on the home page instead.
    }
    globalThis.location.assign(operationTable['begin-login'].path);
  };

  // Back to the address the sign-in was carrying, once the pages exist to
  // draw it. It runs before the screen at `/` is mounted, which sends the
  // page on to the printers.
  $effect.pre(() => {
    if (session.status !== 'signed-in' || !listed) return;
    let search = router.search;
    try {
      const kept = globalThis.sessionStorage.getItem(RETURN_KEY);
      if (kept !== null) {
        globalThis.sessionStorage.removeItem(RETURN_KEY);
        if (search === '') search = kept;
      }
    } catch {
      // Nothing was kept.
    }
    const address = addressAfterSignIn(PATHS.home, search);
    if (address !== undefined) router.replace(address);
  });
</script>

{#snippet crashPanel()}
  <div class="flex flex-col gap-4">
    <h1 class="text-lg font-semibold">{t('app.crash.title')}</h1>
    <p class="text-sm">{t('app.crash.body')}</p>
    {#if downloadFailed}
      <Alert variant="destructive">
        <AlertDescription>{t('app.crash.downloadFailed')}</AlertDescription>
      </Alert>
    {/if}
    <div class="flex gap-2">
      <Button
        onclick={() => {
          location.reload();
        }}>{t('app.crash.reload')}</Button
      >
      {#if session.status === 'signed-in' && settings.diagnostics}
        <Button
          variant="secondary"
          onclick={() => {
            downloadFailed = false;
            void downloadDiagnostics(client, entries(), {
              modules: false,
              extensions: false,
            })
              .then(({ blob, name }) => {
                saveBlob(blob, name);
              })
              .catch((failure: unknown) => {
                downloadFailed = true;
                crashLog.warn('diagnostics download failed', {
                  error: String(failure),
                });
              });
          }}>{t('app.crash.download')}</Button
        >
      {/if}
    </div>
  </div>
{/snippet}

<svelte:boundary onerror={logCrash}>
  {#if !session.setupAsked || session.status === 'unknown'}
    <!-- Neither question has been answered yet. -->
  {:else if firstRun}
    <FirstRun
      flow={setupFlow}
      {host}
      version={session.setupState?.version}
      docsHref={session.setupState?.documentation}
      onlanguage={chooseLocale}
      lastFailure={session.setupState?.last_error?.message}
      onfinish={async (
        credentials: { username: string; password: string } | undefined,
      ) => {
        if (credentials !== undefined) {
          await session.signIn(credentials.username, credentials.password);
        }
        await session.readSetup();
        await session.read();
      }}
    />
  {:else if session.status === 'signed-out'}
    <SignInScreen
      {host}
      instanceName={settings.title}
      ended={session.ended}
      busy={session.busy}
      failure={signInFailure}
      error={session.error}
      redirectProvider={session.redirectProvider}
      onRedirect={signInThroughProvider}
      onSignIn={(username: string, password: string) => {
        void session.signIn(username, password);
      }}
    />
  {:else if !listed}
    <!-- The listing is in flight. -->
  {:else}
    <AppShell
      productName={settings.title}
      navItems={withModuleNav(
        routes(settings.dashboards),
        moduleNav(person.modules.pages),
      )}
      navGroups={navGroups()}
      settings={settingsRoute(settings.diagnostics)}
      currentPath={railPath}
      homeHref={PATHS.home}
      connection={live.connection}
      {host}
      appVersion={appVersion.version}
      apiReference={settings.apiReference}
      documentation={settings.documentation}
      supportUrl={settings.supportUrl}
      notifications={person.notices}
      {noticeHref}
      issueSummary={(reason: string) => deviceIssues.entryOf(reason)?.summary}
      sends={person.sends}
      principal={session.principal?.name ?? session.principal?.id}
      principalDetail={session.principal?.email}
      onSignOut={() => {
        void session.signOut();
      }}
    >
      {#snippet banner()}
        {#if settings.safeMode !== undefined}
          <SafeModeBanner
            since={settings.safeMode.since.toISOString()}
            restarting={restart.phase === 'sending' ||
              restart.phase === 'waiting'}
            timedOut={restart.phase === 'timeout'}
            allowed={restartMayRun}
            refusal={restart.refusal}
            error={restart.phase === 'failed' ? restart.error : undefined}
            onRestartNormally={() => {
              void restart.restart('normal');
            }}
          />
        {/if}
      {/snippet}
      <!-- A screen that throws leaves the navigation standing, and going to
           another page draws it afresh. -->
      {#key Screen}
        <svelte:boundary onerror={screenFailed}>
          <PersonScreen
            person={session.principal?.id}
            diagnostics={settings.diagnostics}
          >
            <Screen {ctx} />
          </PersonScreen>
          {#snippet failed()}
            <div class="mx-auto max-w-md p-8">{@render crashPanel()}</div>
          {/snippet}
        </svelte:boundary>
      {/key}
    </AppShell>
  {/if}

  {#snippet failed()}
    <main class="mx-auto max-w-md p-8">{@render crashPanel()}</main>
  {/snippet}
</svelte:boundary>
