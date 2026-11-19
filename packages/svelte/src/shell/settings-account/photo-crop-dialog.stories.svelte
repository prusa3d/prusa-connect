<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PhotoCropDialog from './photo-crop-dialog.svelte';
  const { Story } = defineMeta({
    title: 'Parts/Screen components/Account Photo Crop',
    component: PhotoCropDialog,
  });
  const sample = new File(
    [
      '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="500"><defs><linearGradient id="a"><stop stop-color="#fa6831"/><stop offset="1" stop-color="#364976"/></linearGradient></defs><rect width="800" height="500" fill="url(#a)"/><circle cx="540" cy="230" r="110" fill="#f6e7d8"/><path d="M300 500 Q540 150 780 500" fill="#1b1b1b"/></svg>',
    ],
    'sample.svg',
    { type: 'image/svg+xml' },
  );
  const onConfirm = () => undefined;
  const onCancel = () => undefined;
</script>

<Story name="Default" args={{ file: sample, onConfirm, onCancel }} />
<Story
  name="Unreadable"
  args={{
    file: new File(['broken'], 'broken.png', { type: 'image/png' }),
    onConfirm,
    onCancel,
  }}
/>
