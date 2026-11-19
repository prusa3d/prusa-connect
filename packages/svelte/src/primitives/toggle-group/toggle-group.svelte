<script lang="ts">
  import { ToggleGroup as ToggleGroupPrimitive } from 'bits-ui';

  import { setGroupLook } from './context';
  import type { ToggleGroupProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    variant = 'default',
    tone = 'default',
    size = 'md',
    value = $bindable(),
    onValueChange,
    ...rest
  }: ToggleGroupProps = $props();

  // The primitive types `value` per `type`; one wrapper forwards both arms.
  const single = $derived(
    rest as unknown as ToggleGroupPrimitive.RootProps & { type: 'single' },
  );

  setGroupLook(() => ({ variant, tone, size }));

  // A group that picks one option behaves like radios: clicking the picked
  // item would clear it, so the clear is dropped and the pick restored.
  let picked = '';
  $effect.pre(() => {
    if (typeof value === 'string' && value !== '') picked = value;
  });
  const change = (next: string | string[]) => {
    if (typeof next === 'string') {
      if (next === '' && picked !== '') {
        value = picked;
        return;
      }
      picked = next;
    }
    (onValueChange as ((v: string | string[]) => void) | undefined)?.(next);
  };
</script>

<ToggleGroupPrimitive.Root
  bind:ref
  data-slot="toggle-group"
  data-variant={variant}
  data-tone={tone}
  data-size={size}
  class={[
    // Every item is `shrink-0`, because a segmented control whose labels are
    // squeezed is unreadable. Without `flex-wrap` a group wider than its
    // column leaves the card: at 320px the theme choice overflowed by 95px
    // and its last option could not be reached.
    'group/toggle-group flex w-fit flex-wrap items-center',
    className,
  ]}
  bind:value={value as string}
  onValueChange={change}
  {...single}
/>
