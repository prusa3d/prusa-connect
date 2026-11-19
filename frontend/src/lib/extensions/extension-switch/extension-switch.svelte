<!--
  One module's on/off switch. It draws what the server holds and moves once
  the server applied the change, as the marketplace switch does. Switching on
  a module that is not installed, or one installed without approval, opens
  the install question; only an approved module has its `enabled` written.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import { Switch } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import type { ModuleCatalogue } from '../../stores/module-catalogue.svelte';
  import type { AskingAct } from '../extension-dialog';
  import type { ExtensionRow } from '../rows';
  import { switchNotesOf, switchOf } from '../switch';

  let {
    row,
    catalogue,
    ask,
    notes = true,
  }: {
    row: ExtensionRow;
    catalogue: ModuleCatalogue;
    ask: (row: ExtensionRow, act: AskingAct, version?: string) => void;
    /**
     * Whether the switch draws its own words beside it. A listing row turns
     * them off and draws them in its body, where they can wrap.
     */
    notes?: boolean;
  } = $props();

  const t = translateFor(APP_NAMESPACE);

  const state = $derived(
    switchOf(
      row,
      catalogue.mayConfigure(row.kind),
      catalogue.modulesOn,
      catalogue.safeMode,
    ),
  );
  const words = $derived(switchNotesOf(row, state, t));
  const on = $derived(
    state.kind === 'toggle' || state.kind === 'locked' ? state.on : false,
  );
  const busy = $derived(
    catalogue.writing !== undefined &&
      catalogue.writing === (row.instance?.namespace ?? row.instance?.name),
  );

  const set = (next: boolean): void => {
    if (state.kind === 'install') ask(row, 'install');
    else if (state.kind === 'approve') ask(row, 'install', state.version);
    else if (state.kind === 'toggle' && row.instance !== undefined) {
      // A service module has a memory budget the person sees before it is
      // enabled; switching off needs no question.
      if (next && row.instance.status.memory !== undefined) ask(row, 'enable');
      else void catalogue.writeInstance(row.instance, { enabled: next });
    }
  };
</script>

{#if state.kind !== 'none'}
  <span
    class="flex min-w-0 flex-wrap items-center gap-2"
    data-part="extension-switch"
  >
    <Switch
      aria-label={t('app.settings.extensions.switch.label', {
        name: row.title,
      })}
      bind:checked={() => on, set}
      disabled={state.kind === 'locked' ||
        busy ||
        catalogue.acting !== undefined ||
        catalogue.switching}
    />
    {#if notes && words.observed !== undefined}
      <span class="text-sm text-muted-foreground" data-part="observed">
        {words.observed}
      </span>
    {/if}
    {#if notes && words.locked !== undefined}
      <span class="text-sm text-muted-foreground" data-part="locked">
        {words.locked}
      </span>
    {/if}
  </span>
{/if}
