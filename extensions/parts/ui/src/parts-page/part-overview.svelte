<!--
  What the part is: its description, and the facts about it.

  The description is the long text a person keeps about the part: what it
  fits, how it prints, what to watch for. It is edited in place, in a box the
  size of the text. The change is sent with the revision the page read, so a
  change somebody else saved in between is refused rather than overwritten,
  and the person's text stays in the box while they read the other one.
-->
<script lang="ts">
  import FileTextIcon from '@lucide/svelte/icons/file-text';
  import InfoIcon from '@lucide/svelte/icons/info';
  import PencilLineIcon from '@lucide/svelte/icons/pencil-line';
  import { formatBytes, formatDateTime } from '@prusa/connect-ui/i18n';
  import {
    Button,
    KeyValueList,
    Separator,
    Textarea,
    type KeyValueRow,
  } from '@prusa/connect-ui/primitives';

  import { byteLength, MAX_DESCRIPTION } from '../presentation';
  import { heldSession } from '../session';
  import FaultNote from './fault-note.svelte';
  import { personOf } from './person';

  const { catalogue, host } = heldSession();
  const t = host.t;
  const uid = $props.id();

  let draft = $state<string | undefined>(undefined);
  let refused = $state<unknown>(undefined);

  const part = $derived(catalogue.detail);
  const latest = $derived(part?.versions.at(-1));

  const folderName = $derived(
    part === undefined || part.folder === 0
      ? t('parts.tree.all')
      : (catalogue.folder(part.folder)?.name ?? ''),
  );

  const facts: KeyValueRow[] = $derived(
    part === undefined
      ? []
      : (
          [
            { id: 'folder', label: t('parts.fact.folder'), value: folderName },
            {
              id: 'created',
              label: t('parts.fact.created'),
              value: t('parts.fact.when', {
                when: formatDateTime(part.created),
                who: personOf(part.created_by, catalogue.who, t),
              }),
            },
            {
              id: 'changed',
              label: t('parts.fact.changed'),
              value: t('parts.fact.when', {
                when: formatDateTime(part.changed),
                who: personOf(part.changed_by, catalogue.who, t),
              }),
            },
            {
              id: 'latest',
              label: t('parts.fact.latest'),
              value:
                latest === undefined
                  ? t('parts.fact.noVersion')
                  : t('parts.fact.latestValue', {
                      number: latest.number,
                      count: latest.files.length,
                      size: formatBytes(
                        latest.files.reduce((sum, file) => sum + file.size, 0),
                      ),
                    }),
            },
            {
              id: 'dir',
              label: t('parts.fact.dir'),
              value: `${part.storage}:${part.dir}`,
              mono: true,
              copyable: true,
            },
          ] satisfies KeyValueRow[]
        ).filter((row) => row.value !== ''),
  );

  const save = async (): Promise<void> => {
    if (part === undefined || draft === undefined) return;
    refused = undefined;

    if (await catalogue.changePart(part.id, { description: draft })) {
      draft = undefined;
    } else {
      refused = catalogue.fault;
      catalogue.dismiss();
    }
  };
</script>

{#if part}
  <div class="grid gap-y-6">
    <section class="grid gap-y-3" aria-labelledby="{uid}-description">
      <div class="flex items-center justify-between gap-2">
        <h2
          id="{uid}-description"
          class="flex items-center gap-2 text-sm font-medium"
        >
          <FileTextIcon
            aria-hidden="true"
            class="size-4 text-muted-foreground"
          />
          {t('parts.overview.description')}
        </h2>
        {#if catalogue.allowed.write && draft === undefined}
          <Button
            variant="ghost"
            size="sm"
            data-part="edit-description"
            onclick={() => {
              draft = part.description ?? '';
              refused = undefined;
            }}
          >
            <PencilLineIcon aria-hidden="true" />
            {t('parts.overview.edit')}
          </Button>
        {/if}
      </div>

      {#if draft !== undefined}
        <FaultNote fault={refused} />
        <Textarea
          aria-labelledby="{uid}-description"
          class="field-sizing-content min-h-40"
          data-byte-limit={MAX_DESCRIPTION}
          bind:value={draft}
        />
        <div class="flex flex-wrap items-center gap-2">
          <Button
            disabled={catalogue.busy}
            data-part="save-description"
            onclick={() => void save()}
          >
            {t('parts.action.save')}
          </Button>
          <Button
            variant="secondary"
            onclick={() => {
              draft = undefined;
              refused = undefined;
            }}
          >
            {t('parts.action.cancel')}
          </Button>
          <span class="ms-auto text-xs text-muted-foreground tabular-nums">
            {t('parts.overview.length', {
              used: byteLength(draft),
              limit: MAX_DESCRIPTION,
            })}
          </span>
        </div>
      {:else if part.description}
        <p
          class="text-sm leading-relaxed whitespace-pre-line"
          data-part="description"
        >
          {part.description}
        </p>
      {:else}
        <p class="text-sm text-muted-foreground" data-part="description">
          {catalogue.allowed.write
            ? t('parts.overview.empty')
            : t('parts.overview.emptyViewer')}
        </p>
      {/if}
    </section>

    <Separator />

    <section class="grid gap-y-3" aria-labelledby="{uid}-facts">
      <h2 id="{uid}-facts" class="flex items-center gap-2 text-sm font-medium">
        <InfoIcon aria-hidden="true" class="size-4 text-muted-foreground" />
        {t('parts.overview.facts')}
      </h2>
      <KeyValueList rows={facts} labelWidth="14rem" />
    </section>
  </div>
{/if}
