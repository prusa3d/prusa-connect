<script module lang="ts">
  import LayersIcon from '@lucide/svelte/icons/layers';
  import ListChecksIcon from '@lucide/svelte/icons/list-checks';
  import { PrinterGlyphIcon } from '../../primitives';
  import ServerIcon from '@lucide/svelte/icons/server';
  import SettingsIcon from '@lucide/svelte/icons/settings';
  import UserIcon from '@lucide/svelte/icons/user';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { NavGroup } from '../../slots';
  import Rail from './rail.svelte';
  import type { RailNavItem } from './types';

  const { Story } = defineMeta({
    title: 'Shell/Rail',
    component: Rail,
  });

  const items: RailNavItem[] = [
    {
      id: 'printers',
      label: 'Printers',
      href: '/printers',
      icon: PrinterGlyphIcon,
    },
    { id: 'jobs', label: 'Jobs', href: '/jobs', icon: ListChecksIcon },
    {
      id: 'spools',
      label: 'Spools',
      href: '/m/spools',
      icon: LayersIcon,
      group: 'extensions',
    },
  ];

  const groups: NavGroup[] = [{ id: 'extensions', label: 'Extensions' }];

  const settings: RailNavItem = {
    id: 'settings',
    label: 'Settings',
    href: '/settings',
    icon: SettingsIcon,
  };

  const settingsWithSections: RailNavItem = {
    ...settings,
    href: '/settings/account',
    children: [
      {
        id: 'account',
        label: 'Account',
        href: '/settings/account',
        icon: UserIcon,
      },
      {
        id: 'server',
        label: 'Server',
        href: '/settings/server',
        icon: ServerIcon,
      },
    ],
  };

  // A module with several pages: its row folds them, as Settings does.
  const grouped: RailNavItem[] = [
    ...items,
    {
      id: 'queue',
      label: 'Queue',
      href: '/m/queue/plan',
      icon: ListChecksIcon,
      children: [
        { id: 'plan', label: 'Plan', href: '/m/queue/plan' },
        { id: 'queue-jobs', label: 'Jobs', href: '/m/queue/jobs' },
      ],
    },
  ];
</script>

<Story name="Open" asChild>
  <div class="h-96">
    <Rail
      {items}
      {groups}
      {settings}
      currentPath="/printers"
      onToggle={() => undefined}
    />
  </div>
</Story>

<Story name="Open with sections" asChild>
  <div class="h-96">
    <Rail
      items={grouped}
      {groups}
      settings={settingsWithSections}
      currentPath="/m/queue/jobs"
      onToggle={() => undefined}
    />
  </div>
</Story>

<Story name="Collapsed" asChild>
  <div class="h-96">
    <Rail
      {items}
      {groups}
      {settings}
      currentPath="/printers"
      collapsed
      onToggle={() => undefined}
    />
  </div>
</Story>

<Story name="Collapsed with sections" asChild>
  <div class="h-96">
    <Rail
      {items}
      {groups}
      settings={settingsWithSections}
      currentPath="/settings/server"
      collapsed
      onToggle={() => undefined}
    />
  </div>
</Story>
