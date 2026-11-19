// Numbers, dates, sizes and durations, all through `Intl`.
import { formatting } from './formatting.svelte';
import { locale } from './locale.svelte';

type Cache<T> = Map<string, T>;

const held = <T>(cache: Cache<T>, key: string, make: () => T): T => {
  const found = cache.get(key);
  if (found) return found;

  const made = make();
  cache.set(key, made);

  return made;
};

const NUMBERS: Cache<Intl.NumberFormat> = new Map();
const DATES: Cache<Intl.DateTimeFormat> = new Map();
const RELATIVES: Cache<Intl.RelativeTimeFormat> = new Map();
const LISTS: Cache<Intl.ListFormat> = new Map();

const numberFormat = (options?: Intl.NumberFormatOptions): Intl.NumberFormat =>
  held(
    NUMBERS,
    `${locale.current}:${JSON.stringify(options ?? {})}`,
    () => new Intl.NumberFormat(locale.current, options),
  );

/** A number as this language writes it, separators and decimals included. */
export const formatNumber = (
  value: number,
  options?: Intl.NumberFormatOptions,
): string => numberFormat(options).format(value);

/** The reader's clock, applied to options that ask for a time of day. */
const clocked = (
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormatOptions =>
  options.timeStyle === undefined && options.hour === undefined
    ? options
    : {
        ...options,
        hourCycle: formatting.clock === '12-hour' ? 'h12' : 'h23',
      };

/** A date as this language writes it. */
export const formatDate = (
  value: string | number | Date,
  options: Intl.DateTimeFormatOptions = { dateStyle: 'medium' },
): string => {
  const when = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(when.getTime())) return String(value);

  const wanted = clocked(options);

  return held(
    DATES,
    `${locale.current}:${JSON.stringify(wanted)}`,
    () => new Intl.DateTimeFormat(locale.current, wanted),
  ).format(when);
};

/** A date and a time together. The same rule for a value that is not a date. */
export const formatDateTime = (value: string | number | Date): string =>
  formatDate(value, { dateStyle: 'medium', timeStyle: 'short' });

/** How long ago, or how long until: "3 minutes ago", "in 2 days". */
export const formatRelative = (
  value: number,
  unit: Intl.RelativeTimeFormatUnit,
): string =>
  held(
    RELATIVES,
    locale.current,
    () => new Intl.RelativeTimeFormat(locale.current, { numeric: 'auto' }),
  ).format(value, unit);

/** A list of words as this language joins them. */
export const formatList = (
  items: readonly string[],
  type: Intl.ListFormatType = 'disjunction',
): string =>
  held(
    LISTS,
    `${locale.current}:${type}`,
    () => new Intl.ListFormat(locale.current, { style: 'long', type }),
  ).format(items);

const BYTE_UNITS = ['byte', 'kilobyte', 'megabyte', 'gigabyte', 'terabyte'];
const STEP = 1024;

/** A file size a person reads. */
export const formatBytes = (bytes: number): string => {
  if (!Number.isFinite(bytes)) return formatNumber(0);

  let value = Math.max(0, bytes);
  let step = 0;
  while (value >= STEP && step < BYTE_UNITS.length - 1) {
    value /= STEP;
    step += 1;
  }

  return formatNumber(value, {
    style: 'unit',
    unit: BYTE_UNITS[step] ?? 'byte',
    unitDisplay: 'short',
    // One decimal above a kilobyte.
    minimumFractionDigits: step === 0 ? 0 : 1,
    maximumFractionDigits: step === 0 ? 0 : 1,
  });
};

/** The `Intl` unit behind each symbol a connector writes. */
const UNIT_NAMES: Readonly<Record<string, string>> = {
  '°C': 'celsius',
  '%': 'percent',
  mm: 'millimeter',
};

/** A Celsius reading as the number the reader asked to see. */
export const degreesFor = (celsius: number): number =>
  formatting.temperature === 'fahrenheit' ? celsius * 1.8 + 32 : celsius;

/** A figure the reader typed, in the unit they chose, back in Celsius. */
export const celsiusFor = (degrees: number): number =>
  formatting.temperature === 'fahrenheit' ? (degrees - 32) / 1.8 : degrees;

const scaled = (
  value: number,
  unit: string,
): { value: number; name: string | undefined } =>
  unit === '°C' && formatting.temperature === 'fahrenheit'
    ? { value: degreesFor(value), name: 'fahrenheit' }
    : { value, name: UNIT_NAMES[unit] };

export const formatUnit = (
  value: number,
  unit: string,
  options?: Intl.NumberFormatOptions,
): string => {
  const reading = scaled(value, unit);
  if (reading.name === undefined) {
    return `${formatNumber(reading.value, options)} ${unit}`;
  }

  return formatNumber(reading.value, {
    ...options,
    style: 'unit',
    unit: reading.name,
    unitDisplay: 'short',
  });
};

const MINUTE = 60;
const HOUR = 3600;

const unitText = (value: number, unit: string): string =>
  formatNumber(value, { style: 'unit', unit, unitDisplay: 'short' });

/** A span of seconds a person reads. */
export const formatDuration = (seconds: number): string => {
  if (!Number.isFinite(seconds) || seconds <= 0) return '';

  const whole = Math.floor(seconds);
  if (whole < MINUTE) return unitText(whole, 'second');
  if (whole < HOUR) return unitText(Math.floor(whole / MINUTE), 'minute');

  const hours = unitText(Math.floor(whole / HOUR), 'hour');
  const minutes = Math.floor((whole % HOUR) / MINUTE);

  // A whole number of hours reads "4 hr", not "4 hr, 0 min".
  return minutes === 0
    ? hours
    : formatList([hours, unitText(minutes, 'minute')], 'unit');
};
