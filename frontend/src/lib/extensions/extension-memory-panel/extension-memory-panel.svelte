<!--
  The Memory tab of one service module, one pane: its last hour, then its
  budget and what the host does when it goes over. Every choice is written through
  `write`, which says whether the server took it.
-->
<script lang="ts">
  import type { ResourceInstance } from '@prusa/connect-ui/data';

  import type { InstanceChange } from '../../stores/module-catalogue.svelte';
  import type { ResourceMemoryHistory } from '../../stores/module-operations.svelte';
  import { ExtensionMemory } from '../extension-memory';
  import { type ModuleMemoryPool, memoryEstimate } from '../memory';
  import { MemoryHistory } from '../memory-history';
  import type { ResourceMemory } from '../stops';

  let {
    instance,
    memory,
    pool,
    locked,
    saving,
    fault,
    conflict,
    history,
    historyFault,
    missing,
    write,
  }: {
    instance: ResourceInstance;
    memory: ResourceMemory;
    /** Undefined when the person may not read the pool. */
    pool: ModuleMemoryPool | undefined;
    locked?: string | undefined;
    saving: boolean;
    /** What the last write of these choices failed with. */
    fault?: unknown;
    conflict: boolean;
    history?: ResourceMemoryHistory | undefined;
    historyFault?: unknown;
    missing: boolean;
    /** Writes a change to the instance; true when the server took it and the page is read again. */
    write: (changes: InstanceChange) => Promise<boolean>;
  } = $props();
</script>

<div class="flex flex-col gap-4" data-part="extension-memory-panel">
  <ExtensionMemory
    estimate={memoryEstimate({
      budgetBytes: memory.budget_bytes,
      source: memory.budget_source,
      pool,
      heldBytes: instance.enabled ? memory.budget_bytes : 0,
    })}
    usedBytes={memory.used_bytes}
    overBudget={memory.over_budget}
    nearBudget={memory.near_budget}
    onOverMemory={instance.on_over_memory ?? memory.on_over_memory}
    budgetMiB={instance.memory_mib}
    recommendedMiB={instance.module?.asked?.memory_mib}
    running={instance.enabled}
    {locked}
    {saving}
    onbudget={(mib: number) => write({ memory_mib: mib })}
    {fault}
    {conflict}
    onchange={(next: 'stop' | 'warn') => {
      void write({ on_over_memory: next });
    }}
    switchOff={{
      failures: memory.switch_off_failures,
      windowSeconds: memory.switch_off_window_seconds,
      source: memory.switch_off_source,
    }}
    onswitchoff={(
      next: { failures: number; window_seconds: number } | undefined,
    ) => write({ switch_off: next ?? null })}
  >
    <MemoryHistory {history} {memory} {missing} fault={historyFault} />
  </ExtensionMemory>
</div>
