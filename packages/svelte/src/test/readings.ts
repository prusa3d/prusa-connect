// The state members a driver writes to describe one reading, for fixtures.

export interface DescribedReading {
  readonly id: string;
  readonly valuePath: string;
  readonly targetPath?: string;
  readonly role?: string;
  readonly name?: string;
}

export const describedReading = (
  reading: DescribedReading,
): Record<string, string> => {
  const at = `/readings/${reading.id}`;

  return {
    [`${at}/kind`]: 'temperature',
    [`${at}/unit`]: 'celsius',
    [`${at}/value_path`]: reading.valuePath,
    ...(reading.targetPath === undefined
      ? {}
      : { [`${at}/target_path`]: reading.targetPath }),
    ...(reading.role === undefined ? {} : { [`${at}/role`]: reading.role }),
    ...(reading.name === undefined ? {} : { [`${at}/name`]: reading.name }),
  };
};

/** The bed as the xBuddy driver describes it. */
export const DESCRIBED_BED = describedReading({
  id: 'bed',
  valuePath: '/temperature/bed',
  targetPath: '/temperature/bed_target',
  role: 'bed',
});

/** The chamber of an enclosed printer. */
export const DESCRIBED_CHAMBER = describedReading({
  id: 'chamber',
  valuePath: '/temperature/chamber',
  targetPath: '/temperature/chamber_target',
  role: 'chamber',
});

/** A sensor with no role, such as one a person named on a Klipper machine. */
export const DESCRIBED_ENCLOSURE = describedReading({
  id: 'enclosure',
  valuePath: '/sensors/enclosure',
  name: 'Enclosure',
});
