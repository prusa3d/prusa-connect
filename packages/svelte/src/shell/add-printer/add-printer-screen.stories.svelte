<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import AddPrinterScreen from './add-printer-screen.svelte';
  import type { JoinedPrinter, PendingPrinter } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Add Printer',
    component: AddPrinterScreen,
  });

  const connection = {
    host: 'printers.local',
    port: 8080,
    tls: false,
    wsUrl: 'ws://printers.local:8080/p/ws',
    registerUrl: 'http://printers.local:8080/p/register',
  };

  const pending: PendingPrinter[] = [
    {
      code: 'K7QF2RXA9C',
      fingerprint: 'a1b2c3d4e5f60718',
      model: 'MK4S',
      firmware: '6.5.1',
      serial: 'CZPX4021X004XK12345',
    },
    {
      code: 'M3PZ9WQHTE',
      fingerprint: '99ffee8877665544',
      model: 'CORE One',
      firmware: '6.4.0',
    },
  ];

  const joined: JoinedPrinter[] = [
    {
      deviceId: 'd-1',
      name: 'MK4S CZPX4021X004XK12345',
      model: 'MK4S',
      href: '/printers/d-1',
    },
  ];

  const ports = {
    printersHref: '/printers',
    admission: 'approval' as const,
    pending: [],
    joining: [],
    joined: [],
    refused: undefined,
    onBegin: () => undefined,
    onApprove: () => undefined,
    onReject: () => undefined,
  };
</script>

<!--
  One story per step, then the states of the waiting step a person meets:
  nothing yet, two printers asking, a printer approved and connecting, a
  printer turned away, and the help that opens when nothing comes. The last
  stories are pairing at the printer's screen: the form, the printers heard
  on the network and none heard, the code while the
  printer asks, the printer registering, the two ways a pairing ends without
  a yes, and two refusals from the printer.
-->

<Story name="Prepare The File" args={{ ...ports, connection }} />

<Story
  name="Prepare The File – Own Authority"
  args={{
    ...ports,
    connection: { ...connection, tls: true, customCert: true },
    authority: new Blob(['DER'], { type: 'application/pkix-cert' }),
  }}
/>

<Story
  name="Prepare The File – Own Authority Missing"
  args={{
    ...ports,
    connection: { ...connection, tls: true, customCert: true },
    authority: null,
    docsHref: 'https://docs.example/',
  }}
/>

<Story
  name="On The Printer"
  args={{ ...ports, connection, initialStep: 'printer' }}
/>

<Story
  name="Waiting For The Printer"
  args={{ ...ports, connection, initialStep: 'waiting' }}
/>

<Story
  name="Two Printers Asking"
  args={{ ...ports, connection, pending, initialStep: 'waiting' }}
/>

<Story
  name="Joining Without Approval"
  args={{
    ...ports,
    connection,
    admission: 'automatic',
    pending: pending.slice(0, 1),
    initialStep: 'waiting',
  }}
/>

<Story
  name="Approved And Connecting"
  args={{
    ...ports,
    connection,
    joining: [{ code: 'K7QF2RXA9C', model: 'MK4S' }],
    initialStep: 'waiting',
  }}
/>

<Story
  name="Refused At The Door"
  args={{
    ...ports,
    connection,
    refused: { reason: 'unknown_credentials', model: 'MK4S' },
    initialStep: 'waiting',
  }}
/>

<Story
  name="Help After A Quiet Wait"
  args={{ ...ports, connection, initialStep: 'waiting', helpAfterMs: 0 }}
/>

<Story
  name="Connected"
  args={{ ...ports, connection, joined, initialStep: 'waiting' }}
/>

<Story
  name="Printers Waiting Beside The Guide"
  args={{ ...ports, connection, pending }}
/>

<Story
  name="Still Arriving"
  args={{ ...ports, connection, admission: undefined }}
/>

<Story
  name="Refused"
  args={{
    ...ports,
    connection,
    error: { code: 'core.auth/ACCESS_DENIED', status: 403 },
  }}
/>

<Story
  name="Pair At The Printer"
  args={{ ...ports, connection, onPair: () => undefined }}
/>

<Story
  name="Pair At The Printer – Printers Nearby"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    nearby: [
      { name: 'prusa-core-one', address: '192.168.1.21', port: 80 },
      { name: 'prusa-mk4', address: '192.168.1.22', port: 80 },
      { name: 'workshop-xl', address: '192.168.1.23', port: 8080 },
    ],
  }}
/>

<Story
  name="Pair At The Printer – None Nearby"
  args={{ ...ports, connection, onPair: () => undefined, nearby: [] }}
/>

<Story
  name="Pairing – Waiting For Yes"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    onCancelPairing: () => undefined,
    pairing: {
      code: 'K7QX-29FM',
      printer: '192.168.1.20:80',
      state: 'waiting',
    },
  }}
/>

<Story
  name="Pairing – Printer Registering"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    pairing: {
      code: 'K7QX-29FM',
      printer: '192.168.1.20:80',
      state: 'approved',
    },
  }}
/>

<Story
  name="Pairing – Cancelled At The Printer"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    onPairAgain: () => undefined,
    pairing: {
      code: 'K7QX-29FM',
      printer: '192.168.1.20:80',
      state: 'refused',
    },
  }}
/>

<Story
  name="Pairing – Nobody Answered"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    onPairAgain: () => undefined,
    pairing: {
      code: 'K7QX-29FM',
      printer: '192.168.1.20:80',
      state: 'expired',
    },
  }}
/>

<Story
  name="Pairing – Firmware Cannot Pair"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    pairingError: {
      code: 'core.printer/PAIRING_UNSUPPORTED',
      status: 422,
      message: "the printer's firmware cannot pair",
    },
  }}
/>

<Story
  name="Pairing – Printer Locked"
  args={{
    ...ports,
    connection,
    onPair: () => undefined,
    pairingError: {
      code: 'core.printer/PAIRING_LOCKED',
      status: 429,
      message: 'the printer takes no pairing request for now',
      params: { retry_after_seconds: 240 },
    },
  }}
/>
