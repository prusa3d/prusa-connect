<script module lang="ts">
  import FolderIcon from '@lucide/svelte/icons/folder';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FileThumbnail from './file-thumbnail.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/FileThumbnail',
    component: FileThumbnail,
  });

  // A plate with three parts on it, drawn rather than fetched so the story
  // needs no server and no network.
  const plate =
    'data:image/svg+xml;utf8,' +
    encodeURIComponent(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">' +
        '<rect width="120" height="120" fill="%23222"/>' +
        '<rect x="14" y="18" width="40" height="30" fill="%23f90"/>' +
        '<rect x="64" y="18" width="40" height="30" fill="%23f90"/>' +
        '<rect x="30" y="62" width="60" height="40" fill="%23f90"/>' +
        '</svg>',
    );
</script>

<script lang="ts">
  let showing = $state(false);
  let alreadyShowing = $state(true);
</script>

<!--
  Beyond the React set. React ships no file-thumbnail.stories.tsx, so nothing
  here is a port. These are the states the props allow: a preview, no preview,
  and the preview already open.
-->

<Story name="Default" asChild>
  <FileThumbnail
    name="Bracket_Left_Reinforced_v4_FINAL.gcode"
    thumbnailUrl={plate}
    glyph={FolderIcon}
    bind:open={showing}
  />
</Story>

<Story name="Nothing To Preview" asChild>
  <FileThumbnail name="Brackets" glyph={FolderIcon} />
</Story>

<Story name="Preview Showing" asChild>
  <div class="p-8">
    <FileThumbnail
      name="Bracket_Left_Reinforced_v4_FINAL.gcode"
      thumbnailUrl={plate}
      glyph={FolderIcon}
      bind:open={alreadyShowing}
    />
  </div>
</Story>

<Story name="Wide" asChild>
  <FileThumbnail name="Brackets" glyph={FolderIcon} size="wide" class="w-60" />
</Story>

<Story name="Square Corners" asChild>
  <FileThumbnail name="Brackets" glyph={FolderIcon} radius="none" />
</Story>
