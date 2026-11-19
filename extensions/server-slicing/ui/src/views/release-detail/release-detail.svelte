<!--
  One PrusaSlicer version: where it stands on this server, the file an
  install downloads and the checksum it is held to, each installed copy with
  its executable, and the install's error when it failed. It is drawn the way
  the app's other detail screens are: a trail back to the list, a header with
  the actions, and the facts in sections.
-->
<script lang="ts">
  import {
    formatBytes,
    formatDate,
    formatDateTime,
  } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Badge,
    Button,
    Card,
    CardContent,
    DetailHeader,
    EmptyState,
    KeyValueList,
    type KeyValueRow,
    PageTrail,
    Separator,
  } from '@prusa/connect-ui/primitives';
  import DownloadIcon from '@lucide/svelte/icons/download';
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
  import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
  import LayersIcon from '@lucide/svelte/icons/layers';
  import SearchXIcon from '@lucide/svelte/icons/search-x';
  import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';
  import StarIcon from '@lucide/svelte/icons/star';
  import StarOffIcon from '@lucide/svelte/icons/star-off';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

  import type { InstalledSlicer, Translate } from '../../api';
  import {
    canInstall,
    canPin,
    canRemove,
    canUnpin,
    type ReleaseActs,
  } from '../../release-acts.svelte';
  import type { ReleaseRow } from '../../releases';
  import { DigestBadge } from '../digest-badge';
  import { ReleaseState } from '../release-state';

  let {
    version,
    row,
    platform,
    administrator,
    acts,
    backHref,
    t,
  }: {
    /** The version the address names, which the trail shows before anything else. */
    version: string;
    /** Undefined when the module neither offers nor holds that version. */
    row: ReleaseRow | undefined;
    platform: string;
    administrator: boolean;
    acts: ReleaseActs;
    /** The Slicers tab, with the filters the person left it with. */
    backHref: string;
    t: Translate;
  } = $props();

  const name = $derived(t('slicer.release.name', { version }));
  const day = (date: string) =>
    formatDate(date, { day: 'numeric', month: 'short', year: 'numeric' });

  /** A row with no value is dropped rather than drawn with an absent word. */
  const known = (rows: readonly KeyValueRow[]): KeyValueRow[] =>
    rows.filter((entry) => entry.value !== undefined && entry.value !== '');

  const release = $derived(
    row === undefined
      ? []
      : known([
          {
            id: 'version',
            label: t('slicer.column.version'),
            value: version,
            mono: true,
          },
          {
            id: 'channel',
            label: t('slicer.column.channel'),
            value: t(`slicer.channel.${row.channel}`),
          },
          {
            id: 'released',
            label: t('slicer.column.released'),
            value: row.released ? day(row.released) : undefined,
          },
          {
            id: 'source',
            label: t('slicer.release.from'),
            value: row.offer
              ? t(`slicer.source.${row.offer.source}`)
              : undefined,
          },
          {
            id: 'platform',
            label: t('slicer.release.platform'),
            value: platform,
            mono: true,
          },
        ]),
  );

  const download = $derived(
    row?.offer === undefined
      ? []
      : known([
          {
            id: 'asset',
            label: t('slicer.detail.asset'),
            value: row.offer.asset,
            mono: true,
            copyable: true,
          },
          {
            id: 'size',
            label: t('slicer.column.size'),
            value: row.offer.size ? formatBytes(row.offer.size) : undefined,
          },
          {
            id: 'sha256',
            label: t('slicer.detail.sha256'),
            value: row.offer.sha256 ?? t('slicer.detail.sha256None'),
            mono: row.offer.sha256 !== undefined,
            copyable: row.offer.sha256 !== undefined,
          },
        ]),
  );

  const copyOf = (held: InstalledSlicer): KeyValueRow[] =>
    known([
      {
        id: 'source',
        label: t('slicer.release.from'),
        value: t(`slicer.source.${held.source}`),
      },
      {
        id: 'installed',
        label: t('slicer.detail.installedAt'),
        value: held.installed_at
          ? formatDateTime(held.installed_at)
          : undefined,
      },
      {
        id: 'disk',
        label: t('slicer.detail.onDisk'),
        value: held.bytes ? formatBytes(held.bytes) : undefined,
      },
      {
        id: 'sha256',
        label: t('slicer.detail.sha256'),
        value: held.sha256,
        mono: true,
        copyable: true,
      },
      {
        id: 'path',
        label: t('slicer.detail.path'),
        value: held.path,
        mono: true,
        copyable: true,
      },
    ]);
</script>

