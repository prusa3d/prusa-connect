// A card's settings, checked against the card's schema (ADR-20261008-sww rule 19),
// and a card's declaration, checked against the rules the core holds a
// manifest to (rules 16 to 18).
//
// The settings of a card are kept on the dashboard as the page wrote them,
// and the core checks only that they are an object of a bounded size. So the
// page checks every value against the schema before a card receives it, and
// a value that fails takes the field's default. A card never draws from a
// value nobody checked, whatever an older page or another client stored.
import type {
  CardDeclaration,
  CardSetting,
  CardSettings,
  CardSize,
  SettingOption,
} from './card-types';

/** One DNS label, the grammar of a card's id and of each namespace label. */
const LABEL = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

/**
 * A card's type: a namespace, a slash and the card's id. The namespace is the
 * reversed module path, dots and all, or `core` for the core's own cards. It
 * is the pattern the core holds a dashboard's card types to.
 */
const CARD_TYPE =
  /^[a-z0-9]([a-z0-9.-]{0,126}[a-z0-9])?\/[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$/;

/** A key a card's settings keep a value under. */
const SETTING_KEY = /^[a-z][a-z0-9_]{0,31}$/;

/** True for a card type a dashboard may hold, such as `core/printer`. */
export const isCardType = (value: string): boolean => CARD_TYPE.test(value);

/** The bounds of a declaration, as the core holds a manifest to them. */
export const CARD_COLUMNS = 12;
export const CARD_ROWS = 32;
const MAX_DESCRIPTION_BYTES = 200;
const MAX_SETTINGS = 16;
const MAX_OPTIONS = 32;
const MAX_TEXT_BYTES = 2000;
const MAX_NOTE_BYTES = 3000;
const DEFAULT_TEXT_BYTES = 200;
const DEFAULT_NOTE_BYTES = 2000;
/** The most printers a printers field keeps, a bound on a stored list. */
const MAX_PRINTERS = 512;

/** How many bytes a text takes as UTF-8, which is how the core counts. */
const bytesOf = (value: string): number =>
  new TextEncoder().encode(value).length;

/** True for a list of distinct strings, each one of `allowed` when given. */
const distinctTexts = (
  value: unknown,
  most: number,
  allowed?: readonly SettingOption[],
): value is readonly string[] =>
  Array.isArray(value) &&
  value.length <= most &&
  value.every(
    (item: unknown) =>
      typeof item === 'string' &&
      (allowed === undefined ||
        allowed.some((option) => option.value === item)),
  ) &&
  new Set(value).size === value.length;

/** True for a number a number field takes. */
const fitsNumber = (
  value: unknown,
  setting: Extract<CardSetting, { kind: 'number' }>,
): value is number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) return false;
  if (setting.min !== undefined && value < setting.min) return false;
  if (setting.max !== undefined && value > setting.max) return false;
  if (setting.step === undefined) return true;
  const steps = (value - (setting.min ?? 0)) / setting.step;

  // A step written as 0.1 cannot be held exactly, so a value is on a step
  // when it is within a millionth of one.
  return Math.abs(steps - Math.round(steps)) < 1e-6;
};

/**
 * The number a number field keeps for one a person typed: on the nearest
 * step, counted from the field's minimum, then inside its bounds. A maximum
 * that is not on a step gives the last step under it.
 */
export const keptNumber = (
  setting: Extract<CardSetting, { kind: 'number' }>,
  typed: number,
): number => {
  const { min, max, step } = setting;
  const base = min ?? 0;
  // A step written as 0.1 cannot be held exactly, so the result is rounded
  // to twelve significant digits, far below any step a schema writes.
  const onStep = (value: number, round: (steps: number) => number): number =>
    step === undefined
      ? value
      : Number((base + round((value - base) / step) * step).toPrecision(12));

  let kept = onStep(typed, Math.round);
  if (min !== undefined && kept < min) kept = min;
  if (max !== undefined && kept > max) kept = onStep(max, Math.floor);

  return kept;
};

/** True when a raw value fits one field of the schema. */
const fits = (setting: CardSetting, value: unknown): boolean => {
  switch (setting.kind) {
    case 'printer':
    case 'tag':
    case 'camera':
      return typeof value === 'string' && bytesOf(value) <= 256;
    case 'printers':
      return distinctTexts(value, MAX_PRINTERS);
    case 'choice':
      return (
        typeof value === 'string' &&
        setting.options.some((option) => option.value === value)
      );
    case 'choices':
      return setting.open === true
        ? distinctTexts(value, MAX_OPTIONS) &&
            value.every((item) => item !== '' && bytesOf(item) <= 64)
        : distinctTexts(value, setting.options.length, setting.options);
    case 'number':
      return fitsNumber(value, setting);
    case 'text':
      return (
        typeof value === 'string' &&
        bytesOf(value) <= setting.max_bytes &&
        !/[\r\n]/.test(value)
      );
    case 'note':
      return typeof value === 'string' && bytesOf(value) <= setting.max_bytes;
    case 'switch':
      return typeof value === 'boolean';
  }
};

