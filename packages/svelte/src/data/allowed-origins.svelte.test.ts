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
  // Moves with every stored list, as the server's version does.
  version: number;
  written: string[][];
  refusal?: StubApiError;
}

// The server as the store sees it: a write must quote the version of the list
// it replaces.
const stubClient = (
  stored: string[] = [],
): { client: AllowedOriginsClient; held: Held } => {
  const held: Held = {
    fixed: [FILE, EXTENSION],
    stored,
    version: 1,
    written: [],
  };
  const listed = (): { items: AllowedOrigin[]; version: string } => ({
    items: [
      ...held.stored.map((origin): AllowedOrigin => ({
        origin,
        source: 'settings',
      })),
      ...held.fixed,
    ],
    version: String(held.version),
  });
  const client: AllowedOriginsClient = {
    listAllowedOrigins: () => Promise.resolve(listed()),
    setAllowedOrigins: (request) => {
      held.written.push(request.body.origins);
      if (held.refusal) return Promise.reject(held.refusal);
      if (request.header?.['If-Match'] !== `"${String(held.version)}"`) {
        return Promise.reject(
          new StubApiError('core.resource/VERSION_CONFLICT', 412),
        );
      }

      held.stored = request.body.origins;
      held.version += 1;

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

  it('refuses a change to a list another person changed, and shows theirs', async () => {
    const { client, held } = stubClient(['https://old.example']);
    const store = new AllowedOrigins(client);
    await store.load();

    // Another person removes the origin this tab still shows.
    held.stored = [];
    held.version += 1;

    expect(await store.add('https://new.example')).toBe(false);

    expect(held.stored).toEqual([]);
    expect(store.error?.code).toBe('core.resource/VERSION_CONFLICT');
    expect(store.items.map((item) => item.origin)).toEqual([
      FILE.origin,
      EXTENSION.origin,
    ]);

    expect(await store.add('https://new.example')).toBe(true);
    expect(held.stored).toEqual(['https://new.example']);
  });

  it('forgets the list on reset', async () => {
    const store = new AllowedOrigins(stubClient(['https://a.example']).client);
    await store.load();

    store.reset();

    expect(store.loaded).toBe(false);
    expect(store.items).toEqual([]);
  });
});
