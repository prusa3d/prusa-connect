<!--
  The colour families a list narrows to, as a row of swatches: the materials
  list draws it in its Colour dropdown and the add dialog under its search.

  A second press on the picked swatch lets the filter go. A pick shuts the
  dropdown it is drawn in, as any single-choice filter does.

  The dropdown is portalled out of the module's slot, so the group carries the
  module's name for its utilities.
-->
<script lang="ts">
  import {
    SwatchPicker,
    facetPopoverClose,
  } from '@prusa/connect-ui/primitives';

  import { PALETTE } from './colour-palette';
  import { OWN_MODULE } from './own-module';
  import { heldHost } from './host';

  let {
    value,
    onPick,
    size = 'sm',
  }: {
    /** The picked family, undefined for every colour. */
    value: string | undefined;
    onPick: (family: string | undefined) => void;
    size?: 'xs' | 'sm';
  } = $props();

  const t = heldHost().t;
  const close = facetPopoverClose();
  const swatches = PALETTE.map((s) => ({
    key: s.key,
    hex: s.hex,
    label: t(`materials.colour.${s.key}`),
  }));
</script>

<div data-module={OWN_MODULE}>
  <SwatchPicker
    {swatches}
    {value}
    label={t('materials.add.colour')}
    deselectable
    {size}
    onPick={(family: string | undefined) => {
      onPick(family);
      close();
    }}
  />
</div>
