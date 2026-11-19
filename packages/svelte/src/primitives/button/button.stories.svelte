<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
  import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
  import PauseIcon from '@lucide/svelte/icons/pause';

  import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
  } from '../dropdown-menu';
  import Button from './button.svelte';
  import type { ButtonIconSize, ButtonTextSize, ButtonVariant } from './types';

  const { Story } = defineMeta({
    title: 'Primitives/Button',
    component: Button,
  });

  const variants: ButtonVariant[] = [
    'default',
    'secondary',
    'ghost',
    'destructive',
    'link',
  ];
  const sizes: ButtonTextSize[] = ['sm', 'compact', 'md', 'lg'];
  const iconSizes: ButtonIconSize[] = ['icon-sm', 'icon-md', 'icon-lg'];
</script>

<Story name="Default">Print</Story>

<Story name="Slicer Reference" asChild>
  <div
    class="flex w-button-wide max-w-full flex-col items-start gap-control-inline"
  >
    <Button size="sm" variant="secondary">Label</Button>
    <Button size="sm" variant="secondary" disabled>Label</Button>
    <Button
      size="sm"
      variant="secondary"
      data-reference-state="hover"
      class="reference-state-hover:bg-secondary-hovered dark:reference-state-hover:bg-button-neutral-hovered"
      >Label</Button
    >
    <Button size="sm">Label</Button>
    <Button
      size="sm"
      data-reference-state="hover"
      class="reference-state-hover:bg-button-accent-hovered">Label</Button
    >
    <Button size="md">Label</Button>
    <Button
      size="md"
      data-reference-state="hover"
      class="reference-state-hover:bg-button-accent-hovered">Label</Button
    >
    <Button size="lg">Label</Button>
    <Button
      size="lg"
      data-reference-state="hover"
      class="reference-state-hover:bg-button-accent-hovered">Label</Button
    >
  </div>
</Story>

<Story name="All Variants" asChild>
  <div class="flex flex-wrap gap-3">
    <Button variant="default">Print</Button>
    <Button variant="secondary">Pause</Button>
    <Button variant="ghost">Details</Button>
    <Button variant="destructive">Stop</Button>
    <Button variant="default" disabled>Disabled</Button>
  </div>
</Story>

<Story name="Defaults To Type Button">Print</Story>

<Story name="As Child Renders An Anchor" asChild>
  <Button>
    {#snippet child({ props })}
      <a {...props} href="#printers">Printers</a>
    {/snippet}
  </Button>
</Story>

<Story name="Ghost Has A Resting Colour" asChild>
  <Button variant="ghost">Details</Button>
</Story>

<Story name="Has One Radius Across Sizes" asChild>
  <div class="flex gap-3">
    <Button size="sm">Print</Button>
    <Button size="lg">Print</Button>
  </div>
</Story>

<Story name="Keyboard Focus Is Visible">Print</Story>

<Story name="Has A Pointer Cursor Unless Disabled" asChild>
  <div class="flex gap-3">
    <Button>Print</Button>
    <Button disabled>Print</Button>
  </div>
</Story>

<Story name="Every Variant Acknowledges A Press" asChild>
  <div class="flex flex-wrap gap-3">
    <Button variant="default">Print</Button>
    <Button variant="secondary">Pause</Button>
    <Button variant="ghost">Details</Button>
    <Button variant="destructive">Stop</Button>
  </div>
</Story>

<Story name="Touch Sizes Measure Forty Four Pixels Under A Finger" asChild>
  <div class="flex items-center gap-3">
    <Button size="lg">Print</Button>
    <Button size="icon-lg" label="Print once more">
      <span aria-hidden="true">+</span>
    </Button>
  </div>
</Story>

<!-- Beyond the React set -->

<Story name="Link" asChild>
  <p class="text-sm">
    Read the <Button variant="link">setup guide</Button> before you start.
  </p>
</Story>

<Story name="Link In Paragraph" asChild>
  <p class="max-w-xs text-sm">
    A longer sentence that has to wrap onto a second and third line, with a
    <Button variant="link">setup guide link that wraps with the text</Button>
    in the middle of it.
  </p>
</Story>

<Story name="Link As Anchor" asChild>
  <p class="text-base">
    Open the
    <Button variant="link">
      {#snippet child({ props })}
        <a {...props} href="#api">API reference</a>
      {/snippet}
    </Button>
    for the endpoints.
  </p>
</Story>

<Story name="Aria Disabled Per Variant" asChild>
  <div class="flex flex-wrap gap-3">
    {#each variants as variant (variant)}
      <Button {variant} aria-disabled="true">{variant}</Button>
    {/each}
  </div>
</Story>

<Story name="Pending Per Variant" asChild>
  <div class="flex flex-wrap gap-3">
    {#each variants as variant (variant)}
      <Button {variant} pending>{variant}</Button>
    {/each}
  </div>
</Story>

<Story name="Sizes" asChild>
  <div class="flex flex-wrap items-center gap-3">
    {#each sizes as size (size)}
      <Button {size}>{size}</Button>
    {/each}
    {#each iconSizes as size (size)}
      <Button {size} label={size}>+</Button>
    {/each}
  </div>
</Story>

<Story name="Icon Button Names Itself In A Tooltip" asChild>
  <Button variant="ghost" size="icon-md" label="Pause the print">
    <PauseIcon aria-hidden="true" />
  </Button>
</Story>

<Story name="Icon Button Opens A Menu" asChild>
  <DropdownMenu>
    <DropdownMenuTrigger>
      {#snippet child({ props })}
        <Button {...props} variant="ghost" size="icon-md" label="More actions">
          <EllipsisIcon aria-hidden="true" />
        </Button>
      {/snippet}
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem>Rename</DropdownMenuItem>
      <DropdownMenuItem>Duplicate</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</Story>

<Story name="Link with an icon" asChild>
  <div class="flex w-56 flex-col gap-3 text-sm">
    <Button variant="link" size="inline-icon">
      <ExternalLinkIcon aria-hidden="true" />
      Setup guide
    </Button>
    <Button variant="link" size="inline-icon">
      <ExternalLinkIcon aria-hidden="true" />
      A guide whose title is long enough to wrap onto a second line
    </Button>
  </div>
</Story>

<!-- `sm` in the body size, for an action standing among settings. -->
<Story name="Control Size" asChild>
  <Button size="control" variant="secondary">Save</Button>
</Story>

<!-- `md` at the height of the text field it stands beside. -->
<Story name="Field Size" asChild>
  <Button size="field" variant="secondary">New folder</Button>
</Story>

<!-- A person's picture that is pressed. -->
<Story name="Avatar Size" asChild>
  <Button size="avatar" variant="ghost" radius="full" label="Edit photo">
    <span aria-hidden="true">AB</span>
  </Button>
</Story>

<Story name="Round" asChild>
  <Button size="icon-lg" variant="secondary" radius="full" label="Edit photo">
    <span aria-hidden="true">+</span>
  </Button>
</Story>
