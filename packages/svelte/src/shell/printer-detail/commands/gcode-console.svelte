<!--
  The console, and the log of what this page has sent.

  `GET /gcode` and the command route answer one send at a time, so the log
  here is this page's own, bounded by the store. The stored command record is
  read by `DeviceCommandLog`; the empty state says only that nothing was sent
  from this page (ADR-20260917-a7d rule 9, the second absence).

  A sent line and an answer are values, so both take the mono face.
-->
<script lang="ts">
  import HistoryIcon from '@lucide/svelte/icons/history';

  import { formatDateTime, t } from '../../../i18n';
  import {
    Badge,
    Button,
    Card,
    CardAction,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
    CodeBlock,
    EmptyState,
    Field,
    ScrollArea,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    Textarea,
  } from '../../../primitives';
  import type { GcodeConsoleProps } from './types';

  let { commands }: GcodeConsoleProps = $props();

  let text = $state('');
  let preset = $state('');

  // Newest first: the answer a person is waiting for is the one they just
  // caused, and the log grows from the oldest end.
  const entries = $derived([...commands.log].reverse());

  const presetItems = $derived(
    commands.presets.map((one) => ({ value: one.name, label: one.name })),
  );

  const write = (event: Event) => {
    const field = event.currentTarget;
    if (field instanceof HTMLTextAreaElement) text = field.value;
  };

  const run = () => {
    void commands.sendGcode(text);
    text = '';
  };
</script>

<Card>
  <CardHeader>
    <CardTitle>
      <h2 class="contents">{t('printerDetailCommands.console.heading')}</h2>
    </CardTitle>
    <CardDescription>
      {t('printerDetailCommands.console.detail')}
    </CardDescription>
  </CardHeader>
  <CardContent class="flex flex-col gap-4">
    <Field
      label={t('printerDetailCommands.console.label')}
      description={t('printerDetailCommands.console.hint')}
      control={Textarea}
      controlProps={{
        value: text,
        oninput: write,
        rows: 4,
        resizable: true,
        spellcheck: false,
        class: 'font-mono',
      }}
    />
    <div
      class="flex flex-col items-stretch gap-2 @md/commands:flex-row @md/commands:items-center"
    >
      <Button disabled={commands.sending || text.trim() === ''} onclick={run}>
        {t('printerDetailCommands.send')}
      </Button>
      {#if presetItems.length > 0}
        <Select bind:value={preset} items={presetItems}>
          <SelectTrigger
            aria-label={t('printerDetailCommands.preset.label')}
            class="@md/commands:w-56"
          >
            <SelectValue
              placeholder={t('printerDetailCommands.preset.placeholder')}
            />
          </SelectTrigger>
          <SelectContent>
            {#each commands.presets as one (one.name)}
              <SelectItem value={one.name} label={one.name}>
                {one.name}
              </SelectItem>
            {/each}
          </SelectContent>
        </Select>
        <Button
          variant="secondary"
          disabled={commands.sending || preset === ''}
          onclick={() => void commands.runPreset(preset)}
        >
          {t('printerDetailCommands.preset.run')}
        </Button>
      {/if}
    </div>
    {#if commands.answer}
      <CodeBlock
        label={t('printerDetailCommands.answer.label')}
        value={commands.answer}
      />
    {/if}
  </CardContent>
</Card>

<Card>
  <CardHeader>
    <CardTitle>
      <h2 class="contents">{t('printerDetailCommands.log.heading')}</h2>
    </CardTitle>
    <CardAction>
      {#if commands.refused > 0}
        <Badge variant="destructive">
          {t('printerDetailCommands.log.refused', { count: commands.refused })}
        </Badge>
      {/if}
      <Button
        variant="ghost"
        size="sm"
        disabled={entries.length === 0}
        onclick={() => commands.clear()}
      >
        {t('printerDetailCommands.log.clear')}
      </Button>
    </CardAction>
  </CardHeader>
  <CardContent>
    {#if entries.length === 0}
      <EmptyState
        icon={HistoryIcon}
        heading={t('printerDetailCommands.log.empty')}
        headingLevel="h3"
        description={t('printerDetailCommands.log.notServed')}
      />
    {:else}
      <ScrollArea class="h-64">
        <Table>
          <TableCaption>
            {t('printerDetailCommands.log.caption', { count: commands.limit })}
          </TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('printerDetailCommands.log.column.sent')}
              </TableHead>
              <TableHead>
                {t('printerDetailCommands.log.column.answer')}
              </TableHead>
              <TableHead>
                {t('printerDetailCommands.log.column.code')}
              </TableHead>
              <TableHead>{t('printerDetailCommands.log.column.at')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {#each entries as line (line.id)}
              <TableRow>
                <TableCell>
                  <code class="font-mono text-xs break-all">{line.sent}</code>
                </TableCell>
                <TableCell>
                  {#if line.answer}
                    <code class="font-mono text-xs break-all"
                      >{line.answer}</code
                    >
                  {/if}
                </TableCell>
                <TableCell>
                  {#if line.code}
                    <Badge variant={line.ok ? 'outline' : 'destructive'}>
                      {line.code}
                    </Badge>
                  {/if}
                </TableCell>
                <TableCell class="text-xs whitespace-nowrap tabular-nums">
                  {formatDateTime(line.at)}
                </TableCell>
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      </ScrollArea>
    {/if}
  </CardContent>
</Card>
