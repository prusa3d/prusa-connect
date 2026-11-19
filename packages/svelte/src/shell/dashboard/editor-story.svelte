<!--
  One card's own settings editor, for the dashboard's stories. It sets the
  context the screen sets and keeps the settings the editor changes.
-->
<script lang="ts">
  import type { Component } from 'svelte';

  import type { CardEditorProps, CardSettings } from '../../slots';
  import { type DashboardContext, setDashboardContext } from './context';

  let {
    context,
    editor: Editor,
    config = {},
  }: {
    context: DashboardContext;
    editor: Component<CardEditorProps>;
    config?: CardSettings;
  } = $props();

  // svelte-ignore state_referenced_locally
  setDashboardContext(context);

  // svelte-ignore state_referenced_locally
  let settings = $state.raw(config);
</script>

<div
  class="w-96 rounded-sm border border-border bg-card p-3 text-card-foreground"
>
  <Editor config={settings} onchange={(next) => (settings = next)} />
</div>
