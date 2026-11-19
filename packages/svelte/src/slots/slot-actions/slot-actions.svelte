<!-- The action menu of an action slot, such as a printer page's header or a
     file's row: one kebab, drawn only once a manifest declares an entry or the
     screen hands its own items, running a module's registration and reporting
     a failure as a toast rather than losing it in the console. -->
<script lang="ts">
  import { onMount } from 'svelte';

  import { t } from '../../i18n';
  import {
    Button,
    DropdownMenuItem,
    DropdownMenuSeparator,
    RowActions,
    toast,
  } from '../../primitives';
  import { getSlotContext } from '../context';
  import { matchesDevice } from '../device-selector';
  import { matchesFileRef } from '../file-types';
  import { moduleIcon } from '../navigation';
  import type { SlotActionsProps } from './types';

  // Kept whole rather than destructured: the subject's prop sits on only one
  // member of the `ActionOutletTarget` union, and reading it needs the 'in'
  // narrowing below.
  const props: SlotActionsProps = $props();
  const context = getSlotContext();
  const registryInUse = $derived(props.registry ?? context?.registry);
  const loaderInUse = $derived(props.loader ?? context?.loader);
  const subject = $derived(
    'deviceId' in props ? props.deviceId : props.fileRef,
  );
  // An entry that names file types is in the menu of a file of one of them
  // only (ADR-20261027-wvj rule 17). An entry with a devices selector is in
  // the menu of a device that meets it only (ADR-20261110-5np).
  const entries = $derived(
    (registryInUse?.declaredActions(props.name) ?? []).filter(
      (entry) =>
        (!('fileRef' in props) ||
          matchesFileRef(entry.fileTypes, props.fileRef)) &&
        (!('deviceId' in props) || matchesDevice(entry.devices, props.device)),
    ),
  );
  // The page of the subject draws a primary entry as a button, and its menu
  // holds the rest.
  const promoted = $derived(
    props.buttons === true ? entries.filter((entry) => entry.primary) : [],
  );
  const listed = $derived(
    props.buttons === true
      ? entries.filter((entry) => !entry.primary)
      : entries,
  );
  // Keyed by namespace and id rather than by row: two modules could declare
  // the same id, and only one of them may be running at a time.
  let running = $state<ReadonlySet<string>>(new Set());

  // Activation is lazy (ADR-20260911-4p6 rule 25): the bundle loads when its first
  // slot first renders, and the loader runs once however many outlets ask.
  onMount(() => {
    void loaderInUse?.activate();
  });

  const keyOf = (namespace: string, id: string): string => `${namespace}.${id}`;

  const select = async (
    namespace: string,
    id: string,
    title: string,
  ): Promise<void> => {
    const key = keyOf(namespace, id);
    // The run acts on the subject the person chose the entry for, even when
    // the page shows another subject before the bundle has loaded.
    const target = subject;
    running = new Set([...running, key]);
    try {
      // The bundle may still be loading when the menu opens: wait for it
      // before looking for the run function the declaration promised.
      await loaderInUse?.activate();
      const row = registryInUse
        ?.actions(props.name)
        .find((action) => action.namespace === namespace && action.id === id);
      if (row === undefined) {
        toast.error(t('slotActions.missing', { module: namespace }));

        return;
      }

      await row.run(target);
    } catch (thrown) {
      // A synchronous throw and a rejection both land here: `await` turns
      // the former into the latter before `catch` sees it.
      toast.error(title, {
        description: thrown instanceof Error ? thrown.message : String(thrown),
      });
    } finally {
      running = new Set([...running].filter((held) => held !== key));
    }
  };
</script>

{#each promoted as entry (keyOf(entry.namespace, entry.id))}
  {@const Icon = entry.icon === undefined ? undefined : moduleIcon(entry.icon)}
  <Button
    size="lg"
    data-slot={props.name}
    data-module={entry.namespace}
    data-section={entry.id}
    variant={entry.destructive ? 'destructive' : 'default'}
    disabled={running.has(keyOf(entry.namespace, entry.id))}
    onclick={() => {
      void select(entry.namespace, entry.id, entry.title);
    }}
  >
    {#if Icon}<Icon aria-hidden="true" />{/if}
    {entry.title}
  </Button>
{/each}

{#if listed.length > 0 || props.items !== undefined}
  <RowActions
    name={props.label}
    class={[props.class, listed.length === 0 && props.itemsOnlyClass]}
  >
    {#if props.items}
      {@render props.items()}
      {#if listed.length > 0}<DropdownMenuSeparator />{/if}
    {/if}
    {#each listed as entry (keyOf(entry.namespace, entry.id))}
      {@const Icon =
        entry.icon === undefined ? undefined : moduleIcon(entry.icon)}
      <DropdownMenuItem
        data-slot={props.name}
        data-module={entry.namespace}
        data-section={entry.id}
        variant={entry.destructive ? 'destructive' : 'default'}
        disabled={running.has(keyOf(entry.namespace, entry.id))}
        onSelect={() => {
          void select(entry.namespace, entry.id, entry.title);
        }}
      >
        {#if Icon}<Icon aria-hidden="true" />{/if}
        {entry.title}
      </DropdownMenuItem>
    {/each}
  </RowActions>
{/if}
