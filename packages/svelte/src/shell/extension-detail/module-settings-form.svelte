<!--
  The settings a module declares, one field each, generated from its
  manifest's options. A value is checked against its type here because the
  server checks it only when the module starts. A stored secret is never
  shown: its field starts empty and saving it untouched keeps it.
-->
<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';

  import { describeFault, t } from '../../i18n';
  import {
    Button,
    FaultAlert,
    Field,
    Input,
    PasswordInput,
    SettingsPane,
    SettingsRow,
    Switch,
    Textarea,
  } from '../../primitives';
  import {
    configOf,
    controlOf,
    draftOf,
    type FieldError,
    type ResourceOption,
  } from './module-settings';
  import type { ModuleSettingsFormProps } from './types';

  let {
    options,
    config,
    locked,
    saving,
    fault,
    conflict = false,
    onsave,
    onreload,
    ondirty,
  }: ModuleSettingsFormProps = $props();

  const start = $derived(draftOf(options, config));
  let draft = $state<Record<string, string | boolean>>({});
  const kept = new SvelteSet<string>();
  let errors = $state<Readonly<Record<string, FieldError>>>({});
  let dirty = $state(false);
  let saved = $state(false);
  let dismissed = $state(false);

  $effect(() => {
    ondirty?.(dirty);
  });

  // Until the person edits, the form follows what the server holds.
  $effect.pre(() => {
    if (dirty) return;
    draft = { ...start.draft };
    kept.clear();
    for (const key of start.stored) kept.add(key);
  });

  const edit = (key: string, value: string | boolean): void => {
    draft[key] = value;
    dirty = true;
    saved = false;
  };

  const clear = (key: string): void => {
    kept.delete(key);
    dirty = true;
  };

  // The edits stay until the save is applied: a refused one is re-read, and
  // the re-read must not replace what the person typed.
  const save = async (event: SubmitEvent): Promise<void> => {
    event.preventDefault();
    const result = configOf(options, draft, kept, config);
    errors = result.errors;
    if (Object.keys(result.errors).length > 0) return;
    dismissed = false;
    saved = false;
    if (await onsave(result.config)) {
      dirty = false;
      saved = true;
    }
  };

  // A manifest names a key, not a title: `event_wakeups` reads "Event wakeups".
  const titleOf = (key: string): string => {
    const words = key.replaceAll(/[_-]+/gu, ' ').trim();
    return words.charAt(0).toUpperCase() + words.slice(1);
  };

  const labelOf = (option: ResourceOption, stored: boolean): string =>
    option.required === true && option.default === undefined && !stored
      ? t('extensionDetail.settings.requiredLabel', {
          name: titleOf(option.key),
        })
      : titleOf(option.key);

  const MESSAGE = $derived<Record<FieldError, string>>({
    required: t('extensionDetail.settings.error.required'),
    integer: t('extensionDetail.settings.error.integer'),
    reserved: t('extensionDetail.settings.error.reserved'),
    number: t('extensionDetail.settings.error.number'),
    url: t('extensionDetail.settings.error.url'),
    list: t('extensionDetail.settings.error.list'),
    object: t('extensionDetail.settings.error.object'),
  });

  const errorOf = (key: string): string => {
    const error = errors[key];
    return error === undefined ? '' : MESSAGE[error];
  };
</script>

<form onsubmit={save} novalidate data-part="module-settings">
  <SettingsPane
    class="w-full"
    title={t('extensionDetail.settings.title')}
    message={saved ? t('extensionDetail.settings.saved') : undefined}
  >
    <div
      class="flex max-w-3xl flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
    >
      {#each options as option (option.key)}
        {@const control = controlOf(option)}
        {#if control === 'switch'}
          <SettingsRow
            layout="switch"
            label={titleOf(option.key)}
            description={option.summary}
          >
            {#snippet control()}
              <Switch
                aria-label={titleOf(option.key)}
                checked={draft[option.key] === true}
                disabled={locked !== undefined}
                onCheckedChange={(next: boolean) => {
                  edit(option.key, next);
                }}
              />
            {/snippet}
          </SettingsRow>
        {:else if control === 'json'}
          <Field
            label={labelOf(option, false)}
            description={option.summary}
            error={errorOf(option.key)}
            control={Textarea}
            controlProps={{
              value: String(draft[option.key] ?? ''),
              placeholder: option.default ?? '',
              readonly: locked !== undefined,
              oninput: (e: Event) => {
                edit(
                  option.key,
                  (e.currentTarget as HTMLTextAreaElement).value,
                );
              },
            }}
          />
        {:else}
          <Field
            label={labelOf(option, kept.has(option.key))}
            error={errorOf(option.key)}
            description={control === 'secret' && kept.has(option.key)
              ? t('extensionDetail.settings.secretStored')
              : option.summary}
            control={control === 'secret' ? PasswordInput : Input}
            controlProps={{
              label:
                control === 'secret'
                  ? labelOf(option, kept.has(option.key))
                  : undefined,
              type: control === 'secret' ? 'password' : 'text',
              // A number input reports '' for text that is not a number, which would hide the error.
              inputmode: control === 'number' ? 'decimal' : undefined,
              value: String(draft[option.key] ?? ''),
              placeholder: option.default ?? '',
              readonly: locked !== undefined,
              autocomplete: control === 'secret' ? 'new-password' : undefined,
              oninput: (e: Event) => {
                edit(option.key, (e.currentTarget as HTMLInputElement).value);
              },
            }}
          />
          {#if control === 'secret' && kept.has(option.key) && locked === undefined}
            <div>
              <Button
                variant="ghost"
                size="sm"
                type="button"
                onclick={() => {
                  clear(option.key);
                }}
              >
                {t('extensionDetail.settings.secretClear')}
              </Button>
            </div>
          {/if}
        {/if}
      {/each}
    </div>

    {#snippet actions()}
      {#if locked !== undefined}
        <p class="text-sm text-muted-foreground" data-part="locked">{locked}</p>
      {:else}
        <Button type="submit" disabled={saving}>
          {t('extensionDetail.settings.save')}
        </Button>
      {/if}
    {/snippet}
  </SettingsPane>

  {#if conflict && !dismissed}
    <p class="text-sm" data-part="conflict">
      {t('extensionDetail.settings.conflict')}
      <Button
        variant="secondary"
        size="sm"
        type="button"
        onclick={() => {
          dirty = false;
          dismissed = true;
          onreload();
        }}
      >
        {t('extensionDetail.settings.reload')}
      </Button>
    </p>
  {/if}
  <FaultAlert fault={fault === undefined ? undefined : describeFault(fault)} />
</form>
