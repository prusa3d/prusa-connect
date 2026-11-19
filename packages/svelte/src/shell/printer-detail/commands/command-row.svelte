<script lang="ts">
  import ArrowDownToLineIcon from '@lucide/svelte/icons/arrow-down-to-line';
  import ArrowUpFromLineIcon from '@lucide/svelte/icons/arrow-up-from-line';
  import AudioLinesIcon from '@lucide/svelte/icons/audio-lines';
  import BoxesIcon from '@lucide/svelte/icons/boxes';
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
  import CircleMinusIcon from '@lucide/svelte/icons/circle-minus';
  import CircleIcon from '@lucide/svelte/icons/circle';
  import FanIcon from '@lucide/svelte/icons/fan';
  import PauseIcon from '@lucide/svelte/icons/pause';
  import PlayIcon from '@lucide/svelte/icons/play';
  import SettingsIcon from '@lucide/svelte/icons/settings';
  import SquareIcon from '@lucide/svelte/icons/square';

  import { formatList, t } from '../../../i18n';
  import {
    Button,
    ConfirmDialog,
    Field,
    Input,
    ChamberIcon,
    DisableSteppersIcon,
    HeatbedIcon,
    HomeIcon,
    MeshBedLevelingIcon,
    NozzleIcon,
    RestartIcon,
    SpeedIcon,
    SliderField,
    ZAxisIcon,
  } from '../../../primitives';
  import {
    TUNING_REFERENCE_PERCENT,
    VERBS,
  } from '../controls/machine-controls';
  import bulb from './assets/prusa-slicer-bulb.svg';
  import type { CommandRowProps, TuningArgument } from './types';
  import {
    descriptionKeyOf,
    groupOf,
    haltsTheMachine,
    verbText,
  } from './vocabulary';

  let {
    entry,
    projection,
    marked,
    sending,
    pending,
    readingDrive = false,
    onSend,
    tuning,
  }: CommandRowProps = $props();

  const icons: Partial<Record<string, typeof PauseIcon | typeof HomeIcon>> = {
    beep: AudioLinesIcon,
    cancel_object: BoxesIcon,
    cancel_printer_ready: CircleMinusIcon,
    disable_steppers: DisableSteppersIcon,
    home: HomeIcon,
    load_filament: ArrowDownToLineIcon,
    mesh_bed_leveling: MeshBedLevelingIcon,
    move_e: NozzleIcon,
    pause_print: PauseIcon,
    reset_printer: RestartIcon,
    resume_print: PlayIcon,
    set_bed_temperature: HeatbedIcon,
    set_chamber_target_temp: ChamberIcon,
    set_chamber_fan_target: FanIcon,
    set_flow: NozzleIcon,
    set_heatbed_temperature: HeatbedIcon,
    set_idle: CircleIcon,
    set_nozzle_temperature: NozzleIcon,
    set_printer_ready: CircleCheckIcon,
    set_speed: SpeedIcon,
    stop_print: SquareIcon,
    uncancel_object: BoxesIcon,
    unload_filament: ArrowUpFromLineIcon,
  };
  const groupIcons: Partial<Record<string, typeof HomeIcon>> = {
    movement: ZAxisIcon,
    chamber: ChamberIcon,
  };
  const Icon = $derived.by(() => {
    const key = entry.verb.slice('core.command.'.length);
    const icon = Object.hasOwn(icons, key) ? icons[key] : undefined;
    return icon ?? groupIcons[groupOf(entry.verb)] ?? SettingsIcon;
  });
  const uid = $props.id();

  let draft = $state<Record<string, string>>({});

  const percentMin = 20;
  const percentMax = 999;
  let percent = $state(TUNING_REFERENCE_PERCENT);
  const tuningArgument = $derived.by((): TuningArgument | undefined => {
    if (entry.args.length !== 1) return undefined;
    if (entry.verb === VERBS.speed && entry.args[0] === 'speed') return 'speed';
    if (entry.verb === VERBS.flow && entry.args[0] === 'flow') return 'flow';
    return undefined;
  });

  function resetTuning(open: boolean) {
    if (!open || tuningArgument === undefined) return;
    const current = tuning?.[tuningArgument] ?? TUNING_REFERENCE_PERCENT;
    percent = Math.min(percentMax, Math.max(percentMin, Math.round(current)));
  }

  const refusal = $derived(
    projection.kind === 'disabled' ? projection.reason : undefined,
  );

  // The refusal is a catalogue key and the states are a list.
  const reason = $derived(
    refusal === undefined
      ? undefined
      : t(
          refusal.key,
          refusal.states === undefined
            ? undefined
            : { states: formatList([...refusal.states]) },
        ),
  );

  const blocked = $derived(projection.kind !== 'available' || sending);

  // An argument the printer expects has to be filled in before the send.
  const asks = $derived(entry.destructive || entry.args.length > 0);

  const name = $derived(verbText(entry.verb, true));

  const confirmation = $derived.by(() => {
    if (tuningArgument)
      return t('printerDetailCommands.confirm.tuning', {
        min: percentMin,
        max: percentMax,
      });
    if (entry.verb === 'core.command.stop_print')
      return t('printerDetailStatus.stop.detail');
    return entry.destructive
      ? t('printerDetailCommands.confirm.destructive')
      : t('printerDetailCommands.confirm.arguments');
  });

  const description = $derived.by(() => {
    const key = descriptionKeyOf(entry.verb);

    return key === undefined ? undefined : t(key);
  });

  // The colour warns about the machine stopping where it stands.
  const variant = $derived.by(() => {
    if (haltsTheMachine(entry.verb) || entry.destructive) return 'destructive';
    if (
      !blocked &&
      (entry.verb === 'core.command.pause_print' ||
        entry.verb === 'core.command.resume_print')
    )
      return 'default';
    return 'secondary';
  });

  const write = (argument: string) => (event: Event) => {
    const field = event.currentTarget;
    if (field instanceof HTMLInputElement) draft[argument] = field.value;
  };

  const send = () => {
    if (blocked) return;
    if (tuningArgument) {
      onSend(entry.verb, { [tuningArgument]: String(percent) });
      return;
    }
    onSend(entry.verb, entry.args.length === 0 ? undefined : { ...draft });
  };