/**
 * A card's settings as the card receives them: one value for every field of
 * the schema, the stored one when it fits the field and the default when it
 * does not. A key the schema does not name is dropped, and a key the stored
 * object carries only through its prototype is never read.
 */
export const readCardSettings = (
  schema: readonly CardSetting[],
  raw: unknown,
): CardSettings => {
  const stored =
    typeof raw === 'object' && raw !== null && !Array.isArray(raw)
      ? (raw as Readonly<Record<string, unknown>>) // narrowed on the line above
      : {};
  const out: Record<string, unknown> = {};

  for (const setting of schema) {
    const value = Object.hasOwn(stored, setting.key)
      ? stored[setting.key]
      : undefined;
    out[setting.key] = fits(setting, value) ? value : setting.default;
  }

  return out;
};

/** The settings a new card of one declaration starts with. */
export const defaultCardSettings = (
  declaration: CardDeclaration,
): CardSettings => readCardSettings(declaration.settings, {});

const sizeProblem = (size: CardSize, name: string): string | undefined => {
  if (!Number.isInteger(size.w) || size.w < 1 || size.w > CARD_COLUMNS) {
    return `${name}.w is ${String(size.w)}, and a card covers 1 to ${String(CARD_COLUMNS)} columns`;
  }
  if (!Number.isInteger(size.h) || size.h < 1 || size.h > CARD_ROWS) {
    return `${name}.h is ${String(size.h)}, and a card covers 1 to ${String(CARD_ROWS)} rows`;
  }

  return undefined;
};

const optionsProblem = (
  options: readonly SettingOption[],
  where: string,
): string | undefined => {
  if (options.length === 0 || options.length > MAX_OPTIONS) {
    return `${where}.options has ${String(options.length)} options, and a choice has 1 to ${String(MAX_OPTIONS)}`;
  }
  if (options.some((option) => option.value === '' || option.label === '')) {
    return `${where}.options holds an empty value or label`;
  }
  if (new Set(options.map((option) => option.value)).size !== options.length) {
    return `${where}.options holds one value twice`;
  }

  return undefined;
};

const settingProblem = (
  setting: CardSetting,
  where: string,
): string | undefined => {
  if (!SETTING_KEY.test(setting.key)) {
    return `${where}.key is "${setting.key}", and a key is a lower-case letter followed by at most 31 lower-case letters, digits and underscores`;
  }
  if (setting.label.trim() === '') return `${where}.label is empty`;

  switch (setting.kind) {
    case 'choice':
    case 'choices': {
      const problem = optionsProblem(setting.options, where);
      if (problem !== undefined) return problem;
      break;
    }
    case 'number':
      if (
        setting.min !== undefined &&
        setting.max !== undefined &&
        setting.min > setting.max
      ) {
        return `${where}.min is larger than ${where}.max`;
      }
      if (setting.step !== undefined && setting.step <= 0) {
        return `${where}.step is ${String(setting.step)}, and a step is larger than 0`;
      }
      break;
    case 'text':
    case 'note': {
      const most = setting.kind === 'text' ? MAX_TEXT_BYTES : MAX_NOTE_BYTES;
      if (
        !Number.isInteger(setting.max_bytes) ||
        setting.max_bytes < 1 ||
        setting.max_bytes > most
      ) {
        return `${where}.max_bytes is ${String(setting.max_bytes)}, and a ${setting.kind} field holds 1 to ${String(most)} bytes`;
      }
      break;
    }
    default:
      break;
  }

  if (!fits(setting, setting.default)) {
    return `${where}.default does not fit the field`;
  }

  return undefined;
};

/**
 * Why a declaration breaks the rules the core holds a manifest to, or
 * undefined when it keeps them. The core's own cards are held to it, because
 * they never pass through a manifest the core checks at install.
 */
