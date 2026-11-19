<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { userEvent, within } from 'storybook/test';

  import { capabilityFor, LIFECYCLE_VERBS, type Device } from '../../../domain';
  import { fakeDevice } from '../../../test/fakes';
  import type { Camera } from '../../../data';
  import type { CameraFeed } from '../../printer-cameras';
  import type { PrinterHistorySource, PrintRow } from '../info/types';
  import type { TelemetrySource } from '../types';
  import PrinterStatusPanel from './printer-status-panel.svelte';
  import type { PrinterTransferSource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Printer Detail/Status',
    component: PrinterStatusPanel,
  });

  const NOW = new Date('2026-08-19T19:09:00Z');

  const JOB = {
    display_name: 'Enclosure_Panel_Left.gcode',
    id: '65d10bad-b890-4dc6-8fd2-c1cf626f8ea2',
    device_job_id: '14',
    path: 'usb/Enclosure_Panel_Left.gcode',
    progress: 64,
    running: true,
    state: 'PRINTING',
    time_remaining_ms: 1_860_000,
  };

  const LONG_NAME =
    'Customer_order_4471_enclosure_panel_left_reinforced_v12_FINAL.bgcode';

  const LONG_JOB = {
    ...JOB,
    display_name: LONG_NAME,
    path: `usb/${LONG_NAME}`,
  };

  /** A machine with nothing on the bed: the record leaves `job` out entirely. */
  const printer = (state: string, over: Partial<Device> = {}): Device => ({
    ...fakeDevice('p0', 'MK4S #1042'),
    model: 'MK4S',
    capabilities: LIFECYCLE_VERBS.map(capabilityFor),
    state: { '/state': state },
    ...over,
  });

  /** What the machine says it has been printing for, in seconds. */
  const PRINTING_FOR = 5100;

  const printing = (
    state: string,
    job = JOB,
    over: Partial<Device> = {},
  ): Device => ({
    ...printer(state, over),
    job,
    state: {
      '/state': state,
      '/job/time_printing': PRINTING_FOR,
      ...over.state,
    },
  });

  const telemetry: TelemetrySource = {
    value: () => undefined,
    points: () => [],
  };

  const commands = { onCommand: () => undefined };

  /** A filament runout dialog, as the firmware puts it on the state tree. */
  const DIALOG = {
    '/dialog/id': 7,
    '/dialog/title': 'Filament runout',
    '/dialog/text': 'Insert filament, then press Retry.',
    '/dialog/buttons': ['Retry', 'Cancel'],
  };

  /**
   * A dialog as a newer firmware puts it there: the buttons and a number the
   * catalogue of this build does not hold, and no words at all.
   */
  const WORDLESS_DIALOG = {
    '/dialog/id': 11,
    '/dialog/code': '13999',
    '/dialog/buttons': ['Yes', 'No', 'Purge_more'],
  };

  /**
   * A stopped upload on an MK4: the buttons and the firmware's number for the
   * question, which the catalogue names.
   */
  const CODE_DIALOG = {
    '/dialog/id': 12,
    '/dialog/code': '13807',
    '/dialog/buttons': ['Ok'],
  };

  /**
   * A code whose sentence the firmware fills in as it shows it, so the
   * catalogue names it by the title alone.
   */
  const TITLE_DIALOG = {
    '/dialog/id': 13,
    '/dialog/code': '13848',
    '/dialog/buttons': ['Yes', 'No'],
  };

  // Where the camera store's tick would put a still frame.
  const SNAPSHOT = '/img/art/camera-grid.svg';
  const BED_CAM: Camera = {
    name: 'bed-cam',
    type: 'push',
    device_id: 'p0',
    formats: ['jpeg', 'mjpeg'],
    serving: true,
    snapshot: true,
    stream: true,
    watch: false,
  };
  const FEED: CameraFeed = { camera: BED_CAM, pictures: { still: SNAPSHOT } };
  const LIVE_FEED: CameraFeed = {
    camera: BED_CAM,
    pictures: { still: SNAPSHOT, live: SNAPSHOT },
  };

  /** One file part way to the printer, as the transfers store keeps it. */
  const transfers: PrinterTransferSource = {
    inFlight: [
      {
        transferId: '01M1GRZD931RAKEXTQ7V15GC1K',
        storage: 'uploads',
        source: 'Bracket_Left_Reinforced_v4_FINAL.bgcode',
        path: 'usb/Bracket_Left_Reinforced_v4_FINAL.bgcode',
        bytesTotal: 8_400_000,
        bytesSent: 5_400_000,
        state: 'sending',
        reason: undefined,
      },
    ],
    stop: () => Promise.resolve(),
  };

  /** What `GET /api/v1/devices/{id}/jobs` answers. */
  const history: PrinterHistorySource = {
    jobs: {
      last: {
        display_name: 'Camera_Mount_Bracket_v3.bgcode',
        ended: '2026-08-19T16:52:00Z',
        id: 'c7f4008e-c4d3-4f78-8888-0a6067f93b7f',
        device_id: 'p0',
        device_job_id: '13',
        path: 'usb/Camera_Mount_Bracket_v3.bgcode',
        progress: 100,
        started: '2026-08-19T13:31:00Z',
        state: 'FINISHED',
        time_remaining_ms: 0,
      },
    },
    jobsError: undefined,
    jobsLoaded: true,
  };

  /** What the paged prints read answers, which a host hands in as `recent`. */
  const recent: readonly PrintRow[] = [
    {
      display_name: 'Camera_Mount_Bracket_v3.bgcode',
      ended: '2026-08-19T16:52:00Z',
      id: 'c7f4008e-c4d3-4f78-8888-0a6067f93b7f',
      device_id: 'p0',
      device_job_id: '13',
      path: 'usb/Camera_Mount_Bracket_v3.bgcode',
      progress: 100,
      started: '2026-08-19T13:31:00Z',
      state: 'FINISHED',
      time_remaining_ms: 0,
    },
    {
      display_name: 'Enclosure_Frame_Rear.bgcode',
      ended: '2026-08-19T12:04:00Z',
      id: '73869087-5789-4ea6-83b5-fd9fcfbb0549',
      device_id: 'p0',
      device_job_id: '12',
      path: 'usb/Enclosure_Frame_Rear.bgcode',
      progress: 41,
      started: '2026-08-19T10:18:00Z',
      state: 'STOPPED',
      time_remaining_ms: 0,
    },
    {
      display_name: 'Calibration_Cube_PLA.bgcode',
      ended: '2026-08-18T21:40:00Z',
      id: '31959222-b8dc-4da1-8c1d-ba219350529b',
      device_id: 'p0',
      device_job_id: '11',
      path: 'usb/Calibration_Cube_PLA.bgcode',
      progress: 100,
      started: '2026-08-18T21:12:00Z',
      state: 'FINISHED',
      time_remaining_ms: 0,
    },
  ];

  /** A printer whose prints have not been read yet draws no block at all. */
  const unread: PrinterHistorySource = {
    jobs: undefined,
    jobsError: undefined,
    jobsLoaded: false,
  };

  const receive = (): void => undefined;

  const HISTORY_HREF = '#/printers/p0/info';

  /** A file kept in a folder of the one storage, as the toggle draws it on. */
  const keep = {
    keep: true,
    onKeepChange: () => undefined,
    storages: [{ name: 'local', label: 'Local' }],
    storage: 'local',
    onStorageChange: () => undefined,
    folder: 'prints/brackets',
    onFolderChange: () => undefined,
    pickerAt: '',
    pickerFolders: [],
    onPickerWalk: () => undefined,
  };
