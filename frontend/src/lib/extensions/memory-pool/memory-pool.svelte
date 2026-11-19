<!--
  The memory the enabled service modules promise against the pool they share:
  one segment per module and one for the modules the page does not list.
  Promising more than the pool is a warning and refuses nothing, so the bar
  turns amber and says by how much.

  A pool the server could not learn is said to be unknown, never drawn as
  empty. A person who may not read the pool is shown nothing: `pool` is then
  undefined.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    Button,
    chartPalette,
    SettingsPane,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import { mibText, type ModuleMemoryPool } from '../memory';

  let {
    pool,
    budgets,
    configHref,
  }: {
    pool: ModuleMemoryPool | undefined;
    /** The budget of each enabled service module the page lists, by title. */
    budgets: readonly { key: string; title: string; bytes: number }[];
    /** The documentation of `core.modules_memory`; absent draws no link. */
    configHref?: string | undefined;
  } = $props();

  const t = translateFor(APP_NAMESPACE);

  const KEY = 'app.settings.extensions.pool';
  const poolBytes = $derived(pool?.pool_bytes);
  const committed = $derived(pool?.committed_bytes ?? 0);
  const over = $derived(
    poolBytes !== undefined && committed > poolBytes
      ? committed - poolBytes
      : 0,
  );
  // Modules the person may not list still count in the pool.
  const other = $derived(
    Math.max(0, committed - budgets.reduce((sum, one) => sum + one.bytes, 0)),
  );
  const segments = $derived([
    ...budgets.map((one, index) => ({
      key: one.key,
      label: one.title,
      bytes: one.bytes,
      other: false,
      fill: chartPalette[index % chartPalette.length]?.swatch ?? 'bg-primary',
    })),
    ...(other > 0
      ? [
          {
            key: '',
            label: t(`${KEY}.other`),
            bytes: other,
            other: true,
            fill: 'bg-muted-foreground',
          },
        ]
      : []),
  ]);
  // A promise past the pool fills the bar and no further.
  const whole = $derived(Math.max(poolBytes ?? 0, committed));
  const summary = $derived(
    poolBytes === undefined
      ? t(`${KEY}.promisedOnly`, { promised: mibText(committed, t) })
      : t(`${KEY}.summary`, {
          promised: mibText(committed, t),
          pool: mibText(poolBytes, t),
          free: mibText(Math.max(0, poolBytes - committed), t),
        }),
  );
</script>

{#snippet headerEnd()}
  <p class="text-caption text-muted-foreground" data-part="pool-summary">
    {summary}
  </p>
{/snippet}

{#if pool !== undefined}
  <div data-part="memory-pool" data-over={over > 0 || undefined}>
    <SettingsPane title={t(`${KEY}.title`)} {headerEnd}>
      <div
        class="flex flex-col gap-2 px-panel-padding-mobile py-section-gap @lg/pane:px-panel-padding"
      >
        {#if poolBytes === undefined}
          <p
            class="text-caption text-muted-foreground"
            data-part="pool-unknown"
          >
            {t(`${KEY}.unknown`)}
          </p>
        {:else}
          <div
            role="img"
            aria-label={summary}
            class={[
              'flex h-2 w-full overflow-hidden rounded-sm bg-border',
              over > 0 && 'outline-2 outline-warning',
            ]}
            data-part="pool-bar"
          >
            {#each segments as segment (segment.key)}
              <div
                class={[
                  'h-full border-r border-card last:border-r-0',
                  segment.fill,
                ]}
                style={`width: ${String((segment.bytes / whole) * 100)}%`}
                data-part="pool-segment"
                data-other={segment.other || undefined}
              ></div>
            {/each}
          </div>
          {#if over > 0}
            <p class="text-caption text-warning-text" data-part="pool-over">
              {t(`${KEY}.over`, { over: mibText(over, t) })}
            </p>
          {/if}
          <ul class="flex flex-wrap gap-x-4 text-caption text-muted-foreground">
            {#each segments as segment (segment.key)}
              <li class="flex items-center gap-1.5" data-part="pool-legend">
                <span
                  class={['size-2 shrink-0 rounded-xs', segment.fill]}
                  aria-hidden="true"
                  data-part="pool-swatch"
                ></span>
                {segment.label}: {mibText(segment.bytes, t)}
              </li>
            {/each}
          </ul>
        {/if}

        {#if configHref !== undefined}
          <p class="text-caption">
            <Button variant="link" data-part="pool-config">
              {#snippet child({ props })}
                <a {...props} href={configHref}>{t(`${KEY}.configure`)}</a>
              {/snippet}
            </Button>
          </p>
        {/if}
      </div>
    </SettingsPane>
  </div>
{/if}
