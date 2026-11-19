<!--
  The fixed box that leads a row naming a file: a thumbnail of the sliced plate
  when one exists, a type glyph when it does not. The file browser's listing,
  the jobs table and a printer's recent jobs all read it, so the three show a
  file the same way and at the same size.

  A glyph box is inert, because the icon alone needs no control. A thumbnail
  box becomes a trigger for a larger preview, open on hover, on visible
  (keyboard) focus and on a tap alike: this interface's stated primary device
  is a workshop tablet, where hover does not exist, so hover alone would leave
  a touch reader with no way in. Focus a script moves, such as a dialog's
  autofocus, opens nothing, because nobody asked to see the picture.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import Popover from '../popover/popover.svelte';
  import PopoverContent from '../popover/popover-content.svelte';
  import PopoverTrigger from '../popover/popover-trigger.svelte';
  import { cancelUnanswered } from './cancel-unanswered';
  import { takeThumbnailSlot } from './slots';
  import type { FileThumbnailProps } from './types';
  import { radii, sizes, THUMBNAIL_BASE, THUMBNAIL_GLYPH } from './variants';

  let {
    name,
    thumbnailUrl,
    glyph: Glyph,
    glyphSrc,
    open = $bindable(false),
    size = 'md',
    radius = 'sm',
    class: className,
  }: FileThumbnailProps = $props();

  const label = $derived(t('primitives.fileThumbnail.preview', { name }));

  // The thumbnail route answers 404 for a copy that carries no picture, and
  // nothing in a listing says which copies do. Asking ahead would cost a
  // request per row, so the row draws the image and falls back on its failure.
  // Keeping the address that failed rather than a flag is what resets the
  // fallback when the box is handed a different file.
  let brokenUrl = $state<string | undefined>(undefined);
  const broken = $derived(
    brokenUrl !== undefined && brokenUrl === thumbnailUrl,
  );

  // The picture is asked for once the box comes into view and a slot in the
  // page's thumbnail queue is free (`slots.ts` says why there is a queue). The
  // fallback, the artwork or else the icon, stands in until then. A box
  // scrolled away before its turn leaves the queue; one that started keeps its
  // slot until the picture answers, so a slow printer is never asked twice, or
  // until the queue's bound runs out, when the request is cancelled and the
  // fallback stays. Unmounting gives the slot back and cancels a picture that
  // has not answered.
  let src = $state<string | undefined>(undefined);
  let giveBack: (() => void) | undefined;

  const done = (): void => {
    giveBack?.();
  };

  const loadWhenSeen =
    (url: string) =>
    (box: Element): (() => void) => {
      const observer = new IntersectionObserver((entries) => {
        const seen = entries.some((entry) => entry.isIntersecting);
        if (seen) {
          giveBack ??= takeThumbnailSlot(
            () => {
              src = url;
            },
            () => {
              // The picture leaves, and `cancelUnanswered` takes its request.
              src = undefined;
            },
          );
        } else if (src !== url) {
          giveBack?.();
          giveBack = undefined;
        }
      });
      observer.observe(box);

      return () => {
        observer.disconnect();
        giveBack?.();
        giveBack = undefined;
        src = undefined;
      };
    };

  // The trigger already holds focus, by hover, by Tab or by a press, and
  // nothing inside the preview can be reached, so there is nothing worth
  // moving focus onto. Returning it on close would fire the trigger's own
  // focus handler and reopen what a pointer just left.
  const keepFocus = (event: Event): void => {
    event.preventDefault();
  };

  // Escape shuts a preview while the pointer may still rest on the box, which
  // goes on receiving pointer moves there, so the next twitch of the hand
  // would open again what the reader has just dismissed. The dismissal holds
  // back hover alone, and lasts until the pointer leaves the box or focus
  // moves away, which is when the next arrival is a fresh one.
  let dismissed = $state(false);

  const dismiss = (): void => {
    dismissed = true;
  };

  // Opened by a pointer that moved onto the box, and never by a box that
  // arrived under a pointer standing still. A browser dispatches pointerenter
  // for the second one too, so a dialog or a freshly drawn row landing under
  // the cursor would open its own preview over the page nobody is pointing at.
  // Crossing into the box is itself a move, so a reader who hovers still gets
  // the preview on the first event. It then fires for every further move over
  // the box, and assigning a boolean it already holds redraws nothing.
  const showOnMove = (): void => {
    if (dismissed) return;

    open = true;
  };
  // A press is asked for by name, so it opens whatever Escape said before.
  const show = (): void => {
    dismissed = false;
    open = true;
  };
  // The same test bits-ui's Tooltip makes to ignore focus that no keyboard
  // moved.
  const showOnVisibleFocus = (event: FocusEvent): void => {
    if (
      event.currentTarget instanceof Element &&
      event.currentTarget.matches(':focus-visible')
    ) {
      open = true;
    }
  };
  const hide = (): void => {
    dismissed = false;
    open = false;
  };
