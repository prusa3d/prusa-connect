<!--
  The marketplace in a few words, for the head of the listing: on, off, not
  answering, or switched off in the configuration. It is what stays on screen
  while the marketplace settings are closed, so a marketplace that does not
  answer is seen without opening them. When the rows are a copy kept from an
  earlier read, the badge says when that read was, never a plain "on".
-->
<script lang="ts">
  import StoreIcon from '@lucide/svelte/icons/store';
  import { formatDateTime, translateFor } from '@prusa/connect-ui/i18n';
  import { Badge } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import type { ModuleMarketplace } from '../../stores/module-catalogue.svelte';

  let { marketplace }: { marketplace: ModuleMarketplace | undefined } =
    $props();

  const t = translateFor(APP_NAMESPACE);

  type Shown = 'none' | 'off' | 'unreachable' | 'stale' | 'on';

  const shown = $derived.by((): Shown => {
    if (marketplace === undefined) return 'none';
    if (!marketplace.enabled) return 'off';
    if (marketplace.reachable) return 'on';

    return marketplace.stale && marketplace.read_at !== undefined
      ? 'stale'
      : 'unreachable';
  });

  // Every key and variant is written out rather than assembled from a value.
  const KEY = {
    none: 'app.settings.extensions.marketplace.state.none',
    off: 'app.settings.extensions.marketplace.state.off',
    unreachable: 'app.settings.extensions.marketplace.state.unreachable',
    stale: 'app.settings.extensions.marketplace.state.stale',
    on: 'app.settings.extensions.marketplace.state.on',
  } as const;

  const VARIANT = {
    none: 'outline',
    off: 'outline',
    unreachable: 'warning',
    stale: 'warning',
    on: 'success',
  } as const;

  const when = $derived(
    marketplace?.read_at === undefined
      ? ''
      : formatDateTime(marketplace.read_at),
  );
</script>

<Badge
  variant={VARIANT[shown]}
  size="text"
  data-part="marketplace-state"
  data-state={shown}
>
  <StoreIcon aria-hidden="true" />
  {t(KEY[shown], { when })}
</Badge>
