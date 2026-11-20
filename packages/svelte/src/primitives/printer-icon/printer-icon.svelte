<!--
  The machine itself, drawn: the artwork a card, a row or a header identifies a
  printer by.

  The drawings are full-colour SVGs and PNGs the application serves, and they
  are loaded by URL rather than inlined. The files come to about 3 MB together,
  which is a bundle sized for drawing all of them on every screen that draws a
  handful. `telemetry-icon` inlines its own glyphs for the opposite reason:
  those are one-colour line art that has to take its container's colour, and an
  `<img>` cannot inherit `currentColor`.

  The name is never empty. A model this instance does not recognise says so in
  words, because the icon is how a card names its machine and a nameless
  picture leaves a screen reader with nothing to read.
-->
<script lang="ts">
  import { withBase } from '../../router/base-path';
  import { declaredModels } from './declared';
  import { printerModelName } from './names';
  import { printerArtworkFile, type PrinterIconProps } from './types';

  let {
    model,
    reported,
    base = '/img/printers',
    class: className,
    ...rest
  }: PrinterIconProps = $props();

  // A machine this library has no artwork for may still be one a driver in
  // the server's build declared, and then it has a name to show and may have
  // a drawing of its own (ADR-20261016-qrp).
  const lookup = declaredModels();
  const declared = $derived(
    model === 'unknown' && reported !== undefined
      ? lookup?.(reported)
      : undefined,
  );

  // A declared drawing is served by the server and can fail to load, for one
  // the build no longer carries. The icon then draws the machine as unknown
  // and keeps the declared name.
  let broken = $state<string | undefined>(undefined);
  const drawing = $derived(
    declared?.src !== undefined && declared.src !== broken
      ? declared.src
      : undefined,
  );

  // `artwork.test.ts` reads the folder and fails when a model's file is
  // missing.
  // Both are paths from the root, moved under the prefix a reverse proxy
  // serves the app under (base-path.ts).
  const source = $derived(
    withBase(drawing ?? `${base}/${printerArtworkFile(model)}`),
  );
</script>

<img
  data-slot="printer-icon"
  data-variant={model}
  src={source}
  alt={declared?.name ?? printerModelName(model)}
  onerror={() => {
    // Only a declared drawing is marked. When the fallback fails as well there
    // is nothing left to try, and marking it would bring the declared drawing
    // back.
    if (drawing !== undefined) broken = drawing;
  }}
  {...rest}
  class={['shrink-0 object-contain', className]}
/>