{#snippet part(text: string, Glyph: typeof LayersIcon)}
  <h2 class="flex items-center gap-2 text-sm font-medium">
    <Glyph aria-hidden="true" class="size-4 text-muted-foreground" />
    {text}
  </h2>
{/snippet}

{#snippet titleAside()}
  {#if row}
    <Badge variant="outline">{t(`slicer.channel.${row.channel}`)}</Badge>
    {#if row.isDefault}<Badge variant="secondary">{t('slicer.default')}</Badge
      >{/if}
  {/if}
{/snippet}

{#snippet meta()}
  {#if row}
    <div class="flex flex-wrap items-center gap-3">
      <ReleaseState {row} {t} detailed />
      <DigestBadge digest={row.digest} {t} />
    </div>
  {/if}
{/snippet}

{#snippet actions()}
  {#if row}
    {#if canInstall(row, administrator)}
      <Button onclick={() => acts.install(row)}>
        <DownloadIcon aria-hidden="true" />
        {row.state === 'failed'
          ? t('slicer.action.retry')
          : t('slicer.action.install')}
      </Button>
    {/if}
    {#if canPin(row, administrator)}
      <Button variant="secondary" onclick={() => acts.pin(row)}>
        <StarIcon aria-hidden="true" />
        {t('slicer.action.pin')}
      </Button>
    {/if}
    {#if canUnpin(row, administrator)}
      <Button variant="secondary" onclick={() => acts.unpin()}>
        <StarOffIcon aria-hidden="true" />
        {t('slicer.action.unpin')}
      </Button>
    {/if}
    {#if canRemove(row, administrator)}
      <Button variant="destructive" onclick={() => acts.remove(row)}>
        <Trash2Icon aria-hidden="true" />
        {t('slicer.action.remove')}
      </Button>
    {/if}
  {/if}
{/snippet}

<div
  class="grid w-full grid-cols-1 content-start gap-4"
  data-part="release-detail"
>
  <PageTrail
    trail={[{ label: t('slicer.tab.slicers'), href: backHref }]}
    page={name}
  />

  {#if row === undefined}
    <Card>
      <CardContent>
        <EmptyState
          icon={SearchXIcon}
          heading={t('slicer.detail.releaseMissing')}
          headingLevel="h1"
          description={t('slicer.detail.releaseMissingNote', { version })}
        />
      </CardContent>
    </Card>
  {:else}
    <DetailHeader title={name} {titleAside} {meta} {actions} />

    {#if row.state === 'unavailable'}
      <Alert>
        <TriangleAlertIcon aria-hidden="true" />
        <AlertDescription class="break-words">
          {t('slicer.release.incompatible', {
            reason: row.offer?.reason ?? '',
          })}
        </AlertDescription>
      </Alert>
    {/if}
    {#if row.state === 'failed' && row.progress}
      <Alert variant="destructive" data-part="install-error">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>{t('slicer.detail.installFailed')}</AlertTitle>
        <AlertDescription class="break-words">
          {row.progress.error ?? ''}
        </AlertDescription>
      </Alert>
    {/if}
    {#if row.digest === 'unverified' && row.state !== 'installed'}
      <Alert>
        <ShieldAlertIcon aria-hidden="true" />
        <AlertDescription>{t('slicer.detail.unverifiedNote')}</AlertDescription>
      </Alert>
    {/if}

    <Card>
      <CardContent class="grid grid-cols-1 content-start gap-6">
        <section class="grid grid-cols-1 content-start gap-4">
          {@render part(t('slicer.detail.release'), LayersIcon)}
          <KeyValueList rows={release} labelWidth="14rem" />
          {#if row.offer?.notes}
            {@const notes = row.offer.notes}
            <div class="text-sm">
              <Button variant="link" size="inline-icon">
                {#snippet child({ props })}
                  <a
                    {...props}
                    href={notes}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {t('slicer.release.notes')}
                    <ExternalLinkIcon aria-hidden="true" class="size-3.5" />
                  </a>
                {/snippet}
              </Button>
            </div>
          {/if}
        </section>

        {#if download.length > 0}
          <Separator />
          <section
            class="grid grid-cols-1 content-start gap-4"
            data-part="download"
          >
            {@render part(t('slicer.detail.download'), DownloadIcon)}
            <KeyValueList rows={download} labelWidth="14rem" />
          </section>
        {/if}

        {#each row.installed as held (held.source)}
          <Separator />
          <section
            class="grid grid-cols-1 content-start gap-4"
            data-part="installed-copy"
          >
            {@render part(
              t('slicer.detail.installed', {
                source: t(`slicer.source.${held.source}`),
              }),
              HardDriveIcon,
            )}
            <KeyValueList rows={copyOf(held)} labelWidth="14rem" />
          </section>
        {/each}
      </CardContent>
    </Card>
  {/if}
</div>
