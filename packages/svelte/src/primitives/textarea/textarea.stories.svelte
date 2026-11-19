<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Label from '../label/label.svelte';
  import Textarea from './textarea.svelte';

  const threeLines = 'Line one\nLine two\nLine three';

  const { Story } = defineMeta({
    title: 'Primitives/Textarea',
    component: Textarea,
    args: { placeholder: 'Printer notes' },
  });
</script>

<Story name="Default" />

<Story name="Resizable" args={{ resizable: true, rows: 4 }} />

<Story name="All States" asChild>
  <div class="flex flex-col gap-3">
    <Textarea placeholder="Rest" />
    <Textarea aria-label="Filled" value={threeLines} />
    <Textarea placeholder="Disabled" disabled />
  </div>
</Story>

<Story
  name="Label Association Finds The Textarea"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col gap-2">
    <Label for="textarea-with-label">Printer notes</Label>
    <Textarea id="textarea-with-label" value="Nozzle 0.4 mm" />
  </div>
</Story>

<Story name="Typing Lands In The Value" tags={['!dev', '!autodocs']} />

<Story
  name="Disabled Textarea Is Not Reachable By Keyboard"
  tags={['!dev', '!autodocs']}
  args={{ disabled: true }}
/>
