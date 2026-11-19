import { describe, expect, it } from 'vitest';

import {
  cardDeclarationProblem,
  defaultCardSettings,
  isCardType,
  keptNumber,
  readCardDeclaration,
  readCardSettings,
} from './card-settings';
import type { CardDeclaration, CardSetting } from './card-types';

// One field of every kind, the schema every case below reads against.
const schema: readonly CardSetting[] = [
  { key: 'device', kind: 'printer', label: 'Printer', default: '' },
  { key: 'fleet', kind: 'printers', label: 'Printers', default: [] },
  { key: 'tag', kind: 'tag', label: 'Tag', default: '' },
  { key: 'camera', kind: 'camera', label: 'Camera', default: '' },
  {
    key: 'unit',
    kind: 'choice',
    label: 'Unit',
    options: [
      { value: 'g', label: 'Grams' },
      { value: 'm', label: 'Metres' },
    ],
    default: 'g',
  },
  {
    key: 'show',
    kind: 'choices',
    label: 'Show',
    options: [
      { value: 'g', label: 'Grams' },
      { value: 'm', label: 'Metres' },
    ],
    default: ['m'],
  },
  {
    key: 'warn',
    kind: 'number',
    label: 'Warn below',
    min: 0,
    max: 1000,
    step: 10,
    default: 100,
  },
  { key: 'line', kind: 'text', label: 'Line', max_bytes: 4, default: '' },
  { key: 'note', kind: 'note', label: 'Note', max_bytes: 6, default: '' },
  { key: 'alarm', kind: 'switch', label: 'Alarm', default: true },
];

const defaults = {
  device: '',
  fleet: [],
  tag: '',
  camera: '',
  unit: 'g',
  show: ['m'],
  warn: 100,
  line: '',
  note: '',
  alarm: true,
};

const spool: CardDeclaration = {
  id: 'spool',
  title: 'Spool',
  description: 'The spool on one printer.',
  size: { w: 4, h: 3 },
  least: { w: 2, h: 2 },
  settings: schema,
};

describe('readCardSettings', () => {
  it('keeps a stored value of every kind that fits its field', () => {
    const stored = {
      device: 'p1',
      fleet: ['p1', 'p2'],
      tag: 'shop',
      camera: 'c1',
      unit: 'm',
      show: ['g', 'm'],
      // 30.000000000000004, which is on a step of 10 once the float is forgiven.
      warn: (0.1 + 0.2) * 100,
      line: 'éé',
      note: 'a\nb\nc',
      alarm: false,
    };

    expect(readCardSettings(schema, stored)).toEqual(stored);
  });

  it('puts the default in place of every value that does not fit', () => {
    expect(
      readCardSettings(schema, {
        device: 7,
        fleet: ['p1', 'p1'],
        tag: null,
        camera: ['c1'],
        unit: 'kg',
        show: ['g', 'x'],
        warn: 105,
        line: 'ééé',
        note: 'seven b',
        alarm: 'yes',
      }),
    ).toEqual(defaults);
  });

  it('refuses a number past a bound, one that is not finite, and a line break in text', () => {
    const read = (warn: unknown, line: string) =>
      readCardSettings(schema, { warn, line });

    expect(read(1010, 'a\nb')).toMatchObject({ warn: 100, line: '' });
    expect(read(Number.NaN, 'a\rb')).toMatchObject({ warn: 100, line: '' });
    expect(read(-10, 'ab')).toMatchObject({ warn: 100, line: 'ab' });
  });

  it('drops a key the schema does not name, and never reads one through the prototype', () => {
    const stored = Object.create({ alarm: false }) as Record<string, unknown>;
    stored.extra = 'x';

    const read = readCardSettings(schema, stored);

    expect(read).toEqual(defaults);
    expect(Object.keys(read)).not.toContain('extra');
  });

  it('reads anything that is not an object as no settings at all', () => {
    for (const raw of [null, undefined, 'x', 3, ['device']]) {
      expect(readCardSettings(schema, raw)).toEqual(defaults);
    }
  });

  it('starts a new card at its defaults', () => {
    expect(defaultCardSettings(spool)).toEqual(defaults);
  });

  it('keeps a value no option names in an open field, and nothing empty or repeated', () => {
    const open: readonly CardSetting[] = [
      {
        key: 'lines',
        kind: 'choices',
        label: 'Lines',
        options: [{ value: 'nozzle', label: 'Nozzle' }],
        default: ['nozzle'],
        open: true,
      },
    ];
    const read = (lines: unknown) => readCardSettings(open, { lines }).lines;

    expect(read(['nozzle', 'enclosure'])).toEqual(['nozzle', 'enclosure']);
    expect(read(['nozzle', ''])).toEqual(['nozzle']);
    expect(read(['x', 'x'])).toEqual(['nozzle']);
    expect(read(['a'.repeat(65)])).toEqual(['nozzle']);
  });
});

describe('isCardType', () => {
  it('takes a namespace, a slash and one label', () => {
    expect(isCardType('core/printer')).toBe(true);
    expect(isCardType('com.github.aux.acme.spools/spool')).toBe(true);
  });

  it('refuses anything else', () => {
    for (const type of [
      'core',
      'core/',
      '/printer',
      'Core/printer',
      'core/a/b',
      'core/-a',
    ]) {
      expect(isCardType(type)).toBe(false);
    }
  });
});

