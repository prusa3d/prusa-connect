<!--
  The printer page over the story core, one story per tab (Status, Controls,
  Files, Commands, History, Info; History also failing) and the page's own states: loading, failed,
  refused and a printer the server does not know. Module tabs and the header
  actions come from installed modules, which the story core does not install,
  so they are not drawn. A printer that was retired needs a record the core
  tables do not hold, so it is not drawn either.
-->
<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import ControlsPrototype from '../../test/controls-prototype.svelte';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({ title: 'Screens/Printer detail' });

  const ID = '34e039fc-13fd-4f4a-b16b-92392643f63a';
  const DEVICES = 'GET /api/v1/devices';
  const HISTORY = `GET /api/v1/devices/${ID}/history`;
  const COMMANDS = `GET /api/v1/devices/${ID}/commands`;

  // The core tables hold no transfers and no grants for a printer, and the
  // History and Info tabs draw a not-found error in their place without them.
  // A printer with nothing sent and no grant of its own answers both empty.
  const printerRecords = {
    [`GET /api/v1/devices/${ID}/transfers`]: () => json(200, { items: [] }),
    [`GET /api/v1/grants/device/${ID}`]: () => json(200, { items: [] }),
  };

  const minutesAgo = (minutes: number): string =>
    new Date(Date.now() - minutes * 60_000).toISOString();

  // The core tables hold no history, and the tab draws a not-found error
  // without these rows. The window is the last two hours, so the times are
  // taken from now.
  const history = {
    ...printerRecords,
    [`${HISTORY}/facts`]: () =>
      json(200, {
        cursor: '',
        incomplete: false,
        items: [
          {
            at: minutesAgo(40),
            epoch: '01K3Q7W0',
            origin: `device:${ID}`,
            payload: { connected: true, device_id: ID, driver: 'xbuddy' },
            sequence: 412,
            type: 'com.prusa.connect.device.presence.changed',
          },
        ],
      }),
    [`${HISTORY}/samples`]: () =>
      json(200, {
        cursor: '',
        incomplete: false,
        items: [90, 60, 30, 0].flatMap((ago, index) => [
          {
            at: minutesAgo(ago),
            metric: '/tools/0/nozzle_temp',
            origin: `device:${ID}`,
            value: 190 + index * 8,
          },
          {
            at: minutesAgo(ago),
            metric: '/temperature/bed',
            origin: `device:${ID}`,
            value: 55 + index,
          },
        ]),
      }),
    [COMMANDS]: () =>
      json(200, {
        cursor: '',
        items: [
          {
            auth_id: 'operator',
            auth_type: 'user',
            authorized_at: minutesAgo(12),
            command_id: '01K3Q7W0YQ4S2C4H2S8W9YQ7V1',
            elapsed_ms: 34,
            ok: true,
            origin: `device:${ID}`,
            requested_at: minutesAgo(12),
            settled_at: minutesAgo(12),
            status: 'succeeded',
            verb: 'core.command.beep',
          },
        ],
      }),
  };
</script>

<Story name="Status"><AppAt path="/printers/{ID}/status" /></Story>
<Story name="Controls"><AppAt path="/printers/{ID}/controls" /></Story>
<Story name="Controls prototype MK4S" globals={{ theme: 'dark' }}>
  <ControlsPrototype />
</Story>
<Story name="Controls prototype Error" globals={{ theme: 'dark' }}>
  <ControlsPrototype state="ERROR" />
</Story>
<Story name="Files"><AppAt path="/printers/{ID}/files" /></Story>
<Story name="Commands"><AppAt path="/printers/{ID}/commands" /></Story>
<Story name="History">
  <AppAt path="/printers/{ID}/history" overrides={history} />
</Story>

<Story name="History failed">
  <AppAt
    path="/printers/{ID}/history"
    overrides={{
      [`${HISTORY}/facts`]: failed,
      [`${HISTORY}/samples`]: failed,
      [COMMANDS]: failed,
    }}
  />
</Story>
<Story name="Info">
  <AppAt path="/printers/{ID}/info" overrides={printerRecords} />
</Story>

<Story name="Loading">
  <AppAt path="/printers/{ID}" overrides={{ [DEVICES]: pending }} />
</Story>

<Story name="Failed">
  <AppAt path="/printers/{ID}" overrides={{ [DEVICES]: failed }} />
</Story>

<Story name="Refused">
  <AppAt path="/printers/{ID}" overrides={{ [DEVICES]: refused }} />
</Story>

<Story name="Unknown printer">
  <AppAt path="/printers/00000000-0000-4000-8000-000000000000" />
</Story>
