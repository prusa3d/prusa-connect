<!--
  What every story is drawn inside: the theme on the document element.
  Nothing is wrapped around the story itself, so a screen measures the width
  Storybook says it runs at.

  The theme is a class and an attribute. `tokens.css` selects `.dark`, which is
  what `ThemeToggle` writes on a real page, and `data-theme` is written beside
  it so a reader inspecting the page sees which of the two is on. The page's
  own background and text colour come from the theme's base layer, which
  `preview.css` imports.
-->
<script lang="ts">
  import { untrack, type Snippet } from 'svelte';

  import { applyTheme, theme } from '../src/styles/theme.svelte';

  interface Props {
    /** Whether the dark side of the theme is on. */
    dark: boolean;
    children?: Snippet;
  }

  const { dark, children }: Props = $props();

  $effect(() => {
    const selected = dark;
    untrack(() => {
      if (theme.dark !== selected)
        theme.preference = selected ? 'dark' : 'light';
    });
  });

  $effect(() => {
    const actual = theme.dark;
    applyTheme(actual);
    const root = document.documentElement;
    root.dataset.theme = actual ? 'dark' : 'light';
  });
</script>

{@render children?.()}
