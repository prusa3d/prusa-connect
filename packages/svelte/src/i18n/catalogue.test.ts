// The gate behind `.agents/rules/frontend-i18n.md` rules 6.
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  catalogueModule,
  catalogues,
  CATALOGUES_FILE,
  ERRORS_FILE,
  errorCatalogue,
  formatted,
  LOCALES_DIR,
  WIZARD_CATALOGUES_FILE,
  WIZARD_PREFIX,
  wizardCatalogueModule,
} from '../../scripts/emit-catalogues.mjs';
import { WIZARD_CATALOGUES } from '../shell/setup-wizard/catalogues';
import { CORE_CATALOGUES } from './catalogues';

const SRC = fileURLToPath(new URL('..', import.meta.url));

/** A key whose last part is built at run time. `stateLabel` reads. */
const FAMILIES = [
  'state.',
  'error.',
  'severity.',
  // Built from the verb's own tail. `vocabulary.test.ts` checks both.
  'printerDetail.commands.verb.',
  'printerDetail.commands.detail.',
  // Built from the code the printer sent. `printer-code.test.ts` reads them.
  'printerCode.',
  // Built from the answer's name the printer sent.
  'printerButton.',
  // Built from the event type the server stored.
  'printerDetailHistory.eventType.',
];

const sourceFiles = async (at: string): Promise<string[]> => {
  const entries = await readdir(at, { withFileTypes: true });
  const found: string[] = [];

  for (const entry of entries) {
    const path = join(at, entry.name);
    if (entry.isDirectory()) {
      found.push(...(await sourceFiles(path)));
      continue;
    }
    if (entry.name.endsWith('.svelte') || entry.name.endsWith('.ts')) {
      found.push(path);
    }
  }

  return found;
};

const BLOCK_COMMENT = /\/\*[\s\S]*?\*\//g;
const LINE_COMMENT = /(^|\s)\/\/.*/gm;
const T_CALL = /\bt\(\s*'([^']+)'/g;
const TRANSLATE_CALL = /\btranslate\([^,]+,\s*'([^']+)'/g;
const QUOTED = /'([^'\r\n]+)'/g;

/** One file with its comments removed. A key named in a doc comment is an. */
const withoutComments = (text: string): string =>
  text.replace(BLOCK_COMMENT, ' ').replace(LINE_COMMENT, ' ');

const sources = async (): Promise<string> => {
  const files = (await sourceFiles(SRC)).filter(
    (path) => !path.endsWith('.test.ts') && !path.includes('catalogues.ts'),
  );
  const read = await Promise.all(files.map((path) => readFile(path, 'utf8')));

  // Comments come off one file at a time. Stripping them from the joined text.
  return read.map(withoutComments).join('\n');
};

const matched = (text: string, pattern: RegExp): Set<string> => {
  const found = new Set<string>();
  for (const match of text.matchAll(pattern)) {
    const key = match[1];
    if (key !== undefined) found.add(key);
  }

  return found;
};

// The wizard's lines travel in its own chunk, and the library asks for them
// all the same.
const english = { ...CORE_CATALOGUES.en, ...WIZARD_CATALOGUES.en };

/** An English file named after one screen, capturing the prefix it owns. */
const SCREEN_FILE = /^en\.(.+)\.json$/;

describe('the English catalogue', () => {
  it('holds every key the library asks for', async () => {
    const text = await sources();
    const asked = [...matched(text, T_CALL), ...matched(text, TRANSLATE_CALL)];

    expect(asked.filter((key) => english[key] === undefined)).toEqual([]);
  });

  it('holds no key the library never asks for', async () => {
    const text = await sources();
    const named = new Set([
      ...matched(text, QUOTED),
      ...matched(text, T_CALL),
      ...matched(text, TRANSLATE_CALL),
    ]);
    const unused = Object.keys(english).filter(
      (key) =>
        !named.has(key) && !FAMILIES.some((prefix) => key.startsWith(prefix)),
    );

    expect(unused).toEqual([]);
  });

  it('carries a line for every fault code the core can answer', async () => {
    const codes = await errorCatalogue();

    for (const key of Object.keys(codes)) {
      expect(english[key]).toBe(codes[key]);
    }
  });
});

describe('the generated module', () => {
  it('matches the locale files it was generated from', async () => {
    const written = await readFile(CATALOGUES_FILE, 'utf8');

    expect(written).toBe(await formatted(catalogueModule(await catalogues())));
  });

  it('keeps the wizard lines in a module of their own', async () => {
    const written = await readFile(WIZARD_CATALOGUES_FILE, 'utf8');

    expect(written).toBe(
      await formatted(wizardCatalogueModule(await catalogues())),
    );
    for (const messages of Object.values(CORE_CATALOGUES)) {
      expect(
        Object.keys(messages).filter((key) => key.startsWith(WIZARD_PREFIX)),
      ).toEqual([]);
    }
  });

  it('matches the fault registry', async () => {
    const written = await readFile(ERRORS_FILE, 'utf8');

    expect(JSON.parse(written)).toEqual(await errorCatalogue());
  });
});

describe('the English files that split by screen', () => {
  // Two files holding one key merge silently.
  it('name no key twice', async () => {
    const files = (await readdir(LOCALES_DIR))
      .filter((name) => name.startsWith('en.'))
      .sort();
    const owner = new Map<string, string>();
    const clashes: string[] = [];

    for (const name of files) {
      const parsed = JSON.parse(
        await readFile(join(LOCALES_DIR, name), 'utf8'),
      ) as Record<string, string>;

      for (const key of Object.keys(parsed)) {
        if (key.startsWith('_')) continue;
        const held = owner.get(key);
        if (held === undefined) owner.set(key, name);
        else clashes.push(`${key}: ${held} and ${name}`);
      }
    }

    expect(clashes).toEqual([]);
  });

  // A file named after a screen that reaches into another screen's prefix is.
  it('keeps every key under the prefix its file is named for', async () => {
    const screens = (await readdir(LOCALES_DIR)).flatMap((name) => {
      const prefix = SCREEN_FILE.exec(name)?.[1];

      return prefix === undefined || name === 'en.errors.json'
        ? []
        : [{ name, prefix }];
    });
    const strays: string[] = [];

    for (const { name, prefix } of screens) {
      const parsed = JSON.parse(
        await readFile(join(LOCALES_DIR, name), 'utf8'),
      ) as Record<string, string>;

      for (const key of Object.keys(parsed)) {
        if (key.startsWith('_') || key.startsWith(`${prefix}.`)) continue;
        strays.push(`${name}: ${key}`);
      }
    }

    expect(strays).toEqual([]);
  });
});
