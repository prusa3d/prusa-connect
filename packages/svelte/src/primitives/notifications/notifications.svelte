<!--
  The chrome's notification control: a bell carrying its count, and the list it
  opens.

  A kind, not a severity level. Every row names a fact the backend emits, and
  `src/domain/notification.ts` owns that union and maps each kind to its tone.
  The tone is derived, so a row cannot claim an urgency its own event does not
  have.

  Tone is the marker and the order. `critical` is a printer that stopped or a
  print that failed, `attention` is one waiting for a person, `success` is work
  that finished, `info` is the instance talking. Each gets its own glyph, its
  own word and a place in the sort: a stopped printer stands above a finished
  job whichever happened first, because the list is read to find out what needs
  doing.

  `needsAttention` is pinned above the history and is not part of it. That
  count is a state which is still true, printers waiting right now, where every
  row below it is an event that happened. It is also why the header carries no
  separate badge beside the bell: one fleet stated twice in one row of chrome
  is one too many, so the count marks the bell and leads the panel.

  There is no All, Unread and Action filter. Three tabs over a list whose rows
  are nearly always unread were three names for one panel. What is left is the
  one control that changes something, Mark all read, and the unread and earlier
  split, which shows up only once there is something on both sides of it.

  One row shape, not one per presentation. A dense row for the phone and a
  roomy one for the header popover gave the device with the most need for a
  legible tap target the smaller one. What changes between the two is the
  panel's height, not the size of anything inside it.

  The list scrolls, and its scroll stays in it. A fleet generates more than a
  screenful, so the body is a scroll box with a definite height, `flex-1
  min-h-0` under a capped panel, and `overscroll-contain`, which stops a fling
  at the end of the list from scrolling the page behind it. The sheet blocks
  page scroll outside the panel; this is the other half, inside it.

  Two presentations. On a wide header the list is a popover under the bell. On
  the phone bar it is a sheet that slides out of the bar: the bar stays put,
  and the bell becomes a close control in the same spot while the panel is
  open. Where that sheet docks is the caller's: `side` says which edge, and
  `sheetClass` and `sheetOverlayClass` carry the offsets. A primitive that
  reached for a variable only the app shell publishes offset a standalone sheet
  from a bar that was not there, and could not serve a bar docked to the top of
  the screen at all. The panel owns how tall it may get; the shell owns where
  it hangs.

  Read state is the caller's. Which notifications exist, and which have been
  seen, outlive any one screen.
-->
<script lang="ts">
  import BellIcon from '@lucide/svelte/icons/bell';
  import BellOffIcon from '@lucide/svelte/icons/bell-off';
  import CheckIcon from '@lucide/svelte/icons/check';
  import CheckCheckIcon from '@lucide/svelte/icons/check-check';
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
  import XIcon from '@lucide/svelte/icons/x';

  import { compareByTone, unreadCount } from '../../domain';
  import { formatNumber, t } from '../../i18n';
  import Badge from '../badge/badge.svelte';
  import Button from '../button/button.svelte';
  import Popover from '../popover/popover.svelte';
  import PopoverContent from '../popover/popover-content.svelte';
  import PopoverTrigger from '../popover/popover-trigger.svelte';
  import Sheet from '../sheet/sheet.svelte';
  import SheetContent from '../sheet/sheet-content.svelte';
  import SheetTitle from '../sheet/sheet-title.svelte';
  import SheetTrigger from '../sheet/sheet-trigger.svelte';
  import NotificationRow from './notification-row.svelte';
  import type { NotificationItem, NotificationsProps } from './types';

  // How tall the list may get in each presentation.
  //
  // The phone panel is the taller of the two, and deliberately so: it is the
  // only thing on the screen while it is open, and a 22rem panel over a 740px
  // phone showed two and a half rows, which is enough to hide that the list
  // scrolls at all. It is capped against the viewport as well as in `rem`, so
  // it cannot grow past the space the bar leaves it on a small phone or past a
  // comfortable reading measure on a tall one. Both terms are edge agnostic,
  // so the cap holds whichever edge the panel is docked to.
  //
  // The header popover leaves the page it floats over visible, which is the
  // point of a popover, so it stops sooner.
  const SHEET_PANEL_HEIGHT = 'max-h-notifications-sheet-height';
  const POPOVER_PANEL_HEIGHT = 'max-h-notifications-popover-height';

  let {
    items,
    needsAttention = 0,
    onToggleRead,
    onMarkAllRead,
    onAction,
    triggerSize = 'icon-lg',
    presentation = 'popover',
    side = 'bottom',
    align = 'end',
    sheetClass,
    sheetOverlayClass,
    class: className,
  }: NotificationsProps = $props();

  let sheetOpen = $state(false);
  let popoverOpen = $state(false);

  // An action that goes somewhere is the person leaving the list, so the
  // panel closes behind it in both presentations. One that acts in place
  // leaves the panel open.
  const act = (notificationId: string, actionId: string): void => {
    onAction?.(notificationId, actionId);
    const leaves = items
      .find((item) => item.id === notificationId)
      ?.actions?.some(
        (action) => action.id === actionId && action.href !== undefined,
      );
    if (leaves !== true) return;
    sheetOpen = false;
    popoverOpen = false;
  };

  const unread = $derived(unreadCount(items));
  // Printers waiting outrank events: whatever else is unread, a machine
  // standing still is what the mark has to be about. Both are counts rather
  // than a bare dot, because five machines standing still is a different
  // morning from one.
  const triggerCount = $derived(needsAttention > 0 ? needsAttention : unread);
  const triggerCountLabel = $derived(
    triggerCount > 99 ? '99+' : formatNumber(triggerCount),
  );
  const showsClose = $derived(presentation === 'sheet' && sheetOpen);

  const attentionSentence = $derived(
    t('primitives.notifications.attention', { count: needsAttention }),
  );
  const summary = $derived.by(() => {
    if (needsAttention > 0) return attentionSentence;
    if (unread > 0)
      return t('primitives.notifications.unread', { count: unread });
    return t('primitives.notifications.caughtUp');
  });
  const triggerLabel = $derived.by(() => {
    if (showsClose) return t('primitives.notifications.close');
    const read = t('primitives.notifications.unread', { count: unread });
    if (needsAttention === 0) {
      return t('primitives.notifications.trigger', { unread: read });
    }
    return t('primitives.notifications.triggerAttention', {
      attention: attentionSentence,
      unread: read,
    });
  });

  const byTone = (left: NotificationItem, right: NotificationItem): number =>
    compareByTone(left.kind, right.kind);

  /**
   * Unread first, then the rest, in two groups and with no headings. `UNREAD`
   * and `EARLIER` were two bars of capitals in a panel whose rows already say
   * which they are, since the unread ones carry a tone dot and a heavier
   * title. The split survives as the order, which is the part a reader uses.
   */
  const sections = $derived.by(() => {
    const fresh = items.filter((item) => !item.read).sort(byTone);
    const seen = items.filter((item) => item.read).sort(byTone);
    if (fresh.length === 0 || seen.length === 0) {
      return [{ id: 'all', items: [...fresh, ...seen] }];
    }
    return [
      { id: 'unread', items: fresh },
      { id: 'read', items: seen },
    ];
  });
