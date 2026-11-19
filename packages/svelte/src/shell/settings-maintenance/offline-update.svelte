<script lang="ts">
  import { formatBytes, t } from '../../i18n';
  import {
    Badge,
    Button,
    FileDropzone,
    Progress,
    SettingsPane,
  } from '../../primitives';
  import type { MaintenanceSource } from './types';

  let { source }: { source: MaintenanceSource } = $props();

  let selected = $state<File | undefined>(undefined);

  const working = $derived(source.busy !== undefined);
  const uploading = $derived(source.busy === 'offline-upload');

  const artifactKind = (kind: 'core' | 'extension'): string =>
    kind === 'core'
      ? t('settingsMaintenance.offline.artifacts.core')
      : t('settingsMaintenance.offline.artifacts.extension');

  const impactOutcome = (
    outcome: 'compatible' | 'disabled' | 'replaced' | 'required',
  ): string => {
    if (outcome === 'compatible')
      return t('settingsMaintenance.offline.impacts.compatible');
    if (outcome === 'disabled')
      return t('settingsMaintenance.offline.impacts.disabled');
    if (outcome === 'replaced')
      return t('settingsMaintenance.offline.impacts.replaced');

    return t('settingsMaintenance.offline.impacts.required');
  };

  const upload = (): void => {
    if (selected === undefined) return;
    const bundle = selected;
    selected = undefined;
    void source.uploadOfflineBundle(bundle);
  };
</script>

{#snippet actions()}
  <Button
    variant={source.offlineBundle ? 'secondary' : 'default'}
    disabled={working || selected === undefined}
    onclick={upload}
  >
    {uploading
      ? t('settingsMaintenance.offline.bundle.uploading')
      : t('settingsMaintenance.offline.bundle.upload')}
  </Button>
  {#if source.offlineBundle}
    <Button
      disabled={working || source.upgrade?.rehearsal !== undefined}
      onclick={() => void source.prepare()}
    >
      {t('settingsMaintenance.action.prepare')}
    </Button>
  {/if}
{/snippet}

<SettingsPane
  class="w-full"
  title={t('settingsMaintenance.offline.heading')}
  actions={source.deliveryAvailable ? actions : undefined}
>
  <div
    class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <p class="text-body text-muted-foreground">
      {t('settingsMaintenance.offline.description')}
    </p>

    {#if !source.deliveryAvailable}
      <p class="text-body text-muted-foreground">
        {t('settingsMaintenance.delivery.unavailable')}
      </p>
    {:else}
      <div class="flex flex-col gap-2">
        <FileDropzone
          compact
          label={t('settingsMaintenance.offline.bundle.label')}
          disabled={working}
          onFiles={(files: File[]) => {
            selected = files[0];
          }}
        />
        {#if selected !== undefined}
          <p class="text-body font-medium break-all">{selected.name}</p>
        {/if}
        <p class="text-caption text-muted-foreground">
          {t('settingsMaintenance.offline.bundle.help')}
        </p>
      </div>

      {#if source.offlineUploadProgress !== undefined}
        <div aria-live="polite" aria-busy={uploading}>
          <div
            class="mb-1 flex justify-between text-caption text-muted-foreground"
          >
            <span>{t('settingsMaintenance.offline.progress')}</span>
            <span>{source.offlineUploadProgress}%</span>
          </div>
          <Progress
            value={source.offlineUploadProgress}
            aria-label={t('settingsMaintenance.offline.progress')}
          />
        </div>
      {/if}

      {#if source.offlineBundle}
        <div class="flex flex-col gap-section-gap" data-part="offline-bundle">
          <div>
            <h3 class="text-body font-medium">{source.offlineBundle.name}</h3>
            <p class="text-caption text-muted-foreground">
              {t('settingsMaintenance.offline.bundle.version', {
                version: source.offlineBundle.version,
              })}
            </p>
          </div>

          <div>
            <h4 class="text-body font-medium">
              {t('settingsMaintenance.offline.artifacts.heading')}
            </h4>
            <ul class="mt-2 flex flex-col divide-y divide-border">
              {#each source.offlineBundle.artifacts as artifact (`${artifact.kind}:${artifact.name}`)}
                <li
                  class="text-body flex flex-wrap items-center justify-between gap-2 py-2"
                >
                  <span>
                    <Badge>{artifactKind(artifact.kind)}</Badge>
                    <span class="ms-2 font-mono">{artifact.name}</span>
                    <span class="text-muted-foreground">
                      {artifact.version}
                    </span>
                  </span>
                  <span class="text-caption text-muted-foreground">
                    {formatBytes(artifact.size)}
                  </span>
                </li>
              {/each}
            </ul>
          </div>

          <div>
            <h4 class="text-body font-medium">
              {t('settingsMaintenance.offline.impacts.heading')}
            </h4>
            {#if source.offlineBundle.impacts.length === 0}
              <p class="text-body mt-1 text-muted-foreground">
                {t('settingsMaintenance.offline.impacts.empty')}
              </p>
            {:else}
              <ul class="mt-2 flex flex-col gap-2">
                {#each source.offlineBundle.impacts as impact (impact.extension)}
                  <li class="text-body">
                    <span class="font-mono">{impact.extension}</span>
                    <Badge
                      class="ms-2"
                      variant={impact.outcome === 'compatible'
                        ? 'success'
                        : 'warning'}
                    >
                      {impactOutcome(impact.outcome)}
                    </Badge>
                    {#if impact.reason}
                      <p class="mt-1 text-caption text-muted-foreground">
                        {impact.reason}
                      </p>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </div>
      {/if}
    {/if}
  </div>
</SettingsPane>
