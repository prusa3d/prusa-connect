// Which lines a temperature card draws. A card stores names: `nozzle`, a role
// such as `bed`, or the id of a reading the printer describes.
import {
  type Reading,
  readingName,
  READING_ROLES,
  type ReadingRole,
} from '../../../domain';
import { t } from '../../../i18n';

/** The name that selects the nozzle of the first head. */
export const NOZZLE = 'nozzle';

const isRole = (name: string): name is ReadingRole =>
  READING_ROLES.some((role) => role === name);

/** The name a card stores to select one reading: its role, else its id. */
export const heaterNameOf = (reading: Reading): string =>
  reading.role ?? reading.id;

/**
 * Whether a stored name selects a reading. A role name selects the reading
 * with that role, and any other name the reading with that id, so a reading
 * whose id is a role name or `nozzle` and that has no role is never drawn.
 */
export const selects = (name: string, reading: Reading): boolean =>
  isRole(name) ? reading.role === name : name !== NOZZLE && reading.id === name;

/** The readings a card draws, in the order the printer describes them. */
export const selectedReadings = (
  readings: readonly Reading[],
  names: readonly string[],
): Reading[] =>
  readings.filter((reading) => names.some((name) => selects(name, reading)));

/** The readings a card can offer: every one a stored name can select. */
export const offeredReadings = (readings: readonly Reading[]): Reading[] =>
  readings.filter((reading) => selects(heaterNameOf(reading), reading));

const ROLE_LABEL: Readonly<Record<ReadingRole, string>> = {
  bed: 'dashboard.temperature.bed',
  chamber: 'dashboard.temperature.chamber',
};

const ROLE_TARGET_LABEL: Readonly<Record<ReadingRole, string>> = {
  bed: 'dashboard.temperature.bedTarget',
  chamber: 'dashboard.temperature.chamberTarget',
};

/** The words of a reading's line, in the language on screen. */
export const readingLabel = (reading: Reading): string =>
  reading.role === undefined
    ? readingName(reading)
    : t(ROLE_LABEL[reading.role]);

/** The words of the line of a reading's target. */
export const readingTargetLabel = (reading: Reading): string =>
  reading.role === undefined
    ? t('dashboard.temperature.target', { name: readingName(reading) })
    : t(ROLE_TARGET_LABEL[reading.role]);
