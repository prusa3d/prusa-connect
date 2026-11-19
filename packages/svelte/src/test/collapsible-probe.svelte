<!--
  A trigger and its content, for the test to render. The parts only work
  together, and a test cannot write markup of its own.

  Every word comes in as a prop, so the test says what it asserts on.
-->
<script lang="ts">
  import CollapsibleContent from '../primitives/collapsible/collapsible-content.svelte';
  import CollapsibleTrigger from '../primitives/collapsible/collapsible-trigger.svelte';
  import Collapsible from '../primitives/collapsible/collapsible.svelte';

  interface Props {
    label: string;
    body: string;
    disabled?: boolean;
  }

  let { label, body, disabled = false }: Props = $props();
</script>

<Collapsible>
  <CollapsibleTrigger {disabled}>{label}</CollapsibleTrigger>
  <CollapsibleContent>{body}</CollapsibleContent>
</Collapsible>
