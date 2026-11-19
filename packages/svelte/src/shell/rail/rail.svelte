<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import PanelLeftIcon from '@lucide/svelte/icons/panel-left';

  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  import { t } from '../../i18n';
  import {
    Button,
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuTrigger,
    Separator,
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
  } from '../../primitives';
  import {
    isCurrent,
    isCurrentRow,
    type RailNavItem,
    type RailProps,
    sectionEntries,
  } from './types';

  let {
    items,
    groups = [],
    currentPath,
    collapsed = false,
    onToggle,
    settings,
    footer,
  }: RailProps = $props();

  const ungrouped = $derived(items.filter((item) => item.group === undefined));
  const rowIcon = 'size-icon-min shrink-0';

  const grouped = $derived(
    groups
      .map((group) => ({
        group,
        items: items.filter((item) => item.group === group.id),
      }))
      .filter((run) => run.items.length > 0),
  );

  const openSections = new SvelteSet<string>();
  // A tooltip opened over an open menu pauses the menu's focus scope in
  // bits-ui, so focus and Escape never reach the menu, and as the topmost
  // dismissable layer it swallows the outside pointerdown that should close
  // it. Every rail tooltip stays shut while a menu is showing.
  const openMenus = new SvelteSet<string>();

  // A menu belongs to the page it opened on: arriving elsewhere closes it.
  let menusPath = untrack(() => currentPath);
  $effect(() => {
    if (currentPath === menusPath) return;
    menusPath = currentPath;
    untrack(() => openMenus.clear());
  });

  const sectioned = $derived(
    [...items, ...(settings === undefined ? [] : [settings])].filter(
      (item) => (item.children?.length ?? 0) > 0,
    ),
  );

  // A group holding the open page opens itself, so arriving at a section by
  // a link or a reload shows where in the group the reader is. The group's
  // row only folds and never navigates; it closes the group, and only a move
  // to another section opens it again.
  $effect(() => {
    for (const item of sectioned) {
      const holds =
        isCurrent(item.href, currentPath) ||
        (item.children ?? []).some((child) =>
          isCurrent(child.href, currentPath),
        );
      if (holds) openSections.add(item.id);
    }
  });

  const rowClass = (current: boolean, nested = false) => [
    'flex min-h-9 items-center gap-2.5 rounded-sm border px-2.5 py-1.5 text-sm tracking-normal transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring motion-reduce:transition-none',
    nested && !current ? 'font-normal' : 'font-medium',
    collapsed && 'justify-center px-0',
    current
      ? 'border-selected bg-selected text-selected-foreground hover:border-selected-hovered hover:bg-selected-hovered'
      : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
  ];

  const toggleSection = (id: string) => {
    if (openSections.has(id)) openSections.delete(id);
    else openSections.add(id);
  };
</script>

