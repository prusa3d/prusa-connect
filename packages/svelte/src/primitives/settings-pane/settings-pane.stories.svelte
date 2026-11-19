<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '../button/button.svelte';
  import Switch from '../switch/switch.svelte';
  import SettingsPane from './settings-pane.svelte';
  import SettingsRow from './settings-row.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/SettingsPane',
    component: SettingsPane,
  });
</script>

<Story name="Default" asChild>
  <SettingsPane title="Account" description="Who this browser is signed in as.">
    <SettingsRow label="Name" description="What other people see.">
      {#snippet control()}
        <span class="text-body">Alice</span>
      {/snippet}
    </SettingsRow>
    <SettingsRow label="Password" description="Last changed three months ago.">
      {#snippet control()}
        <Button size="sm" variant="secondary">Change</Button>
      {/snippet}
    </SettingsRow>
  </SettingsPane>
</Story>

<Story name="With actions" asChild>
  <SettingsPane title="Account">
    <SettingsRow label="Name">
      {#snippet control()}
        <span class="text-body">Alice</span>
      {/snippet}
    </SettingsRow>
    {#snippet actions()}
      <Button size="sm">Save</Button>
    {/snippet}
  </SettingsPane>
</Story>

<Story name="Switch Row" asChild>
  <SettingsPane title="Network">
    <SettingsRow
      layout="switch"
      label="Hide this server"
      description="Printers on the network no longer find it."
    >
      {#snippet control()}
        <Switch aria-label="Hide this server" />
      {/snippet}
    </SettingsRow>
  </SettingsPane>
</Story>

<Story name="Saved" args={{ title: 'Account', message: 'Saved.' }} />

<Story
  name="Refused"
  args={{
    title: 'Account',
    message: 'The server refused that name.',
    messageIsFailure: true,
  }}
/>
