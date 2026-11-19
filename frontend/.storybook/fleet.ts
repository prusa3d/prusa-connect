// The printers every app story sees, in three fleet sizes. The first five are
// the devices of the UX fixture table, in its order, so the jobs, files and
// history that table answers for them still belong to a printer on screen. A
// farm adds seven copies of them with their own ids, names and states.
import type { components } from '../../packages/typescript/src/generated/core-api';
import type {
  FilamentColor,
  PreviewFilaments,
} from '../../packages/svelte/src/shell/printers/filament-colors';

import fixtures from '../test/ux/fixtures/api.json';

export type FleetSize = 'one' | 'two' | 'many';

interface DemoJob {
  readonly id: string;
  readonly device_job_id: string;
  readonly path: string;
  readonly display_name: string;
  readonly state: string;
  readonly progress: number;
  readonly time_remaining_ms: number;
  readonly running: boolean;
}

interface DemoTool {
  features: { nozzle_diameter: number };
  feeder?: { slot_count: number };
}

type DeviceIssue = components['schemas']['DeviceIssue'];

/** One item of `GET /api/v1/devices`, as the core writes it. */
export interface DeviceJson {
  readonly id: string;
  readonly name: string;
  readonly model: string;
  readonly serial: string;
  readonly connected?: boolean;
  readonly issues?: readonly DeviceIssue[] | null;
  readonly job?: DemoJob | undefined;
  readonly tools?: {
    tool_count: number;
    tools: Readonly<Record<string, DemoTool>>;
  };
  readonly state: Readonly<Record<string, unknown>>;
  readonly [field: string]: unknown;
}

interface Listing {
  readonly items: DeviceJson[];
  readonly [field: string]: unknown;
}

const listing = JSON.parse(fixtures['GET /api/v1/devices'].body) as Listing;

const recorded = listing.items;

const STATES = ['PRINTING', 'IDLE', 'ATTENTION', 'ERROR', 'FINISHED'] as const;

const FARM = 12;

// A copy keeps the model's capabilities and actions. Demo jobs get their own ids.
const copy = (number: number): DeviceJson => {
  const index = (number - recorded.length - 1) % recorded.length;
  const source = recorded[index];
  if (source === undefined) throw new Error('the fixture has no devices');
  const serial = `DEMO-${String(number).padStart(2, '0')}`;
  const telemetry = Object.fromEntries(
    Object.entries(source.state).filter(([key]) => !key.startsWith('/job/')),
  );
  const state = STATES[index] ?? 'IDLE';

  return {
    ...source,
    job: undefined,
    id: `00000000-0000-4000-8000-${String(number).padStart(12, '0')}`,
    name: `${source.model} ${serial}`,
    serial,
    unit_uid: `DEMO${String(number).padStart(11, '0')}`,
    state:
      state === 'PRINTING'
        ? { ...telemetry, '/state': state, '/job/progress': 42 }
        : { ...telemetry, '/state': state },
  };
};

const copies = Array.from({ length: FARM - recorded.length }, (_, i) =>
  copy(recorded.length + 1 + i),
);

