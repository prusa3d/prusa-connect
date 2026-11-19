<!--
  The first-run wizard (ADR-20261015-ejz). Its code and its texts in every language
  are a chunk of their own, loaded only while the server has not been set up,
  so a server that has been never sends them to a browser again.

  A chunk that does not load is most often one a newer build of the server
  replaced after this page opened. The page then says so and offers a reload,
  which fetches the new build.
-->
<script lang="ts">
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
  import { translateFor } from '@prusa/connect-ui/i18n';
  import { Alert, Button, EmptyState } from '@prusa/connect-ui/primitives';
  import type { SetupWizardProps } from '@prusa/connect-ui/setup-wizard';

  import { APP_NAMESPACE } from './i18n';

  const t = translateFor(APP_NAMESPACE);

  const loadChunk = () => import('@prusa/connect-ui/setup-wizard');

  let {
    load = loadChunk,
    ...wizard
  }: SetupWizardProps & {
    /** Loads the library's wizard chunk. A test hands one that fails. */
    load?: typeof loadChunk;
  } = $props();

  // svelte-ignore state_referenced_locally
  const chunk = load();
</script>

{#await chunk then lib}
  <lib.SetupWizard {...wizard} />
{:catch}
  <div
    class="flex min-h-svh items-center justify-center bg-background p-4 not-dark:bg-shell-canvas"
    data-part="setup-wizard-unloaded"
  >
    <Alert variant="destructive" class="max-w-xl">
      <EmptyState
        class="col-start-2"
        icon={RefreshCwIcon}
        headingLevel="h1"
        heading={t('app.setupWizard.unloaded.heading')}
        description={t('app.setupWizard.unloaded.note')}
      >
        {#snippet action()}
          <Button
            onclick={() => {
              window.location.reload();
            }}
          >
            {t('app.setupWizard.unloaded.reload')}
          </Button>
        {/snippet}
      </EmptyState>
    </Alert>
  </div>
{/await}
