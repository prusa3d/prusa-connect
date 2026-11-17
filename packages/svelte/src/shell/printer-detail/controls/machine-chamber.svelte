<!-- Chamber: how hard the enclosure fan runs, and how bright its light is. -->
<script lang="ts">
  import {
    CHAMBER_FAN_PATH,
    CHAMBER_LIGHT_PATH,
    CHAMBER_TARGET_PATH,
  } from '../../../domain';
  import { t } from '../../../i18n';
  import {
    ChamberIcon,
    SliderField,
    ToggleGroup,
    ToggleGroupItem,
  } from '../../../primitives';
  import {
    blocked,
    FAN_AUTOMATIC,
    FAN_OFF,
    groupReason,
    PERCENT_MAX,
    READING_TILE,
    SECTION_TITLE,
    VERBS,
    type SectionProps,
  } from './machine-controls';

  type FanMode = 'off' | 'auto' | 'manual';

  let { resolve, reading, busy, onCommand }: SectionProps = $props();

  const fan = $derived(resolve(VERBS.chamberFan));
  const light = $derived(resolve(VERBS.chamberLight));
  const drawn = $derived(fan.kind !== 'absent' || light.kind !== 'absent');
  const reason = $derived(groupReason([fan, light]));

  const fanBlocked = $derived(blocked(fan, busy));
  const lightBlocked = $derived(blocked(light, busy));

  const uid = $props.id();

  // A CORE One reports its fan target and its light; an XL takes the light
  // and reports nothing of it. Each control shows what the printer reports,
  // what a person picked until the next report, or the firmware's default.
  const reportedFan = $derived(reading(CHAMBER_FAN_PATH));
  const reportedLight = $derived(reading(CHAMBER_LIGHT_PATH));

  const modeOf = (fan: number | undefined): FanMode => {
    if (fan === undefined || fan < 0) return 'auto';

    return fan === FAN_OFF ? 'off' : 'manual';
  };

  let mode = $derived<FanMode>(modeOf(reportedFan));

  // The speed Manual starts from: the last one above Off the printer reported
  // or a person picked, so Off and back to Manual returns to it rather than to
  // full speed. It is a plain variable rather than state: it changes only as
  // a side note of a report or a pick, and nothing draws it.
  let lastSpeed = PERCENT_MAX;
  let fanPercent = $derived.by(() => {
    if (reportedFan !== undefined && reportedFan > FAN_OFF) {
      lastSpeed = reportedFan;
    }

    return lastSpeed;
  });
  let lightPercent = $derived(reportedLight ?? PERCENT_MAX);

  // The fans hold the chamber under its target only while they run on their
  // own, so a target set does nothing while they are set by hand.
  const targetIgnored = $derived(
    mode !== 'auto' &&
      resolve(VERBS.chamberTemp).kind !== 'absent' &&
      (reading(CHAMBER_TARGET_PATH) ?? 0) > 0,
  );

  const sendFan = (target: number) =>
    onCommand(VERBS.chamberFan, {
      'chamber.fan_pwm_target': String(Math.round(target)),
    });

  const chooseMode = (next: FanMode) => {
    mode = next;
    if (next === 'off') sendFan(FAN_OFF);
    else if (next === 'auto') sendFan(FAN_AUTOMATIC);
    else sendFan(fanPercent);
  };

  // The group hands back a string.
  const onMode = (next: string | undefined) => {
    if (next === 'off' || next === 'auto' || next === 'manual') {
      chooseMode(next);
    }
  };

  const commitFan = (next: number) => {
    if (next > FAN_OFF) lastSpeed = next;
    fanPercent = next;
    sendFan(next);
  };

  const commitLight = (next: number) => {
    lightPercent = next;
    onCommand(VERBS.chamberLight, {
      'chamber.led_intensity': String(Math.round(next)),
    });
  };
</script>

{#if drawn}
  <section class="flex flex-col gap-3 {READING_TILE}">
    <h3 class={SECTION_TITLE}>
      <ChamberIcon aria-hidden="true" class="size-4 text-muted-foreground" />
      {t('printerDetailControls.chamber.heading')}
    </h3>

    {#if fan.kind !== 'absent'}
      <div class="flex flex-col gap-2">
        <span id="{uid}-fan" class="text-xs font-medium text-muted-foreground">
          {t('printerDetailControls.chamber.fan')}
        </span>
        <ToggleGroup
          type="single"
          variant="outline"
          size="lg"
          value={mode}
          onValueChange={onMode}
          aria-labelledby="{uid}-fan"
          disabled={fanBlocked}
        >
          <ToggleGroupItem value="off">
            {t('printerDetailControls.chamber.off')}
          </ToggleGroupItem>
          <ToggleGroupItem value="auto">
            {t('printerDetailControls.chamber.auto')}
          </ToggleGroupItem>
          <ToggleGroupItem value="manual">
            {t('printerDetailControls.chamber.manual')}
          </ToggleGroupItem>
        </ToggleGroup>

        {#if targetIgnored}
          <p class="text-xs text-muted-foreground">
            {t('printerDetailControls.chamber.targetIgnored')}
          </p>
        {/if}

        {#if mode === 'manual'}
          <SliderField
            value={fanPercent}
            onCommit={commitFan}
            min={FAN_OFF}
            max={PERCENT_MAX}
            unit="%"
            label={t('printerDetailControls.chamber.fanSpeed')}
            disabled={fanBlocked}
          />
        {/if}
      </div>
    {/if}

    {#if light.kind !== 'absent'}
      <div class="flex flex-col gap-2">
        <span class="text-xs font-medium text-muted-foreground">
          {t('printerDetailControls.chamber.light')}
        </span>
        <SliderField
          value={lightPercent}
          onCommit={commitLight}
          min={0}
          max={PERCENT_MAX}
          unit="%"
          label={t('printerDetailControls.chamber.light')}
          disabled={lightBlocked}
        />
      </div>
    {/if}

    {#if reason}
      <p class="text-xs text-muted-foreground">{reason}</p>
    {/if}
  </section>
{/if}
