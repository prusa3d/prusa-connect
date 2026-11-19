<!-- One installed module: its state and switch, and a tab for each thing it carries. -->
<script lang="ts">
  import { t } from '../../i18n';
  import {
    Badge,
    DetailHeader,
    DetailTabs,
    FaultAlert,
    PageTrail,
  } from '../../primitives';
  import type { ExtensionDetailScreenProps } from './types';

  let {
    title,
    version,
    source,
    module,
    observed,
    result,
    control,
    actions,
    tabs,
    tab,
    onTabChange,
    extensionsHref,
    loading = false,
  }: ExtensionDetailScreenProps = $props();

  const VARIANT = {
    running: 'success',
    degraded: 'warning',
    starting: 'outline',
    retrying: 'warning',
    stopped: 'outline',
    'switched-off': 'destructive',
    failed: 'destructive',
  } as const;

  const LABEL = $derived({
    running: t('extensionDetail.observed.running'),
    degraded: t('extensionDetail.observed.degraded'),
    starting: t('extensionDetail.observed.starting'),
    retrying: t('extensionDetail.observed.retrying'),
    stopped: t('extensionDetail.observed.stopped'),
    'switched-off': t('extensionDetail.observed.switched-off'),
    failed: t('extensionDetail.observed.failed'),
  });
</script>

<!-- The header, the strip and every tab's panes share the page width, as
     the other settings screens do; the text inside a pane stops at
     `max-w-3xl`. -->
<div class="flex w-full flex-col gap-section-gap">
  <PageTrail
    trail={[{ label: t('extensionDetail.extensions'), href: extensionsHref }]}
    page={title}
  />
  <DetailHeader {title} {loading} {actions} {titleAside} {meta} />
  <DetailTabs {tabs} active={tab} onChange={onTabChange} segmented />
</div>

{#snippet titleAside()}
  <Badge variant={VARIANT[observed]} data-part="observed">
    {LABEL[observed]}
  </Badge>
{/snippet}

{#snippet meta()}
  <p class="text-sm text-muted-foreground" data-part="meta">
    {[version, source].filter((part) => part !== '').join(' · ')}
  </p>
  {#if module}
    <p class="text-xs text-muted-foreground" data-part="module">
      {t('extensionDetail.module')}
      <code class="font-mono break-all">{module}</code>
    </p>
  {/if}
  {#if result}
    <FaultAlert fault={{ code: undefined, title: result, detail: undefined }} />
  {/if}
  {@render control?.()}
{/snippet}
