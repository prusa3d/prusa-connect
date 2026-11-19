<!--
  An installed module's memory: its budget, who chose it, what its process
  uses now against that budget, and what the host does when the process goes
  over the budget. The
  choices are written with the rest of the instance. The operator's budget is
  edited here too; empty follows the author's recommendation. So is the
  automatic switch-off: how many failures inside a window make the host switch
  the module off.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    Badge,
    Button,
    FaultAlert,
    Input,
    Label,
    Progress,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Separator,
    SettingsPane,
    SettingsRow,
  } from '@prusa/connect-ui/primitives';

  import type { Snippet } from 'svelte';

  import { APP_NAMESPACE } from '../../i18n';
  import { describeExtensionFault } from '../fault';
  import {
    budgetOfText,
    DEFAULT_MEMORY_MIB,
    MAX_MEMORY_MIB,
    MIN_MEMORY_MIB,
    type MemoryEstimate as Estimate,
    usageTone,
  } from '../memory';

  let {
    estimate,
    usedBytes,
    overBudget,
    nearBudget = false,
    onOverMemory,
    budgetMiB,
    recommendedMiB,
    running = false,
    locked,
    saving,
    fault,
    conflict = false,
    onchange,
    onbudget,
    switchOff,
    onswitchoff,
    children,
  }: {
    estimate: Estimate;
    /** What the process uses now, in bytes. Absent when the host cannot read it. */
    usedBytes?: number | undefined;
    /** Whether the last reading was over the budget, which only a module set to warn survives. */
    overBudget: boolean;
    /** Whether the last reading was close to the budget. */
    nearBudget?: boolean;
    onOverMemory: 'stop' | 'warn';
    /** The operator's budget in MiB; absent or 0 means none is set. */
    budgetMiB?: number | undefined;
    /** What the author recommends in MiB, when the module makes a recommendation. */
    recommendedMiB?: number | undefined;
    /** Whether the module is enabled, so saving restarts it. */
    running?: boolean;
    /** Why the choice cannot be changed here, already worded. */
    locked?: string | undefined;
    saving: boolean;
    fault?: unknown;
    conflict?: boolean;
    onchange: (next: 'stop' | 'warn') => void;
    /**
     * Writes the budget in MiB; 0 clears the operator's choice. A promise of
     * false keeps what was typed, because the write failed.
     */
    onbudget: (mib: number) => Promise<boolean>;
    /** The switch-off in force. Absent hides the block. */
    switchOff?:
      | {
          failures: number;
          windowSeconds: number;
          source: 'default' | 'operator';
        }
      | undefined;
    /**
     * Writes the operator's switch-off; undefined returns to the server
     * default. A promise of false keeps what was typed.
     */
    onswitchoff?: (
      next: { failures: number; window_seconds: number } | undefined,
    ) => Promise<boolean>;
    /** What the pane shows above the current use, such as the last hour. */
    children?: Snippet;
  } = $props();

  const t = translateFor(APP_NAMESPACE);
  const id = $props.id();

  const usedMiB = $derived(
    usedBytes === undefined ? undefined : Math.round(usedBytes / 1_048_576),
  );
  const usage = $derived(
    usedMiB === undefined
      ? t('app.settings.extensions.memory.budgetOnly', { mib: estimate.mib })
      : t('app.settings.extensions.memory.usage', {
          used: usedMiB,
          budget: estimate.mib,
        }),
  );
  const tone = $derived(usageTone(overBudget, nearBudget));

  const set = $derived(budgetMiB !== undefined && budgetMiB > 0);
  let typed = $state<string | undefined>(undefined);
  const shown = $derived(typed ?? (set ? String(budgetMiB) : ''));
  const parsed = $derived(budgetOfText(shown));
  const invalid = $derived(shown.trim() !== '' && parsed === undefined);
  const changed = $derived(
    parsed !== undefined && parsed !== (set ? budgetMiB : undefined),
  );
  const empty = $derived(recommendedMiB !== undefined && recommendedMiB > 0);
  // One whole sentence per case, so a translation can order each freely.
  const budgetHint = $derived.by(() => {
    if (empty) {
      return t(
        running
          ? 'app.settings.extensions.memory.budget.hint.authorRestart'
          : 'app.settings.extensions.memory.budget.hint.author',
        { mib: recommendedMiB ?? 0 },
      );
    }
    return t(
      running
        ? 'app.settings.extensions.memory.budget.hint.defaultRestart'
        : 'app.settings.extensions.memory.budget.hint.default',
      { mib: DEFAULT_MEMORY_MIB },
    );
  });

  const MAX_FAILURES = 100;
  const WINDOWS = [
    {
      seconds: 900,
      key: 'app.settings.extensions.memory.switchOff.window.15m',
    },
    {
      seconds: 3600,
      key: 'app.settings.extensions.memory.switchOff.window.1h',
    },
    {
      seconds: 21600,
      key: 'app.settings.extensions.memory.switchOff.window.6h',
    },
    {
      seconds: 86400,
      key: 'app.settings.extensions.memory.switchOff.window.24h',
    },
  ] as const;

  let typedFailures = $state<string | undefined>(undefined);
  let pickedWindow = $state<number | undefined>(undefined);
  const failuresShown = $derived(
    typedFailures ?? String(switchOff?.failures ?? ''),
  );
  const failures = $derived(
    /^\d+$/.test(failuresShown.trim()) && Number(failuresShown) <= MAX_FAILURES
      ? Number(failuresShown)
      : undefined,
  );
  const windowSeconds = $derived(pickedWindow ?? switchOff?.windowSeconds ?? 0);
  const windowItems = $derived([
    // A window the server file set outside the list is still shown.
    ...(WINDOWS.some((one) => one.seconds === switchOff?.windowSeconds)
      ? []
      : [
          {
            value: String(switchOff?.windowSeconds ?? 0),
            label: t('app.settings.extensions.memory.switchOff.window.other', {
              minutes: Math.round((switchOff?.windowSeconds ?? 0) / 60),
            }),
          },
        ]),
    ...WINDOWS.map((one) => ({
      value: String(one.seconds),
      label: t(one.key),
    })),
  ]);
  const switchOffChanged = $derived(
    switchOff !== undefined &&
      failures !== undefined &&
      (failures !== switchOff.failures ||
        windowSeconds !== switchOff.windowSeconds),
  );

  const items = $derived([
    { value: 'stop', label: t('app.settings.extensions.memory.onOver.stop') },
    { value: 'warn', label: t('app.settings.extensions.memory.onOver.warn') },
  ]);
