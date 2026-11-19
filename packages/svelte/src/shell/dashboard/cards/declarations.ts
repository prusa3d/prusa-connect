// The core's own cards, declared the way a module declares a card in its
// manifest (ADR-20261008-sww rules 16 to 18). Each title, description and label is a
// key of the library's catalogue; the settings form reads it through `t`,
// and a module's plain text reads back as itself.
//
// A core card names no icon. A module picks one from the closed list the
// library publishes, and that list has no clock, camera or note. So the
// gallery takes a core card's glyph from icons.ts, by the card's type.
//
// A card declared here and a card a module declares go through one registry
// and one settings form, so the contract a module author reads is the one
// the core's own cards are held to.
import { PRINTER_ORDERS } from '../printers';
import type { CardDeclaration } from '../../../slots';
import {
  ALIGNS,
  HEATERS,
  NOTE_SIZES,
  PICTURES,
  PRINTER_FIELDS,
  SCOPES,
  SIZES,
  SPANS,
  TONES,
} from './config';
import {
  ALIGN,
  FIELD,
  HEATER,
  NOTE_SIZE,
  ORDER,
  PICTURE,
  SCOPE,
  SIZE,
  TONE,
} from './labels';

/** The line each span of a chart reads. */
const SPAN: Readonly<Record<(typeof SPANS)[number], string>> = {
  5: 'dashboard.span.5',
  15: 'dashboard.span.15',
  30: 'dashboard.span.30',
  60: 'dashboard.span.60',
};

/** The options of a setting, each with the catalogue key it reads. */
const options = <T extends string>(
  values: readonly T[],
  keys: Readonly<Record<T, string>>,
): { value: T; label: string }[] =>
  values.map((value) => ({ value, label: keys[value] }));

/** The printers a card of many shows: chosen ones, or all when none is. */
const devices = {
  key: 'devices',
  kind: 'printers',
  label: 'dashboard.settings.printers',
  default: [],
} as const;

const tag = {
  key: 'tag',
  kind: 'tag',
  label: 'dashboard.settings.tag',
  default: '',
} as const;

const device = {
  key: 'device',
  kind: 'printer',
  label: 'dashboard.settings.printer',
  default: '',
} as const;

export const CORE_CARDS: readonly CardDeclaration[] = [
  {
    id: 'printer',
    title: 'dashboard.kind.printer.name',
    description: 'dashboard.kind.printer.about',
    size: { w: 4, h: 5 },
    least: { w: 2, h: 3 },
    settings: [
      device,
      {
        key: 'picture',
        kind: 'choice',
        label: 'dashboard.settings.picture',
        options: options(PICTURES, PICTURE),
        default: 'art',
      },
      {
        key: 'fields',
        kind: 'choices',
        label: 'dashboard.settings.fields',
        options: options(PRINTER_FIELDS, FIELD),
        default: [...PRINTER_FIELDS],
      },
    ],
  },
  {
    id: 'printers',
    title: 'dashboard.kind.printers.name',
    description: 'dashboard.kind.printers.about',
    size: { w: 8, h: 6 },
    least: { w: 3, h: 3 },
    settings: [
      devices,
      tag,
      {
        key: 'scope',
        kind: 'choice',
        label: 'dashboard.settings.scope',
        options: options(SCOPES, SCOPE),
        default: 'all',
      },
      {
        key: 'order',
        kind: 'choice',
        label: 'dashboard.settings.order',
        options: options(PRINTER_ORDERS, ORDER),
        default: 'state',
      },
      {
        key: 'size',
        kind: 'choice',
        label: 'dashboard.settings.size',
        options: options(SIZES, SIZE),
        default: 'tile',
      },
      {
        key: 'fields',
        kind: 'choices',
        label: 'dashboard.settings.fields',
        options: options(PRINTER_FIELDS, FIELD),
        default: ['state', 'job', 'progress', 'remaining'],
      },
    ],
  },
  {
    id: 'summary',
    title: 'dashboard.kind.summary.name',
    description: 'dashboard.kind.summary.about',
    size: { w: 12, h: 2 },
    least: { w: 3, h: 2 },
    settings: [devices, tag],
  },
  {
    id: 'finishing',
    title: 'dashboard.kind.finishing.name',
    description: 'dashboard.kind.finishing.about',
    size: { w: 4, h: 5 },
    least: { w: 3, h: 3 },
    settings: [
      devices,
      tag,
      {
        key: 'limit',
        kind: 'number',
        label: 'dashboard.settings.limit',
        min: 1,
        max: 20,
        step: 1,
        default: 5,
      },
    ],
  },
  {
    id: 'temperature',
    title: 'dashboard.kind.temperature.name',
    description: 'dashboard.kind.temperature.about',
    size: { w: 6, h: 4 },
    least: { w: 3, h: 3 },
    settings: [
      device,
      {
        key: 'heaters',
        kind: 'choices',
        label: 'dashboard.settings.heaters',
        options: options(HEATERS, HEATER),
        default: [...HEATERS],
        // A printer may describe a reading of its own, kept by its id.
        open: true,
      },
      {
        key: 'minutes',
        kind: 'choice',
        label: 'dashboard.settings.span',
        options: SPANS.map((minutes) => ({
          value: String(minutes),
          label: SPAN[minutes],
        })),
        default: '15',
      },
    ],
  },
  {
    id: 'camera',
    title: 'dashboard.kind.camera.name',
    description: 'dashboard.kind.camera.about',
    size: { w: 4, h: 4 },
    least: { w: 2, h: 2 },
    settings: [
      {
        key: 'camera',
        kind: 'camera',
        label: 'dashboard.settings.camera',
        default: '',
      },
    ],
  },
  {
    id: 'note',
    title: 'dashboard.kind.note.name',
    description: 'dashboard.kind.note.about',
    size: { w: 4, h: 2 },
    least: { w: 1, h: 1 },
    settings: [
      {
        key: 'text',
        kind: 'note',
        label: 'dashboard.settings.text',
        max_bytes: 3000,
        default: '',
      },
      {
        key: 'size',
        kind: 'choice',
        label: 'dashboard.settings.textSize',
        options: options(NOTE_SIZES, NOTE_SIZE),
        default: 'md',
      },
      {
        key: 'align',
        kind: 'choice',
        label: 'dashboard.settings.align',
        options: options(ALIGNS, ALIGN),
        default: 'start',
      },
      {
        key: 'tone',
        kind: 'choice',
        label: 'dashboard.settings.tone',
        options: options(TONES, TONE),
        default: 'plain',
      },
    ],
  },
  {
    id: 'clock',
    title: 'dashboard.kind.clock.name',
    description: 'dashboard.kind.clock.about',
    size: { w: 3, h: 2 },
    least: { w: 2, h: 1 },
    settings: [
      {
        key: 'seconds',
        kind: 'switch',
        label: 'dashboard.settings.seconds',
        default: false,
      },
      {
        key: 'date',
        kind: 'switch',
        label: 'dashboard.settings.date',
        default: true,
      },
    ],
  },
  {
    id: 'issues',
    title: 'dashboard.kind.issues.name',
    description: 'dashboard.kind.issues.about',
    size: { w: 4, h: 4 },
    least: { w: 3, h: 2 },
    settings: [devices, tag],
  },
];
