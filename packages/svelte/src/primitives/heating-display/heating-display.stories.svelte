<script module lang="ts">
  import ThermometerIcon from '@lucide/svelte/icons/thermometer';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { HeaterState } from '../../domain';
  import HeatReading from './heat-reading.svelte';
  import TemperatureReading from './heating-display.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/HeatingDisplay',
    component: TemperatureReading,
  });

  const readings: [HeaterState, number, number | undefined][] = [
    ['heating', 48, 215],
    ['at-temperature', 214, 215],
    ['cooling', 180, 60],
    ['off', 22, undefined],
    ['not-reported', 0, undefined],
  ];
</script>

{#snippet nozzle()}
  <ThermometerIcon aria-hidden="true" class="size-3.5 shrink-0" />
{/snippet}

<!-- Beyond the React set. React ships no stories file for this component, so
     every story here is written from the states it has rather than ported: the
     five the domain names, the two readings with no target to print, and the
     last-known reading from a printer nobody can hear from. -->

<Story name="Default" asChild>
  <TemperatureReading icon={nozzle} current={48} target={215} />
</Story>

<Story name="Every State" asChild>
  <div class="flex flex-col items-start gap-2 text-sm">
    {#each readings as [state, current, target] (state)}
      <div class="flex items-center gap-3">
        <TemperatureReading
          icon={nozzle}
          current={state === 'not-reported' ? undefined : current}
          {target}
        />
        <span class="font-mono text-xs text-muted-foreground">{state}</span>
      </div>
    {/each}
  </div>
</Story>

<Story name="A Heater Told To Go To Zero" asChild>
  <div class="flex items-center gap-6 text-sm">
    <TemperatureReading icon={nozzle} current={48} target={0} />
    <TemperatureReading icon={nozzle} current={48} target={undefined} />
  </div>
</Story>

<!-- A chamber: a heater against its target, so it warms towards one and
     cools under one, and off with none, however warm the air, where a nozzle
     at 48 with no target shows cooling. -->
<Story name="Chamber" asChild>
  <div class="flex items-center gap-6 text-sm">
    <TemperatureReading icon={nozzle} current={30} target={45} chamber />
    <TemperatureReading icon={nozzle} current={50} target={45} chamber />
    <TemperatureReading icon={nozzle} current={48} target={0} chamber />
  </div>
</Story>

<Story name="Last Known Reading" asChild>
  <div class="flex items-center gap-6 text-sm">
    <TemperatureReading icon={nozzle} current={48} target={215} />
    <TemperatureReading icon={nozzle} current={48} target={215} muted />
  </div>
</Story>

<Story name="The Reading Alone" asChild>
  <div class="flex items-center gap-6 text-sm">
    {#each readings as [state] (state)}
      <HeatReading {state}>215 &deg;C</HeatReading>
    {/each}
  </div>
</Story>
