<!-- The appearance room: how this app is drawn, and where it opens. -->
<script lang="ts">
  import MonitorIcon from '@lucide/svelte/icons/monitor';
  import MoonIcon from '@lucide/svelte/icons/moon';
  import SunIcon from '@lucide/svelte/icons/sun';

  import { type HomePreference, homeOn } from '../../domain';
  import {
    type Clock,
    CLOCKS,
    formatNumber,
    formatting,
    t,
    TEMPERATURE_UNITS,
    type TemperatureUnit,
  } from '../../i18n';
  import {
    DEFAULT_VIEW_FIELDS,
    type DefaultViewField,
    Label,
    PAGE_SIZES,
    type PageSize,
    RadioGroup,
    RadioGroupItem,
    SettingsPane,
    ToggleGroup,
    ToggleGroupItem,
    viewModes,
    viewPreferences,
  } from '../../primitives';
  import { textSize, type TextSize } from '../../styles/text-size.svelte';
  import { theme, type ThemePreference } from '../../styles/theme.svelte';
  import type { SettingsAppearanceScreenProps } from './types';

  let { home, dashboards = true }: SettingsAppearanceScreenProps = $props();

  const THEME_ICON = {
    system: MonitorIcon,
    light: SunIcon,
    dark: MoonIcon,
  } as const;

  // The order the reference reads in.
  const THEMES: readonly ThemePreference[] = ['system', 'light', 'dark'];

  const HOMES: readonly HomePreference[] = [
    'dashboard',
    'auto',
    'my-printer',
    'all-printers',
  ];

  // Every key is written out rather than assembled from a value.
  const THEME_NAME = {
    system: 'settingsAppearance.theme.system',
    light: 'settingsAppearance.theme.light',
    dark: 'settingsAppearance.theme.dark',
  } as const;

  const HOME_NAME = {
    dashboard: 'settingsAppearance.home.dashboard',
    auto: 'settingsAppearance.home.auto',
    'my-printer': 'settingsAppearance.home.myPrinter',
    'all-printers': 'settingsAppearance.home.allPrinters',
  } as const;

  const HOME_NOTE = {
    dashboard: 'settingsAppearance.home.dashboardNote',
    auto: 'settingsAppearance.home.autoNote',
    'my-printer': 'settingsAppearance.home.myPrinterNote',
    'all-printers': 'settingsAppearance.home.allPrintersNote',
  } as const;

  const CLOCK_NAME = {
    '24-hour': 'settingsAppearance.clock.24',
    '12-hour': 'settingsAppearance.clock.12',
  } as const;

  const TEMPERATURE_NAME = {
    celsius: 'settingsAppearance.temperature.celsius',
    fahrenheit: 'settingsAppearance.temperature.fahrenheit',
  } as const;

  const id = $props.id();

  /** The stored pick, or the shape the listing opens in until there is one. */
  const chosenView = (field: DefaultViewField): string =>
    viewPreferences.defaultView(field.id) ?? field.initial;

  // Both setters reject a word they do not know.
  const chooseTheme = (next: string): void => {
    theme.preference = next as ThemePreference;
  };

  const chooseTextSize = (next: string): void => {
    textSize.preference = next as TextSize;
  };

  const chooseClock = (next: string): void => {
    formatting.clock = next as Clock;
  };

  const chooseTemperature = (next: string): void => {
    formatting.temperature = next as TemperatureUnit;
  };

  // The store rejects a size no pager offers.
  const chooseRows = (next: string): void => {
    viewPreferences.rowsPerPage = Number(next) as PageSize;
  };

  // The port takes the three words and nothing else.
  // A server without dashboards does not offer them as the home page.
  const homes = $derived(
    dashboards ? HOMES : HOMES.filter((name) => name !== 'dashboard'),
  );
  const homeShown = $derived(homeOn(home.home, dashboards));

  const isHome = (value: string): value is HomePreference =>
    (homes as readonly string[]).includes(value);

  const chooseHome = (next: string): void => {
    if (isHome(next)) home.setHome(next);
  };
</script>

