<script lang="ts">
  import type { Snippet } from 'svelte';
  import { formatNumber, formatUnit, t } from '../../../i18n';
  import type { Axis, Position } from './motion-coordinates';
  import {
    ArrowDownIcon,
    ArrowLeftIcon,
    ArrowRightIcon,
    ArrowUpIcon,
    Button,
    HomeIcon,
    Slider,
    ToggleGroup,
    ToggleGroupItem,
  } from '../../../primitives';

  let {
    limits,
    position,
    xyDisabled,
    zDisabled,
    homeDisabled,
    busy = false,
    reason,
    feedback,
    simulated = false,
    bedOnZ = false,
    onMove,
    onHome,
    commandPanel,
  }: {
    limits: Position;
    position: Partial<Record<Axis, number | undefined>>;
    xyDisabled: boolean;
    zDisabled: boolean;
    homeDisabled: boolean;
    busy?: boolean;
    reason?: string | undefined;
    feedback?: string | undefined;
    simulated?: boolean;
    bedOnZ?: boolean;
    onMove: (target: Partial<Position>) => void;
    onHome: () => void;
    commandPanel?: Snippet<[Snippet]> | undefined;
  } = $props();

  type PlatePosition = Pick<Position, 'x' | 'y'>;
  interface PlateDrag {
    pointerId: number;
    startX: number;
    startY: number;
    width: number;
    height: number;
    base: PlatePosition;
  }

  const steps = ['0.1', '1', '10'] as const;
  const uid = $props.id();

  let step = $state('1');
  let localFeedback = $state<string>();
  let xyTarget = $state<PlatePosition>();
  let zTarget = $state<number>();
  let zStart: number | undefined;
  let zSliderGeneration = $state(0);
  let xyDrag = $state.raw<PlateDrag>();
  let plot: HTMLDivElement;
  const xy = $derived(xyTarget ?? position);
  const moving = $derived(
    busy || xyDrag !== undefined || zTarget !== undefined,
  );

  const mm = (value: number) =>
    formatUnit(value, 'mm', { maximumFractionDigits: 2 });
  const round = (value: number) => Math.round(value * 10) / 10;

  function jog(axis: Axis, sign: -1 | 1) {
    const from = position[axis];
    if (moving || from === undefined || (axis === 'z' ? zDisabled : xyDisabled))
      return;
    const target = Number((from + Number(step) * sign).toFixed(3));
    if (target < 0 || target > limits[axis]) {
      localFeedback = t('printerDetailControls.coordinates.limit');
      return;
    }
    localFeedback = undefined;
    onMove({ [axis]: target });
  }

  function beginPlateDrag(
    event: PointerEvent & { currentTarget: HTMLButtonElement },
  ) {
    const { x, y } = position;
    if (
      xyDisabled ||
      moving ||
      x === undefined ||
      y === undefined ||
      !event.isPrimary ||
      event.button !== 0
    )
      return;
    const { width, height } = plot.getBoundingClientRect();
    if (width === 0 || height === 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    xyDrag = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      width,
      height,
      base: { x, y },
    };
  }

  function movePlateDrag(event: PointerEvent) {
    const drag = xyDrag;
    if (event.pointerId !== drag?.pointerId || xyDisabled) return;
    const clamp = (axis: 'x' | 'y', value: number) =>
      Math.min(limits[axis], Math.max(0, round(value)));
    xyTarget = {
      x: clamp(
        'x',
        drag.base.x + ((event.clientX - drag.startX) / drag.width) * limits.x,
      ),
      y: clamp(
        'y',
        drag.base.y - ((event.clientY - drag.startY) / drag.height) * limits.y,
      ),
    };
  }

  function cancelPlateDrag(event: PointerEvent) {
    if (event.pointerId !== xyDrag?.pointerId) return;
    xyDrag = undefined;
    xyTarget = undefined;
  }

  function commitPlateDrag(event: PointerEvent) {
    const drag = xyDrag;
    if (drag?.pointerId !== event.pointerId) return;
    movePlateDrag(event);
    const target = xyTarget;
    cancelPlateDrag(event);
    if (
      xyDisabled ||
      busy ||
      !target ||
      (event.clientX === drag.startX && event.clientY === drag.startY)
    )
      return;
    if (position.x !== drag.base.x || position.y !== drag.base.y) {
      localFeedback = t('printerDetailControls.coordinates.dragMoved');
      return;
    }
    localFeedback = undefined;
    onMove(target);
  }

  function plateKey(
    event: KeyboardEvent & { currentTarget: HTMLButtonElement },
  ) {
    if (event.key === 'Escape' && xyDrag) {
      const pointerId = xyDrag.pointerId;
      xyDrag = undefined;
      xyTarget = undefined;
      event.currentTarget.releasePointerCapture(pointerId);
      return;
    }
    switch (event.key) {
      case 'ArrowLeft':
        jog('x', -1);
        break;
      case 'ArrowRight':
        jog('x', 1);
        break;
      case 'ArrowUp':
        jog('y', 1);
        break;
      case 'ArrowDown':
        jog('y', -1);
        break;
      default:
        return;
    }
    event.preventDefault();
  }

  function cancelHeightDrag() {
    zStart = undefined;
    zTarget = undefined;
    zSliderGeneration += 1;
  }

  function commitHeight(value: number) {
    const start = zStart;
    zStart = undefined;
    zTarget = undefined;
    if (
      zDisabled ||
      busy ||
      xyDrag ||
      start === undefined ||
      start !== position.z ||
      value === position.z
    )
      return;
    localFeedback = undefined;
    onMove({ z: value });
  }
