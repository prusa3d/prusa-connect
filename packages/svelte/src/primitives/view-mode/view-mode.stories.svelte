<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { viewModes } from './catalogue';
  import { ViewModeState } from './choice.svelte';
  import ViewModeToggle from './view-mode.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ViewModeToggle',
    component: ViewModeToggle,
  });

  const tooNarrow = 'The table needs a wider window.';
</script>

<script lang="ts">
  // A toggle that does not move when it is pressed shows half the component,
  // so each story that is pressed holds its own choice.
  const picked = $state({ unavailable: 'rows', marked: 'table' });

  const fit = new ViewModeState({
    initial: 'table',
    fallback: 'rows',
    minRem: { table: 40 },
  });

  let wide = $state(false);
</script>

<Story name="Default" asChild>
  <ViewModeToggle
    value="table"
    onValueChange={() => undefined}
    options={viewModes(['table', 'rows'])}
  />
</Story>

<Story name="Three Views" asChild>
  <ViewModeToggle
    value="cards"
    onValueChange={() => undefined}
    options={viewModes(['cards', 'table', 'rows'])}
  />
</Story>

<!-- One vocabulary for the whole tree, in one order. The subset is asked for
     backwards and comes back dense first, cameras last. -->
<Story name="One Vocabulary In One Order" tags={['!dev', '!autodocs']} asChild>
  <ViewModeToggle
    value="table"
    onValueChange={() => undefined}
    options={viewModes(['frames', 'cards', 'rows', 'table'])}
  />
</Story>

<!-- The button stays where it was rather than disappearing: a reader learns
     nothing from a control that is simply gone. -->
<Story name="A View That Does Not Fit Says So" asChild>
  <ViewModeToggle
    value="rows"
    onValueChange={() => undefined}
    options={viewModes(['table', 'rows'], { table: tooNarrow })}
  />
</Story>

<Story
  name="Clicking An Unavailable View Changes Nothing"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col items-start gap-2">
    <ViewModeToggle
      value={picked.unavailable}
      onValueChange={(next: string) => (picked.unavailable = next)}
      options={viewModes(['table', 'rows'], { table: tooNarrow })}
    />
    <p data-testid="chosen">{picked.unavailable}</p>
  </div>
</Story>

<!-- The choice is measured against the container and not the window, so a
     listing inside a narrow column falls back on a wide screen, and what the
     reader asked for is what comes back when the room does. -->
<Story
  name="The Hook Falls Back Inside A Narrow Container"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col items-start gap-2">
    <button type="button" onclick={() => (wide = !wide)}>
      {wide ? 'Narrow the column' : 'Widen the column'}
    </button>
    <div class={wide ? 'w-3xl' : 'w-xs'} {@attach fit.measure}>
      <ViewModeToggle
        value={fit.chosen}
        onValueChange={(next: string) => (fit.chosen = next)}
        options={viewModes(['table', 'rows'], {
          table: fit.roomFor('table') ? undefined : tooNarrow,
        })}
      />
      <p data-testid="rendered">{fit.mode}</p>
    </div>
  </div>
</Story>

<!-- The chosen view has to be marked, or the toggle says nothing about which
     of the two a reader is looking at. -->
<Story name="The Chosen View Is Marked" tags={['!dev', '!autodocs']} asChild>
  <ViewModeToggle
    value={picked.marked}
    onValueChange={(next: string) => (picked.marked = next)}
    options={viewModes(['table', 'rows'])}
  />
</Story>
