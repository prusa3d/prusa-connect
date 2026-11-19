import { getContext, setContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';

import {
  browserStorage,
  readRemembered,
  writeRemembered,
} from '../../remembered';
import {
  FILAMENT_COLORS,
  filamentColorOf,
  type FilamentColor,
  type PreviewFilaments,
} from './filament-colors';
import type { PrinterRow } from './types';

interface MaterialChoice {
  material: string;
  brand: string;
}

export interface FavoriteFilament extends MaterialChoice {
  id: string;
  name: string;
  color?: FilamentColor[0] | undefined;
}

const choices = new SvelteMap<string, MaterialChoice | null>();
const materialKey = (device: string, tool: string): string =>
  `prusa.prusa-connect.filament-material:${JSON.stringify([device, tool])}`;

const parse = (key: string): unknown => {
  const stored = readRemembered(browserStorage(), key);
  if (stored === null) return undefined;
  try {
    return JSON.parse(stored) as unknown;
  } catch {
    return undefined;
  }
};

const isChoice = (value: unknown): value is MaterialChoice =>
  typeof value === 'object' &&
  value !== null &&
  'material' in value &&
  typeof value.material === 'string' &&
  value.material !== '' &&
  'brand' in value &&
  typeof value.brand === 'string';

export const materialOverrideOf = (
  device: string,
  tool: string,
): MaterialChoice | undefined => {
  const key = materialKey(device, tool);
  if (choices.has(key)) return choices.get(key) ?? undefined;
  const saved = parse(key);
  return isChoice(saved) ? saved : undefined;
};

export const setMaterialOverride = (
  device: string,
  tool: string,
  choice?: MaterialChoice,
): void => {
  const key = materialKey(device, tool);
  choices.set(key, choice ?? null);
  writeRemembered(browserStorage(), key, JSON.stringify(choice ?? null));
};

const FAVORITES_KEY = 'prusa.prusa-connect.filament-favorites';
const favorites = new SvelteMap<string, readonly FavoriteFilament[]>();
const isFavorite = (value: unknown): value is FavoriteFilament =>
  isChoice(value) &&
  'id' in value &&
  typeof value.id === 'string' &&
  'name' in value &&
  typeof value.name === 'string' &&
  (!('color' in value) || FILAMENT_COLORS.some(([id]) => id === value.color));

const filamentFavorites = (): readonly FavoriteFilament[] => {
  const updated = favorites.get(FAVORITES_KEY);
  if (updated !== undefined) return updated;
  const saved = parse(FAVORITES_KEY);
  return Array.isArray(saved) ? saved.filter(isFavorite) : [];
};

const saveFilamentFavorites = (next: readonly FavoriteFilament[]): void => {
  favorites.set(FAVORITES_KEY, next);
  writeRemembered(browserStorage(), FAVORITES_KEY, JSON.stringify(next));
};

interface FilamentFavoritesStore {
  read: () => readonly FavoriteFilament[];
  save: (next: readonly FavoriteFilament[]) => void;
}

const FAVORITES_PREVIEW = Symbol('filament favorites preview');

export const provideFilamentFavoritesPreview = (
  store: FilamentFavoritesStore,
): void => {
  setContext(FAVORITES_PREVIEW, store);
};

export const useFilamentFavorites = (): FilamentFavoritesStore =>
  getContext<FilamentFavoritesStore | undefined>(FAVORITES_PREVIEW) ?? {
    read: filamentFavorites,
    save: saveFilamentFavorites,
  };

export const displayedFilamentOf = (
  row: PrinterRow,
  preview?: PreviewFilaments,
) => {
  const tool = row.filamentTool ?? preview?.get(row.id)?.tool;
  return {
    color:
      tool === undefined ? undefined : filamentColorOf(row.id, tool, preview),
    material:
      (tool === undefined
        ? undefined
        : materialOverrideOf(row.id, tool)?.material) ?? row.material,
  };
};
