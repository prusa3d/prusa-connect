import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

// A `var(--x)` naming a property nothing declares is dropped by the browser
// without an error, and nothing else in the toolchain reports it. A typo, or a
// token deleted from the theme, would then break a component in silence. This
// file is the only thing that sees it.
//
// A use written as `var(--x, fallback)` is not silent: the fallback is what
// the browser draws, so those are allowed and skipped.

const stylesDir = dirname(fileURLToPath(import.meta.url));
const srcDir = join(stylesDir, '..');

const filesUnder = (directory: string, suffixes: string[]): string[] =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesUnder(path, suffixes);
    return suffixes.some((suffix) => entry.name.endsWith(suffix)) ? [path] : [];
  });

const sources = filesUnder(srcDir, ['.css', '.svelte']);

/** A comment carries no declaration and no use, and reads like both. */
const withoutComments = (text: string): string =>
  text.replace(/\/\*[\s\S]*?\*\//g, '');

/** The `<style>` blocks of a component, joined. */
const styleBlocks = (text: string): string =>
  [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)]
    .map((match) => match[1]!)
    .join('\n');

/**
 * Every name declared anywhere in the package: the theme's own blocks, a
 * component's scoped style, and the properties a component writes onto an
 * element itself, such as a progress bar handing its reading to CSS, through
 * a `style:--x={...}` directive, or from script with `style.setProperty`.
 */
const declared = new Set<string>();
for (const path of sources) {
  for (const match of withoutComments(readFileSync(path, 'utf8')).matchAll(
    /style:(--[A-Za-z0-9_-]+)=|(--[A-Za-z0-9_-]+)\s*:/g,
  )) {
    declared.add((match[1] ?? match[2])!);
  }
}
for (const path of filesUnder(srcDir, ['.ts'])) {
  for (const match of readFileSync(path, 'utf8').matchAll(
    /setProperty\(\s*['"](--[A-Za-z0-9_-]+)['"]/g,
  )) {
    declared.add(match[1]!);
  }
}

/** Every use with no fallback, with the file it is written in. */
const uses: { file: string; name: string }[] = [];
for (const path of sources) {
  const text = readFileSync(path, 'utf8');
  const css = withoutComments(
    path.endsWith('.svelte') ? styleBlocks(text) : text,
  );
  for (const match of css.matchAll(/var\(\s*(--[A-Za-z0-9_-]+)\s*([,)])/g)) {
    if (match[2] === ',') continue;
    uses.push({ file: relative(srcDir, path), name: match[1]! });
  }
}

describe('every custom property a stylesheet reads is declared', () => {
  it('finds the theme and the styles that read it', () => {
    // An empty sweep would make the assertion below pass vacuously.
    expect(declared.size).toBeGreaterThan(250);
    expect(uses.length).toBeGreaterThan(50);
  });

  it('names no property the package does not declare', () => {
    const offenders = uses
      .filter((use) => !declared.has(use.name))
      .map((use) => `${use.file}: ${use.name}`)
      .sort();

    // Every one of them, not the first: a token deleted from the theme takes
    // a whole tier with it, and one name per run would hide the rest.
    expect([...new Set(offenders)]).toEqual([]);
  });
});
