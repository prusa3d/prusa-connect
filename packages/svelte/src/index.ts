export * from './data';
export * from './domain';
export * from './i18n';
export * from './primitives';
export * from './slots';
// Both entries name a CardProps. The root keeps the Card primitive's, which
// it exported first, and names a dashboard card's props after the dashboard;
// a module reads the card contract from the slots entry under its own name.
export type { CardProps } from './primitives';
export type { CardProps as DashboardCardProps } from './slots';
export { browserStorage, forgetRemembered } from './remembered';
export {
  applyTheme,
  THEME_PREFERENCES,
  THEME_STORAGE_KEY,
  theme,
  type ThemeMedia,
  type ThemePreference,
  type ThemeStorage,
  ThemeStore,
  type ThemeStoreOptions,
} from './styles/theme.svelte';
