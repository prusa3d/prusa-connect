<!-- The same facts as the table, one row each, for a narrow screen. -->
<script lang="ts">
  import { formatBytes, formatDate } from '@prusa/connect-ui/i18n';
  import { Badge, DataRow, RowList } from '@prusa/connect-ui/primitives';

  import type { Translate } from '../../api';
  import type { ReleaseActs } from '../../release-acts.svelte';
  import type { ReleaseRow } from '../../releases';
  import { DigestBadge } from '../digest-badge';
  import { ReleaseActions } from '../release-actions';
  import { ReleaseState } from '../release-state';

  let {
    rows,
    shows,
    hrefOf,
    administrator,
    acts,
    t,
  }: {
    rows: readonly ReleaseRow[];
    shows: (field: string) => boolean;
    hrefOf: (row: ReleaseRow) => string;
    administrator: boolean;
    acts: ReleaseActs;
    t: Translate;
  } = $props();

  const day = (date: string) =>
    formatDate(date, { day: 'numeric', month: 'short', year: 'numeric' });
</script>

<RowList data-part="release-rows">
  {#each rows as row (row.version)}
    {#snippet state()}<ReleaseState {row} {t} />{/snippet}
    {#snippet channel()}
      <Badge variant="outline">{t(`slicer.channel.${row.channel}`)}</Badge>
    {/snippet}
    {#snippet marker()}<Badge variant="secondary">{t('slicer.default')}</Badge
      >{/snippet}
    {#snippet digest()}<DigestBadge digest={row.digest} {t} />{/snippet}
    <DataRow
      href={hrefOf(row)}
      primary={row.version}
      wrapValues
      data-version={row.version}
      secondaryValues={[
        shows('state') ? state : undefined,
        row.isDefault ? marker : undefined,
        shows('channel') ? channel : undefined,
        shows('released') && row.released ? day(row.released) : undefined,
        shows('size') && row.size ? formatBytes(row.size) : undefined,
        shows('digest') ? digest : undefined,
      ]}
    >
      {#snippet actions()}
        <ReleaseActions
          {row}
          {administrator}
          {acts}
          detailHref={hrefOf(row)}
          {t}
        />
      {/snippet}
    </DataRow>
  {/each}
</RowList>
