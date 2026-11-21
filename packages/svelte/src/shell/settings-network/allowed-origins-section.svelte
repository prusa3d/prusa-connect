<!--
  The sites whose pages may call this server from a browser. An origin comes
  from the configuration file, from this list or from an extension that is
  switched on, and only one of this list is removed here. The list shows what
  the server holds, and a change is drawn once the server has applied it.
-->
<script lang="ts">
  import PlusIcon from '@lucide/svelte/icons/plus';

  import type { AllowedOrigin } from '../../data';
  import { describeFault, t } from '../../i18n';
  import {
    Button,
    FaultAlert,
    Field,
    Input,
    RowList,
    Skeleton,
  } from '../../primitives';
  import type { AllowedOriginsSectionProps } from './types';

  let { source, extensionHref }: AllowedOriginsSectionProps = $props();

  let draft = $state('');

  const id = $props.id();
  const hintId = `${id}-hint`;

  const fault = $derived(
    source.error === undefined ? undefined : describeFault(source.error),
  );

  const keyOf = (item: AllowedOrigin): string =>
    `${item.source}/${item.extension ?? ''}/${item.origin}`;

  const add = async (event: SubmitEvent): Promise<void> => {
    event.preventDefault();

    const origin = draft.trim();
    if (origin === '') return;
    // A refused value stays in the field, so the person corrects it.
    if (await source.add(origin)) draft = '';
  };
</script>

<div
  class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  data-part="allowed-origins"
>
  <p class="max-w-3xl text-caption text-muted-foreground">
    {t('settingsNetwork.origins.note')}
  </p>

  {#if !source.loaded && fault === undefined}
    <div aria-busy="true" aria-label={t('settingsNetwork.origins.loading')}>
      <Skeleton class="h-10 w-full" />
    </div>
  {:else if source.loaded}
    {#if source.items.length === 0}
      <p class="text-caption text-muted-foreground" data-part="no-origins">
        {t('settingsNetwork.origins.none')}
      </p>
    {:else}
      <RowList>
        <ul>
          {#each source.items as item (keyOf(item))}
            <li
              class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border px-4 py-3 text-sm last:border-b-0"
              data-part="origin"
              data-source={item.source}
            >
              <div class="flex min-w-0 flex-1 basis-64 flex-col gap-0.5">
                <span class="font-mono break-all">{item.origin}</span>
                <span class="text-xs text-muted-foreground">
                  {#if item.source === 'file'}
                    {t('settingsNetwork.origins.file')}
                  {:else if item.source === 'extension'}
                    {@const title = item.title ?? item.extension ?? ''}
                    {#if extensionHref !== undefined && item.extension !== undefined}
                      <a
                        class="underline underline-offset-2"
                        href={extensionHref(item.extension)}
                      >
                        {t('settingsNetwork.origins.extension', { title })}
                      </a>
                    {:else}
                      {t('settingsNetwork.origins.extension', { title })}
                    {/if}
                  {:else}
                    {t('settingsNetwork.origins.added')}
                  {/if}
                </span>
              </div>
              {#if item.source === 'settings'}
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={source.saving}
                  aria-label={t('settingsNetwork.origins.removeNamed', {
                    origin: item.origin,
                  })}
                  onclick={() => {
                    void source.remove(item.origin);
                  }}
                >
                  {t('settingsNetwork.origins.remove')}
                </Button>
              {/if}
            </li>
          {/each}
        </ul>
      </RowList>
    {/if}

    <!-- The hint sits under the row and not in the field, so the button
         lines up with the input and not with the hint's last line. -->
    <div class="flex flex-col gap-2">
      <form class="flex flex-wrap items-end gap-2" onsubmit={add}>
        <Field
          class="min-w-0 flex-1 basis-64"
          label={t('settingsNetwork.origins.label')}
          control={Input}
          controlProps={{
            value: draft,
            type: 'url',
            class: 'font-mono',
            spellcheck: false,
            autocomplete: 'off',
            'aria-describedby': hintId,
            oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
              draft = event.currentTarget.value;
            },
          }}
        />
        <Button type="submit" disabled={source.saving || draft.trim() === ''}>
          <PlusIcon aria-hidden="true" />
          {t('settingsNetwork.origins.add')}
        </Button>
      </form>
      <p id={hintId} class="text-caption text-muted-foreground">
        {t('settingsNetwork.origins.hint')}
      </p>
    </div>
  {/if}

  <FaultAlert {fault} />
</div>
