<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PageTrail from './page-trail.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Page Trail',
    component: PageTrail,
    args: {
      trail: [{ label: 'Printers', href: '#/printers' }],
      page: 'MK4S, Print Farm Rack 3',
    },
  });

  const folders = [
    { label: 'USB Drive', href: '#/storage/usb' },
    { label: 'production', href: '#/storage/usb/folder/production' },
    {
      label: 'brackets',
      href: '#/storage/usb/folder/production%2Fbrackets',
    },
  ];
</script>

<!-- What a detail page reached from a listing carries: one ancestor, then
     itself. -->
<Story name="One Ancestor" />

<!-- A file's page: every folder above it, and the name in the theme's sans
     face. -->
<Story
  name="A Path Of Folders"
  args={{
    trail: folders,
    page: 'left-bracket-v4.bgcode',
    sansPage: true,
  }}
/>

<!-- A detail whose parent is the same screen with the listing back in place,
     not another page. The crumb is a button and reads like the links above. -->
<Story
  name="An Ancestor That Is A State"
  args={{
    trail: [{ label: 'Search results', onSelect: () => undefined }],
    page: 'benchy.gcode',
  }}
/>

<!-- The current page is named, never linked. That is the W3C breadcrumb
     pattern's own rule, and the reason a reader can tell where the trail
     ends. -->
<Story name="The Current Page Is Not A Link" tags={['!dev', '!autodocs']} />