{#snippet navLink(
  item: RailNavItem,
  current: boolean,
  props: Record<string, unknown>,
)}
  {@const Icon = item.icon}
  <a
    {...props}
    href={item.href}
    data-part="rail-link"
    aria-current={current ? 'page' : undefined}
    class={[
      ...rowClass(current, item.nested),
      item.nested && !collapsed && 'ps-7',
    ]}
  >
    {#if Icon}
      <Icon aria-hidden="true" class={rowIcon} />
    {/if}
    <span class={collapsed ? 'sr-only' : 'truncate'}>{item.label}</span>
  </a>
{/snippet}

<!-- A collapsed rail has no room to list the children under their item, so
     the item's row opens them in a menu beside the rail. -->
{#snippet sectionMenu(item: RailNavItem)}
  {@const Icon = item.icon}
  {@const entries = sectionEntries(item)}
  <DropdownMenu
    open={openMenus.has(item.id)}
    onOpenChange={(open) =>
      open ? openMenus.add(item.id) : openMenus.delete(item.id)}
  >
    <Tooltip disabled={openMenus.size > 0}>
      <TooltipTrigger>
        {#snippet child({ props: tip }: { props: Record<string, unknown> })}
          <DropdownMenuTrigger {...tip}>
            {#snippet child({ props })}
              <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- standing exception (ui-integration.md): a menu trigger in the rail's own Tooltip that must match the sibling nav-link rows -->
              <button
                {...props}
                type="button"
                class={[
                  ...rowClass(
                    entries.some((entry) => isCurrent(entry.href, currentPath)),
                  ),
                  'w-full',
                ]}
              >
                {#if Icon}
                  <Icon aria-hidden="true" class={rowIcon} />
                {/if}
                <span class="sr-only">{item.label}</span>
              </button>
            {/snippet}
          </DropdownMenuTrigger>
        {/snippet}
      </TooltipTrigger>
      <TooltipContent side="right">{item.label}</TooltipContent>
    </Tooltip>
    <DropdownMenuContent side="right" align="start">
      <DropdownMenuLabel>{item.label}</DropdownMenuLabel>
      {#each entries as entry (entry.id)}
        {@const EntryIcon = entry.icon}
        {@const current = isCurrentRow(entry, currentPath)}
        <DropdownMenuItem
          data-current={current || undefined}
          class={current
            ? 'bg-selected text-selected-foreground data-current:focus:bg-selected-hovered data-current:focus:text-selected-foreground'
            : undefined}
        >
          {#snippet child({ props })}
            <a
              {...props}
              href={entry.href}
              aria-current={current ? 'page' : undefined}
            >
              {#if EntryIcon}
                <EntryIcon aria-hidden="true" />
              {/if}
              {entry.label}
            </a>
          {/snippet}
        </DropdownMenuItem>
      {/each}
    </DropdownMenuContent>
  </DropdownMenu>
{/snippet}

<!-- `leaf` draws the item's own entry among its sections as a link: the item
     itself is the toggle above them. -->
{#snippet row(item: RailNavItem, leaf = false)}
  {@const current = isCurrentRow(item, currentPath)}
  {@const hasChildren = !leaf && !collapsed && (item.children?.length ?? 0) > 0}
  {@const open = hasChildren && openSections.has(item.id)}
  {@const sectionsId = `${item.id}-sections`}
  {@const isSettings = item.id === settings?.id}
  <Tooltip disabled={!collapsed || openMenus.size > 0}>
    <TooltipTrigger>
      {#snippet child({ props }: { props: Record<string, unknown> })}
        {#if hasChildren}
          {@const Icon = item.icon}
          <Button
            {...props}
            data-part="section-toggle"
            data-current={current || undefined}
            variant="ghost"
            type="button"
            class={[
              ...rowClass(current),
              'mb-1 h-auto w-full justify-start text-start',
            ]}
            aria-expanded={open}
            aria-controls={sectionsId}
            onclick={() => toggleSection(item.id)}
          >
            {#if Icon}<Icon aria-hidden="true" class={rowIcon} />{/if}
            <span class="truncate">{item.label}</span>
            <ChevronRightIcon
              aria-hidden="true"
              class={[
                'ms-auto size-4 shrink-0 opacity-60 transition-transform',
                open && 'rotate-90',
              ]}
            />
          </Button>
        {:else}
          {@render navLink(item, current, props)}
        {/if}
      {/snippet}
    </TooltipTrigger>
    <TooltipContent side="right">{item.label}</TooltipContent>
  </Tooltip>
  {#if open}
    <div
      id={sectionsId}
      class="ms-5 flex flex-col gap-1.5 border-s-0 border-border"
      data-settings-sections={isSettings || undefined}
    >
      {#each sectionEntries(item) as entry (entry.id)}
        {@render row(entry, entry === item)}
      {/each}
    </div>
  {/if}
{/snippet}

{#snippet topRow(item: RailNavItem)}
  {#if collapsed && (item.children?.length ?? 0) > 0}
    {@render sectionMenu(item)}
  {:else}
    {@render row(item)}
  {/if}
{/snippet}

<TooltipProvider delayDuration={0}>
  <!-- The rail shares the top bar's surface. -->
  <div
    data-slot="rail"
    data-collapsed={collapsed || undefined}
    class={[
      'flex h-full min-h-0 flex-col gap-2 border-e border-border bg-muted p-2 not-dark:border-panel-outline not-dark:bg-shell-chrome',
      collapsed ? 'w-15' : 'w-64 pt-6',
    ]}
  >
    <div
      data-slot="rail-scroll"
      class="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto [&>*]:shrink-0"
    >
      {#each ungrouped as item (item.id)}
        {@render topRow(item)}
      {/each}

      {#each grouped as run (run.group.id)}
        <p
          class={[
            'px-2 text-xs font-medium text-muted-foreground uppercase',
            collapsed ? 'sr-only mt-3' : 'mt-6',
          ]}
        >
          {run.group.label}
        </p>
        {#each run.items as item (item.id)}
          {@render topRow(item)}
        {/each}
      {/each}

      {#if settings}
        <Separator decorative={false} class={collapsed ? 'my-1.5' : 'my-4'} />
        {@render topRow(settings)}
      {/if}
    </div>

    <div
      data-slot="rail-footer"
      class={[
        'flex shrink-0 gap-1 border-t border-border px-3 py-3',
        collapsed ? 'flex-col items-center' : 'flex-row items-center',
      ]}
    >
      {@render footer?.()}
      {#if onToggle !== undefined}
        <Button
          variant="ghost"
          size="icon-md"
          class={!collapsed ? 'ms-auto' : undefined}
          label={collapsed ? t('shell.expand') : t('shell.collapse')}
          aria-expanded={!collapsed}
          onclick={onToggle}
        >
          <PanelLeftIcon aria-hidden="true" />
        </Button>
      {/if}
    </div>
  </div>
</TooltipProvider>

<style>
  [data-slot='rail']:not([data-collapsed]) {
    inline-size: calc(var(--spacing) * 64);
    padding-block-start: calc(var(--spacing) * 6);
  }

  [data-slot='rail'] :global([data-part='section-toggle']) {
    padding-inline: calc(var(--spacing) * 2.5);
    font-weight: var(--font-weight-medium);
  }

  [data-settings-sections] > :global(a:not([aria-current])) {
    font-weight: var(--font-weight-normal);
  }

  @media (hover: hover) and (pointer: fine) {
    [data-slot='rail']
      :global(
        :is([data-part='rail-link'], [data-part='section-toggle']):not(
            [aria-current='page'],
            [data-current='true']
          ):hover
      ) {
      background-color: color-mix(in oklab, var(--foreground) 5%, transparent);
      color: var(--foreground);
    }
  }
</style>
