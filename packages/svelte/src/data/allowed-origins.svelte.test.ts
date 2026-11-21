import { describe, expect, it } from 'vitest';

import { AllowedOrigins } from './allowed-origins.svelte';
import type { AllowedOrigin, AllowedOriginsClient } from './services/origins';
import { StubApiError } from './stubs';

const FILE: AllowedOrigin = {
  origin: 'https://file.example',
  source: 'file',
};
const EXTENSION: AllowedOrigin = {
  origin: 'https://slicer.example',
  source: 'extension',
  extension: 'com.github.acme.slicer',
  title: 'Web slicer',
};

interface Held {
  fixed: AllowedOrigin[];
  stored: string[];
  written: string[][];
  refusal?: StubApiError;
}

const stubClient = (
  stored: string[] = [],
): { client: AllowedOriginsClient; held: Held } => {
  const held: Held = { fixed: [FILE, EXTENSION], stored, written: [] };
  const listed = (): { items: AllowedOrigin[] } => ({
    items: [
      ...held.stored.map((origin): AllowedOrigin => ({
        origin,
        source: 'settings',
      })),
      ...held.fixed,
    ],
  });
  const client: AllowedOriginsClient = {
    listAllowedOrigins: () => Promise.resolve(listed()),
    setAllowedOrigins: (request) => {
      held.written.push(request.body.origins);
      if (held.refusal) return Promise.reject(held.refusal);

      held.stored = request.body.origins;

      return Promise.resolve(listed());
    },
  };

  return { client, held };
};

describe('AllowedOrigins', () => {
  it('starts unread', () => {
    const store = new AllowedOrigins(stubClient().client);

    expect(store.loaded).toBe(false);
    expect(store.items).toEqual([]);
  });

  it('reads every origin with its source', async () => {
    const store = new AllowedOrigins(stubClient(['https://a.example']).client);

    await store.load();

    expect(store.loaded).toBe(true);
    expect(store.items.map((item) => item.source)).toEqual([
      'settings',
      'file',
      'extension',
    ]);
  });

  it('adds an origin to the stored ones alone', async () => {
    const { client, held } = stubClient(['https://a.example']);
    const store = new AllowedOrigins(client);
    await store.load();

    expect(await store.add('https://b.example')).toBe(true);

    expect(held.written).toEqual([['https://a.example', 'https://b.example']]);
    expect(store.items).toHaveLength(4);
  });

  it('removes a stored origin and leaves the others', async () => {
    const { client, held } = stubClient([
      'https://a.example',
      'https://b.example',
    ]);
    const store = new AllowedOrigins(client);
    await store.load();

    await store.remove('https://a.example');

    expect(held.written).toEqual([['https://b.example']]);
    expect(store.items.map((item) => item.origin)).toEqual([
      'https://b.example',
      FILE.origin,
      EXTENSION.origin,
    ]);
  });

  it('keeps the list and the fault when the server refuses', async () => {
    const { client, held } = stubClient(['https://a.example']);
    held.refusal = new StubApiError('core.validation/REQUEST_INVALID', 422);
    const store = new AllowedOrigins(client);
    await store.load();

    expect(await store.add('slicer.example')).toBe(false);

    expect(store.error?.code).toBe('core.validation/REQUEST_INVALID');
    expect(store.items).toHaveLength(3);
    expect(store.saving).toBe(false);
  });

  it('forgets the list on reset', async () => {
    const store = new AllowedOrigins(stubClient(['https://a.example']).client);
    await store.load();

    store.reset();

    expect(store.loaded).toBe(false);
    expect(store.items).toEqual([]);
  });
});
