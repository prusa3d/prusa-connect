<!--
  A file name that gives up its middle instead of its end.

  Every name this product lists means something at the end.
  `Bracket_Left_Reinforced_v4_FINAL.gcode` and
  `Bracket_Left_Reinforced_v6_verified.gcode` differ only in the last dozen
  characters, and the extension that says whether a row can be printed at all
  is the very last thing. A plain `truncate` removes exactly that, which is how
  a jobs row at 390px ends up reading `Bracket_Left_R…`.

  CSS has no middle ellipsis, so the name is two elements: a head that shrinks
  and clips, and a tail beside it that never shrinks. Both are plain text in
  one line box, so a person selects and copies the whole name in one gesture
  and a screen reader reads it once, without a break.

  The two sit in grid tracks rather than flex items so the name's own minimum
  width is its tail and an ellipsis: the head track gives way first, and an
  auto table column or a flex row sized by content never squeezes the name
  below the part that tells two files apart. A caller that passes `min-w-0`
  opts out of that floor.
-->
<script lang="ts">
  import type { ClippedNameProps } from './types';

  // Twelve characters is `_v4_FINAL.gcode` minus a little: enough for a
  // version suffix and an extension, which is what tells two names with the
  // same stem apart. Longer than that, and the head has nothing left at a
  // phone width. Shorter, and the extension alone fills the tail.
  const TAIL_LENGTH = 12;

  let {
    name,
    tailLength = TAIL_LENGTH,
    class: className,
  }: ClippedNameProps = $props();

  // The head keeps room for a character and its ellipsis even when squeezed,
  // so a head of three characters or fewer would sit in a box wider than
  // itself and leave a gap before the tail. Such a name is as short as a
  // clipped one anyway.
  const SHORTEST_HEAD = 4;

  const head = $derived(name.slice(0, name.length - tailLength));
  const tail = $derived(name.slice(name.length - tailLength));
</script>

{#if name.length - tailLength < SHORTEST_HEAD}
  <!-- A name that nearly fits its own tail has no middle to give up. -->
  <span data-slot="clipped-name" class={className}>{name}</span>
{:else}
  <!-- `title` sits on the wrapper and not on the head, so a pointer resting
       anywhere on the name offers the whole of it.

       The head and the tail touch. A newline between the two spans is a text
       node, and the name would read with a space in its middle. Each part is
       its own grid item, and a grid item drops a space at its edge, so both
       keep their spaces: `Prusa printer logs` cut after `Prusa ` would read
       `Prusaprinter logs`. -->
  <span
    data-slot="clipped-name"
    title={name}
    class={['clipped-name items-baseline', className]}
    ><span class="overflow-hidden text-ellipsis whitespace-pre">{head}</span
    ><span class="whitespace-pre">{tail}</span></span
  >
{/if}

<style>
  /* `max-content` and not `auto` for the tracks: auto tracks stretch over a
     box wider than the name and would open a gap between head and tail. The
     head keeps 2em, room for one character and the ellipsis. With less, the
     ellipsis itself is clipped and the one character left reads as part of
     the name: `m.ports.bgcode` instead of `m…ports.bgcode`. */
  .clipped-name {
    display: grid;
    grid-template-columns: minmax(2em, max-content) max-content;
  }
</style>
