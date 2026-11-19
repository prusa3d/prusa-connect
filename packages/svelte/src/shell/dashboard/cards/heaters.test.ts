import { describe, expect, it } from 'vitest';

import { readingsOf } from '../../../domain';
import { readCardSettings } from '../../../slots';
import {
  DESCRIBED_CHAMBER,
  DESCRIBED_ENCLOSURE,
  describedReading,
} from '../../../test/readings';
import { CORE_CARDS } from './declarations';
import {
  heaterNameOf,
  offeredReadings,
  readingLabel,
  readingTargetLabel,
  selectedReadings,
} from './heaters';

// A Klipper machine names its bed after its configuration section.
const KLIPPER = readingsOf({
  ...describedReading({
    id: 'heater_bed',
    valuePath: '/heater_bed/temperature',
    targetPath: '/heater_bed/target',
    role: 'bed',
  }),
  ...DESCRIBED_CHAMBER,
  ...DESCRIBED_ENCLOSURE,
});

const temperatureSchema =
  CORE_CARDS.find((card) => card.id === 'temperature')?.settings ?? [];

describe('the lines a temperature card selects', () => {
  it('keeps a saved card drawing its bed, whatever the bed reading is called', () => {
    const saved = readCardSettings(temperatureSchema, {
      device: 'p1',
      heaters: ['nozzle', 'bed'],
      minutes: '15',
    });

    expect(saved.heaters).toEqual(['nozzle', 'bed']);
    expect(
      // The schema read above holds the field to a list of words.
      selectedReadings(KLIPPER, saved.heaters as readonly string[]).map(
        (reading) => reading.id,
      ),
    ).toEqual(['heater_bed']);
  });

  it('selects the chamber by its role, and any other reading by its id', () => {
    expect(
      selectedReadings(KLIPPER, ['chamber', 'enclosure']).map(
        (reading) => reading.id,
      ),
    ).toEqual(['chamber', 'enclosure']);
    expect(selectedReadings(KLIPPER, ['heater_bed'])).toHaveLength(1);
  });

  it('keeps a reading id a printer describes in a saved card', () => {
    const saved = readCardSettings(temperatureSchema, {
      heaters: ['nozzle', 'enclosure'],
    });

    expect(saved.heaters).toEqual(['nozzle', 'enclosure']);
  });

  it('starts a new card on the nozzle, the bed and the chamber', () => {
    expect(readCardSettings(temperatureSchema, {}).heaters).toEqual([
      'nozzle',
      'bed',
      'chamber',
    ]);
  });

  it('selects nothing a printer does not describe', () => {
    expect(selectedReadings([], ['nozzle', 'bed', 'chamber'])).toEqual([]);
    expect(selectedReadings(KLIPPER, ['nozzle'])).toEqual([]);
  });

  it('never selects a reading with no role whose id is a role or the nozzle', () => {
    const odd = readingsOf({
      ...describedReading({ id: 'bed', valuePath: '/b' }),
      ...describedReading({ id: 'nozzle', valuePath: '/n' }),
    });

    expect(selectedReadings(odd, ['bed', 'nozzle'])).toEqual([]);
    expect(offeredReadings(odd)).toEqual([]);
  });

  it('offers each reading under the name that selects it', () => {
    expect(offeredReadings(KLIPPER).map(heaterNameOf)).toEqual([
      'bed',
      'chamber',
      'enclosure',
    ]);
  });
});

describe('the words of a reading line', () => {
  it('translates a reading with a role', () => {
    const [bed, chamber] = KLIPPER;

    expect(bed === undefined ? '' : readingLabel(bed)).toBe('Bed');
    expect(chamber === undefined ? '' : readingTargetLabel(chamber)).toBe(
      'Chamber target',
    );
  });

  it('shows the name of a reading with no role, else its id', () => {
    const [named, plain] = readingsOf({
      ...DESCRIBED_ENCLOSURE,
      ...describedReading({ id: 'probe', valuePath: '/p', targetPath: '/t' }),
    });

    expect(named === undefined ? '' : readingLabel(named)).toBe('Enclosure');
    expect(plain === undefined ? '' : readingLabel(plain)).toBe('probe');
    expect(plain === undefined ? '' : readingTargetLabel(plain)).toBe(
      'probe target',
    );
  });
});
