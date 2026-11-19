<script module lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CollapsibleContent from './collapsible-content.svelte';
  import CollapsibleTrigger from './collapsible-trigger.svelte';
  import Collapsible from './collapsible.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Collapsible',
    component: Collapsible,
  });
</script>

{#snippet advancedSettings()}
  <Collapsible class="w-72">
    <CollapsibleTrigger>
      <span>Advanced settings</span>
      <ChevronDownIcon
        class="size-4 transition-transform group-state-open/collapsible-trigger:rotate-180"
      />
    </CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      Configure bed mesh calibration and input shaper tuning.
    </CollapsibleContent>
  </Collapsible>
{/snippet}

<Story name="Default" asChild children={advancedSettings} />

<Story name="Row" asChild>
  <Collapsible class="w-72">
    <CollapsibleTrigger variant="row">Advanced settings</CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      Configure bed mesh calibration.
    </CollapsibleContent>
  </Collapsible>
</Story>

<Story name="Filled" asChild>
  <Collapsible class="w-72">
    <CollapsibleTrigger variant="filled">Advanced settings</CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      Configure bed mesh calibration.
    </CollapsibleContent>
  </Collapsible>
</Story>

<Story name="Inline" asChild>
  <Collapsible class="w-72">
    <CollapsibleTrigger variant="inline">Advanced settings</CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      Configure bed mesh calibration.
    </CollapsibleContent>
  </Collapsible>
</Story>

<Story name="Inline, warning" asChild>
  <Collapsible class="w-72">
    <CollapsibleTrigger variant="inline" tone="warning">
      <TriangleAlertIcon aria-hidden="true" />
      3 issues
    </CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      The first issue, and the second.
    </CollapsibleContent>
  </Collapsible>
</Story>

<Story name="Row Without Chevron" asChild>
  <Collapsible class="w-72">
    <CollapsibleTrigger variant="row" chevron={false}>
      Advanced settings
    </CollapsibleTrigger>
    <CollapsibleContent class="pt-2 text-sm text-muted-foreground">
      Configure bed mesh calibration.
    </CollapsibleContent>
  </Collapsible>
</Story>

<Story
  name="Disabled Trigger Is Visibly Disabled"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col gap-4">
    <Collapsible class="w-72">
      <CollapsibleTrigger>Advanced settings (enabled)</CollapsibleTrigger>
      <CollapsibleContent>Configure bed mesh calibration.</CollapsibleContent>
    </Collapsible>
    <Collapsible class="w-72">
      <CollapsibleTrigger disabled>
        Advanced settings (disabled)
      </CollapsibleTrigger>
      <CollapsibleContent>Configure bed mesh calibration.</CollapsibleContent>
    </Collapsible>
  </div>
</Story>

<Story
  name="Opens And Closes By Keyboard And Aria Expanded Follows"
  tags={['!dev', '!autodocs']}
  asChild
  children={advancedSettings}
/>
