<!--
  What a person can do with one module of the listing: the one act its state
  calls for as a button, and the rest in the row's menu.

  Installing, updating and removing are asked through the page's one dialog,
  which says what the person agrees to. A person the server would refuse is
  not offered the act: install and update are left out, and remove stays in
  the menu disabled, so the menu has the same shape on every row. For a
  module config.yaml declares, Remove is disabled the same way: the server
  never writes that file, so it refuses the removal.
-->
<script lang="ts">
  import DownloadIcon from '@lucide/svelte/icons/download';
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
  import PanelTopIcon from '@lucide/svelte/icons/panel-top';
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    Button,
    DropdownMenuItem,
    RowActions,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import type { ModuleCatalogue } from '../../stores/module-catalogue.svelte';
  import { extensionDetailHref } from '../../routes';
  import type { AskingAct } from '../extension-dialog';
  import { ExtensionSwitch } from '../extension-switch';
  import type { ExtensionRow } from '../rows';
  import { removableOf, switchOf } from '../switch';

  let {
    row,
    catalogue,
    go,
    ask,
    notes = true,
    showSwitch = true,
  }: {
    showSwitch?: boolean;
    row: ExtensionRow;
    catalogue: ModuleCatalogue;
    /** Moves the app to one of its own addresses, without a reload. */
    go: (href: string) => void;
    /** Opens the page's dialog for this row. */
    ask: (row: ExtensionRow, act: AskingAct, version?: string) => void;
    /** Whether the switch draws its words beside it; see ExtensionSwitch. */
    notes?: boolean;
  } = $props();

  const t = translateFor(APP_NAMESPACE);

  const updateLabel = $derived(t('app.settings.extensions.offer.updateShort'));

  const busy = $derived(catalogue.acting === row.module);
  // One act at a time on the whole server, so every row waits for the one
  // running and for the marketplace switch.
  const blocked = $derived(
    catalogue.acting !== undefined || catalogue.switching,
  );
  // Unknown is left to the server, which decides again on every request.
  const allowed = $derived(catalogue.mayConfigure(row.kind) !== false);
  // A row with a switch is installed through it, so it has no Install button.
  const switched = $derived(
    switchOf(
      row,
      catalogue.mayConfigure(row.kind),
      catalogue.modulesOn,
      catalogue.safeMode,
    ).kind !== 'none',
  );
  const installable = $derived(row.offer !== undefined && allowed);
  const removable = $derived(
    removableOf(row, catalogue.mayConfigure(row.kind)),
  );

  // The marketplace's page is another site, so it opens in a tab of its own;
  // a page the module adds is this app's and opens in place.
  const openOutside = (href: string): void => {
    window.open(href, '_blank', 'noopener,noreferrer');
  };
</script>

<div class="flex max-w-full min-w-0 flex-wrap items-center justify-end gap-2">
  {#if showSwitch}<ExtensionSwitch {row} {catalogue} {ask} {notes} />{/if}
  {#if installable && row.state === 'available' && !switched}
    <Button
      size="sm"
      class="min-h-7 max-w-full min-w-0 py-1.5"
      disabled={blocked}
      data-part="install"
      onclick={() => {
        ask(row, 'install');
      }}
    >
      <DownloadIcon aria-hidden="true" />
      <span class="min-w-0 text-start break-all whitespace-normal">
        {busy
          ? t('app.settings.extensions.offer.working')
          : t('app.settings.extensions.offer.install')}
      </span>
    </Button>
  {:else if installable && row.state === 'update'}
    <Button
      size="sm"
      class="min-h-7 max-w-full min-w-0 py-1.5"
      disabled={blocked}
      data-part="update"
      onclick={() => {
        ask(row, 'update');
      }}
    >
      <RefreshCwIcon aria-hidden="true" />
      <span class="min-w-0 text-start break-all whitespace-normal">
        {busy ? t('app.settings.extensions.offer.working') : updateLabel}
      </span>
    </Button>
  {/if}

  <RowActions name={row.title}>
    {#if row.instance !== undefined && row.namespace}
      <DropdownMenuItem
        onSelect={() => {
          if (row.namespace) go(extensionDetailHref(row.namespace));
        }}
      >
        <ArrowRightIcon aria-hidden="true" />
        {t('app.settings.extensions.open')}
      </DropdownMenuItem>
    {/if}
    {#each row.pages as page (page.href)}
      <DropdownMenuItem
        onSelect={() => {
          go(page.href);
        }}
      >
        <PanelTopIcon aria-hidden="true" />
        {t('app.settings.extensions.openPage', { page: page.title })}
      </DropdownMenuItem>
    {/each}
    {#if row.pageUrl && (row.instance === undefined || !row.namespace)}
      <DropdownMenuItem
        onSelect={() => {
          if (row.pageUrl) openOutside(row.pageUrl);
        }}
      >
        <ExternalLinkIcon aria-hidden="true" />
        {t('app.settings.extensions.offer.open')}
      </DropdownMenuItem>
    {/if}
    <DropdownMenuItem
      variant="destructive"
      disabled={blocked || !removable}
      data-part="remove"
      onSelect={() => {
        ask(row, 'remove');
      }}
    >
      <Trash2Icon aria-hidden="true" />
      {t('app.settings.extensions.offer.remove')}
    </DropdownMenuItem>
  </RowActions>
</div>