</script>

<!-- The question the screen exists to answer. -->
<Story name="Printing" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- A camera that streams: play starts the live view. -->
<Story name="Live view" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    camera={LIVE_FEED}
  />
</Story>

<!-- Open camera stream enlarges the camera in a dialog, on its live view. -->
<Story
  name="Camera enlarged"
  asChild
  play={async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole('button', {
        name: 'Open camera stream on MK4S #1042',
      }),
    );
  }}
>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    camera={LIVE_FEED}
  />
</Story>

<!-- A long file name at phone width gives up its middle, and Pause and Stop
     move to a line of their own below it. -->
<Story name="Long file name at phone width" asChild>
  <div class="max-w-90">
    <PrinterStatusPanel
      device={printing('PRINTING', LONG_JOB)}
      {telemetry}
      {commands}
      now={NOW}
      camera={FEED}
    />
  </div>
</Story>

<!-- The bar stops advancing and Resume takes Pause's slot. -->
<Story name="Paused" asChild>
  <PrinterStatusPanel
    device={printing('PAUSED')}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- The machine is waiting for hands, and says why in its own words. -->
<Story name="Attention" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      issues: [
        {
          reason: 'filament_runout',
          severity: 'warning',
          detail: 'The filament ran out. Load a new spool and resume.',
        },
      ],
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- Waiting for hands with no print on record: the camera takes the card. -->
<Story name="Attention between prints" asChild>
  <PrinterStatusPanel
    device={printer('ATTENTION')}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- The printer asks a question on its own screen, and an operator answers it here. -->
