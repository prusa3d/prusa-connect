<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ConnectLogo from './connect-logo.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ConnectLogo',
    component: ConnectLogo,
  });
</script>

<!-- The PRUSA wordmark, which the app draws until design ships a new logo. -->
<Story name="Default" args={{ size: 'lg' }} />

<!-- The word Connect alone, the simple logo. -->
<Story name="Full" args={{ shape: 'full', size: 'lg' }} />

<!-- The orange square alone, as the app bar draws it. -->
<Story name="Mark" args={{ shape: 'mark', size: 'lg' }} />

<!-- The three heights, each shape on the same left edge. -->
<Story name="Sizes" asChild>
  <div class="flex flex-col items-start gap-6">
    {#each ['sm', 'md', 'lg'] as const as size (size)}
      <div class="flex items-center gap-6">
        <ConnectLogo shape="mark" {size} />
        <ConnectLogo {size} />
      </div>
    {/each}
  </div>
</Story>

<!-- The designer's two versions. Nothing here sets a colour on the logo:
     each panel sets a text colour, the word follows it, and the mark stays
     orange on both. -->
<Story name="Light And Dark Grounds" asChild>
  <div class="flex flex-wrap gap-4">
    <div class="flex-1 rounded-sm bg-white p-8 text-black">
      <ConnectLogo size="lg" />
    </div>
    <div class="flex-1 rounded-sm bg-black p-8 text-white">
      <ConnectLogo size="lg" />
    </div>
  </div>
</Story>
