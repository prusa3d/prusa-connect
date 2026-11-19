<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Input from '../input/input.svelte';
  import { RadioGroup } from '../radio-group';
  import RadioGroupItem from '../radio-group/radio-group-item.svelte';
  import Field from './field.svelte';
  import type { FieldControl } from './types';

  const { Story } = defineMeta({
    title: 'Primitives/Field',
    component: Field,
    args: {
      label: 'Nozzle diameter',
      control: Input,
      controlProps: { placeholder: '0.4' },
    },
  });
</script>

<Story name="Default" />

<Story name="With Description" args={{ description: 'In millimetres.' }} />

<Story name="Row Layout" args={{ layout: 'row' }} />

<Story
  name="With Error"
  args={{
    description: 'In millimetres.',
    error: 'Must be between 0.2 and 1.0.',
    controlProps: { value: '1.6' },
  }}
/>

<Story
  name="Error Reaches The Controls Accessible Description"
  tags={['!dev', '!autodocs']}
  args={{
    description: 'In millimetres.',
    error: 'Must be between 0.2 and 1.0.',
    controlProps: { value: '1.6' },
  }}
/>

<Story
  name="Error Changes The Controls Border Colour"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col gap-4">
    <Field label="Valid" control={Input} controlProps={{ value: '0.4' }} />
    <Field
      label="Invalid"
      error="Required."
      control={Input}
      controlProps={{ value: '' }}
    />
  </div>
</Story>

<Story
  name="Two Fields With The Same Label Do Not Collide"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col gap-4">
    <Field
      label="Nozzle diameter"
      control={Input}
      controlProps={{ value: '0.4' }}
    />
    <Field
      label="Nozzle diameter"
      control={Input}
      controlProps={{ value: '0.6' }}
    />
  </div>
</Story>

<Story
  name="External Aria Described By Is Preserved"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="flex flex-col gap-4">
    <span id="firmware-hint" class="text-sm text-muted-foreground">
      Firmware-reported default.
    </span>
    <Field
      label="Nozzle diameter"
      description="In millimetres."
      control={Input}
      controlProps={{ value: '0.4', 'aria-describedby': 'firmware-hint' }}
    />
  </div>
</Story>

<Story
  name="Radio Groups Own Description Never Reaches The Focused Item"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div>
    <span id="hint">Firmware default is 0.4mm.</span>
    <RadioGroup aria-describedby="hint" value="a">
      <RadioGroupItem value="a" id="a" aria-label="A" />
      <RadioGroupItem value="b" id="b" aria-label="B" />
    </RadioGroup>
  </div>
</Story>

<Story
  name="Radio Group Is Refused With A Clear Error"
  tags={['!dev', '!autodocs']}
  asChild
>
  <svelte:boundary>
    <!-- The type refuses a radio group too, for a reason of its own: its
         props are a union a field cannot satisfy. The cast reaches the
         refusal that names the real reason. -->
    <Field
      label="Nozzle diameter"
      control={RadioGroup as unknown as FieldControl}
    />
    {#snippet failed(error)}
      <!-- eslint-disable-next-line no-restricted-syntax -- story fixture: the boundary prints the refusal it caught as plain text -->
      <p role="alert">{String(error)}</p>
    {/snippet}
  </svelte:boundary>
</Story>
