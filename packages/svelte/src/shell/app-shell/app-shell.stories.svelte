<script module lang="ts">
  import LayersIcon from '@lucide/svelte/icons/layers';
  import ListChecksIcon from '@lucide/svelte/icons/list-checks';
  import { PrinterGlyphIcon } from '../../primitives';
  import SettingsIcon from '@lucide/svelte/icons/settings';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { NavGroup, NavItem } from '../../slots';
  import { SafeModeBanner } from '../safe-mode-banner';
  import type { SendsSource } from '../sends-indicator';
  import AppShell from './app-shell.svelte';

  const { Story } = defineMeta({
    title: 'Shell/AppShell',
    component: AppShell,
  });

  const navItems: NavItem[] = [
    {
      id: 'printers',
      label: 'Printers',
      href: '/printers',
      icon: PrinterGlyphIcon,
    },
    { id: 'jobs', label: 'Jobs', href: '/jobs', icon: ListChecksIcon },
    {
      id: 'settings',
      label: 'Settings',
      href: '/settings',
      icon: SettingsIcon,
    },
    {
      id: 'spools',
      label: 'Spools',
      href: '/m/spools',
      icon: LayersIcon,
      group: 'extensions',
    },
  ];

  const navGroups: NavGroup[] = [{ id: 'extensions', label: 'Extensions' }];

  // One file still going up and one on its way to its printer.
  const sends: SendsSource = {
    rows: [
      {
        id: 'job:2',
        deviceId: 'mk4',
        printerName: 'MK4 #1',
        name: 'cube.bgcode',
        step: 'uploading',
        progress: 0.4,
        mine: true,
        startedBy: undefined,
        canCancel: true,
        stopping: false,
      },
      {
        id: 'transfer:01J',
        deviceId: 'xl',
        printerName: 'XL #2',
        name: 'benchy.bgcode',
        step: 'sending',
        progress: 0.75,
        mine: true,
        startedBy: undefined,
        canCancel: true,
        stopping: false,
      },
    ],
    active: true,
    uploading: true,
    cancel: () => undefined,
  };
</script>

<Story name="Online" asChild>
  <AppShell
    productName="Prusa Connect"
    {navItems}
    {navGroups}
    currentPath="/printers"
    connection="online"
    principal="user:alice"
    onSignOut={() => undefined}
    host="printer.local"
    appVersion="0.1.0"
  >
    <div class="p-4">The fleet</div>
  </AppShell>
</Story>

<!-- The header shows the files on their way to printers on every page. -->
<Story name="Files On Their Way" asChild>
  <AppShell
    productName="Prusa Connect"
    {navItems}
    {navGroups}
    currentPath="/printers"
    connection="online"
    principal="user:alice"
    onSignOut={() => undefined}
    host="printer.local"
    {sends}
  >
    <div class="p-4">The fleet</div>
  </AppShell>
</Story>

<!-- The banner sits above the page, never inside it. -->
<Story name="Safe Mode" asChild>
  <AppShell
    productName="Prusa Connect"
    {navItems}
    {navGroups}
    currentPath="/printers"
    connection="online"
    principal="user:alice"
    onSignOut={() => undefined}
    host="printer.local"
  >
    {#snippet banner()}
      <SafeModeBanner
        since="2026-10-30T09:12:00.000Z"
        restarting={false}
        timedOut={false}
        onRestartNormally={() => undefined}
      />
    {/snippet}
    <div class="p-4">The fleet</div>
  </AppShell>
</Story>

<Story name="Offline" asChild>
  <AppShell
    productName="Prusa Connect"
    {navItems}
    currentPath="/printers"
    connection="offline"
    host="printer.local"
  >
    <div class="p-4">The fleet</div>
  </AppShell>
</Story>

<Story name="Narrow" asChild>
  <div class="w-96 border border-border">
    <AppShell
      productName="Prusa Connect"
      {navItems}
      {navGroups}
      currentPath="/printers"
      connection="online"
      principal="user:alice"
    >
      <div class="p-4">The fleet</div>
    </AppShell>
  </div>
</Story>

<Story name="Long mobile page" asChild>
  <div class="w-80 border border-border">
    <AppShell
      productName="Prusa Connect"
      {navItems}
      currentPath="/printers"
      connection="online"
    >
      <div class="min-h-screen p-4">The fleet</div>
      <div class="min-h-screen p-4">More printers</div>
    </AppShell>
  </div>
</Story>