</script>

<!-- One control, not two. A dismiss cross beside this asked a person to tell
     read from cleared, a distinction nobody on a workshop floor makes, and the
     panel already offers Mark all read above. What is left is the one toggle
     that means something: I have dealt with this, or I have not.

     `icon-sm` under a mouse and 44px under a finger, the same trade the rail
     makes. A row carrying a 44px button for every pointer would be half chrome
     on a desktop. -->
{#snippet readToggle(item: NotificationItem)}
  <Button
    variant="ghost"
    size="icon-sm"
    class="shrink-0 pointer-coarse:size-touch-target"
    label={item.read
      ? t('primitives.notifications.markUnread.label', { title: item.title })
      : t('primitives.notifications.markRead.label', { title: item.title })}
    aria-pressed={item.read}
    onclick={() => onToggleRead?.(item.id)}
  >
    <CheckIcon class="size-4" />
  </Button>
{/snippet}

{#snippet trigger({ props }: { props: Record<string, unknown> })}
  <Button
    {...props}
    variant="ghost"
    size={triggerSize}
    class={['relative', className]}
    label={triggerLabel}
    aria-expanded={presentation === 'sheet' ? sheetOpen : undefined}
    onclick={(event: MouseEvent) => {
      // bits-ui's dialog trigger only opens, where Radix's toggles. The bell
      // stays one control in one place, so it shuts the sheet as well.
      if (presentation === 'sheet') {
        sheetOpen = !sheetOpen;
        return;
      }
      (props.onclick as ((event: MouseEvent) => void) | undefined)?.(event);
    }}
  >
    {#if showsClose}
      <XIcon class={triggerSize === 'icon-md' ? 'size-4' : 'size-5'} />
    {:else}
      <BellIcon class={triggerSize === 'icon-md' ? 'size-4' : 'size-5'} />
    {/if}
    {#if !showsClose && triggerCount > 0}
      <!-- Hung from the bell's middle and lifted over the button's edge, so it
           covers the bell's shoulder and never the whole glyph. -->
      <Badge
        variant={needsAttention > 0 ? 'warning' : 'default'}
        aria-hidden="true"
        class="absolute start-1/2 -top-1 min-w-4 justify-center px-1 py-0 leading-4 tabular-nums"
      >
        {triggerCountLabel}
      </Badge>
    {/if}
  </Button>
{/snippet}

{#snippet panel()}
  <div
    class="flex shrink-0 items-center justify-between gap-2 border-b border-border px-3 py-2.5"
  >
    <div class="min-w-0">
      <!-- The panel draws its own header row, so there is no sheet header
           wrapper. A sheet only needs the title element to exist inside the
           content, so it goes in as the visible heading: one Notifications,
           seen and announced, rather than a visible one and a screen reader
           one saying the same word. -->
      {#if presentation === 'sheet'}
        <SheetTitle class="truncate text-sm leading-none font-semibold">
          {t('primitives.notifications.title')}
        </SheetTitle>
      {:else}
        <p class="truncate text-sm leading-none font-semibold">
          {t('primitives.notifications.title')}
        </p>
      {/if}
      <p class="mt-1 text-xs text-muted-foreground">{summary}</p>
    </div>
    {#if unread > 0 && onMarkAllRead}
      <Button
        variant="ghost"
        size="sm"
        class="shrink-0"
        onclick={onMarkAllRead}
      >
        <CheckCheckIcon aria-hidden="true" />
        {t('primitives.notifications.markAllRead')}
      </Button>
    {/if}
  </div>

  <!-- Pinned above the history and not one of its rows: this is the one line
       about now rather than about something that happened, so it never scrolls
       away with the list and is never marked read. A link, because it goes
       somewhere. The rows below carry buttons instead, which is why they are
       not links themselves. -->
  {#if needsAttention > 0}
    <a
      href="/printers"
      class="flex min-h-11 shrink-0 items-center gap-3 border-b border-border bg-warning/10 px-4 py-2.5 outline-none hover:bg-warning/15 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
    >
      <TriangleAlertIcon aria-hidden="true" class="size-4 text-warning-text" />
      <span class="flex-1 text-sm font-medium">{attentionSentence}</span>
      <ChevronRightIcon
        aria-hidden="true"
        class="size-4 text-muted-foreground"
      />
    </a>
  {/if}

  {#if items.length === 0}
    <div
      class="flex flex-1 flex-col items-center justify-center gap-2 px-6 py-10 text-center"
    >
      <BellOffIcon aria-hidden="true" class="size-6 text-muted-foreground" />
      <p class="text-sm text-muted-foreground">
        {t('primitives.notifications.empty')}
      </p>
    </div>
  {:else}
    <!-- A plain scroll box, not this tree's `ScrollArea`. A scroll area's
         viewport is `size-full`, and a percentage height resolves only against
         a parent whose own height is definite. This panel's is not: it is
         `height: auto` under a `max-height`, which is what lets a short list
         draw a short panel. Giving the viewport a definite height fixes the
         percentage and breaks the panel instead, because the scroller then
         contributes no height of its own and the auto height sheet collapses
         to its header.

         So the box scrolls itself. `min-h-0 flex-1` is what the flex layout
         sizes, `overflow-y-auto` is what scrolls, and `overscroll-contain`
         keeps a fling at either end off the page behind the panel.

         The scrollbar is then the platform's own, and the two `scrollbar-*`
         properties theme it rather than conjure it. Where the platform draws a
         classic scrollbar it is thin and takes this tree's own muted tone;
         where the platform uses overlay scrollbars nothing shows until a
         scroll starts, and the row cut in half at the fold is what says there
         is more. -->
    <div
      data-slot="notification-list"
      class="min-h-0 flex-1 [scrollbar-width:thin] [scrollbar-color:var(--color-scrollbar-thumb)_transparent] overflow-y-auto overscroll-contain"
    >
      <!-- The one thing left saying where unread stops: a rule between the two
           groups, since each group's own rows are divided from each other and
           nothing would otherwise separate the last unread row from the first
           read one. -->
      {#each sections as section (section.id)}
        <section class="border-t border-border first:border-t-0">
          <ul class="divide-y divide-border">
            {#each section.items as item (item.id)}
              <NotificationRow
                {item}
                onAction={act}
                toggle={onToggleRead === undefined ? undefined : readToggle}
              />
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  {/if}
{/snippet}

{#if presentation === 'sheet'}
  <Sheet bind:open={sheetOpen}>
    <SheetTrigger child={trigger} />
    <!-- bits-ui's root takes no `modal`, where Radix's does. The two halves of
         a non-modal panel are named on the content instead: the page behind
         keeps its scroll and its focus. -->
    <SheetContent
      side={side === 'top' ? 'top' : 'bottom'}
      showCloseButton={false}
      preventScroll={false}
      trapFocus={false}
      class={['z-55 gap-0 bg-popover p-0', SHEET_PANEL_HEIGHT, sheetClass]}
      overlayClass={sheetOverlayClass}
      children={panel}
    />
  </Sheet>
{:else}
  <Popover bind:open={popoverOpen}>
    <PopoverTrigger child={trigger} />
    <PopoverContent
      {side}
      {align}
      sideOffset={8}
      collisionPadding={12}
      class={[
        'flex w-notifications-popover flex-col p-0',
        POPOVER_PANEL_HEIGHT,
      ]}
      children={panel}
    />
  </Popover>
{/if}
