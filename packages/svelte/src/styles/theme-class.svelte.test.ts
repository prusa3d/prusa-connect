// `tokens.css` matches the dark variant on a `dark` class on an ancestor, so
// the preference the store works out has to reach the root element.
//
// It runs in a browser rather than under Node because the class is read off a
// real document element.
import { afterEach, describe, expect, it } from 'vitest';

import { applyTheme, REFERENCE_COLOR_MODE_KEY } from './theme.svelte';

afterEach(() => {
  localStorage.removeItem(REFERENCE_COLOR_MODE_KEY);
  document.documentElement.classList.remove('dark');
});

describe('the class the dark variant matches', () => {
  it('adds the class for a dark theme', () => {
    applyTheme(true);

    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('removes the class for a light theme', () => {
    document.documentElement.classList.add('dark');

    applyTheme(false);

    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('is repeatable', () => {
    applyTheme(true);
    applyTheme(true);

    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('hands the side to the API reference page', () => {
    applyTheme(true);
    expect(localStorage.getItem(REFERENCE_COLOR_MODE_KEY)).toBe('dark');

    applyTheme(false);
    expect(localStorage.getItem(REFERENCE_COLOR_MODE_KEY)).toBe('light');
  });
});