</script>

<!-- The id is the verb, so another surface can link straight to one row. -->
<div
  id="command-{entry.verb}"
  data-slot="command-row"
  tabindex="-1"
  aria-current={marked ? true : undefined}
  title={reason ?? description}
  class={[
    'max-w-full rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
    marked && 'ring-2 ring-ring ring-inset',
  ]}
>
  <span id="{uid}-detail" class="sr-only">{reason ?? description}</span>
  <div>
    {#if blocked || !asks}
      <Button
        size="compact"
        {variant}
        disabled={blocked}
        pending={pending === entry.verb}
        class="max-w-full whitespace-normal"
        aria-describedby={reason || description ? `${uid}-detail` : undefined}
        onclick={blocked ? undefined : send}
      >
        {@render buttonText()}
      </Button>
    {:else}
      <ConfirmDialog
        trigger={triggerButton}
        title={t('printerDetailCommands.confirm.title', { verb: name })}
        description={confirmation}
        confirmLabel={name}
        onConfirm={send}
        onOpenChange={resetTuning}
        variant={tuningArgument ? 'confirm' : 'destructive'}
      >
        {#if tuningArgument}
          <SliderField
            value={percent}
            onCommit={(next: number) => {
              percent = Math.round(next);
            }}
            min={percentMin}
            max={percentMax}
            step={1}
            unit="%"
            label={name}
          />
        {:else}
          {#each entry.args as argument (argument)}
            <Field
              label={argument}
              control={Input}
              controlProps={{
                value: draft[argument] ?? '',
                oninput: write(argument),
                class: 'font-mono',
              }}
            />
          {/each}
        {/if}
      </ConfirmDialog>
    {/if}
    {#if readingDrive && pending === entry.verb}
      <p class="text-xs text-muted-foreground">
        {t('printerDetail.readingDriveHint')}
      </p>
    {/if}
  </div>
</div>

{#snippet triggerButton({ props }: { props: Record<string, unknown> })}
  <Button
    {...props}
    size="compact"
    {variant}
    pending={pending === entry.verb}
    class="max-w-full whitespace-normal"
    aria-describedby={reason || description ? `${uid}-detail` : undefined}
  >
    {@render buttonText()}
  </Button>
{/snippet}

{#snippet buttonText()}
  {#if pending !== entry.verb}
    {#if entry.verb === 'core.command.set_chamber_led_intensity'}
      <span
        aria-hidden="true"
        class="slicer-icon size-4 shrink-0 bg-current"
        style:--icon={`url("${bulb}")`}
      ></span>
    {:else}
      <Icon aria-hidden="true" />
    {/if}
  {/if}
  {name}
{/snippet}

<style>
  .slicer-icon {
    mask: var(--icon) center no-repeat;
  }
</style>
