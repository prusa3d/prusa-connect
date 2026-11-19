<!--
  Whether the server keeps the printers' history: the temperature charts, the
  events, the command log and the finished transfers. The list of jobs is
  kept either way.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import {
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    SettingsRow,
    Switch,
  } from '../../primitives';
  import type { SetupDraft } from './setup-draft.svelte';

  let { draft }: { draft: SetupDraft } = $props();

  const id = $props.id();

  const retentions = $derived([
    { value: '48h', label: t('setupWizard.history.days2') },
    { value: '168h', label: t('setupWizard.history.days7') },
    { value: '720h', label: t('setupWizard.history.days30') },
    { value: '0', label: t('setupWizard.history.forever') },
  ]);
</script>

<div class="flex flex-col gap-4" data-part="history">
  <SettingsRow
    layout="switch"
    label={t('setupWizard.history.label')}
    description={t('setupWizard.history.hint')}
  >
    {#snippet control()}
      <Switch
        aria-label={t('setupWizard.history.label')}
        checked={draft.historyOn}
        onCheckedChange={(next: boolean) => {
          draft.historyOn = next;
        }}
      />
    {/snippet}
  </SettingsRow>
  {#if draft.historyOn}
    <div
      class="flex flex-col gap-2 border-s-2 border-primary/40 ps-4"
      data-part="dependents"
    >
      <Label for={`${id}-retention`}>
        {t('setupWizard.history.retention')}
      </Label>
      <Select
        value={draft.historyRetention}
        items={retentions}
        onValueChange={(next: string) => {
          draft.historyRetention = next;
        }}
      >
        <SelectTrigger id={`${id}-retention`} class="w-full">
          <SelectValue placeholder={t('setupWizard.history.retention')} />
        </SelectTrigger>
        <SelectContent>
          {#each retentions as retention (retention.value)}
            <SelectItem value={retention.value}>{retention.label}</SelectItem>
          {/each}
        </SelectContent>
      </Select>
      <p class="text-sm text-muted-foreground">
        {t('setupWizard.history.retentionHint')}
      </p>
    </div>
  {/if}
</div>
