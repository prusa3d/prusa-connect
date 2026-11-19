import { describe, expect, it } from 'vitest';

import fixtures from '../test/ux/fixtures/api.json';
import { devicesAnswer, fleetOf } from './fleet';

const recorded = (
  JSON.parse(fixtures['GET /api/v1/devices'].body) as {
    items: { id: string; name: string; serial: string }[];
  }
).items;
const identity = ({ id, name, serial }: (typeof recorded)[number]) => ({
  id,
  name,
  serial,
});

describe('the fleet', () => {
  it('holds one, two or twelve printers', () => {
    expect(fleetOf('one')).toHaveLength(1);
    expect(fleetOf('two')).toHaveLength(2);
    expect(fleetOf('many')).toHaveLength(12);
  });

  it('gives every printer its own id and name', () => {
    const many = fleetOf('many');
    expect(new Set(many.map((d) => d.id)).size).toBe(12);
    expect(new Set(many.map((d) => d.name)).size).toBe(12);
  });

  it('starts with the recorded printers, so the rest of the table still points at them', () => {
    // The demo decorates their jobs and tools, so only who they are is compared.
    expect(fleetOf('many').slice(0, 5).map(identity)).toEqual(
      recorded.map(identity),
    );
  });

  it('answers a listing whose total is its length', async () => {
    const body = (await devicesAnswer('two').json()) as {
      items: unknown[];
      total: number;
    };
    expect(body.items).toHaveLength(2);
    expect(body.total).toBe(2);
  });
});
