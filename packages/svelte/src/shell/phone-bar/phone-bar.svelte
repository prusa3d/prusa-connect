<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import MenuIcon from '@lucide/svelte/icons/menu';
  import XIcon from '@lucide/svelte/icons/x';

  import { SvelteSet } from 'svelte/reactivity';

  import { t } from '../../i18n';
  import {
    Button,
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
    ConnectLogo,
    Sheet,
    SheetClose,
    SheetContent,
    SheetHeader,
    SheetTitle,
    SheetTrigger,
  } from '../../primitives';
  import { ConnectionStatus } from '../connection-status';
  import {
    isCurrent,
    isCurrentRow,
    type RailNavItem,
    sectionEntries,
  } from '../rail';
  import { ThemeToggle } from '../theme-toggle';
  import { UserMenu } from '../user-menu';
  import type { PhoneBarProps } from './types';

  let {
    productName,
    items,
    settings,
    currentPath,
    connection,
    principal,
    principalDetail,
    onSignOut,
    bell,
  }: PhoneBarProps = $props();

  let open = $state(false);
  const expandedSections = new SvelteSet<string>();

  // `--app-phone-bar-height` is set at runtime by a ResizeObserver, so it
  // cannot be a design token; that forces these two arbitrary values.
  const sheetContentClass =
    // eslint-disable-next-line no-restricted-syntax -- see the comment above.
    'bottom-[calc(var(--app-phone-bar-height)-1px)] max-h-[calc(100svh-var(--app-phone-bar-height)-3rem)] overflow-hidden rounded-b-none bg-popover p-0';
  const sheetOverlayClass =
    // eslint-disable-next-line no-restricted-syntax -- see the comment above.
    'inset-x-0 top-0 bottom-[var(--app-phone-bar-height)]';
</script>

{#snippet label(item: RailNavItem)}
  {@const Icon = item.icon}
  <span class="flex min-w-0 items-center gap-2.5">
    {#if Icon}
      <Icon aria-hidden="true" class="size-6 shrink-0" />
    {/if}
    <span class="truncate">{item.label}</span>
  </span>
{/snippet}

{#snippet row(item: RailNavItem, group = false)}
  {@const current = isCurrentRow(item, currentPath)}
  <li>
    {#if group && item.children?.length}
      {@const holdsCurrent =
        isCurrent(item.href, currentPath) ||
        item.children.some((child) => isCurrent(child.href, currentPath))}
      <Collapsible
        open={expandedSections.has(item.id)}
        onOpenChange={(value) => {
          if (value) expandedSections.add(item.id);
          else expandedSections.delete(item.id);
        }}
      >
        <CollapsibleTrigger
          data-current={holdsCurrent ? '' : undefined}
          class="min-h-touch-target gap-2.5 rounded-sm border border-transparent px-2.5 py-1.5 text-start hover:bg-accent not-aria-expanded:data-current:border-selected not-aria-expanded:data-current:bg-selected not-aria-expanded:data-current:text-selected-foreground"
        >
          {@render label(item)}
          <ChevronRightIcon
            aria-hidden="true"
            class="size-4 shrink-0 rotate-0 transition-transform group-aria-expanded/collapsible-trigger:rotate-90 motion-reduce:transition-none"
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <ul
            class={[
              'ms-5 mt-1 flex flex-col gap-1 border-border ps-2',
              item === settings ? 'border-s-0' : 'border-s',
            ]}
          >
            {#each sectionEntries(item) as child (child.id)}
              {@render row(child)}
            {/each}
          </ul>
        </CollapsibleContent>
      </Collapsible>
    {:else}
      <a
        href={item.href}
        onclick={() => {
          open = false;
        }}
        aria-current={current ? 'page' : undefined}
        class={[
          'flex min-h-touch-target items-center gap-2.5 rounded-sm border px-2.5 py-1.5 text-sm font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset',
          current
            ? 'border-selected bg-selected text-selected-foreground'
            : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
        ]}
      >
        {@render label(item)}
      </a>
    {/if}
  </li>
{/snippet}

<nav
  aria-label={t('shell.nav')}
  data-slot="phone-bar"
  class="grid min-h-16 grid-cols-bar items-center gap-1 border-t border-border bg-background px-2 pt-2 pb-safe-bottom-2"
>
  <span
    class="relative flex w-fit min-w-0 items-center self-center px-2"
    title={connection === 'offline' ? t('shell.offlineDetail') : undefined}
  >
    <ConnectLogo size="sm" label={productName} />
    {#if connection === 'offline'}
      <ConnectionStatus
        state="offline"
        presentation="dot"
        class="absolute top-0 right-1 ring-2 ring-background"
      />
    {/if}
  </span>

  <Sheet
    bind:open
    onOpenChange={(value) => {
      if (value) expandedSections.clear();
    }}
  >
    {#if open}
      <!-- SheetClose, not the trigger again: bits-ui's trigger only opens. -->
      <SheetClose>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="icon-lg"
            aria-expanded={open}
            label={t('shell.closeMenu')}
          >
            <XIcon aria-hidden="true" class="size-6" />
          </Button>
        {/snippet}
      </SheetClose>
    {:else}
      <SheetTrigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="icon-lg"
            aria-expanded={open}
            label={t('shell.menu')}
          >
            <MenuIcon aria-hidden="true" class="size-6" />
          </Button>
        {/snippet}
      </SheetTrigger>
    {/if}

    <!-- preventScroll/trapFocus false: bits-ui has no non-modal option, so
         this skips its scroll lock and focus trap to keep the bar reachable. -->
    <SheetContent
      side="bottom"
      showCloseButton={false}
      preventScroll={false}
      trapFocus={false}
      onOpenAutoFocus={(event: Event) => {
        // Default focus moves to the connection glyph, opening its tooltip over the list.
        event.preventDefault();
      }}
      class={sheetContentClass}
      overlayClass={sheetOverlayClass}
    >
      <!-- One flex child: SheetContent's own gap-4 always wins the Tailwind
           cascade over a caller class, so a lone child avoids a gap. -->
      <div class="flex min-h-0 flex-1 flex-col">
        <SheetHeader
          class="flex-row items-center justify-between border-b border-border py-2"
        >
          <!-- A nested span, not a class on SheetTitle: Tailwind always
               orders text-base before its own text-lg, so a class here would lose. -->
          <SheetTitle
            ><span class="text-base">{t('shell.menu')}</span></SheetTitle
          >
          <span class="flex items-center gap-1">
            <ConnectionStatus state={connection} />
            <ThemeToggle />
            <UserMenu {principal} {principalDetail} {onSignOut} />
          </span>
        </SheetHeader>
        <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
          <ul class="flex flex-col gap-1 p-4">
            {#each items as item (item.id)}
              {@render row(item, true)}
            {/each}
            {#if settings !== undefined}
              {@render row(settings, true)}
            {/if}
          </ul>
        </div>
      </div>
    </SheetContent>
  </Sheet>

  <span class="flex items-center justify-end gap-1">{@render bell?.()}</span>
</nav>
