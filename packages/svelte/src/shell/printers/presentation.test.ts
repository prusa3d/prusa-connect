import { describe, expect, it } from 'vitest';

import { fakeDevice } from '../../test/fakes';
import { DESCRIBED_BED, describedReading } from '../../test/readings';
import {
  printerArtworkOf,
  printerHref,
  printerModelLabelOf,
  printerProductNameOf,
  printerRowOf,
  retiredReasonOf,
} from './presentation';

describe('printerProductNameOf', () => {
  it('names a known model by its product, the HT90 by its type id too', () => {
    expect(printerProductNameOf('MK4')).toBe('Original Prusa MK4');
    expect(printerProductNameOf('6.1.0')).toBe('Prusa Pro HT90');
  });

  it('shows a model it does not know as the printer spelled it', () => {
    expect(printerProductNameOf('ZX-42')).toBe('ZX-42');
    expect(printerModelLabelOf('ZX-42')).toBe('ZX-42');
  });

  it('gives the short label of a known model', () => {
    expect(printerModelLabelOf('6.1.0')).toBe('HT90');
  });
});

describe('printerHref', () => {
  it('names the printer page and one tab on it', () => {
    expect(printerHref('mk4')).toBe('/printers/mk4');
    expect(printerHref('mk4', 'files')).toBe('/printers/mk4/files');
  });

  // A device id is the adapter's own, and a slash or a colon in it must not
  // split it into two segments of the address.
  it('keeps an id with a slash or a colon in one segment', () => {
    expect(printerHref('shop/mk4:2')).toBe('/printers/shop%2Fmk4%3A2');
  });
});

describe('printerArtworkOf', () => {
  const device = (model: string, slots: number) => ({
    ...fakeDevice('p'),
    model,
    state: Object.fromEntries(
      Array.from({ length: slots }, (_, i) => [
        `/tools/${String(i)}/nozzle_temp`,
        215,
      ]),
    ),
  });

  it('draws an XL with as many toolheads as it reports slots', () => {
    expect(printerArtworkOf(device('XL', 0))).toBe('xl');
    expect(printerArtworkOf(device('XL', 1))).toBe('xl');
    expect(printerArtworkOf(device('XL', 3))).toBe('xlmultitool3');
    expect(printerArtworkOf(device('XL', 5))).toBe('xlmultitool5');
  });

  // Slots on any other model are filament changer feeders.
  it('keeps the drawing of a model with slots that is not an XL', () => {
    expect(printerArtworkOf(device('MK4S', 5))).toBe('mk4s');
  });

  // The HT90 names its model by its Prusa Connect type id.
  it('draws an HT90 that registers as 6.1.0', () => {
    expect(printerArtworkOf(device('6.1.0', 1))).toBe('ht90');
  });
});

describe('printerRowOf issues', () => {
  const device = {
    ...fakeDevice('p'),
    issues: [
      {
        reason: 'filament_runout',
        severity: 'warning' as const,
        detail: 'raw',
      },
    ],
  };
  const entry = {
    action: 'Load filament.',
    description: '',
    reason: 'filament_runout',
    severity: 'warning',
    summary: 'The filament ran out.',
  };

  it('takes the sentence and the action from the vocabulary', () => {
    const row = printerRowOf(device, () => entry);

    expect(row.statusNote).toBe('The filament ran out.');
    expect(row.statusAction).toBe('Load filament.');
    expect(row.severity).toBe('warning');
  });

  it('falls back to what the machine said for a reason the vocabulary lacks', () => {
    const row = printerRowOf(device, () => undefined);

    expect(row.statusNote).toBe('raw');
    expect(row.statusAction).toBeUndefined();
  });
});

describe('retiredReasonOf', () => {
  it('shows a reason this build does not know as the server spelled it', () => {
    expect(retiredReasonOf('moved_to_mars')).toBe('moved_to_mars');
  });
});

describe('printerRowOf removable', () => {
  const administering = {
    ...fakeDevice('p'),
    actions: ['observe', 'operate', 'configure', 'administer'],
  };

  it('lets an administrator remove a printer', () => {
    expect(printerRowOf(administering).removable).toBe(true);
  });

  it('keeps a printer that runs this server', () => {
    expect(printerRowOf({ ...administering, resident: true }).removable).toBe(
      false,
    );
  });

  it('keeps a printer from a reader who may not administer it', () => {
    expect(
      printerRowOf({ ...administering, actions: ['observe'] }).removable,
    ).toBe(false);
  });
});

describe('printerRowOf bed', () => {
  it('reads the bed from the reading with the bed role', () => {
    const device = {
      ...fakeDevice('p'),
      state: {
        ...describedReading({
          id: 'heater_bed',
          valuePath: '/heater_bed/temperature',
          targetPath: '/heater_bed/target',
          role: 'bed',
        }),
        '/heater_bed/temperature': 61,
        '/heater_bed/target': 60,
      },
    };

    expect(printerRowOf(device)).toMatchObject({ bed: 61, bedTarget: 60 });
  });

  it('shows no bed for a printer that describes none', () => {
    const device = {
      ...fakeDevice('p'),
      state: { '/temperature/bed': 61, '/temperature/bed_target': 60 },
    };

    expect(printerRowOf(device)).toMatchObject({
      bed: undefined,
      bedTarget: undefined,
    });
  });

  it('shows a described bed with no value yet as no value', () => {
    const device = { ...fakeDevice('p'), state: { ...DESCRIBED_BED } };

    expect(printerRowOf(device).bed).toBeUndefined();
  });
});
