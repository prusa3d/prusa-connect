<!--
  Where a module shows: each contribution it declares, with a switch to hide
  it. A change is written as the whole list of hidden entries.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import type { ResourceInstance } from '@prusa/connect-ui/data';
  import {
    Alert,
    AlertDescription,
    FaultAlert,
    Switch,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import { describeExtensionFault } from '../fault';
  import { type Contribution } from './contributions';

  let {
    instance,
    contributions,
    locked,
    saving,
    fault,
    conflict,
    onhide,
  }: {
    instance: ResourceInstance;
    contributions: readonly Contribution[];
    /** Why the choice cannot be changed here, already worded. */
    locked?: string | undefined;
    saving: boolean;
    fault?: unknown;
    conflict: boolean;
    /** Writes the list of hidden entries; true when the server took it. */
    onhide: (hidden: string[]) => Promise<boolean>;
  } = $props();

  const t = translateFor(APP_NAMESPACE);
  const hidden = $derived(instance.hidden_contributions ?? []);
  const moduleHides = $derived(instance.ui?.hidden ?? []);
</script>

<div class="flex flex-col gap-3" data-part="visibility">
  <p class="text-sm text-muted-foreground">
    {t('app.settings.extensions.visibility.note')}
  </p>
  {#if locked !== undefined}
    <p class="text-sm" data-part="visibility-locked">{locked}</p>
  {/if}
  <ul class="flex flex-col gap-2">
    {#each contributions as one (one.key)}
      <li class="flex items-center justify-between gap-3">
        <span class="flex min-w-0 flex-col break-all">
          <span class="text-sm">{one.title}</span>
          <span class="text-xs text-muted-foreground">
            {t(`app.settings.extensions.visibility.kind.${one.kind}`)}
          </span>
          {#if moduleHides.includes(one.key)}
            <span
              class="text-xs text-muted-foreground"
              data-part="hidden-by-module"
            >
              {t('app.settings.extensions.visibility.byModule')}
            </span>
          {/if}
        </span>
        <Switch
          aria-label={t('app.settings.extensions.visibility.label', {
            name: one.title,
          })}
          checked={!hidden.includes(one.key)}
          disabled={locked !== undefined || saving}
          onCheckedChange={(shown: boolean) => {
            void onhide(
              shown
                ? hidden.filter((entry) => entry !== one.key)
                : [...hidden, one.key],
            );
          }}
        />
      </li>
    {/each}
  </ul>
  {#if conflict}
    <Alert variant="warning">
      <AlertDescription>
        {t('app.settings.extensions.write.conflict')}
      </AlertDescription>
    </Alert>
  {:else if fault !== undefined}
    <FaultAlert fault={describeExtensionFault(fault, t)} />
  {/if}
</div>
