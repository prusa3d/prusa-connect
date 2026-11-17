<script module lang="ts">
  import PencilIcon from '@lucide/svelte/icons/pencil';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '../button/button.svelte';
  import HeaterRow from './heater-row.svelte';
  import HeaterRows from './heater-rows.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/HeaterRow',
    component: HeaterRow,
    args: {
      label: 'Tool 1',
      kind: 'tool',
      nozzleTemp: 234.6,
      targetTemp: 235,
      material: 'PLA',
    },
  });
</script>

{#snippet setTarget()}
  <Button variant="ghost" size="icon-md" class="size-11" label="Set target">
    <PencilIcon />
  </Button>
{/snippet}

{#snippet setToolOneTarget()}
  <Button
    variant="ghost"
    size="icon-sm"
    label="Set Tool 1 nozzle target, currently 235°C"
  >
    <PencilIcon />
  </Button>
{/snippet}

<Story name="Default" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={234.6}
      targetTemp={235}
      material="PLA"
    />
  </HeaterRows>
</Story>

<!-- 62 climbing to 235: the reading only, whole degrees, pulsing, warning
     tint, a rising arrow, never the target. The stub action keeps the story on
     that point: with no action the row carries the target itself. -->
<Story name="Heating" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={62}
      targetTemp={235}
      material="PLA"
      action={setTarget}
    />
  </HeaterRows>
</Story>

<!-- Two shapes of cooling, towards a real target and towards off. They read
     the same now that the target has left the row, and only the editor trigger
     still says which is which. -->
<Story name="Cooling" asChild>
  <HeaterRows rows={2}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={240}
      targetTemp={170}
      material="ABS"
      action={setTarget}
    />
    <HeaterRow
      label="Tool 2"
      kind="tool"
      nozzleTemp={180}
      targetTemp={undefined}
      material="PLA"
      action={setTarget}
    />
  </HeaterRows>
</Story>

<!-- A heating row and a cooling row differ by more than colour: the arrow
     glyph itself, a real up and down pair rather than one icon rotated. -->
<Story
  name="Heating And Cooling Differ By The Glyph Not Only Colour"
  tags={['!dev', '!autodocs']}
  asChild
>
  <HeaterRows rows={2}>
    <HeaterRow label="Tool 1" kind="tool" nozzleTemp={62} targetTemp={235} />
    <HeaterRow label="Tool 2" kind="tool" nozzleTemp={240} targetTemp={170} />
  </HeaterRows>
</Story>

<!-- Within the 2° tolerance: no arrow, no tint, the row's own foreground. The
     action's accessible name is what keeps the target confirmable. -->
<Story name="At Temperature" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={234.6}
      targetTemp={235}
      material="PLA"
      action={setToolOneTarget}
    />
  </HeaterRows>
</Story>

<!-- Room temperature, no target: the bare reading, no arrow, and the word
     itself stays visible. Off and at-temperature both carry no tint and no
     arrow, and off has no colour to begin with. -->
<Story name="Off" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={23}
      targetTemp={undefined}
    />
  </HeaterRows>
</Story>

<Story name="Not Reported" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={undefined}
      targetTemp={undefined}
    />
  </HeaterRows>
</Story>

<Story name="Five Heads" asChild>
  <HeaterRows rows={5}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={234.6}
      targetTemp={235}
      material="PLA"
      active
    />
    <HeaterRow
      label="Tool 2"
      kind="tool"
      nozzleTemp={210}
      targetTemp={210}
      material="PETG"
    />
    <HeaterRow
      label="Tool 3"
      kind="tool"
      nozzleTemp={245}
      targetTemp={250}
      material="ABS"
    />
    <HeaterRow
      label="Tool 4"
      kind="tool"
      nozzleTemp={200}
      targetTemp={200}
      material="PETG"
    />
    <HeaterRow
      label="Tool 5"
      kind="tool"
      nozzleTemp={60}
      targetTemp={60}
      material="TPU"
    />
  </HeaterRows>
</Story>

<Story name="MMU" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={234.6}
      targetTemp={235}
      material="PETG"
      filamentPositions={[
        { position: 1, loaded: false },
        { position: 2, loaded: true },
        { position: 3, loaded: false },
        { position: 4, loaded: false },
        { position: 5, loaded: false },
      ]}
    />
  </HeaterRows>
</Story>

<!-- No action means no control anywhere confirms the target, so the row is the
     only surface left and carries it in its own title and in a
     screen-reader-only suffix. -->
<Story name="Without Action" asChild>
  <HeaterRows rows={1}>
    <HeaterRow
      label="Tool 1"
      kind="tool"
      nozzleTemp={62}
      targetTemp={235}
      material="PLA"
    />
  </HeaterRows>
</Story>

<!-- Chamber: the one kind with no material line. It warms towards its target
     while a print waits on it, and with no target it is off however warm the
     air, as an XL enclosure with no target is. -->
<Story name="Chamber" asChild>
  <HeaterRows rows={3}>
    <HeaterRow
      label="Chamber"
      kind="chamber"
      nozzleTemp={38.4}
      targetTemp={45}
      action={setTarget}
    />
    <HeaterRow
      label="Chamber"
      kind="chamber"
      nozzleTemp={45.2}
      targetTemp={45}
    />
    <HeaterRow
      label="Enclosure"
      kind="chamber"
      nozzleTemp={31}
      targetTemp={undefined}
    />
  </HeaterRows>
</Story>

<!-- A genuinely long material name gives up its width rather than wrapping or
     spilling into the reading. A tool row, not a chamber: a chamber carries no
     material, so the floor it needs is smaller and testing against one misses
     the row shape the floor exists for. -->
<Story name="Truncates At Heat Floor" tags={['!dev', '!autodocs']} asChild>
  <div class="w-[14.25rem]">
    <HeaterRow
      label="Tool 2"
      kind="tool"
      nozzleTemp={28}
      targetTemp={undefined}
      material="A very long material name that must truncate"
      action={setTarget}
    />
  </div>
</Story>

<!-- The regression the floor exists to prevent: an ordinary short material
     name on the widest realistic label renders whole at the floor. -->
<Story
  name="Fits Ordinary Material At Heat Floor"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="w-[14.25rem]">
    <HeaterRow
      label="Tool 2"
      kind="tool"
      nozzleTemp={210}
      targetTemp={210}
      material="PETG"
      action={setTarget}
    />
  </div>
</Story>
