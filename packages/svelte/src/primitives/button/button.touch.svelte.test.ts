import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import Button from './button.svelte';

const label = (text: string) =>
  createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

describe('Button under a finger', () => {
  it('stands icon-lg on the 44px floor under a finger', async () => {
    await render(Button, {
      props: { size: 'icon-lg', label: 'Actions', children: label('+') },
    });
    const style = getComputedStyle(
      page.getByRole('button', { name: 'Actions' }).element(),
    );
    expect(style.height).toBe('44px');
    expect(style.width).toBe('44px');
  });
});