</script>

{#snippet fallback()}
  {#if glyphSrc}
    <img src={glyphSrc} alt="" class="opacity-75 invert dark:invert-0" />
  {:else if Glyph}
    <Glyph
      class={[THUMBNAIL_GLYPH, 'text-muted-foreground']}
      aria-hidden="true"
    />
  {/if}
{/snippet}

{#if thumbnailUrl === undefined || broken}
  <div
    data-slot="file-thumbnail"
    class={[THUMBNAIL_BASE, sizes[size], radii[radius], className]}
  >
    {@render fallback()}
  </div>
{:else}
  <Popover bind:open>
    <PopoverTrigger>
      {#snippet child({ props })}
        <!-- The trigger's own press handler toggles, which is right for a
             mouse click alone. Focus has already opened this by the time a
             tap's synthesised click fires, because a touch browser dispatches
             focus before click for the sake of mouse-only sites, so a toggle
             reads that as "already open" and shuts what the same tap just
             opened. Dropping the trigger's own handler leaves a press that
             only ever opens; shutting stays on pointer leave, blur, Escape and
             an outside press. -->
        <button
          {...props}
          data-slot="file-thumbnail"
          type="button"
          aria-label={label}
          onclick={show}
          onpointermove={showOnMove}
          onpointerleave={hide}
          onfocus={showOnVisibleFocus}
          onblur={hide}
          {@attach loadWhenSeen(thumbnailUrl)}
          class={[
            THUMBNAIL_BASE,
            sizes[size],
            radii[radius],
            'cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
            className,
          ]}
        >
          <!-- Decoration: the file name sits beside it in the row, and again
               as the preview's own caption. -->
          {#if src === thumbnailUrl}
            <img
              {src}
              alt=""
              class="size-full object-cover"
              onload={done}
              onerror={() => {
                done();
                brokenUrl = thumbnailUrl;
              }}
              {@attach cancelUnanswered}
            />
          {:else}
            {@render fallback()}
          {/if}
        </button>
      {/snippet}
    </PopoverTrigger>
    <!-- bits-ui traps focus inside a popover surface by default and pulls it
         off the trigger the moment the surface opens. Nothing in here can be
         reached, so the trap has nothing to hold and only takes the keyboard
         away from the row.

         The cap sits on the surface itself and not on anything inside it. The
         room left beside the anchor is measured from this box's own edge with
         its padding and border counted in, so a cap on the nested image lands
         one padding and border short of overflowing. -->
    <PopoverContent
      side="right"
      align="start"
      sideOffset={8}
      class="w-auto max-w-thumbnail-popover"
      trapFocus={false}
      onOpenAutoFocus={keepFocus}
      onCloseAutoFocus={keepFocus}
      onEscapeKeydown={dismiss}
    >
      <img
        src={thumbnailUrl}
        alt=""
        class="h-auto max-w-full rounded-sm object-contain"
      />
      <!-- Wraps rather than truncates. The row above already truncates, and
           what tells two of these names apart is almost always the tail, which
           is exactly what a truncation eats. The name keeps its width until a
           single unbroken run exceeds the popover, so a short name is never
           cut inside a word to fit a small picture. -->
      <p class="mt-2 font-mono text-xs break-words text-muted-foreground">
        {name}
      </p>
    </PopoverContent>
  </Popover>
{/if}
