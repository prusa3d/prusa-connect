<!-- The stored events of the window, in the host's words. -->
<script lang="ts">
  import InboxIcon from '@lucide/svelte/icons/inbox';

  import { CORE_NAMESPACE, formatDateTime, locale, t } from '../../../i18n';
  import {
    Button,
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    EmptyState,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
  } from '../../../primitives';
  import type { HistoryFact } from '../../../data';

  let {
    rows,
    more,
    describeType,
    onMore,
  }: {
    rows: readonly HistoryFact[];
    more: boolean;
    describeType?: ((type: string) => string) | undefined;
    onMore?: (() => void) | undefined;
  } = $props();

  // The store pages forward through the window, oldest first; a log reads
  // the other way, so the row a reader opened this tab to see is on top.
  // Where the window did not fit one page, "on top" is the newest row that
  // was read and not the newest row there is, which is what `more` says.
  const entries = $derived([...rows].reverse());

  // The catalogue words a type the host knows; a type it does not know yet
  // keeps the server's own summary.
  const wordsFor = (type: string): string => {
    const key = `printerDetailHistory.eventType.${type.replace(/^com\.prusa\.connect\./u, '')}`;

    return locale.message(CORE_NAMESPACE, key) === undefined
      ? (describeType?.(type) ?? type)
      : t(key);
  };
</script>

<Card>
  <CardHeader>
    <CardTitle>
      <h2 class="contents">{t('printerDetailHistory.log.heading')}</h2>
    </CardTitle>
  </CardHeader>
  <CardContent class="flex flex-col gap-4">
    {#if entries.length === 0}
      <EmptyState
        icon={InboxIcon}
        heading={t('printerDetailHistory.log.empty.heading')}
        headingLevel="h3"
      />
    {:else}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('printerDetailHistory.log.time')}</TableHead>
            <TableHead>{t('printerDetailHistory.log.type')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {#each entries as row (`${row.epoch}:${String(row.sequence)}`)}
            <TableRow>
              <TableCell class="text-xs whitespace-nowrap tabular-nums">
                {formatDateTime(row.at)}
              </TableCell>
              <TableCell class="text-sm">
                {wordsFor(row.type)}
              </TableCell>
            </TableRow>
          {/each}
        </TableBody>
      </Table>
    {/if}
    {#if more}
      <p class="text-sm text-muted-foreground">
        {t('printerDetailHistory.log.partial')}
      </p>
      <Button
        variant="secondary"
        size="sm"
        class="self-start"
        onclick={() => onMore?.()}
      >
        {t('printerDetailHistory.log.more')}
      </Button>
    {/if}
  </CardContent>
</Card>