describe('cardDeclarationProblem', () => {
  it('passes a declaration that keeps every rule', () => {
    expect(cardDeclarationProblem(spool)).toBeUndefined();
  });

  it.each<[string, Partial<CardDeclaration>, string]>([
    ['an id that is not a label', { id: 'Spool' }, 'DNS label'],
    ['no title', { title: ' ' }, 'title'],
    ['no description', { description: '' }, 'description'],
    ['a long description', { description: 'd'.repeat(201) }, '200'],
    ['a width past the grid', { size: { w: 13, h: 3 } }, 'columns'],
    ['a height past the grid', { size: { w: 4, h: 33 } }, 'rows'],
    ['a least larger than the size', { least: { w: 5, h: 2 } }, 'larger'],
    [
      'too many settings',
      {
        settings: Array.from({ length: 17 }, (_, index) => ({
          key: `k${String(index)}`,
          kind: 'switch' as const,
          label: 'K',
          default: false,
        })),
      },
      '16',
    ],
    [
      'a key written twice',
      {
        settings: [
          { key: 'k', kind: 'switch', label: 'K', default: false },
          { key: 'k', kind: 'tag', label: 'T', default: '' },
        ],
      },
      'twice',
    ],
    [
      'a key that is not a name',
      {
        settings: [{ key: 'Big', kind: 'switch', label: 'K', default: false }],
      },
      'lower-case',
    ],
    [
      'a choice with no options',
      {
        settings: [
          { key: 'k', kind: 'choice', label: 'K', options: [], default: '' },
        ],
      },
      'options',
    ],
    [
      'min over max',
      {
        settings: [
          { key: 'k', kind: 'number', label: 'K', min: 5, max: 1, default: 5 },
        ],
      },
      'larger',
    ],
    [
      'a step of nothing',
      {
        settings: [
          { key: 'k', kind: 'number', label: 'K', step: 0, default: 0 },
        ],
      },
      'step',
    ],
    [
      'text past its bound',
      {
        settings: [
          { key: 'k', kind: 'text', label: 'K', max_bytes: 2001, default: '' },
        ],
      },
      '2000',
    ],
    [
      'a default that does not fit',
      {
        settings: [
          { key: 'k', kind: 'number', label: 'K', max: 5, default: 6 },
        ],
      },
      'default',
    ],
  ])('names %s', (_name, override, wanted) => {
    expect(cardDeclarationProblem({ ...spool, ...override })).toContain(wanted);
  });
});

describe('readCardDeclaration', () => {
  it('reads a published card, filling what the wire leaves out', () => {
    expect(
      readCardDeclaration({
        id: 'spool',
        title: 'Spool',
        description: 'The spool on one printer.',
        icon: null,
        size: { w: 4, h: 3 },
        least: { w: 2, h: 2 },
        settings: [
          { key: 'n', kind: 'number', label: 'N', min: 5, max: null },
          { key: 'note', kind: 'note', label: 'Note' },
        ],
      }),
    ).toEqual({
      ...spool,
      icon: undefined,
      settings: [
        {
          key: 'n',
          kind: 'number',
          label: 'N',
          min: 5,
          max: undefined,
          step: undefined,
          default: 5,
        },
        {
          key: 'note',
          kind: 'note',
          label: 'Note',
          max_bytes: 2000,
          default: '',
        },
      ],
    });
  });

  it('leaves out a card with a field of a kind it does not know', () => {
    expect(
      readCardDeclaration({
        ...spool,
        settings: [{ key: 'k', kind: 'colour', label: 'K' }],
      }),
    ).toBeUndefined();
  });
});

describe('keptNumber', () => {
  const field = (
    min?: number,
    max?: number,
    step?: number,
  ): Extract<CardSetting, { kind: 'number' }> => ({
    key: 'n',
    kind: 'number',
    label: 'N',
    default: 0,
    ...(min === undefined ? {} : { min }),
    ...(max === undefined ? {} : { max }),
    ...(step === undefined ? {} : { step }),
  });

  it('keeps a number inside the bounds and on a step as it is', () => {
    expect(keptNumber(field(0, 100, 5), 35)).toBe(35);
  });

  it('holds a number to the bounds', () => {
    expect(keptNumber(field(1, 10), 0)).toBe(1);
    expect(keptNumber(field(1, 10), 99)).toBe(10);
  });

  it('snaps to the nearest step counted from the minimum', () => {
    expect(keptNumber(field(1, 100, 5), 8)).toBe(6);
    expect(keptNumber(field(1, 100, 5), 9)).toBe(11);
  });

  it('gives the last step under a maximum that is not on a step', () => {
    expect(keptNumber(field(0, 12, 5), 50)).toBe(10);
  });

  it('snaps a fractional step without a rounding error', () => {
    expect(keptNumber(field(0, 1, 0.1), 0.34)).toBe(0.3);
  });

  it('keeps any number for a field with no bounds and no step', () => {
    expect(keptNumber(field(), -7.25)).toBe(-7.25);
  });
});
