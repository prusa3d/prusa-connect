<!--
  A settings pane under the page's title. The rail lists every pane, as rows
  when it is open and in a menu when it is collapsed, so the page names none of
  the others.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { PageHeader } from '../../primitives';
  import type { SettingsScreenProps } from './types';

  let { title, children }: SettingsScreenProps = $props();
</script>

<div data-section-title={title} class="flex flex-col gap-4">
  {#if title !== undefined}
    <h1
      data-slot="settings-breadcrumb"
      class="flex flex-wrap items-baseline gap-form-gap text-lg font-semibold"
    >
      <span class="font-normal text-muted-foreground"
        >{t('settings.title')}</span
      >
      <span aria-hidden="true" class="font-normal text-muted-foreground"
        >&gt;</span
      >
      <span>{title}</span>
    </h1>
  {:else}
    <PageHeader title={t('settings.title')} />
  {/if}

  {@render children?.()}
</div>

<style lang="postcss">
  @reference '../../styles/index.css';

  [data-section-title] > :global(div > [data-slot='settings-section-title']) {
    @apply hidden;
  }
</style>
