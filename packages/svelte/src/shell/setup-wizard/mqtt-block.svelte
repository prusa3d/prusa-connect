<!--
  Sending the printers' state to an MQTT broker. The server runs no broker of
  its own.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { Button, Field, Input, SettingsRow, Switch } from '../../primitives';
  import { mqttUrlError } from './checks';
  import type { SetupDraft } from './setup-draft.svelte';

  let {
    draft,
    showErrors,
    guide,
  }: { draft: SetupDraft; showErrors: boolean; guide?: string | undefined } =
    $props();

  const urlError = $derived(showErrors ? mqttUrlError(draft) : '');
</script>

<div class="flex flex-col gap-5" data-part="mqtt">
  <SettingsRow
    layout="switch"
    label={t('setupWizard.mqtt.label')}
    description={t('setupWizard.mqtt.hint')}
  >
    {#snippet control()}
      <Switch
        aria-label={t('setupWizard.mqtt.label')}
        checked={draft.mqttOn}
        onCheckedChange={(next: boolean) => {
          draft.mqttOn = next;
        }}
      />
    {/snippet}
  </SettingsRow>
  {#if draft.mqttOn}
    <div
      class="flex flex-col gap-4 border-s-2 border-primary/40 ps-4"
      data-part="dependents"
    >
      <Field
        label={t('setupWizard.mqtt.url')}
        description={t('setupWizard.mqtt.urlHint')}
        error={urlError}
        control={Input}
        controlProps={{
          value: draft.mqttUrl,
          type: 'text',
          class: 'font-mono',
          placeholder: 'tcp://broker.local:1883',
          spellcheck: false,
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            draft.mqttUrl = event.currentTarget.value;
          },
        }}
      />
      <Field
        label={t('setupWizard.mqtt.username')}
        control={Input}
        controlProps={{
          value: draft.mqttUser,
          type: 'text',
          autocomplete: 'off',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            draft.mqttUser = event.currentTarget.value;
          },
        }}
      />
      <Field
        label={t('setupWizard.mqtt.password')}
        control={Input}
        controlProps={{
          value: draft.mqttPassword,
          type: 'password',
          autocomplete: 'off',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            draft.mqttPassword = event.currentTarget.value;
          },
        }}
      />
      <div
        class="flex flex-col gap-2 rounded-md bg-muted p-3 text-sm text-muted-foreground"
      >
        <p>{t('setupWizard.mqtt.namespace')}</p>
        {#if guide !== undefined}
          <Button variant="link" class="self-start">
            {#snippet child({ props }: { props: Record<string, unknown> })}
              <a
                {...props}
                href={guide}
                target="_blank"
                rel="noopener noreferrer"
              >
                {t('setupWizard.mqtt.topics')}
              </a>
            {/snippet}
          </Button>
        {/if}
      </div>
    </div>
  {/if}
</div>
