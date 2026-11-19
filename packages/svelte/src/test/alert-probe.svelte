<!--
  An alert with its title and description, for the test to render. The parts
  only work together, and a test cannot write markup of its own.

  Every word comes in as a prop.
-->
<script lang="ts">
  import InfoIcon from '@lucide/svelte/icons/info';

  import AlertDescription from '../primitives/alert/alert-description.svelte';
  import AlertTitle from '../primitives/alert/alert-title.svelte';
  import Alert from '../primitives/alert/alert.svelte';
  import type { AlertVariant } from '../primitives/alert/types';

  interface Props {
    variant?: AlertVariant;
    title: string;
    description?: string;
    /** Draws an icon as a direct child, which is what opens the first column. */
    icon?: boolean;
  }

  let {
    variant = 'default',
    title,
    description,
    icon = false,
  }: Props = $props();
</script>

<Alert {variant}>
  {#if icon}
    <InfoIcon />
  {/if}
  <AlertTitle>{title}</AlertTitle>
  {#if description}
    <AlertDescription>{description}</AlertDescription>
  {/if}
</Alert>
