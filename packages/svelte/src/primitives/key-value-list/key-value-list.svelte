<script lang="ts">
  import { t } from '../../i18n';
  import CopyButton from '../copy-button/copy-button.svelte';
  import type { KeyValueListProps } from './types';

  let {
    rows,
    absentText,
    labelWidth,
    class: className,
  }: KeyValueListProps = $props();

  // A caller may name its own words.
  const absentWords = $derived(absentText ?? t('primitives.absent'));
</script>

<!-- Below the `sm` container width the name sits above its reading. -->
<dl
  data-slot="key-value-list"
  class={[
    'text-body @container/kv flex flex-col gap-2 [--kv-label:calc(var(--spacing)*40)]',
    className,
  ]}
  style:--kv-label={labelWidth}
>
  {#each rows as row (row.id)}
    <div
      data-slot="key-value-row"
      class="flex flex-col gap-0.5 @sm/kv:flex-row @sm/kv:items-center @sm/kv:gap-4"
    >
      <dt class="shrink-0 text-muted-foreground @sm/kv:w-(--kv-label)">
        {row.label}
      </dt>
      <dd class="flex min-w-0 items-center gap-1">
        {#if row.value}
          <!-- A mono reading is an identifier. -->
          <span
            data-slot="key-value-value"
            data-emphasis={row.emphasis}
            title={row.mono && !row.wrap ? row.value : undefined}
            class={[
              'min-w-0',
              row.wrap && 'font-mono break-all whitespace-pre-line',
              !row.wrap && row.mono && 'truncate font-mono',
              !row.wrap && !row.mono && 'break-words tabular-nums',
              row.emphasis === 'strong' && 'text-base font-semibold',
              row.emphasis === 'muted' && 'text-xs text-muted-foreground',
            ]}>{row.value}</span
          >
          {#if row.copyable}
            <CopyButton
              value={row.value}
              label={t('primitives.copyNamed', { name: row.label })}
            />
          {/if}
        {:else}
          <span class="text-muted-foreground italic">{absentWords}</span>
        {/if}
      </dd>
    </div>
  {/each}
</dl>
