<!-- A field picker over a listing that holds its own visible set, for the
     field-picker tests. The picker is controlled, so a tick with nothing
     behind it cannot move. -->
<script lang="ts">
  import { untrack } from 'svelte';

  import type { ListField, VisibleFields } from '../primitives/field-picker';
  import { FieldPicker } from '../primitives/field-picker';
  import type { ControlSize } from '../primitives/control-trigger/types';

  let {
    fields,
    size = 'sm',
    initial,
  }: {
    fields?: ListField[];
    size?: ControlSize;
    /** What the listing holds at first, when not its defaults. */
    initial?: string[];
  } = $props();

  // The probe seeds itself once. `untrack` says so, where a bare read warns.
  const columns: ListField[] = untrack(() => fields) ?? [
    { id: 'file', label: 'File', required: true },
    { id: 'printer', label: 'Printer' },
    { id: 'state', label: 'State' },
  ];

  const defaults = (): string[] =>
    columns
      .filter((field) => field.required === true || !field.hiddenByDefault)
      .map((field) => field.id);

  let visible = $state(untrack(() => initial) ?? defaults());

  // Not named `state`: a top-level binding of that name in a component
  // shadows the runtime helper `$state` compiles to.
  const fieldState: VisibleFields = {
    get visible() {
      return visible;
    },
    onToggle: (id: string) => {
      visible = visible.includes(id)
        ? visible.filter((held) => held !== id)
        : [...visible, id];
    },
    onReset: () => {
      visible = defaults();
    },
  };
</script>

<FieldPicker fields={columns} state={fieldState} {size} />
<p data-testid="visible">{visible.join(' ')}</p>
