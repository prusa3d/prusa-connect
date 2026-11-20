<!--
  One of the library's drawings.

  The artwork is a quiet layer: a line drawing where a list is empty, a shadow
  line under a silhouette, a faint grid behind a camera frame. Every drawing is
  a file the application serves, so a person can replace one without touching
  this package.

  The file is drawn as a mask over the element's own colour rather than as an
  `<img>`, because a picture cannot read the page. A mask makes the drawing
  `currentColor`, so it takes the tone of wherever it sits, follows the light
  and the dark theme on its own, and needs no second file for either. What the
  file draws at less than full opacity stays the quiet second tone.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { withBase } from '../../router/base-path';
  import type { ArtProps } from './types';
  import { ART_ALT_KEY, ART_FILE, ART_RATIO } from './variants';

  let {
    name,
    base = '/img/art',
    alt,
    decorative = false,
    fit = 'contain',
    class: className,
  }: ArtProps = $props();

  // A path from the root, moved under the prefix a reverse proxy serves the
  // app under (base-path.ts).
  const source = $derived(`url('${withBase(`${base}/${ART_FILE[name]}`)}')`);
  const label = $derived(alt ?? t(ART_ALT_KEY[name]));
</script>

<span
  data-slot="art"
  data-variant={name}
  data-fit={fit}
  role={decorative ? undefined : 'img'}
  aria-label={decorative ? undefined : label}
  aria-hidden={decorative ? 'true' : undefined}
  style="--art-source: {source}; --art-ratio: {ART_RATIO[name]}"
  class={['shrink-0', className]}
></span>

<style>
  [data-slot='art'] {
    display: block;
    background-color: currentColor;
    aspect-ratio: var(--art-ratio);
    -webkit-mask-image: var(--art-source);
    mask-image: var(--art-source);
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: center;
    mask-position: center;
    -webkit-mask-size: contain;
    mask-size: contain;
  }

  [data-slot='art'][data-fit='cover'] {
    -webkit-mask-size: cover;
    mask-size: cover;
  }

  /* A tile keeps its own size at any box size, so the grid behind a small
     camera frame and a large one is drawn at the same weight. */
  [data-slot='art'][data-fit='tile'] {
    aspect-ratio: auto;
    -webkit-mask-repeat: repeat;
    mask-repeat: repeat;
    -webkit-mask-size: 2.5rem 2.5rem;
    mask-size: 2.5rem 2.5rem;
  }
</style>