<!-- The thumbnail is decoration. -->
{#snippet layout(shape: HomePreference)}
  <span
    aria-hidden="true"
    class="rounded-input grid h-12 w-20 shrink-0 grid-cols-2 gap-1 border border-border bg-muted p-1.5"
  >
    {#if shape === 'dashboard'}
      <span class="rounded-badge col-span-2 bg-control-on"></span>
      <span class="rounded-badge bg-border"></span>
      <span class="rounded-badge bg-border"></span>
    {:else if shape === 'auto'}
      <span class="rounded-badge row-span-2 bg-control-on"></span>
      <span class="rounded-badge bg-border"></span>
      <span class="rounded-badge bg-border"></span>
    {:else if shape === 'my-printer'}
      <span class="rounded-badge col-span-2 row-span-2 bg-control-on"></span>
    {:else}
      <span class="rounded-badge bg-border"></span>
      <span class="rounded-badge bg-border"></span>
      <span class="rounded-badge bg-border"></span>
      <span class="rounded-badge bg-border"></span>
    {/if}
  </span>
{/snippet}
{#snippet lookSection()}
  <div class="flex flex-col gap-section-gap pb-0">
    <div class="grid gap-section-block">
      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.theme.label')}
        </h3>
        <ToggleGroup
          type="single"
          variant="outline"
          value={theme.preference}
          onValueChange={chooseTheme}
          aria-label={t('settingsAppearance.theme.label')}
        >
          {#each THEMES as name (name)}
            {@const Icon = THEME_ICON[name]}
            <ToggleGroupItem value={name}>
              <Icon />
              {t(THEME_NAME[name])}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>
    </div>
  </div>
{/snippet}

{#snippet readingSection()}
  <div class="flex flex-col gap-section-gap py-0">
    <div class="grid grid-cols-1 gap-section-block @md/pane:grid-cols-2">
      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.textSize.label')}
        </h3>
        <ToggleGroup
          type="single"
          variant="outline"
          value={textSize.preference}
          onValueChange={chooseTextSize}
          aria-label={t('settingsAppearance.textSize.label')}
        >
          <ToggleGroupItem value="small">
            {t('settingsAppearance.textSize.small')}
          </ToggleGroupItem>
          <ToggleGroupItem value="normal">
            {t('settingsAppearance.textSize.normal')}
          </ToggleGroupItem>
          <ToggleGroupItem value="large">
            {t('settingsAppearance.textSize.large')}
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.clock.label')}
        </h3>
        <ToggleGroup
          type="single"
          variant="outline"
          value={formatting.clock}
          onValueChange={chooseClock}
          aria-label={t('settingsAppearance.clock.label')}
        >
          {#each CLOCKS as name (name)}
            <ToggleGroupItem value={name}>
              {t(CLOCK_NAME[name])}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>

      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.temperature.label')}
        </h3>
        <ToggleGroup
          type="single"
          variant="outline"
          value={formatting.temperature}
          onValueChange={chooseTemperature}
          aria-label={t('settingsAppearance.temperature.label')}
        >
          {#each TEMPERATURE_UNITS as name (name)}
            <ToggleGroupItem value={name}>
              {t(TEMPERATURE_NAME[name])}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>

      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.rows.label')}
        </h3>
        <!-- A toggle group and not the pager's own select. -->
        <ToggleGroup
          type="single"
          variant="outline"
          value={String(viewPreferences.rowsPerPage)}
          onValueChange={chooseRows}
          aria-label={t('settingsAppearance.rows.label')}
        >
          {#each PAGE_SIZES as size (size)}
            <ToggleGroupItem value={String(size)}>
              {formatNumber(size)}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>
    </div>
  </div>
{/snippet}

{#snippet openSection()}
  <div
    class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <div class="grid gap-6 @[44rem]/pane:grid-cols-2">
      <div class="flex flex-col gap-2">
        <h3 class="text-body font-medium">
          {t('settingsAppearance.home.label')}
        </h3>
        <RadioGroup
          gap="section"
          class="pt-1"
          value={homeShown}
          onValueChange={chooseHome}
          aria-label={t('settingsAppearance.home.label')}
        >
          {#each homes as name (name)}
            <div class="flex items-start gap-3">
              <RadioGroupItem value={name} id={`${id}-${name}`} class="mt-1" />
              {@render layout(name)}
              <div class="flex min-w-0 flex-col gap-0.5">
                <Label variant="option" for={`${id}-${name}`}>
                  {t(HOME_NAME[name])}
                </Label>
                <span class="text-sm text-muted-foreground">
                  {t(HOME_NOTE[name])}
                </span>
              </div>
            </div>
          {/each}
        </RadioGroup>
      </div>
    </div>
  </div>
{/snippet}

{#snippet viewsSection()}
  <div class="flex flex-col divide-y divide-border">
    {#each DEFAULT_VIEW_FIELDS as field (field.id)}
      <div
        class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-panel-padding-mobile py-3 @lg/pane:px-panel-padding"
      >
        <h3 class="text-body font-medium">{t(field.labelKey)}</h3>
        <!-- `viewModes()` and not a list written here: the names, the words
             and the order are the listing's own. -->
        <ToggleGroup
          type="single"
          variant="outline"
          value={chosenView(field)}
          onValueChange={(next: string) => {
            viewPreferences.setDefaultView(field.id, next);
          }}
          aria-label={t(field.labelKey)}
        >
          {#each viewModes([...field.modes]) as option (option.value)}
            {@const Icon = option.icon}
            <ToggleGroupItem value={option.value}>
              <Icon />
              {option.label}
            </ToggleGroupItem>
          {/each}
        </ToggleGroup>
      </div>
    {/each}
  </div>
{/snippet}

<div class="@container/appearance flex w-full min-w-0 flex-col gap-section-gap">
  <h2 data-slot="settings-section-title" class="text-lead font-semibold">
    {t('settingsAppearance.title')}
  </h2>
  <div
    data-slot="appearance-panels"
    class="grid min-w-0 grid-cols-settings-panels gap-x-section-gap gap-y-form-gap @appearance-columns/appearance:gap-y-section-gap"
  >
    <div
      class="flex min-w-0 flex-col gap-form-gap @appearance-columns/appearance:gap-section-gap"
    >
      <SettingsPane title={t('settingsAppearance.look.title')} class="min-w-0">
        <div class="min-w-0 p-panel-padding-mobile @lg/pane:p-panel-padding">
          {@render lookSection()}
        </div>
      </SettingsPane>
      <SettingsPane
        title={t('settingsAppearance.reading.title')}
        class="min-w-0 @appearance-columns/appearance:flex-auto"
      >
        <div class="min-w-0 p-panel-padding-mobile @lg/pane:p-panel-padding">
          {@render readingSection()}
        </div>
      </SettingsPane>
    </div>
    <SettingsPane
      title={t('settingsAppearance.startPage.title')}
      class="min-w-0"
    >
      {@render openSection()}
    </SettingsPane>
  </div>
  <SettingsPane
    title={t('settingsAppearance.defaultViews.label')}
    class="min-w-0"
  >
    {@render viewsSection()}
  </SettingsPane>
</div>