export const cardDeclarationProblem = (
  declaration: CardDeclaration,
): string | undefined => {
  const where = `card ${declaration.id}`;
  if (!LABEL.test(declaration.id)) {
    return `${where}: the id is not one DNS label`;
  }
  if (declaration.title.trim() === '') return `${where}: the title is empty`;
  const description = declaration.description.trim();
  if (description === '') return `${where}: the description is empty`;
  if (bytesOf(description) > MAX_DESCRIPTION_BYTES) {
    return `${where}: the description is over ${String(MAX_DESCRIPTION_BYTES)} bytes`;
  }

  const size =
    sizeProblem(declaration.size, 'size') ??
    sizeProblem(declaration.least, 'least');
  if (size !== undefined) return `${where}: ${size}`;
  if (
    declaration.least.w > declaration.size.w ||
    declaration.least.h > declaration.size.h
  ) {
    return `${where}: least is larger than size`;
  }

  if (declaration.settings.length > MAX_SETTINGS) {
    return `${where}: ${String(declaration.settings.length)} settings, and the limit is ${String(MAX_SETTINGS)}`;
  }
  const keys = new Set<string>();
  for (const [index, setting] of declaration.settings.entries()) {
    const problem = settingProblem(setting, `settings[${String(index)}]`);
    if (problem !== undefined) return `${where}: ${problem}`;
    if (keys.has(setting.key)) {
      return `${where}: settings[${String(index)}].key "${setting.key}" is there twice`;
    }
    keys.add(setting.key);
  }

  return undefined;
};

/** A field of a card as the listing publishes it, before it is narrowed. */
interface WireSetting {
  readonly key: string;
  readonly kind: string;
  readonly label: string;
  readonly default?: unknown;
  readonly options?: readonly SettingOption[] | null | undefined;
  readonly min?: number | null | undefined;
  readonly max?: number | null | undefined;
  readonly step?: number | null | undefined;
  readonly max_bytes?: number | null | undefined;
}

/** A card as the listing publishes it, before it is narrowed. */
export interface WireCard {
  readonly id: string;
  readonly title: string;
  readonly description: string;
  readonly icon?: string | null | undefined;
  readonly size: CardSize;
  readonly least: CardSize;
  readonly settings?: readonly WireSetting[] | null | undefined;
}

const orUndefined = (value: number | null | undefined): number | undefined =>
  value ?? undefined;

/** One published field as the schema reads it, or undefined for a kind it does not know. */
const settingOf = (wire: WireSetting): CardSetting | undefined => {
  const base = { key: wire.key, label: wire.label };
  const options = wire.options ?? [];

  switch (wire.kind) {
    case 'printer':
    case 'tag':
    case 'camera':
      return {
        ...base,
        kind: wire.kind,
        default: typeof wire.default === 'string' ? wire.default : '',
      };
    case 'printers':
      return {
        ...base,
        kind: 'printers',
        default: distinctTexts(wire.default, MAX_PRINTERS) ? wire.default : [],
      };
    case 'choice':
      return {
        ...base,
        kind: 'choice',
        options,
        default:
          typeof wire.default === 'string'
            ? wire.default
            : (options[0]?.value ?? ''),
      };
    case 'choices':
      return {
        ...base,
        kind: 'choices',
        options,
        default: distinctTexts(wire.default, options.length, options)
          ? wire.default
          : [],
      };
    case 'number':
      return {
        ...base,
        kind: 'number',
        min: orUndefined(wire.min),
        max: orUndefined(wire.max),
        step: orUndefined(wire.step),
        default:
          typeof wire.default === 'number'
            ? wire.default
            : (orUndefined(wire.min) ?? 0),
      };
    case 'text':
    case 'note':
      return {
        ...base,
        kind: wire.kind,
        max_bytes:
          wire.max_bytes ??
          (wire.kind === 'text' ? DEFAULT_TEXT_BYTES : DEFAULT_NOTE_BYTES),
        default: typeof wire.default === 'string' ? wire.default : '',
      };
    case 'switch':
      return {
        ...base,
        kind: 'switch',
        default: typeof wire.default === 'boolean' ? wire.default : false,
      };
    default:
      return undefined;
  }
};

/**
 * A published card as the page reads it, or undefined when it breaks the
 * rules. The core checked it at install, so undefined means a listing from a
 * core this library does not understand, and the card is left out.
 */
export const readCardDeclaration = (
  wire: WireCard,
): CardDeclaration | undefined => {
  const settings: CardSetting[] = [];
  for (const field of wire.settings ?? []) {
    const setting = settingOf(field);
    if (setting === undefined) return undefined;
    settings.push(setting);
  }

  const declaration: CardDeclaration = {
    id: wire.id,
    title: wire.title,
    description: wire.description,
    icon: wire.icon ?? undefined,
    size: { w: wire.size.w, h: wire.size.h },
    least: { w: wire.least.w, h: wire.least.h },
    settings,
  };

  return cardDeclarationProblem(declaration) === undefined
    ? declaration
    : undefined;
};
