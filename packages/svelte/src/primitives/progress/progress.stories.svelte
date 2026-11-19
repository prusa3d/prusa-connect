<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Progress from './progress.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Progress',
    component: Progress,
    args: { value: 42, 'aria-label': 'Print progress' },
  });
</script>

<Story name="Default" />

<Story name="All Values" asChild>
  <div class="flex w-64 flex-col gap-4">
    <Progress value={0} aria-label="0%" />
    <Progress value={42} aria-label="42%" />
    <Progress value={100} aria-label="100%" />
    <Progress value={null} aria-label="Indeterminate" />
  </div>
</Story>

<Story name="All Tones" asChild>
  <div class="flex w-64 flex-col gap-4">
    <Progress value={42} aria-label="In motion" />
    <Progress value={42} tone="muted" aria-label="Stopped advancing" />
    <Progress value={100} tone="success" aria-label="Finished" />
    <Progress value={85} tone="warning" aria-label="Close to its limit" />
    <Progress value={100} tone="destructive" aria-label="Past its limit" />
  </div>
</Story>

<Story
  name="Indeterminate Fills Its Track"
  tags={['!dev', '!autodocs']}
  args={{ value: null }}
/>

<Story name="Determinate Aria Reflects Value" tags={['!dev', '!autodocs']} />

<Story
  name="Indeterminate Omits Aria Valuenow"
  tags={['!dev', '!autodocs']}
  args={{ value: null }}
/>

<Story name="Small" args={{ size: 'sm' }} />
