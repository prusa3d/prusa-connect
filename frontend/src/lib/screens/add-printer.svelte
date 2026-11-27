<!--
  This product's add-printer page: the library's guide, fed this server's own
  address, how this server admits printers, and the registrations the event
  stream keeps current.

  The file is built from `core.frontend.printer_address`, or from the address
  this page was served on when the server names none. Whether a person
  approves each printer is read from the server and never guessed. A printer
  counts as the one just added when its approval, its arrival or its refusal
  came after the guide began to wait.

  The page can also ask a printer on the network to pair at its own screen.
  The server sends the request and approves the registration that follows,
  so the guide learns of that printer from the same approval and arrival
  frames as of any other.
-->
<script lang="ts">
  import {
    AddPrinterScreen,
    type JoinedPrinter,
    type JoiningPrinter,
    type NearbyCandidate,
    type PairingRequest,
    type PendingPrinter,
    printerHref,
    printersToAdd,
  } from '@prusa/connect-ui/shell';
  import { deviceIdOf } from '@prusa/connect-ui/data';
  import { onMount } from 'svelte';

  import { client } from '../client';
  import type { AppContext } from '../screens';
  import { PrinterAuthority } from '../stores/printer-authority.svelte';
  import { PrinterPairingStore } from '../stores/printer-pairing.svelte';

  let { ctx }: { ctx: AppContext } = $props();

  const registrations = $derived(ctx.registrations);
  const authority = new PrinterAuthority(client);
  // One pairing at a time, owned by this page: a pairing waits about a
  // minute and means nothing once the person leaves.
  const pairing = new PrinterPairingStore(client);

  // The paths a printer dials are the connector's, not this page's, and they
  // are fixed: `internal/connector/xbuddy/xbuddy.go` mounts /p/register and
  // /p/ws and nothing configures them.
  const REGISTER_PATH = '/p/register';
  const WEBSOCKET_PATH = '/p/ws';

  const DEFAULT_PORTS = { 'http:': 80, 'https:': 443 };

  const ARRIVED = 'com.prusa.connect.device.arrived';

  // The server validated printer_address as a bare http or https origin.
  const address = $derived(
    new URL(ctx.settings.printerAddress ?? window.location.origin),
  );
  const tls = $derived(address.protocol === 'https:');

  const connection = $derived({
    host: address.hostname,
    port:
      address.port === ''
        ? DEFAULT_PORTS[tls ? 'https:' : 'http:']
        : Number(address.port),
    tls,
    customCert: ctx.settings.printerCustomCa,
    wsUrl: `${tls ? 'wss' : 'ws'}://${address.host}${WEBSOCKET_PATH}`,
    registerUrl: `${address.origin}${REGISTER_PATH}`,
  });

  // When the person began to add a printer. Approvals and refusals before it
  // belong to an earlier printer.
  let since = $state(Date.now());

  // The devices that dialled in while this page was open. A printer that
  // joined stays joined here when it drops again, and its arrival counts
  // whichever of the two frames the stream delivers first. Replaced as a
  // whole, never edited in place (svelte-5.md).
  let arrivals = $state.raw<ReadonlySet<string>>(new Set());

  /**
   * What a person reads for a model, and what the guide draws it by. A
   * registration carries the code some firmware reports where a model name is
   * expected, and the model vocabulary turns it into the name the machine is
   * sold as and into the row's own spelling, which the drawings are filed
   * under. The row's icon key names a family and no drawing.
   */
  const described = (
    model: string | undefined,
  ): { model: string | undefined; icon: string | undefined } => {
    const entry = ctx.deviceModels.entryOf(model);

    return { model: entry?.display_name ?? model, icon: entry?.model };
  };

  const pending = $derived<PendingPrinter[]>(
    registrations.pending.map((row) => ({
      ...row,
      ...described(row.model),
      reported: row.model,
    })),
  );

  const recent = $derived(
    registrations.approvals.filter((approval) => approval.at >= since),
  );

  // A printer has joined once its device dialled in while this page was open,
  // or is connected now.
  const arrived = (deviceId: string | undefined) => {
    if (deviceId === undefined) return undefined;
    const device = ctx.fleet.devices.find((row) => row.id === deviceId);

    return device !== undefined && (arrivals.has(deviceId) || device.connected)
      ? device
      : undefined;
  };

  const joined = $derived<JoinedPrinter[]>(
    recent.flatMap((approval) => {
      const device = arrived(approval.deviceId);
      if (device === undefined) return [];
      const { model, icon } = described(device.model);

      return [
        {
          deviceId: device.id,
          name: device.name,
          model,
          icon,
          reported: device.model,
          href: printerHref(device.id),
        },
      ];
    }),
  );

  const joining = $derived<JoiningPrinter[]>(
    recent
      .filter((approval) => arrived(approval.deviceId) === undefined)
      .map((approval) => ({
        code: approval.code,
        serial: approval.serial,
        ...described(approval.model),
      })),
  );

  const refused = $derived.by(() => {
    // A caller that presented no identity is not a printer: a browser tab or
    // a port scanner knocking on the wire.
    const last = registrations.refusals.find(
      (one) => one.at >= since && one.digest !== undefined,
    );
    if (last === undefined) return undefined;

    return { reason: last.reason, ...described(last.model) };
  });

  // The printers the picker offers. A printer the server already names in
  // device_id connects from that address, or last did, so it is here; and
  // another server is no printer. Nothing is offered until the list is read,
  // nor to a person who may not read it.
  const nearby = $derived<NearbyCandidate[] | undefined>(
    ctx.nearby.loaded && ctx.nearby.error === undefined
      ? printersToAdd(ctx.nearby.items).map((entry) => ({
          name: entry.name,
          address: entry.address,
          port: entry.port,
        }))
      : undefined,
  );

  onMount(() => {
    void registrations.load();
    void ctx.deviceModels.read();
    if (connection.customCert && connection.tls) void authority.load();

    const unsubscribe = ctx.fleet.subscribe((event) => {
      if (event.type !== ARRIVED) return;
      const id = deviceIdOf(event);
      if (id !== undefined && !arrivals.has(id)) {
        arrivals = new Set([...arrivals, id]);
      }
    });

    void ctx.nearby.load();
    const stopNearby = ctx.nearby.listen();

    return () => {
      unsubscribe();
      stopNearby();
      pairing.dispose();
    };
  });
</script>

<AddPrinterScreen
  {connection}
  admission={ctx.settings.printerAdmission}
  {pending}
  {joining}
  {joined}
  {refused}
  printersHref="/printers"
  error={registrations.error}
  authority={authority.file}
  docsHref={ctx.settings.documentation}
  onBegin={() => {
    since = Date.now();
  }}
  onApprove={(code: string) => {
    void registrations.approve(code);
  }}
  onReject={(code: string) => {
    void registrations.reject(code);
  }}
  pairing={pairing.pairing}
  {nearby}
  pairingBusy={pairing.busy}
  pairingError={pairing.error}
  onPair={(request: PairingRequest) => {
    void pairing.start(request);
  }}
  onCancelPairing={() => {
    void pairing.cancel();
  }}
  onPairAgain={() => {
    pairing.reset();
  }}
/>
