import type { HomePreference } from '../../domain';
import type { ModuleThemeOption } from '../../styles/module-theme.svelte';

/** Which page this browser opens on. */
export interface HomeSource {
  /** Where this browser lands, `auto` being "read the printer count". */
  readonly home: HomePreference;
  setHome(next: HomePreference): void;
}

export interface SettingsAppearanceScreenProps {
  home: HomeSource;
  /** False on a server that leaves dashboards out, which hides that choice. */
  dashboards?: boolean;
  /** The themes installed modules offer. None hides the choice. */
  themes?: readonly ModuleThemeOption[];
}
