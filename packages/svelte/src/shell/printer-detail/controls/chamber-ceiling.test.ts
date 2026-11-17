import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { chamberMaxCelsius } from './machine-controls';

// The host holds a chamber target to the bound internal/printer/profile
// declares for the model, and the editor stops at the same figure. Two copies
// of one number drift, so this reads the host's and holds the editor to it.
describe('chamberMaxCelsius', () => {
  const bounds: unknown = JSON.parse(
    readFileSync(
      new URL(
        '../../../../../../internal/printer/profile/bounds.json',
        import.meta.url,
      ),
      'utf8',
    ),
  );
  const rows =
    typeof bounds === 'object' &&
    bounds !== null &&
    'bounds' in bounds &&
    Array.isArray(bounds.bounds)
      ? (bounds.bounds as { argument: string; models: string[]; max: number }[])
      : [];
  const chamberRows = rows.filter(
    (row) =>
      row.argument ===
      'core.command.set_chamber_target_temp#chamber.target_temp',
  );

  it('finds the chamber rows', () => {
    expect(chamberRows.length).toBeGreaterThan(0);
  });

  it.each(
    chamberRows.flatMap((row) =>
      row.models.map((model) => ({ model, max: row.max })),
    ),
  )('stops a $model at $max', ({ model, max }) => {
    expect(chamberMaxCelsius(model)).toBe(max);
  });
});
