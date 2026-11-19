import { libraryConfig } from '@prusa/frontend-standards/eslint/library';

import svelteConfig from './svelte.config.js';

export default [
  ...libraryConfig({ tsconfigRootDir: import.meta.dirname, svelteConfig }),

  // build/testdata holds a module author's own tree. It is built by
  // build/module.test.ts through Vite and reaches this package by its
  // published name, which no tsconfig here resolves, so the typed rules have
  // no project to read it in.
  {
    ignores: ['storybook-static/', 'build/testdata/'],
  },

  {
    // The raw-control ban is off here: the primitives are where the raw
    // elements live.
    files: ['src/primitives/**'],
    rules: {
      'svelte/no-restricted-html-elements': 'off',
    },
  },
  {
    // Spinner and Alert are where the spinner and role="alert" are drawn. One
    // rule carries those two bans and the arbitrary-value ban, so it is off
    // for the two files; neither writes a class of its own with a bracket.
    files: [
      'src/primitives/spinner/spinner.svelte',
      'src/primitives/alert/alert.svelte',
    ],
    rules: {
      'no-restricted-syntax': 'off',
    },
  },

  {
    // ADR-20260914-244 rule 1: the `core` tier imports the SDK, the domain and the
    // catalogue, and nothing above it. The matcher's own files sit directly
    // under src/router/, so a sibling import is one `../` deep, written bare
    // (`'../primitives'`) or with a subpath (`'../primitives/button'`); the
    // group lists both forms for each tier it must not reach.
    files: ['src/router/**'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: [
                '../primitives',
                '../primitives/*',
                '../shell',
                '../shell/*',
                '../data',
                '../data/*',
                '../slots',
                '../slots/*',
              ],
              message:
                'the router is in the core tier and imports nothing above it',
            },
          ],
        },
      ],
    },
  },
  {
    // ADR-20260914-244 rule 1 and rule 3: the `screens` tier imports what is below
    // it, and the application is a leaf nothing here may reach. A file under
    // src/shell/ sits at a different depth per component
    // (src/shell/index.ts, src/shell/rail/rail.svelte, ...), so a pattern
    // pinned to one `../` count would miss every other depth. `**/frontend`
    // matches the segment at any depth, bare or with a subpath after it.
    files: ['src/shell/**'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/frontend'],
              message:
                'the shell is a library tier and never reads the application',
            },
          ],
        },
      ],
    },
  },
];