</script>

{#snippet direction(axis: Axis, sign: -1 | 1, icon: typeof ArrowUpIcon)}
  {@const label = `${axis.toUpperCase()}${sign === 1 ? '+' : '−'}`}
  {@const Icon = icon}
  <Button
    variant="secondary"
    size="md"
    class="jog-button min-w-0 gap-1 font-mono tabular-nums"
    disabled={axis === 'z' ? zDisabled : xyDisabled}
    aria-disabled={moving || undefined}
    onclick={() => jog(axis, sign)}
  >
    <Icon aria-hidden="true" class="size-4" />
    {label}
  </Button>
{/snippet}

{#snippet positionReading(axis: Axis)}
  {@const value = position[axis]}
  <div class="flex min-w-0 flex-wrap items-baseline justify-center gap-x-2">
    <dt
      class="font-mono text-xs font-medium tracking-wider text-muted-foreground"
    >
      {axis.toUpperCase()}
    </dt>
    <dd class="font-mono text-base tabular-nums" data-axis={axis}>
      {value === undefined ? '—' : mm(value)}
    </dd>
  </div>
{/snippet}

{#snippet homeAction()}
  <Button
    variant="secondary"
    size="compact"
    disabled={homeDisabled}
    aria-disabled={moving || undefined}
    onclick={() => {
      if (!moving && !homeDisabled) {
        localFeedback = undefined;
        onHome();
      }
    }}
  >
    <HomeIcon aria-hidden="true" class="size-4" />
    {t('printerDetailControls.coordinates.home')}
  </Button>
{/snippet}

<div class="motion-coordinates flex flex-col gap-5">
  <header class="flex flex-wrap items-center justify-between gap-4">
    <h3 class="text-base font-semibold">
      {t('printerDetailControls.coordinates.heading')}
    </h3>
    <span id="{uid}-position" class="sr-only">
      {t(
        simulated
          ? 'printerDetailControls.coordinates.simulatedPosition'
          : 'printerDetailControls.coordinates.reportedPosition',
      )}
    </span>
    <div class="flex flex-wrap items-center gap-2">
      <span id="{uid}-step" class="text-sm text-muted-foreground"
        >{t('printerDetailControls.coordinates.step')}</span
      >
      <ToggleGroup
        type="single"
        variant="outline"
        tone="primary"
        value={step}
        disabled={(xyDisabled && zDisabled) || moving}
        onValueChange={(value: string) => {
          if (value) step = value;
        }}
        aria-labelledby="{uid}-step"
      >
        {#each steps as size (size)}
          <ToggleGroupItem value={size} aria-label={mm(Number(size))}>
            {formatNumber(Number(size))}
          </ToggleGroupItem>
        {/each}
      </ToggleGroup>
    </div>
  </header>

  {#if reason}
    <div
      class="rounded-md border border-state-fault-edge bg-state-fault px-3 py-2 text-sm text-state-fault-text"
    >
      <p class="font-semibold">
        {t('printerDetailControls.coordinates.unavailable')}
      </p>
      <p>{reason}</p>
    </div>
  {/if}

  <div class="motion-layout grid gap-3">
    <section
      class="flex flex-col rounded-sm bg-surface p-3 sm:p-4"
      aria-labelledby="{uid}-xy"
    >
      <div class="mb-3 flex flex-wrap items-baseline justify-between gap-x-3">
        <h4 id="{uid}-xy" class="text-sm font-semibold">
          {t('printerDetailControls.jog.xy.label')}
        </h4>
        <span class="text-xs text-muted-foreground"
          >{t('printerDetailControls.coordinates.plate')}</span
        >
      </div>
      <span id="{uid}-nozzle-keys" class="sr-only">
        {t('printerDetailControls.coordinates.keys')}
      </span>
      <div
        class="xy-layout mx-auto mb-3 grid w-full max-w-lg items-center justify-center gap-2"
        role="group"
        aria-labelledby="{uid}-xy"
      >
        <div class="col-start-2 row-start-1 justify-self-center">
          {@render direction('y', 1, ArrowUpIcon)}
        </div>
        <div class="col-start-1 row-start-2">
          {@render direction('x', -1, ArrowLeftIcon)}
        </div>
        <div
          class="plate relative col-start-2 row-start-2 w-full rounded-md border border-border"
          style:aspect-ratio={String(limits.x / limits.y)}
        >
          <span
            class="absolute start-2 top-2 text-xs font-medium text-muted-foreground"
            >{t('printerDetailControls.coordinates.rear')}</span
          >
          <span
            class="absolute start-2 bottom-2 text-xs font-medium text-muted-foreground"
            >{t('printerDetailControls.coordinates.front')}</span
          >
          <span
            class="absolute end-2 bottom-2 font-mono text-xs text-muted-foreground"
            >X →</span
          >
          <span
            class="absolute end-2 top-2 font-mono text-xs text-muted-foreground"
            >Y ↑</span
          >
          <div class="plot absolute inset-x-6 inset-y-12" bind:this={plot}>
            {#if xy.x !== undefined && xy.y !== undefined && xy.x >= 0 && xy.x <= limits.x && xy.y >= 0 && xy.y <= limits.y}
              <div
                class="nozzle"
                data-stale={xyDisabled}
                data-dragging={xyTarget !== undefined}
                style:--x={String(xy.x / limits.x)}
                style:--y={String(1 - xy.y / limits.y)}
              >
                <Button
                  variant="ghost"
                  size="md"
                  class="nozzle-button"
                  aria-label={t('printerDetailControls.coordinates.nozzle')}
                  aria-describedby="{uid}-nozzle-keys"
                  aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown"
                  disabled={xyDisabled}
                  aria-disabled={busy || zTarget !== undefined || undefined}
                  onpointerdown={beginPlateDrag}
                  onpointermove={movePlateDrag}
                  onpointerup={commitPlateDrag}
                  onpointercancel={cancelPlateDrag}
                  onlostpointercapture={cancelPlateDrag}
                  onkeydown={plateKey}
                  onclick={(event: MouseEvent) => {
                    if (event.detail === 0 && !moving)
                      localFeedback = undefined;
                  }}
                >
                  <span class="nozzle-grip" aria-hidden="true"
                    ><span></span></span
                  >
                </Button>
              </div>
            {/if}
          </div>
        </div>
        <div class="col-start-3 row-start-2">
          {@render direction('x', 1, ArrowRightIcon)}
        </div>
        <div class="col-start-2 row-start-3 justify-self-center">
          {@render direction('y', -1, ArrowDownIcon)}
        </div>
      </div>
      <dl
        aria-labelledby="{uid}-position {uid}-xy"
        class="mt-auto grid grid-cols-2 divide-x divide-border border-t border-border py-2"
      >
        {@render positionReading('x')}
        {@render positionReading('y')}
      </dl>
    </section>

    <section
      class="flex flex-col rounded-sm bg-surface p-3 sm:p-4"
      aria-labelledby="{uid}-z"
    >
      <div class="mb-3 flex flex-wrap items-baseline justify-between gap-x-3">
        <h4 id="{uid}-z" class="text-sm font-semibold">Z</h4>
        <span class="text-xs text-muted-foreground"
          >{t(
            bedOnZ
              ? 'printerDetailControls.coordinates.bedZ'
              : 'printerDetailControls.coordinates.nozzleZ',
          )}</span
        >
      </div>
      <div
        class="z-layout mb-3 grid flex-1 justify-items-center gap-2"
        role="group"
        aria-labelledby="{uid}-z"
      >
        {@render direction('z', 1, ArrowUpIcon)}
        <div class="z-ruler relative flex w-20 justify-center px-6">
          <span
            aria-hidden="true"
            class="absolute end-0 top-0 font-mono text-xs text-muted-foreground"
            >{limits.z}</span
          >
          <span
            aria-hidden="true"
            class="absolute end-0 bottom-0 font-mono text-xs text-muted-foreground"
            >0</span
          >
          {#key zSliderGeneration}
            <Slider
              type="single"
              tone="primary"
              orientation="vertical"
              min={0}
              max={limits.z}
              step={0.1}
              value={zTarget ?? position.z ?? 0}
              disabled={zDisabled || xyDrag !== undefined || busy}
              aria-label={t('printerDetailControls.coordinates.zSlider')}
              onValueChange={(value: number) => {
                if (zDisabled || xyDrag || busy) return;
                zStart ??= position.z;
                zTarget = value;
              }}
              onValueCommit={commitHeight}
              onpointercancel={cancelHeightDrag}
            />
          {/key}
        </div>
        {@render direction('z', -1, ArrowDownIcon)}
      </div>
      <dl
        aria-labelledby="{uid}-position {uid}-z"
        class="border-t border-border py-2"
      >
        {@render positionReading('z')}
      </dl>
    </section>
  </div>

  {#if commandPanel}
    {@render commandPanel(homeAction)}
  {:else}
    <footer class="flex flex-wrap items-center gap-2">
      {@render homeAction()}
    </footer>
  {/if}
  <p
    class={localFeedback || feedback
      ? 'text-sm text-muted-foreground'
      : 'sr-only'}
    role="status"
  >
    {localFeedback ?? feedback}
  </p>
</div>

<style>
  .motion-coordinates {
    container-type: inline-size;
  }

  .motion-layout {
    grid-template-columns: minmax(0, 1fr) minmax(8rem, 0.32fr);
  }

  .xy-layout {
    grid-template-columns: auto minmax(0, 17rem) auto;
  }

  .z-layout {
    grid-template-rows: auto 1fr auto;
  }

  .plate {
    min-block-size: 9rem;
    background-color: var(--card);
    background-image:
      linear-gradient(to right, var(--border) 1px, transparent 1px),
      linear-gradient(to bottom, var(--border) 1px, transparent 1px);
    background-size: 10% 10%;
  }

  .nozzle {
    position: absolute;
    inset-inline-start: calc(var(--x) * 100%);
    inset-block-start: calc(var(--y) * 100%);
    translate: -50% -50%;
    transition:
      inset-inline-start 160ms ease-out,
      inset-block-start 160ms ease-out;
  }

  .nozzle[data-dragging='true'] {
    transition: none;
  }

  .nozzle :global(.nozzle-button) {
    inline-size: 2.75rem;
    block-size: 2.75rem;
    padding: 0;
    touch-action: none;
    cursor: grab;
    border-radius: 50%;
    background: transparent;
  }

  .nozzle[data-dragging='true'] :global(.nozzle-button) {
    cursor: grabbing;
  }

  .nozzle-grip {
    display: grid;
    place-items: center;
    inline-size: 1.5rem;
    block-size: 1.5rem;
    border: 2px solid var(--primary);
    border-radius: 50%;
    background: var(--background);
  }

  .nozzle-grip span {
    inline-size: 0.5rem;
    block-size: 0.5rem;
    border-radius: 50%;
    background: var(--primary);
  }

  .nozzle :global(.nozzle-button:disabled) {
    cursor: default;
  }

  .nozzle[data-stale='true'] .nozzle-grip {
    border-color: var(--muted-foreground);
  }

  .nozzle[data-stale='true'] .nozzle-grip span {
    background: var(--muted-foreground);
  }

  .z-ruler::before {
    content: '';
    position: absolute;
    inset-block: 0;
    inset-inline-start: 50%;
    inline-size: 1rem;
    pointer-events: none;
    background-image: repeating-linear-gradient(
      to bottom,
      var(--border) 0 1px,
      transparent 1px 10%
    );
  }

  .z-ruler :global([data-slot='slider']) {
    inline-size: 2.75rem;
    min-block-size: 7.5rem;
  }

  .z-ruler :global([data-slot='slider-range']) {
    display: none;
  }

  .z-ruler :global([data-slot='slider-thumb']) {
    inline-size: 1.75rem;
    block-size: 1rem;
    border-width: 2px;
    cursor: ns-resize;
  }

  /* Tighter than the `md` button's inset, so the axis label fits a grid cell. */
  .motion-layout :global(.jog-button) {
    padding-inline: 0.5rem;
  }

  .motion-layout :global(.jog-button[data-moving='true']) {
    background: var(--primary);
    color: var(--primary-foreground);
  }

  @container (max-width: 26rem) {
    .motion-layout {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @container (max-width: 32rem) {
    .motion-layout :global(.jog-button) {
      inline-size: 3.25rem;
      padding-inline: 0.25rem;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .nozzle {
      transition: none;
    }
  }
</style>
