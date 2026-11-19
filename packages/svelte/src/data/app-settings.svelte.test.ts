// What the page is told about how it should look, and what it draws when the
// server cannot say. The three values decide the browser tab, the first theme
// and the first language, so a wrong fallback is a page a person watches
// change under them.
import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';

import { AppSettings, UI_DEFAULTS } from './app-settings.svelte';
import type { AppSettingsClient, UiSettings } from './services/app-settings';
import { StubApiError } from './stubs';

/** A server that answers the three keys, counting how often it was asked. */
const stubUi = (
  answer: Partial<UiSettings>,
): { client: AppSettingsClient; readonly calls: number } => {
  let calls = 0;

  return {
    client: {
      getUiSettings: () => {
        calls += 1;

        return Promise.resolve({
          enabled: true,
          title: 'Prusa Connect',
          default_language: 'en',
          theme: 'system',
          dashboards: true,
          printer_custom_ca: false,
          printer_admission: 'approval',
          diagnostics: true,
          modules: true,
          upgrades: true,
          ...answer,
        });
      },
    },
    get calls() {
      return calls;
    },
  };
};

/** A server that refuses the read. */
const refusing = (): AppSettingsClient => ({
  getUiSettings: () => Promise.reject(new StubApiError('core.api/INTERNAL')),
});

describe('before the server has answered', () => {
  it('draws the same three values a server nobody configured serves', () => {
    const settings = new AppSettings(stubUi({}).client);

    expect(settings.asked).toBe(false);
    expect(settings.title).toBe(UI_DEFAULTS.title);
    expect(settings.defaultLanguage).toBe(UI_DEFAULTS.defaultLanguage);
    expect(settings.theme).toBe(UI_DEFAULTS.theme);
  });
});

describe('the support link', () => {
  it('is the value the server gave, and undefined when it names none', async () => {
    const named = new AppSettings(
      stubUi({ support_url: 'https://portal.example' }).client,
    );
    const unnamed = new AppSettings(stubUi({}).client);

    await named.read();
    await unnamed.read();
    flushSync();

    expect(named.supportUrl).toBe('https://portal.example');
    expect(unnamed.supportUrl).toBeUndefined();
  });
});

describe('the diagnostics send address', () => {
  it('is the value the server gave, and undefined when sending is off', async () => {
    const named = new AppSettings(
      stubUi({ diagnostics_send_url: 'https://beta.example' }).client,
    );
    const off = new AppSettings(stubUi({}).client);

    await named.read();
    await off.read();
    flushSync();

    expect(named.diagnosticsSendUrl).toBe('https://beta.example');
    expect(off.diagnosticsSendUrl).toBeUndefined();
  });
});

// The add-printer page shows its steps from this word, so it is never
// guessed: before the read lands, and after a read that failed, it is absent.
describe('how a new printer joins', () => {
  it('is the word the server gave, and undefined until it has given one', async () => {
    const automatic = new AppSettings(
      stubUi({ printer_admission: 'automatic' }).client,
    );
    const failed = new AppSettings(refusing());

    expect(automatic.printerAdmission).toBeUndefined();

    await automatic.read();
    await failed.read();
    flushSync();

    expect(automatic.printerAdmission).toBe('automatic');
    expect(failed.printerAdmission).toBeUndefined();
  });
});

describe('what the server said', () => {
  it('reaches all three getters', async () => {
    const stub = stubUi({
      title: 'Workshop floor',
      default_language: 'cs',
      theme: 'dark',
    });
    const settings = new AppSettings(stub.client);

    await settings.read();
    flushSync();

    expect(settings.asked).toBe(true);
    expect(settings.error).toBeUndefined();
    expect(settings.title).toBe('Workshop floor');
    expect(settings.defaultLanguage).toBe('cs');
    expect(settings.theme).toBe('dark');
  });

  // A server of an older build still names the brand file it was configured
  // with. The page has one appearance and reads past it.
  it('reads an answer that still carries skin', async () => {
    const older = {
      title: 'Workshop floor',
      theme: 'dark',
      skin: 'slicer',
    } as const;
    const settings = new AppSettings(stubUi(older).client);

    await settings.read();
    flushSync();

    expect(settings.error).toBeUndefined();
    expect(settings.title).toBe('Workshop floor');
    expect(settings.theme).toBe('dark');
    expect(settings).not.toHaveProperty('skin');
  });

  it('keeps the default for a key it left empty', async () => {
    const settings = new AppSettings(stubUi({ title: '   ' }).client);

    await settings.read();
    flushSync();

    expect(settings.title).toBe(UI_DEFAULTS.title);
  });

  // The server refuses a fourth theme at startup, so this is a server of
  // another build. The page has three palettes and draws one of them.
  it('falls back to system for a theme this library has no palette for', async () => {
    const settings = new AppSettings(stubUi({ theme: 'sepia' }).client);

    await settings.read();
    flushSync();

    expect(settings.theme).toBe('system');
  });
});

