<!--
  The stores a dashboard screen reads, built once for the page and handed to
  the library's screen as its context (ADR-20261008-sww).

  The editor, the shared telemetry stream, the camera frames and the clock
  belong to the page: they are built when it opens and stopped when it
  closes, so a page without a dashboard holds none of them. The dashboards
  themselves are a person's store and outlive the page.
-->
<script lang="ts">
  import { DashboardEditor, SharedTelemetry } from '@prusa/connect-ui/data';
  import type * as Dashboard from '@prusa/connect-ui/dashboard';
  import { withBase } from '@prusa/connect-ui/router';
  import { onDestroy, onMount } from 'svelte';

  import { client } from '../client';
  import { dashboardHref, PATHS } from '../routes';
  import type { AppContext } from '../screens';

  let {
    ctx,
    lib,
    id,
    kiosk,
    edit,
  }: {
    ctx: AppContext;
    /** The library's dashboard chunk, loaded by the screen. */
    lib: typeof Dashboard;
    id: number | undefined;
    kiosk: boolean;
    edit: boolean;
  } = $props();

  // Built once, as the frame is made: the stores are the page's for its life.
  // svelte-ignore state_referenced_locally
  const { registry, loader } = ctx.slots;
  // svelte-ignore state_referenced_locally
  lib.registerCoreCards(registry);
  // svelte-ignore state_referenced_locally
  const cards = new lib.CardCatalogue(registry, loader);
  // svelte-ignore state_referenced_locally
  const printers = new lib.FleetViews(ctx.fleet, ctx.deviceIssues);
  // svelte-ignore state_referenced_locally
  const cameras = new lib.CameraFrames(ctx.cameras);
  // svelte-ignore state_referenced_locally
  const telemetry = new SharedTelemetry(client, {
    ...(ctx.session.principal?.id === undefined
      ? {}
      : { principal: ctx.session.principal.id }),
  });

  /** Counts the visits home, so a visit to the address on screen draws it anew. */
  let homeVisits = $state(0);

  // svelte-ignore state_referenced_locally
  const context: Dashboard.DashboardContext = {
    dashboards: ctx.dashboards,
    editor: new DashboardEditor((type) => cards.leastOf(type)),
    printers,
    telemetry,
    cameras,
    clock: new lib.Clock(lib.browserTicker),
    cards,
    navigation: {
      open: (next, options) => {
        const query = options?.edit === true ? '?edit=1' : '';
        ctx.go(`${dashboardHref(next)}${query}`, {
          replace: options?.replace === true,
        });
      },
      // The wall view keeps the address it was opened from, the home page
      // among them, so leaving it lands where the person was.
      kiosk: (on) => {
        ctx.go(on ? `${ctx.path}?kiosk=1` : ctx.path, { replace: true });
      },
      link: (next, wall) =>
        `${window.location.origin}${withBase(dashboardHref(next))}${wall ? '?kiosk=1' : ''}`,
      home: () => {
        homeVisits += 1;
        ctx.go(PATHS.dashboards, { replace: true });
      },
      checkLeave: (check) => ctx.checkLeave(check),
      addPrinter: () => {
        ctx.go(PATHS.printersAdd);
      },
    },
  };

  // The fleet hears the app's one stream and hands every event on, so a
  // dashboard changed on another screen is read again here.
  // svelte-ignore state_referenced_locally
  const stop = ctx.fleet.subscribe((event) => {
    ctx.dashboards.hear(event);
  });

  // The words for why a printer is not ready, which a printer card shows.
  // The store reads them once and hands every later call that one read.
  onMount(() => {
    void ctx.deviceIssues.read();
    void ctx.deviceModels.read();
  });

  onDestroy(() => {
    stop();
    telemetry.close();
  });
</script>

<!-- The home page after a delete is often the address already on screen,
     and only a new screen chooses the dashboard it shows again. -->
{#key `${String(id)}:${String(homeVisits)}`}
  <lib.DashboardScreen {context} {id} {kiosk} {edit} />
{/key}