<Story name="Dialog" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      actions: ['observe', 'operate'],
      capabilities: [
        ...LIFECYCLE_VERBS.map(capabilityFor),
        'core.command.dialog_action',
      ],
      state: DIALOG,
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- The firmware sent a code and no words, and the catalogue names the code in
     the language on screen. -->
<Story name="Dialog named by its code" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      actions: ['observe', 'operate'],
      capabilities: [
        ...LIFECYCLE_VERBS.map(capabilityFor),
        'core.command.dialog_action',
      ],
      driver: 'xbuddy',
      state: CODE_DIALOG,
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- The catalogue names the code by its title alone, because the firmware fills
     the sentence in as it shows it. The banner still sends the reader to the
     machine before an answer. -->
<Story name="Dialog named by its title alone" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      actions: ['observe', 'operate'],
      capabilities: [
        ...LIFECYCLE_VERBS.map(capabilityFor),
        'core.command.dialog_action',
      ],
      driver: 'xbuddy',
      state: TITLE_DIALOG,
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- The firmware sent the answers and not the question, under a code the
     catalogue does not hold. The banner says so and gives the code, so a
     reader away from the machine knows what the buttons are about before
     pressing one. -->
<Story name="Dialog with no question text" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      actions: ['observe', 'operate'],
      capabilities: [
        ...LIFECYCLE_VERBS.map(capabilityFor),
        'core.command.dialog_action',
      ],
      state: WORDLESS_DIALOG,
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<!-- A caller who may only watch sees the question and cannot answer it. -->
<Story name="Dialog, observe only" asChild>
  <PrinterStatusPanel
    device={printing('ATTENTION', JOB, {
      actions: ['observe'],
      capabilities: [
        ...LIFECYCLE_VERBS.map(capabilityFor),
        'core.command.dialog_action',
      ],
      state: DIALOG,
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<Story name="Error" asChild>
  <PrinterStatusPanel
    device={printing('ERROR', JOB, {
      issues: [
        {
          reason: 'thermal_runaway',
          severity: 'error',
          detail: 'The hot end lost temperature control and the print stopped.',
        },
      ],
    })}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
  />
</Story>

<Story name="Error without reason" asChild>
  <PrinterStatusPanel
    device={printer('ERROR')}
    {telemetry}
    {commands}
    problemHref="#/printers/p0/history"
  />
</Story>

<!-- A camera whose still does not load says it has no picture yet. -->
<Story name="Camera with no picture" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    camera={{
      camera: BED_CAM,
      pictures: { still: 'data:image/gif;base64,broken' },
    }}
  />
</Story>

<!-- Nothing on the bed and nothing on record. -->
<Story name="Idle" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
  />
</Story>

<Story name="Finished" asChild>
  <PrinterStatusPanel
    device={printing('FINISHED', {
      ...JOB,
      progress: 100,
      running: false,
      state: 'FINISHED',
    })}
    {telemetry}
    {commands}
    now={NOW}
  />
</Story>

<!-- A printer this server cannot hear from keeps its last reading. -->
<Story name="Offline" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING', JOB, {
      connected: false,
      departed: { at: '2026-08-19T17:44:00Z', reason: 'session_ended' },
    })}
    {telemetry}
    {commands}
    now={NOW}
  />