describe('one flight', () => {
  it('joins a second read to the first', async () => {
    const stub = stubUi({ title: 'Workshop floor' });
    const settings = new AppSettings(stub.client);

    await Promise.all([settings.read(), settings.read()]);
    flushSync();

    expect(stub.calls).toBe(1);
    expect(settings.title).toBe('Workshop floor');
  });

  it('asks again after the first flight has landed', async () => {
    const stub = stubUi({});
    const settings = new AppSettings(stub.client);

    await settings.read();
    await settings.read();

    expect(stub.calls).toBe(2);
  });
});

describe('a server that cannot answer', () => {
  it('answers the question too, keeps the fault, and draws the defaults', async () => {
    const settings = new AppSettings(refusing());

    await settings.read();
    flushSync();

    expect(settings.asked).toBe(true);
    expect(settings.error).toBeInstanceOf(Error);
    expect(settings.title).toBe(UI_DEFAULTS.title);
    expect(settings.defaultLanguage).toBe(UI_DEFAULTS.defaultLanguage);
    expect(settings.theme).toBe(UI_DEFAULTS.theme);
  });

  it('says whether the server keeps dashboards, and assumes it does until it answers', async () => {
    const off = new AppSettings(stubUi({ dashboards: false }).client);
    expect(off.dashboards).toBe(true);

    await off.read();
    expect(off.dashboards).toBe(false);

    const failed = new AppSettings(refusing());
    await failed.read();
    expect(failed.dashboards).toBe(true);
  });

  it('says whether the server runs installed modules, and assumes it does until it answers', async () => {
    const off = new AppSettings(stubUi({ modules: false }).client);
    expect(off.modules).toBe(true);

    await off.read();
    expect(off.modules).toBe(false);

    const failed = new AppSettings(refusing());
    await failed.read();
    expect(failed.modules).toBe(true);
  });

  it('says whether the server prepares upgrades, and assumes it does until it answers', async () => {
    const off = new AppSettings(stubUi({ upgrades: false }).client);
    expect(off.upgrades).toBe(true);

    await off.read();
    expect(off.upgrades).toBe(false);

    const failed = new AppSettings(refusing());
    await failed.read();
    expect(failed.upgrades).toBe(true);
  });

  it('reports safe mode with its reason and a date, and none outside it', async () => {
    const on = new AppSettings(
      stubUi({
        safe_mode: { reason: 'restart', since: '2026-10-30T10:00:00Z' },
      }).client,
    );
    expect(on.safeMode).toBeUndefined();

    await on.read();
    expect(on.safeMode).toEqual({
      reason: 'restart',
      since: new Date('2026-10-30T10:00:00Z'),
    });

    const off = new AppSettings(stubUi({}).client);
    await off.read();
    expect(off.safeMode).toBeUndefined();
  });

  it('says whether the server serves diagnostics, and assumes it does not until it answers', async () => {
    const on = new AppSettings(stubUi({ diagnostics: true }).client);
    expect(on.diagnostics).toBe(false);

    await on.read();
    expect(on.diagnostics).toBe(true);

    const off = new AppSettings(stubUi({ diagnostics: false }).client);
    await off.read();
    expect(off.diagnostics).toBe(false);

    const failed = new AppSettings(refusing());
    await failed.read();
    expect(failed.diagnostics).toBe(false);
  });
});
