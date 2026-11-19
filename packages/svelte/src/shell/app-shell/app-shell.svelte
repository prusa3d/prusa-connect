<script lang="ts">
  import { t } from '../../i18n';
  import { ConnectLogo, Notifications, Toaster } from '../../primitives';
  import { applyTheme, theme } from '../../styles/theme.svelte';
  import { AppFooter } from '../app-footer';
  import { ConnectionStatus } from '../connection-status';
  import { PhoneBar } from '../phone-bar';
  import { Rail } from '../rail';
  import { SendsIndicator } from '../sends-indicator';
  import { ThemeToggle } from '../theme-toggle';
  import { UserMenu } from '../user-menu';
  import { noticeItem } from './notice-item';
  import type { AppShellProps } from './types';

  let {
    productName,
    navItems,
    navGroups,
    settings,
    currentPath,
    homeHref,
    connection,
    principal,
    principalDetail,
    onSignOut,
    host,
    appVersion,
    apiReference,
    documentation,
    supportUrl,
    notifications,
    noticeHref,
    issueSummary,
    sends,
    banner,
    children,
  }: AppShellProps = $props();

  const noticeItems = $derived(
    (notifications?.items ?? []).map((notice) =>
      noticeItem(notice, noticeHref?.(notice), issueSummary),
    ),
  );

  // Following a notice's action is reading it.
  const followed = (id: string): void => {
    const notice = notifications?.items.find((item) => item.id === id);
    if (notice !== undefined && !notice.read) notifications?.toggleRead(id);
  };

  let collapsed = $state(false);
  let root = $state<HTMLElement>();
  let header = $state<HTMLElement>();
  let bar = $state<HTMLElement>();

  $effect(() => {
    applyTheme(theme.dark);
  });

  const publishHeight = (element: HTMLElement | undefined, name: string) => {
    if (element === undefined) return;
    let last = '';
    const write = () => {
      const next = `${String(Math.round(element.getBoundingClientRect().height))}px`;
      // Writing back inside a ResizeObserver without this guard loops.
      if (next === last) return;
      last = next;
      root?.style.setProperty(name, next);
      document.documentElement.style.setProperty(name, next);
    };
    write();
    const observer = new ResizeObserver(write);
    observer.observe(element);
    return () => {
      observer.disconnect();
    };
  };

  $effect(() => publishHeight(header, '--app-header-height'));
  $effect(() => publishHeight(bar, '--app-phone-bar-height'));
</script>

{#snippet brand()}
  <ConnectLogo size="xs" label={productName} />
{/snippet}

{#snippet bell()}
  {#if sends !== undefined}
    <SendsIndicator {sends} />
  {/if}
  <Notifications
    items={noticeItems}
    triggerSize="icon-md"
    onToggleRead={(id: string) => notifications?.toggleRead(id)}
    onMarkAllRead={() => notifications?.markAllRead()}
    onAction={followed}
  />
{/snippet}

<div
  bind:this={root}
  data-slot="app-shell"
  class="@container/app-shell flex min-h-svh flex-col bg-background text-foreground not-dark:bg-shell-canvas"
>
  <!-- The first Tab stop, so a keyboard reader passes the header and the nine
       navigation stops in one key. The router leaves a fragment link to the
       browser, which moves focus to `main`. It waits above the
       screen until it has focus. -->
  <a
    href="#main"
    class="fixed start-2 -top-full z-70 rounded-sm bg-background px-4 py-2 text-sm font-medium text-foreground outline-none focus-visible:top-2 focus-visible:ring-2 focus-visible:ring-ring"
  >
    {t('shell.skipToContent')}
  </a>
  <header
    bind:this={header}
    class="sticky top-0 z-10 hidden items-center justify-between gap-x-4 border-b border-border bg-muted px-4 py-4 not-dark:border-panel-outline not-dark:bg-shell-chrome @xl/app-shell:px-6 @3xl/app-shell:flex"
  >
    <span class="shrink-0">
      {#if homeHref}
        <a
          href={homeHref}
          class="rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
          aria-label={t('shell.home')}
        >
          {@render brand()}
        </a>
      {:else}
        {@render brand()}
      {/if}
    </span>
    <div class="flex items-center gap-2">
      <ConnectionStatus state={connection} />
      {@render bell()}
    </div>
  </header>

  <div class="flex min-h-0 flex-1">
    <nav
      aria-label={t('shell.nav')}
      class="sticky hidden shrink-0 @3xl/app-shell:flex"
      style="top: var(--app-header-height, 0px); height: calc(100svh - var(--app-header-height, 0px))"
    >
      <Rail
        items={navItems}
        groups={navGroups}
        {settings}
        {currentPath}
        {collapsed}
        onToggle={() => {
          collapsed = !collapsed;
        }}
        footer={railFoot}
      />
    </nav>

    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      {@render banner?.()}
      <main
        id="main"
        tabindex="-1"
        class="isolate min-w-0 flex-1 px-4 py-6 outline-none @xl/app-shell:px-6"
      >
        {@render children()}
      </main>
      <AppFooter
        {host}
        {appVersion}
        {apiReference}
        {documentation}
        {supportUrl}
      />
    </div>
  </div>

  <div bind:this={bar} class="sticky bottom-0 z-60 @3xl/app-shell:hidden">
    <PhoneBar
      {productName}
      items={navItems}
      groups={navGroups}
      {settings}
      {currentPath}
      {connection}
      {principal}
      {principalDetail}
      {onSignOut}
      {bell}
    />
  </div>

  <Toaster />
</div>

{#snippet railFoot()}
  <UserMenu {principal} {principalDetail} {onSignOut} />
  <ThemeToggle />
{/snippet}

<style>
  header {
    padding-block: calc(var(--spacing) * 4);
    min-block-size: 3.5rem;
  }
</style>
