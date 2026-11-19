<!--
  The settings of a temperature card: the form every card draws, with the
  lines offered from the printer picked. A printer describes its own readings,
  so the choice of lines cannot be written in the card's schema.
-->
<script lang="ts">
  import { t } from '../../../i18n';
  import type { CardEditorProps, CardSetting } from '../../../slots';
  import { dashboardContext } from '../context';
  import SettingsForm from '../settings/settings-form.svelte';
  import { temperatureSettings } from './config';
  import { CORE_CARDS } from './declarations';
  import {
    heaterNameOf,
    NOZZLE,
    offeredReadings,
    readingLabel,
  } from './heaters';

  let { config, onchange }: CardEditorProps = $props();

  const { printers } = dashboardContext();

  const declared: readonly CardSetting[] =
    CORE_CARDS.find((card) => card.id === 'temperature')?.settings ?? [];

  const device = $derived.by(() => {
    const id = temperatureSettings(config).device;

    return id === '' ? undefined : printers.view(id);
  });

  // The options carry words already, so the form shows them as they are.
  const offered = $derived(
    device === undefined
      ? undefined
      : [
          { value: NOZZLE, label: t('dashboard.temperature.nozzle') },
          ...offeredReadings(device.readings).map((reading) => ({
            value: heaterNameOf(reading),
            label: readingLabel(reading),
          })),
        ],
  );

  const schema = $derived(
    declared.map((field) =>
      field.key === 'heaters' && field.kind === 'choices' && offered
        ? { ...field, options: offered }
        : field,
    ),
  );

  const words = $derived.by(() => {
    const shown = new Set(offered?.map((option) => option.label));

    return (text: string): string => (shown.has(text) ? text : t(text));
  });
</script>

<SettingsForm {schema} {config} label={words} {onchange} />