</Story>

<!-- The panel's own shape while the record is on its way, not a grey page. -->
<Story name="Loading" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    loading
  />
</Story>

<!-- A file on its way to the printer leads the second card. -->
<Story name="Uploading" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    {transfers}
  />
</Story>

<!-- A file dropped on this page goes up to the server before the printer
     fetches it, and it can be cancelled until then. -->
<Story name="Going up to the server" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    uploads={[
      {
        id: 'job:1',
        deviceId: 'mk4',
        printerName: 'MK4 #1',
        name: 'cube.bgcode',
        step: 'uploading',
        progress: 0.4,
        mine: true,
        startedBy: undefined,
        canCancel: true,
        stopping: false,
      },
    ]}
    onCancelUpload={() => undefined}
  />
</Story>

<Story name="Upload a file" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    onUpload={receive}
  />
</Story>

<!-- The person keeps the file in a storage folder as well as sending it. -->
<Story name="Upload a file and keep it" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    onUpload={receive}
    {keep}
  />
</Story>

<!-- No USB drive in the printer: the reason stands where the drop zone was. -->
<Story name="No USB Drive" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    uploadBlock="noStorage"
    onRecheck={() => undefined}
  />
</Story>

<Story name="Asking The Printer Again" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    uploadBlock="storagesUnknown"
    checkingStorages
    onRecheck={() => undefined}
  />
</Story>

<Story name="No Server Storage For Uploads" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    uploadBlock="noServerStorage"
  />
</Story>

<!-- What this printer has finished, with the way out to the whole history. -->
<Story name="Last print" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    {history}
    historyHref={HISTORY_HREF}
  />
</Story>

<Story name="Files and history" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    camera={FEED}
    {transfers}
    {history}
    historyHref={HISTORY_HREF}
    onUpload={receive}
  />
</Story>

<!-- A host that reads the paged prints hands them in. -->
<Story name="Recent prints" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    {history}
    {recent}
    historyHref={HISTORY_HREF}
    onUpload={receive}
  />
</Story>

<!-- The first print is on the bed and none has ended: the block says where it lands. -->
<Story name="First print on the bed" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING')}
    {telemetry}
    {commands}
    now={NOW}
    history={{ jobs: {}, jobsError: undefined, jobsLoaded: true }}
    historyHref={HISTORY_HREF}
    onUpload={receive}
  />
</Story>

<!-- A jobs read still in flight draws no block. -->
<Story name="History unread" asChild>
  <PrinterStatusPanel
    device={printer('IDLE')}
    {telemetry}
    {commands}
    now={NOW}
    history={unread}
    historyHref={HISTORY_HREF}
  />
</Story>

<!-- A printer this server cannot hear from keeps the transfer it. -->
<Story name="Offline with files" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING', JOB, {
      connected: false,
      departed: { at: '2026-08-19T17:44:00Z', reason: 'session_ended' },
    })}
    {telemetry}
    {commands}
    now={NOW}
    {transfers}
    {history}
    historyHref={HISTORY_HREF}
    onUpload={receive}
  />
</Story>

<Story name="Print Details Unavailable" asChild>
  <PrinterStatusPanel
    device={{ ...fakeDevice('missing'), state: { '/state': 'PRINTING' } }}
    {telemetry}
    {commands}
  />
</Story>
<Story name="Print Name Unavailable" asChild>
  <PrinterStatusPanel
    device={printing('PRINTING', {
      ...JOB,
      display_name: '',
      path: '',
      time_remaining_ms: 0,
    })}
    {telemetry}
    {commands}
  />
</Story>
<Story name="Busy Without Print" asChild>
  <PrinterStatusPanel
    device={{ ...fakeDevice('busy'), state: { '/state': 'BUSY' } }}
    {telemetry}
    {commands}
  />
</Story>