// Presentation data only: no printer or database is changed. The raw UX
// recordings stay intact for tests of missing and incomplete telemetry.
const withDemoJob = (device: DeviceJson, index: number): DeviceJson => {
  const state = device.state['/state'];
  if (state !== 'FINISHED' && state !== 'PRINTING') return device;

  const finished = state === 'FINISHED';
  const laterPrint = device.serial === 'DEMO-11';
  const runningProgress = laterPrint ? 76 : 42;
  const runningMinutes = laterPrint ? 38 : 138;
  const runningName = laterPrint
    ? 'spool-holder.bgcode'
    : 'gearbox-housing.bgcode';
  const progress = finished ? 100 : runningProgress;
  const remaining = finished ? 0 : runningMinutes * 60_000;
  const name =
    device.job?.display_name ?? (finished ? 'phone-stand.bgcode' : runningName);
  const path = device.job?.path ?? `/usb/${name}`;

  return {
    ...device,
    state: {
      ...device.state,
      '/job/path': path,
      '/job/state': state,
      '/job/progress': progress,
      '/job/time_remaining': remaining / 1000,
    },
    job: {
      id:
        device.job?.id ??
        `10000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
      device_job_id: device.job?.device_job_id ?? String(index + 1),
      path,
      display_name: name,
      state,
      progress,
      time_remaining_ms: remaining,
      running: !finished,
    },
  };
};

interface DemoFilament {
  setup: 'single' | 'mmu3' | 'indx';
  colors: readonly FilamentColor[0][];
  current?: number;
}

// These configurations describe installed demo hardware, not model defaults.
const DEMO_FILAMENTS: Partial<Record<string, DemoFilament>> = {
  'DEMO-01': {
    setup: 'mmu3',
    colors: ['green', 'white', 'orange', 'blue', 'black'],
  },
  'DEMO-02': {
    setup: 'mmu3',
    colors: ['orange', 'red', 'gray', 'teal', 'white'],
  },
  'DEMO-03': { setup: 'single', colors: ['purple'] },
  'DEMO-04': {
    setup: 'mmu3',
    colors: ['cyan', 'yellow', 'purple', 'white', 'black'],
  },
  'DEMO-05': {
    setup: 'mmu3',
    colors: ['red', 'orange', 'yellow', 'blue', 'gray'],
  },
  'DEMO-06': {
    setup: 'indx',
    current: 2,
    colors: [
      'white',
      'orange',
      'yellow',
      'cyan',
      'purple',
      'green',
      'red',
      'black',
    ],
  },
  'DEMO-07': {
    setup: 'mmu3',
    colors: ['teal', 'green', 'beige', 'red', 'white'],
  },
  'DEMO-08': { setup: 'single', colors: ['white'] },
  'DEMO-09': {
    setup: 'mmu3',
    colors: ['gray', 'blue', 'red', 'white', 'black'],
  },
  'DEMO-10': {
    setup: 'mmu3',
    colors: ['amber', 'white', 'blue', 'green', 'purple'],
  },
  'DEMO-11': { setup: 'indx', colors: ['beige', 'teal', 'red', 'blue'] },
  'DEMO-12': {
    setup: 'mmu3',
    colors: ['blue', 'cyan', 'orange', 'purple', 'white'],
  },
};

const positionId = (demo: DemoFilament, index: number): string =>
  demo.setup === 'mmu3' ? `0.${String(index)}` : String(index);

const withDemoFilaments = (device: DeviceJson): DeviceJson => {
  const demo = DEMO_FILAMENTS[device.serial];
  if (demo === undefined) return device;

  const active =
    device.job?.running === true
      ? positionId(demo, demo.current ?? 0)
      : undefined;
  const activeHead = active?.split('.')[0] ?? '0';
  const toolCount = demo.setup === 'indx' ? demo.colors.length : 1;
  const material = { package: { material: { abbreviation: 'PLA' } } };
  const tools: Record<
    string,
    DemoTool & { material_package_instance: typeof material | null }
  > = Object.fromEntries(
    demo.colors.map((_, index) => [
      positionId(demo, index),
      {
        features: { nozzle_diameter: 0.4 },
        material_package_instance: material,
      },
    ]),
  );
  if (demo.setup === 'mmu3') {
    tools['0'] = {
      features: { nozzle_diameter: 0.4 },
      feeder: { slot_count: demo.colors.length },
      material_package_instance: null,
    };
  }

  return {
    ...device,
    state: {
      ...device.state,
      '/tool_count': toolCount,
      ...(active === undefined
        ? {}
        : {
            ...Object.fromEntries(
              [...new Set([activeHead, active])].map((id) => [
                `/tools/${id}/active`,
                true,
              ]),
            ),
            [`/tools/${activeHead}/nozzle_temp`]:
              device.state['/tools/0/nozzle_temp'],
            [`/tools/${activeHead}/nozzle_target_temp`]:
              device.state['/tools/0/nozzle_target_temp'],
            [`/tools/${active}/material_package_instance/package/material/abbreviation`]:
              'PLA',
          }),
    },
    tools: { tool_count: toolCount, tools },
  };
};

const farm: readonly DeviceJson[] = [...recorded, ...copies]
  .map(withDemoJob)
  .map(withDemoFilaments)
  .map((device) =>
    device.serial === 'DEMO-04'
      ? {
          ...device,
          issues: [{ reason: 'no_capability_profile', severity: 'info' }],
        }
      : device,
  );

export const demoFilamentPreview: PreviewFilaments = new Map(
  farm.flatMap((device) => {
    const demo = DEMO_FILAMENTS[device.serial];
    if (demo === undefined) return [];
    return [
      [
        device.id,
        {
          tool: positionId(demo, demo.current ?? 0),
          colors: new Map(
            demo.colors.map((color, index) => [positionId(demo, index), color]),
          ),
        },
      ],
    ];
  }),
);

/** The recorded printer each copy was made from, by the copy's id. */
export const copiedFrom: ReadonlyMap<string, string> = new Map(
  copies.map((device, i) => [
    device.id,
    recorded[i % recorded.length]?.id ?? '',
  ]),
);

const COUNT: Record<FleetSize, number> = { one: 1, two: 2, many: FARM };

export const fleetOf = (size: FleetSize): DeviceJson[] =>
  farm.slice(0, COUNT[size]);

const stateOf = (device: DeviceJson): string => {
  const state = device.state['/state'];
  return typeof state === 'string' ? state : '';
};

function urgency(device: DeviceJson): number {
  if (!device.connected) return 3;
  const state = stateOf(device).toUpperCase();
  if (state === 'ERROR') return 0;
  if (state === 'ATTENTION') return 1;
  if (['PAUSED', 'FINISHED', 'STOPPED'].includes(state)) return 2;
  return ['PRINTING', 'BUSY', 'MANIPULATING', 'IDLE', 'READY'].includes(state)
    ? 4
    : 3;
}

function facetValues(
  printers: DeviceJson[],
  valueOf: (printer: DeviceJson) => string,
  selected: string[],
) {
  const counts = new Map(selected.map((value) => [value, 0]));
  for (const printer of printers) {
    const value = valueOf(printer);
    if (value !== '') counts.set(value, (counts.get(value) ?? 0) + 1);
  }
  return [...counts]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, count]) => ({ value, count }));
}

/** The whole `GET /api/v1/devices` answer for a fleet of this size. */
export const devicesAnswer = (
  size: FleetSize,
  query = new URLSearchParams(),
): Response => {
  const printers = query.get('retired') === 'only' ? [] : fleetOf(size);
  const search = query.get('search')?.trim().toLowerCase() ?? '';
  const states = query.getAll('state').flatMap((value) => value.split(','));
  const models = query.getAll('model').flatMap((value) => value.split(','));
  const severities = query
    .getAll('severity')
    .flatMap((value) => value.split(','));
  const matchesState = (device: DeviceJson) =>
    states.length === 0 || states.includes(stateOf(device));
  const matchesModel = (device: DeviceJson) =>
    models.length === 0 || models.includes(device.model);
  const searched = printers.filter((device) =>
    [device.name, device.model, device.serial].some((value) =>
      value.toLowerCase().includes(search),
    ),
  );
  const matchesSeverity = (device: DeviceJson) =>
    severities.length === 0 ||
    (device.issues ?? []).some(({ severity }) => severities.includes(severity));
  const candidates = searched.filter(matchesSeverity);
  const items = candidates.filter(
    (device) => matchesState(device) && matchesModel(device),
  );
  const sort = query.get('sort') ?? 'urgency';
  const order = query.get('order') === 'desc' ? 'desc' : 'asc';
  const compare = (a: DeviceJson, b: DeviceJson): number => {
    if (sort === 'name' || sort === 'model')
      return a[sort].localeCompare(b[sort]);
    if (sort === 'state') return stateOf(a).localeCompare(stateOf(b));
    return urgency(a) - urgency(b);
  };
  items.sort(
    (a, b) =>
      (order === 'asc' ? 1 : -1) * compare(a, b) || a.id.localeCompare(b.id),
  );
  const sorts = [
    { field: 'urgency', default_order: 'asc' },
    { field: 'name', default_order: 'asc' },
    { field: 'model', default_order: 'asc' },
    { field: 'state', default_order: 'asc' },
  ];
  const affected = printers.filter(
    (device) => (device.issues?.length ?? 0) > 0,
  );
  const issues = affected.flatMap((device) => device.issues ?? []);
  const counts: Record<string, number> = {};
  const by_severity = { error: 0, info: 0, warning: 0 };
  for (const issue of issues) {
    counts[issue.reason] = (counts[issue.reason] ?? 0) + 1;
    by_severity[issue.severity] += 1;
  }
  const filters = [
    { param: 'search', kind: 'text', max_length: 96 },
    {
      param: 'state',
      kind: 'multi',
      values: facetValues(candidates.filter(matchesModel), stateOf, states),
    },
    {
      param: 'model',
      kind: 'multi',
      values: facetValues(
        candidates.filter(matchesState),
        (device) => device.model,
        models,
      ),
    },
  ];
  const offset = Math.max(0, Number(query.get('offset')) || 0);
  const limit = Math.max(0, Number(query.get('limit')) || 0);

  return new Response(
    JSON.stringify({
      ...listing,
      items:
        limit > 0 ? items.slice(offset, offset + limit) : items.slice(offset),
      total: items.length,
      offset,
      limit,
      filters,
      sorts,
      sort,
      order,
      issues: { affected: affected.length, counts, by_severity },
    }),
    { status: 200, headers: { 'content-type': 'application/json' } },
  );
};
