<script lang="ts">
  import { Badge, Card, CardContent } from '@prusa/connect-ui';
  import type { Device, PrinterState } from '@prusa/connect-ui/domain';
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import MotionCoordinates from '../../../packages/svelte/src/shell/printer-detail/controls/motion-coordinates.svelte';
  import type { Position } from '../../../packages/svelte/src/shell/printer-detail/controls/motion-coordinates';

  let {
    device,
    state: printerState = 'IDLE',
    embedded = false,
    commandPanel,
  }: {
    device?: Device;
    state?: PrinterState;
    embedded?: boolean;
    commandPanel?: Snippet<[Snippet]> | undefined;
  } = $props();

  // Older fleet fixtures omit build volume. Use the demo models' dimensions
  // from internal/connector/xbuddy/buildvolume.go in that case.
  const mk4sVolume = { x: 250, y: 210, z: 220 };
  const demoVolumes: Partial<Record<string, Position>> = {
    MK4S: mk4sVolume,
    MINI: { x: 180, y: 180, z: 180 },
    CoreOne: { x: 250, y: 220, z: 270 },
  };
  const volume = $derived(device?.build_volume);
  const limits = $derived(
    volume
      ? { x: volume.x_mm, y: volume.y_mm, z: volume.z_mm }
      : (demoVolumes[device?.model ?? 'MK4S'] ?? mk4sVolume),
  );
  const blocked = $derived(printerState !== 'IDLE' && printerState !== 'READY');
  const error = $derived(printerState === 'ERROR');
  const reason = $derived.by(() => {
    if (error)
      return 'Clear the printer error before moving or homing the axes.';
    if (blocked) return `The printer is in state ${printerState}.`;
    return undefined;
  });
  const origin: Position = { x: 0, y: 0, z: 0 };

  let position = $state<Position>(
    untrack(() => ({
      x: limits.x / 2,
      y: limits.y / 2,
      z: Math.min(24, limits.z),
    })),
  );
  let pending = $state(false);
  let feedback = $state<string>();
  let confirmation: ReturnType<typeof setTimeout> | undefined;

  function simulate(next: Partial<Position>) {
    pending = true;
    feedback = 'Moving…';
    confirmation = setTimeout(() => {
      if (!blocked) position = { ...position, ...next };
      feedback = 'Position confirmed';
      pending = false;
    }, 180);
  }

  onDestroy(() => clearTimeout(confirmation));
</script>

{#snippet controls()}
  <MotionCoordinates
    {limits}
    {position}
    xyDisabled={blocked}
    zDisabled={blocked}
    homeDisabled={blocked}
    busy={pending}
    {reason}
    {feedback}
    simulated
    {commandPanel}
    onMove={simulate}
    onHome={() => simulate(origin)}
  />
{/snippet}

{#if embedded}
  {@render controls()}
{:else}
  <main class="mx-auto flex max-w-4xl flex-col gap-4 p-4 sm:p-6">
    <p class="text-sm text-muted-foreground">Printers / MK4S DEMO-09</p>
    <div class="flex flex-wrap items-center gap-3">
      <h1 class="text-xl font-semibold">MK4S DEMO-09</h1>
      <Badge variant={error ? 'stateFault' : 'stateIdle'}
        >{error ? 'ERROR' : 'IDLE'}</Badge
      >
    </div>

    <Card>
      <CardContent>
        {@render controls()}
      </CardContent>
    </Card>
  </main>
{/if}
