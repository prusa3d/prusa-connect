<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';

  import {
    AXIS_X_PATH,
    AXIS_Y_PATH,
    AXIS_Z_PATH,
    type Device,
    type PrinterState,
  } from '../../../domain';
  import { formatNumber, formatUnit, t } from '../../../i18n';
  import {
    ArrowDownIcon,
    ArrowLeftIcon,
    ArrowRightIcon,
    ArrowUpIcon,
    Button,
    ConfirmDialog,
    DisableSteppersIcon,
    HomeIcon,
    MeshBedLevelingIcon,
    RestartIcon,
    ToggleGroup,
    ToggleGroupItem,
    ZAxisIcon,
  } from '../../../primitives';
  import {
    blocked,
    groupReason,
    refusalText,
    JOG_STEPS_MM,
    JOG_XY_FEEDRATE,
    JOG_Z_FEEDRATE,
    SECTION_TITLE,
    VERBS,
    type SectionProps,
  } from './machine-controls';
  import MachineUtilities from './machine-utilities.svelte';
  import MotionCoordinates from './motion-coordinates.svelte';
  import type { Axis, Position } from './motion-coordinates';

  let {
    resolve,
    reading,
    busy,
    onCommand,
    printing,
    bedOnZ,
    motion,
    buildVolume,
    printerState,
    homing,
    coordinatesOnly = false,
  }: SectionProps & {
    printing: boolean;
    bedOnZ: boolean;
    motion?: Snippet<[Snippet<[Snippet]>]> | undefined;
    buildVolume?: Device['build_volume'];
    printerState: PrinterState;
    homing: boolean;
    coordinatesOnly?: boolean;
  } = $props();

  const move = $derived(resolve(VERBS.move));
  const moveZ = $derived(resolve(VERBS.moveZ));
  const home = $derived(resolve(VERBS.home));
  const disableSteppers = $derived(resolve(VERBS.disableSteppers));
  const levelling = $derived(resolve(VERBS.meshBedLevelling));
  const reset = $derived(resolve(VERBS.resetPrinter));
  const hasAxisActions = $derived(
    home.kind !== 'absent' ||
      disableSteppers.kind !== 'absent' ||
      levelling.kind !== 'absent',
  );
  // Where the plate is drawn it states the reason itself: the group refuses
  // only when every verb in it does, move and move_z included.
  const reason = $derived(
    groupReason([move, moveZ, home, disableSteppers, levelling]),
  );

  const motionBusy = $derived(busy || homing);
  const xyBlocked = $derived(blocked(move, motionBusy));
  const zBlocked = $derived(blocked(moveZ, motionBusy));

  const limits = $derived.by(() => {
    if (!buildVolume) return undefined;
    const { x_mm: x, y_mm: y, z_mm: z } = buildVolume;
    return [x, y, z].every((value) => Number.isFinite(value) && value > 0)
      ? { x, y, z }
      : undefined;
  });
  const exactPosition = $derived({
    x: reading(AXIS_X_PATH),
    y: reading(AXIS_Y_PATH),
    z: reading(AXIS_Z_PATH),
  });
  const rangeOK = (axis: Axis, position: number | undefined) =>
    limits !== undefined &&
    position !== undefined &&
    Number.isFinite(position) &&
    position >= 0 &&
    position <= limits[axis];
  const coordinateXyDisabled = $derived(
    xyBlocked ||
      !rangeOK('x', exactPosition.x) ||
      !rangeOK('y', exactPosition.y),
  );
  const coordinateZDisabled = $derived(
    zBlocked || !rangeOK('z', exactPosition.z),
  );
  const coordinateHomeDisabled = $derived(blocked(home, motionBusy));
  const coordinateReason = $derived.by(() => {
    if (coordinateXyDisabled && coordinateZDisabled && !motionBusy) {
      const refusal = [move, moveZ].find((item) => item.kind === 'disabled');
      if (refusal?.kind === 'disabled') return refusalText(refusal.reason);
      if (move.kind === 'absent' && moveZ.kind === 'absent')
        return t('printerDetailControls.coordinates.unsupported');
      return t('printerDetailControls.coordinates.noPosition');
    }
    // One axis can be off by its reading alone while the other still moves.
    const outOfRange =
      (!xyBlocked &&
        (!rangeOK('x', exactPosition.x) || !rangeOK('y', exactPosition.y))) ||
      (!zBlocked && !rangeOK('z', exactPosition.z));
    if (outOfRange && !motionBusy)
      return t('printerDetailControls.coordinates.noPosition');
    return undefined;
  });

  const positionTolerance = 0.01;
  // Past the travel itself, the wait covers acceleration and the time the
  // reported position takes to arrive.
  const CONFIRMATION_MARGIN_MS = 12_000;
  // The time a move takes at the feedrate it was sent with, in mm/min.
  const travelMs = (args: Record<string, string> | undefined): number => {
    const feedrate = Number(args?.feedrate);
    const length = Math.hypot(
      Number(args?.x ?? 0),
      Number(args?.y ?? 0),
      Number(args?.distance ?? 0),
    );
    const ms = (length / feedrate) * 60_000;
    return Number.isFinite(ms) && ms > 0 ? ms : 0;
  };
  let expected = $state<Partial<Position>>();
  let sendingMove = $state(false);
  let coordinateFeedback = $state<string>();
  let confirmation: ReturnType<typeof setTimeout> | undefined;
  const matches = $derived(
    expected !== undefined &&
      (['x', 'y', 'z'] as const).every((axis) => {
        const target = expected?.[axis];
        const actual = exactPosition[axis];
        return (
          target === undefined ||
          (actual !== undefined &&
            Math.abs(actual - target) <= positionTolerance)
        );
      }),
  );

  function settle(feedback: string) {
    clearTimeout(confirmation);
    expected = undefined;
    coordinateFeedback = feedback;
  }

  // A reading on the way to the target holds the controls: a jog worked out
  // from it would add to a move the printer has not finished.
  $effect(() => {
    if (matches && !sendingMove)
      settle('printerDetailControls.coordinates.confirmed');
  });

  let mounted = true;
  onDestroy(() => {
    mounted = false;
    clearTimeout(confirmation);
  });

  async function request(
    verb: string,
    args?: Record<string, string>,
    target?: Partial<Position>,
  ) {
    if (
      sendingMove ||
      expected ||
      motionBusy ||
      blocked(resolve(verb), motionBusy)
    )
      return;
    sendingMove = true;
    expected = target;
    coordinateFeedback = 'printerDetailControls.coordinates.sending';
    try {
      const result = await onCommand(verb, args);
      if (!mounted) return;
      if (result?.kind === 'refused' || result?.kind === 'unsent') {
        expected = undefined;
        coordinateFeedback = 'printerDetailControls.coordinates.notConfirmed';
        return;
      }
      coordinateFeedback = target
        ? 'printerDetailControls.coordinates.waiting'
        : 'printerDetailControls.coordinates.homeAccepted';
      if (target)
        confirmation = setTimeout(
          () => {
            settle('printerDetailControls.coordinates.timeout');
          },
          travelMs(args) + CONFIRMATION_MARGIN_MS,
        );
    } catch {
      expected = undefined;
      coordinateFeedback = 'printerDetailControls.coordinates.notConfirmed';
    } finally {
      sendingMove = false;
    }
  }

  function moveTo(target: Partial<Position>) {
    const delta = (axis: Axis) => {
      const from = exactPosition[axis];
      const to = target[axis];
      return to !== undefined && rangeOK(axis, to) && from !== undefined
        ? Number((to - from).toFixed(3))
        : 0;
    };
    const z = delta('z');
    if (target.z !== undefined) {
      if (!coordinateZDisabled && z)
        void request(
          VERBS.moveZ,
          { feedrate: String(JOG_Z_FEEDRATE), distance: String(z) },
          { z: target.z },
        );
      return;
    }
    const x = delta('x');
    const y = delta('y');
    if (coordinateXyDisabled || (!x && !y)) return;
    void request(
      VERBS.move,
      {
        feedrate: String(JOG_XY_FEEDRATE),
        ...(x ? { x: String(x) } : {}),
        ...(y ? { y: String(y) } : {}),
      },
      { ...(x ? { x: target.x } : {}), ...(y ? { y: target.y } : {}) },
    );
  }

  let step = $state(1);

  const uid = $props.id();

  const millimetres = (value: number | undefined, digits: number) =>
    value === undefined
      ? undefined
      : formatUnit(value, 'mm', {
          minimumFractionDigits: digits,
          maximumFractionDigits: digits,
        });

  const xy = $derived.by(() => {
    if (xyBlocked) return undefined;
    const parts = [
      millimetres(reading(AXIS_X_PATH), 1),
      millimetres(reading(AXIS_Y_PATH), 1),
    ].filter((part) => part !== undefined);

    return parts.length === 0 ? undefined : parts.join(' · ');
  });
  const z = $derived(millimetres(reading(AXIS_Z_PATH), 2));

  const jogXy = (axis: 'x' | 'y', direction: 1 | -1) =>
    onCommand(VERBS.move, {
      feedrate: String(JOG_XY_FEEDRATE),
      [axis]: String(direction * step),
    });

  const jogZ = (direction: 1 | -1) =>
    onCommand(VERBS.moveZ, {
      feedrate: String(JOG_Z_FEEDRATE),
      distance: String(direction * step),
    });

  // The up arrow moves up what the reader sees move: the bed comes up as Z
  // falls, and the nozzle goes up as Z grows.
  const zUp = $derived(
    bedOnZ
      ? { label: t('printerDetailControls.jog.bedUp'), direction: -1 as const }
      : { label: t('printerDetailControls.jog.zUp'), direction: 1 as const },
  );
  const zDown = $derived(
    bedOnZ
      ? { label: t('printerDetailControls.jog.bedDown'), direction: 1 as const }
      : { label: t('printerDetailControls.jog.zDown'), direction: -1 as const },
  );