</script>

<div class="flex flex-col gap-4" data-part="extension-memory">
  <SettingsPane title={t('app.settings.extensions.memory.title')}>
    <div
      class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
    >
      {#if children}
        {@render children()}
        <Separator />
      {/if}

      <div class="flex flex-col gap-3" data-part="memory-summary">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <p class="text-base font-medium tabular-nums" data-part="memory-used">
            {usage}
          </p>
          <Badge variant="outline" data-part="memory-source">
            {t(`app.settings.extensions.memory.source.${estimate.source}`)}
          </Badge>
        </div>
        {#if usedMiB !== undefined}
          <!-- A reading past the budget fills the bar and no further; the text
               beside it carries the real figure. -->
          <Progress
            value={Math.min(usedMiB, estimate.mib)}
            max={estimate.mib}
            {tone}
            aria-label={t('app.settings.extensions.memory.meter')}
            aria-valuetext={usage}
            data-part="memory-meter"
            data-tone={tone}
          />
        {/if}
        {#if overBudget}
          <Alert variant="warning" data-part="memory-over-budget">
            <AlertDescription>
              {t('app.settings.extensions.memory.overBudget')}
            </AlertDescription>
          </Alert>
        {/if}
        {#if estimate.overPool && estimate.poolMiB !== undefined}
          <Alert variant="warning" data-part="memory-over-pool">
            <AlertDescription>
              {t('app.settings.extensions.memory.overPool', {
                committed: estimate.committedMiB,
                pool: estimate.poolMiB,
              })}
            </AlertDescription>
          </Alert>
        {/if}
        {#if locked !== undefined}
          <p class="text-sm text-muted-foreground" data-part="memory-locked">
            {locked}
          </p>
        {/if}
      </div>

      <Separator />

      <!-- The rows take the switch layout: the label and its help keep the
           width the control leaves, where the form layout caps them at a
           narrow column that the help wraps in. -->
      <div class="flex flex-col gap-form-note" data-part="memory-budget">
        <SettingsRow
          id={`${id}-budget`}
          layout="switch"
          label={t('app.settings.extensions.memory.budget.label')}
          description={budgetHint}
        >
          {#snippet control()}
            <div class="flex flex-wrap items-center gap-2">
              <div class="w-24 shrink-0">
                <Input
                  type="number"
                  inputmode="numeric"
                  min={MIN_MEMORY_MIB}
                  max={MAX_MEMORY_MIB}
                  size="sm"
                  class="tabular-nums"
                  value={shown}
                  disabled={locked !== undefined || saving}
                  aria-labelledby={`${id}-budget-label`}
                  aria-describedby={invalid
                    ? `${id}-budget-error ${id}-budget-description`
                    : `${id}-budget-description`}
                  aria-invalid={invalid}
                  oninput={(event: Event) => {
                    typed = (event.currentTarget as HTMLInputElement).value;
                  }}
                />
              </div>
              <span class="text-sm text-muted-foreground">
                {t('app.settings.extensions.memory.budget.unit')}
              </span>
              <Button
                size="control"
                variant="secondary"
                disabled={locked !== undefined || saving || !changed}
                onclick={async () => {
                  if (parsed !== undefined && (await onbudget(parsed)))
                    typed = undefined;
                }}
              >
                {t('app.settings.extensions.memory.budget.save')}
              </Button>
              {#if set}
                <Button
                  size="control"
                  variant="ghost"
                  disabled={locked !== undefined || saving}
                  onclick={async () => {
                    if (await onbudget(0)) typed = undefined;
                  }}
                >
                  {t('app.settings.extensions.memory.budget.reset')}
                </Button>
              {/if}
            </div>
          {/snippet}
        </SettingsRow>
        {#if invalid}
          <p
            id={`${id}-budget-error`}
            class="text-xs text-destructive-text"
            role="status"
          >
            {t('app.settings.extensions.memory.budget.invalid', {
              min: String(MIN_MEMORY_MIB),
              max: String(MAX_MEMORY_MIB),
            })}
          </p>
        {/if}
      </div>

      <Separator />

      <SettingsRow
        id={`${id}-over`}
        layout="switch"
        label={t('app.settings.extensions.memory.onOver.label')}
        description={t('app.settings.extensions.memory.onOver.help')}
      >
        {#snippet control()}
          <Select
            value={onOverMemory}
            {items}
            disabled={locked !== undefined || saving}
            onValueChange={(next: string) => {
              if (next === 'stop' || next === 'warn') onchange(next);
            }}
          >
            <SelectTrigger
              class="w-44"
              size="sm"
              aria-labelledby={`${id}-over-label`}
              aria-describedby={`${id}-over-description`}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {#each items as item (item.value)}
                <SelectItem value={item.value} label={item.label}>
                  {item.label}
                </SelectItem>
              {/each}
            </SelectContent>
          </Select>
        {/snippet}
      </SettingsRow>

      {#if switchOff !== undefined}
        <Separator />

        <div class="flex flex-col gap-3" data-part="memory-switch-off">
          <!-- The controls do not fit beside the help, so they stand under
               it, and the row keeps the full width for the label. -->
          <SettingsRow
            id={`${id}-switch-off`}
            layout="switch"
            label={t('app.settings.extensions.memory.switchOff.label')}
            description={t('app.settings.extensions.memory.switchOff.hint')}
          >
            {#snippet labelHelp()}
              <Badge variant="outline" data-part="switch-off-source">
                {t(`app.settings.extensions.memory.source.${switchOff.source}`)}
              </Badge>
            {/snippet}
          </SettingsRow>
          <!-- Labelled pieces rather than one sentence around the controls: a
               sentence split into parts cannot be reordered by a translation. -->
          <div
            role="group"
            aria-labelledby={`${id}-switch-off-label`}
            aria-describedby={`${id}-switch-off-description`}
            class="flex flex-wrap items-center gap-x-4 gap-y-2"
          >
            <div class="flex items-center gap-2">
              <Label for={`${id}-failures`}>
                {t('app.settings.extensions.memory.switchOff.failures')}
              </Label>
              <div class="w-20 shrink-0">
                <Input
                  id={`${id}-failures`}
                  type="number"
                  inputmode="numeric"
                  min={0}
                  max={MAX_FAILURES}
                  size="sm"
                  class="tabular-nums"
                  value={failuresShown}
                  disabled={locked !== undefined || saving}
                  aria-invalid={failures === undefined}
                  aria-describedby={failures === undefined
                    ? `${id}-failures-error`
                    : undefined}
                  oninput={(event: Event) => {
                    typedFailures = (event.currentTarget as HTMLInputElement)
                      .value;
                  }}
                />
              </div>
            </div>
            <div class="flex items-center gap-2">
              <Label for={`${id}-window`}>
                {t('app.settings.extensions.memory.switchOff.window.label')}
              </Label>
              <Select
                value={String(windowSeconds)}
                items={windowItems}
                disabled={locked !== undefined || saving}
                onValueChange={(next: string) => {
                  pickedWindow = Number(next);
                }}
              >
                <SelectTrigger id={`${id}-window`} class="w-32" size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {#each windowItems as item (item.value)}
                    <SelectItem value={item.value} label={item.label}>
                      {item.label}
                    </SelectItem>
                  {/each}
                </SelectContent>
              </Select>
            </div>
            <div class="flex items-center gap-2">
              <Button
                size="control"
                variant="secondary"
                disabled={locked !== undefined || saving || !switchOffChanged}
                onclick={async () => {
                  if (failures === undefined) return;
                  const written = await onswitchoff?.({
                    failures,
                    window_seconds: windowSeconds,
                  });
                  if (written === false) return;
                  typedFailures = undefined;
                  pickedWindow = undefined;
                }}
              >
                {t('app.settings.extensions.memory.switchOff.save')}
              </Button>
              {#if switchOff.source === 'operator'}
                <Button
                  size="control"
                  variant="ghost"
                  disabled={locked !== undefined || saving}
                  onclick={async () => {
                    if ((await onswitchoff?.(undefined)) === false) return;
                    typedFailures = undefined;
                    pickedWindow = undefined;
                  }}
                >
                  {t('app.settings.extensions.memory.switchOff.reset')}
                </Button>
              {/if}
            </div>
          </div>
          {#if failures === undefined}
            <p
              id={`${id}-failures-error`}
              class="text-xs text-destructive-text"
              role="status"
            >
              {t('app.settings.extensions.memory.switchOff.invalid', {
                max: String(MAX_FAILURES),
              })}
            </p>
          {/if}
        </div>
      {/if}
    </div>
  </SettingsPane>

  {#if conflict}
    <Alert variant="warning">
      <AlertDescription>
        {t('app.settings.extensions.write.conflict')}
      </AlertDescription>
    </Alert>
  {:else if fault !== undefined}
    <FaultAlert fault={describeExtensionFault(fault, t)} />
  {/if}
</div>
