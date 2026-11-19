import { describe, expect, it } from 'vitest';

import {
  DESCRIBED_BED,
  DESCRIBED_CHAMBER,
  DESCRIBED_ENCLOSURE,
  describedReading,
} from '../test/readings';
import { readingName, readingsOf, readingWithRole } from './readings';

describe('readingsOf', () => {
  it('reads a described reading with every member', () => {
    expect(readingsOf({ ...DESCRIBED_BED, '/temperature/bed': 60 })).toEqual([
      {
        id: 'bed',
        kind: 'temperature',
        unit: 'celsius',
        valuePath: '/temperature/bed',
        targetPath: '/temperature/bed_target',
        role: 'bed',
      },
    ]);
  });

  it('reads none from a state that describes none', () => {
    expect(
      readingsOf({ '/temperature/bed': 60, '/tools/0/nozzle_temp': 215 }),
    ).toEqual([]);
  });

  it('keeps a reading whose value is not there yet', () => {
    expect(readingsOf(DESCRIBED_ENCLOSURE)).toEqual([
      {
        id: 'enclosure',
        kind: 'temperature',
        unit: 'celsius',
        valuePath: '/sensors/enclosure',
        name: 'Enclosure',
      },
    ]);
  });

  it('puts the bed first, the chamber second and the rest by id', () => {
    const state = {
      ...describedReading({ id: 'zone_b', valuePath: '/b' }),
      ...DESCRIBED_CHAMBER,
      ...describedReading({ id: 'alpha', valuePath: '/a' }),
      ...DESCRIBED_BED,
    };

    expect(readingsOf(state).map((reading) => reading.id)).toEqual([
      'bed',
      'chamber',
      'alpha',
      'zone_b',
    ]);
  });

  it('orders by role, whatever the id', () => {
    const state = {
      ...describedReading({
        id: 'a_chamber',
        valuePath: '/c',
        role: 'chamber',
      }),
      ...describedReading({ id: 'z_bed', valuePath: '/b', role: 'bed' }),
    };

    expect(readingsOf(state).map((reading) => reading.id)).toEqual([
      'z_bed',
      'a_chamber',
    ]);
  });

  it.each([
    ['kind', { '/readings/x/unit': 'celsius', '/readings/x/value_path': '/x' }],
    [
      'unit',
      { '/readings/x/kind': 'temperature', '/readings/x/value_path': '/x' },
    ],
    [
      'value_path',
      { '/readings/x/kind': 'temperature', '/readings/x/unit': 'celsius' },
    ],
  ])('has no reading while %s is missing', (_, state) => {
    expect(readingsOf(state)).toEqual([]);
  });

  it('skips a reading of a kind this build does not know', () => {
    const state = {
      ...describedReading({ id: 'x', valuePath: '/x' }),
      '/readings/x/kind': 'humidity',
    };

    expect(readingsOf(state)).toEqual([]);
  });

  it('skips a reading in a unit this build does not know', () => {
    const state = {
      ...describedReading({ id: 'x', valuePath: '/x' }),
      '/readings/x/unit': 'fahrenheit',
    };

    expect(readingsOf(state)).toEqual([]);
  });

  it.each([['Bed'], ['1bed'], ['_bed'], ['bed-top'], ['a'.repeat(65)]])(
    'skips the invalid id %s',
    (id) => {
      expect(readingsOf(describedReading({ id, valuePath: '/x' }))).toEqual([]);
    },
  );

  it('takes an id of one letter and one of 64 characters', () => {
    const long = `a${'_9'.repeat(31)}b`;
    const state = {
      ...describedReading({ id: 'a', valuePath: '/a' }),
      ...describedReading({ id: long, valuePath: '/long' }),
    };

    expect(readingsOf(state).map((reading) => reading.id)).toEqual(['a', long]);
  });

  it.each([
    ['kind', 1],
    ['unit', true],
    ['value_path', 7],
    ['target_path', 7],
    ['name', ['Enclosure']],
    ['role', 2],
  ])('skips a reading whose %s has the wrong type', (member, value) => {
    const state = {
      ...describedReading({ id: 'x', valuePath: '/x' }),
      [`/readings/x/${member}`]: value,
    };

    expect(readingsOf(state)).toEqual([]);
  });

  it('reads an unknown role as no role', () => {
    const [reading] = readingsOf(
      describedReading({ id: 'x', valuePath: '/x', role: 'hotend' }),
    );

    expect(reading).toBeDefined();
    expect(reading?.role).toBeUndefined();
  });

  it('ignores a path under /readings/ that is not a member', () => {
    expect(
      readingsOf({
        ...DESCRIBED_BED,
        '/readings/bed': 'x',
        '/readings/bed/deep/kind': 'temperature',
      }),
    ).toHaveLength(1);
  });
});

describe('readingWithRole', () => {
  it('finds the reading of a role, and none the device lacks', () => {
    const readings = readingsOf({ ...DESCRIBED_BED, ...DESCRIBED_ENCLOSURE });

    expect(readingWithRole(readings, 'bed')?.id).toBe('bed');
    expect(readingWithRole(readings, 'chamber')).toBeUndefined();
  });
});

describe('readingName', () => {
  it('is the name, else the id', () => {
    const [named, plain] = readingsOf({
      ...DESCRIBED_ENCLOSURE,
      ...describedReading({ id: 'probe', valuePath: '/p' }),
    });

    expect(named === undefined ? '' : readingName(named)).toBe('Enclosure');
    expect(plain === undefined ? '' : readingName(plain)).toBe('probe');
  });
});