</script>

{#snippet axisLabel(id: string, label: string, value: string | undefined)}
  <span
    {id}
    class="flex items-baseline gap-1.5 text-xs font-medium text-muted-foreground"
  >
    {label}
    {#if value !== undefined}
      <span class="font-mono text-foreground tabular-nums">{value}</span>
    {/if}
  </span>
{/snippet}

<section class="flex flex-col gap-3 @3xl:col-start-1 @3xl:row-start-1">
  {#if motion}
    {@render motion(coordinatesOnly ? noAxisActions : commandPanel)}
  {:else if limits}
    <MotionCoordinates
      {limits}
      position={exactPosition}
      xyDisabled={coordinateXyDisabled}
      zDisabled={coordinateZDisabled}
      homeDisabled={coordinateHomeDisabled}
      busy={motionBusy || sendingMove || expected !== undefined}
      {bedOnZ}
      reason={coordinateReason}
      feedback={coordinateFeedback ? t(coordinateFeedback) : undefined}
      commandPanel={coordinatesOnly ? noAxisActions : commandPanel}
      onMove={moveTo}
      onHome={() => {
        void request(VERBS.home);
      }}
    />
  {:else if !coordinatesOnly}
    <h3 class={SECTION_TITLE}>
      <ZAxisIcon aria-hidden="true" class="size-4 text-muted-foreground" />
      {t('printerDetailControls.motion.heading')}
    </h3>

    {#if move.kind !== 'absent' || moveZ.kind !== 'absent'}
      <div class="flex flex-col gap-1">
        <span id="{uid}-step" class="text-xs font-medium text-muted-foreground">
          {t('printerDetailControls.jog.step')}
        </span>
        <ToggleGroup
          type="single"
          variant="outline"
          tone="primary"
          size="lg"
          value={String(step)}
          onValueChange={(next) => {
            if (next) step = Number(next);
          }}
          aria-labelledby="{uid}-step"
        >
          {#each JOG_STEPS_MM as millimetre (millimetre)}
            <!-- The cell shows the bare number; its accessible name carries the unit. -->
            <ToggleGroupItem
              value={String(millimetre)}
              aria-label={formatUnit(millimetre, 'mm')}
            >
              {formatNumber(millimetre)}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>

      <div class="flex flex-wrap items-start gap-3">
        {#if move.kind !== 'absent'}
          <div class="flex flex-col gap-1">
            {@render axisLabel(
              `${uid}-xy`,
              t('printerDetailControls.jog.xy.label'),
              xy,
            )}
            <div
              role="group"
              aria-labelledby="{uid}-xy"
              class="grid grid-cols-3 grid-rows-3 gap-1"
            >
              <div></div>
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={t('printerDetailControls.jog.yForward')}
                disabled={xyBlocked}
                onclick={() => jogXy('y', 1)}
              >
                <ArrowUpIcon aria-hidden="true" />
              </Button>
              <div></div>
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={t('printerDetailControls.jog.xLeft')}
                disabled={xyBlocked}
                onclick={() => jogXy('x', -1)}
              >
                <ArrowLeftIcon aria-hidden="true" />
              </Button>
              <div></div>
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={t('printerDetailControls.jog.xRight')}
                disabled={xyBlocked}
                onclick={() => jogXy('x', 1)}
              >
                <ArrowRightIcon aria-hidden="true" />
              </Button>
              <div></div>
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={t('printerDetailControls.jog.yBack')}
                disabled={xyBlocked}
                onclick={() => jogXy('y', -1)}
              >
                <ArrowDownIcon aria-hidden="true" />
              </Button>
              <div></div>
            </div>
          </div>
        {/if}

        {#if moveZ.kind !== 'absent'}
          <div class="flex flex-col gap-1">
            {@render axisLabel(
              `${uid}-z`,
              t('printerDetailControls.jog.z.label'),
              z,
            )}
            <div
              role="group"
              aria-labelledby="{uid}-z"
              class="flex flex-col gap-1"
            >
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={zUp.label}
                disabled={zBlocked}
                onclick={() => jogZ(zUp.direction)}
              >
                <ArrowUpIcon aria-hidden="true" />
              </Button>
              <Button
                variant="secondary"
                size="icon-md"
                class="size-12"
                label={zDown.label}
                disabled={zBlocked}
                onclick={() => jogZ(zDown.direction)}
              >
                <ArrowDownIcon aria-hidden="true" />
              </Button>
            </div>
          </div>
        {/if}
      </div>
    {/if}
  {/if}

  {#if !coordinatesOnly && !motion && !limits}
    {@render commandPanel(hasAxisActions ? fallbackHomeAction : undefined)}
  {/if}
</section>

{#snippet noAxisActions()}{/snippet}

{#snippet commandPanel(homeAction: Snippet | undefined)}
  {#snippet motionActions()}
    {@render homeAction?.()}
    {@render axisActions()}
  {/snippet}

  <MachineUtilities
    {resolve}
    {busy}
    {onCommand}
    ready={printerState === 'READY'}
    motionActions={homeAction ? motionActions : undefined}
    motionReason={limits && !motion ? undefined : reason}
    resetAction={reset.kind !== 'absent' ? resetAction : undefined}
  />
{/snippet}

{#snippet fallbackHomeAction()}
  {#if home.kind !== 'absent'}
    <Button
      variant="secondary"
      size="compact"
      disabled={blocked(home, motionBusy)}
      onclick={() => onCommand(VERBS.home)}
    >
      <HomeIcon aria-hidden="true" />
      {t('printerDetailControls.home.label')}
    </Button>
  {/if}
{/snippet}

{#snippet axisActions()}
  {#if disableSteppers.kind !== 'absent'}
    <Button
      variant="secondary"
      size="compact"
      disabled={blocked(disableSteppers, motionBusy)}
      onclick={() => onCommand(VERBS.disableSteppers)}
    >
      <DisableSteppersIcon aria-hidden="true" />
      {t('printerDetailControls.disableSteppers.label')}
    </Button>
  {/if}
  {#if levelling.kind !== 'absent'}
    <Button
      variant="secondary"
      size="compact"
      disabled={blocked(levelling, busy)}
      onclick={() => onCommand(VERBS.meshBedLevelling)}
    >
      <MeshBedLevelingIcon aria-hidden="true" />
      {t('printerDetailControls.meshBedLevelling')}
    </Button>
  {/if}
{/snippet}

{#snippet resetAction()}
  {#if reset.kind !== 'absent'}
    <ConfirmDialog
      trigger={resetTrigger}
      title={t('printerDetailControls.reset.title')}
      description={printing
        ? t('printerDetailControls.reset.printing')
        : t('printerDetailControls.reset.idle')}
      confirmLabel={t('printerDetailControls.reset.label')}
      onConfirm={() => onCommand(VERBS.resetPrinter)}
    />
  {/if}
{/snippet}

{#snippet resetTrigger({ props }: { props: Record<string, unknown> })}
  <Button
    {...props}
    variant="destructive"
    size="md"
    disabled={blocked(reset, motionBusy)}
  >
    <RestartIcon aria-hidden="true" />
    {t('printerDetailControls.reset.label')}
  </Button>
{/snippet}
