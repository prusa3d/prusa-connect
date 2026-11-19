<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PlusIcon from '@lucide/svelte/icons/plus';

  import Button from '../button/button.svelte';
  import PageHeader from './page-header.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageHeader',
    component: PageHeader,
  });
</script>

<!--
  Beyond the React set. React writes this row into each screen rather than
  shipping a component, so nothing here is a port. These are the states the
  props allow: a title alone, the one action most screens carry, and a screen
  that needs more than one.
-->

<Story name="Title Only" asChild>
  <div class="w-3xl">
    <PageHeader title="Printers" />
  </div>
</Story>

<Story name="With The One Action" asChild>
  <div class="w-3xl">
    <PageHeader
      title="Printers"
      action={{
        label: 'Add printer',
        icon: PlusIcon,
        onclick: () => undefined,
      }}
    />
  </div>
</Story>

<Story name="With Actions Of Its Own" asChild>
  <div class="w-3xl">
    <PageHeader title="Storage">
      {#snippet actions()}
        <Button variant="secondary">Refresh</Button>
        <Button>Upload</Button>
      {/snippet}
    </PageHeader>
  </div>
</Story>
