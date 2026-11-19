<!--
  Whether a download is checked against a SHA-256 its source published. A
  local binary was never downloaded, so it has nothing to check.
-->
<script lang="ts">
  import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';

  import type { Translate } from '../../api';
  import type { DigestState } from '../../releases';

  let { digest, t }: { digest: DigestState; t: Translate } = $props();
</script>

{#if digest === 'verified'}
  <span
    class="inline-flex items-center gap-1 text-sm text-muted-foreground"
    data-digest="verified"
  >
    <ShieldCheckIcon class="size-3.5" aria-hidden="true" />{t(
      'slicer.digest.verified',
    )}
  </span>
{:else if digest === 'unverified'}
  <span
    class="inline-flex items-center gap-1 text-sm text-foreground"
    data-digest="unverified"
  >
    <ShieldAlertIcon class="size-3.5" aria-hidden="true" />{t(
      'slicer.digest.unverified',
    )}
  </span>
{:else}
  <span class="text-sm text-muted-foreground" data-digest="none"
    >{t('slicer.digest.none')}</span
  >
{/if}
