// The device-wide readings a driver describes in a device's state, under
// `/readings/<id>/`. The value stays at the path the description names, so a
// reader looks it up there. The nozzle is not one: it lives in the tool tree.

/** The kinds and units this build knows. A reading in any other is skipped. */
export const READING_KINDS = ['temperature'] as const;
export type ReadingKind = (typeof READING_KINDS)[number];

export const READING_UNITS = ['celsius'] as const;
export type ReadingUnit = (typeof READING_UNITS)[number];

/** The roles a client may name a fixed label or topic by. */
export const READING_ROLES = ['bed', 'chamber'] as const;
export type ReadingRole = (typeof READING_ROLES)[number];

export interface Reading {
  readonly id: string;
  readonly kind: ReadingKind;
  readonly unit: ReadingUnit;
  /** The state path of the same device that holds the value. */
  readonly valuePath: string;
  readonly targetPath?: string | undefined;
  /** The driver's own name for it. */
  readonly name?: string | undefined;
  readonly role?: ReadingRole | undefined;
}

const READING_PATH = /^\/readings\/([^/]+)\/[^/]+$/;
const READING_ID = /^[a-z][a-z0-9_]{0,63}$/;

const isOneOf = <T extends string>(
  values: readonly T[],
  value: unknown,
): value is T => values.some((one) => one === value);

const ROLE_ORDER: Readonly<Record<ReadingRole, number>> = {
  bed: 0,
  chamber: 1,
};

const rankOf = (reading: Reading): number =>
  reading.role === undefined ? READING_ROLES.length : ROLE_ORDER[reading.role];

/**
 * The readings one device describes, the bed first, the chamber second and
 * the rest by id. A reading with an unknown kind or unit, an invalid id or a
 * member of the wrong type is left out. An unknown role reads as no role.
 */
export const readingsOf = (
  state: Readonly<Record<string, unknown>>,
): Reading[] => {
  const ids = new Set<string>();
  for (const path of Object.keys(state)) {
    const id = READING_PATH.exec(path)?.[1];
    if (id !== undefined && READING_ID.test(id)) ids.add(id);
  }

  const readings: Reading[] = [];
  for (const id of ids) {
    const member = (name: string): unknown => state[`/readings/${id}/${name}`];
    const kind = member('kind');
    const unit = member('unit');
    const valuePath = member('value_path');
    const targetPath = member('target_path');
    const name = member('name');
    const role = member('role');

    if (!isOneOf(READING_KINDS, kind) || !isOneOf(READING_UNITS, unit)) {
      continue;
    }
    if (typeof valuePath !== 'string' || valuePath === '') continue;
    if (targetPath !== undefined && typeof targetPath !== 'string') continue;
    if (name !== undefined && typeof name !== 'string') continue;
    if (role !== undefined && typeof role !== 'string') continue;

    readings.push({
      id,
      kind,
      unit,
      valuePath,
      ...(targetPath === undefined || targetPath === '' ? {} : { targetPath }),
      ...(name === undefined || name === '' ? {} : { name }),
      ...(isOneOf(READING_ROLES, role) ? { role } : {}),
    });
  }

  return readings.sort(
    (left, right) =>
      rankOf(left) - rankOf(right) ||
      Number(left.id > right.id) - Number(left.id < right.id),
  );
};

/** The reading of one role, or undefined when the device has none. */
export const readingWithRole = (
  readings: readonly Reading[],
  role: ReadingRole,
): Reading | undefined => readings.find((reading) => reading.role === role);

/** What a reading with no role is called: the driver's name, else its id. */
export const readingName = (reading: Reading): string =>
  reading.name ?? reading.id;
