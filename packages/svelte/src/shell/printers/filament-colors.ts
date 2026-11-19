// Swatches from the PrusaSlicer color wheel, clockwise from twelve o'clock.
import { getContext, setContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';

import {
  browserStorage,
  readRemembered,
  writeRemembered,
} from '../../remembered';

export const FILAMENT_COLORS = [
  ['red', '#BD013C', 'printerDetailInfo.filament.color.red'],
  ['brown', '#703C31', 'printerDetailInfo.filament.color.brown'],
  ['purple', '#6C328C', 'printerDetailInfo.filament.color.purple'],
  ['cyan', '#007BBF', 'printerDetailInfo.filament.color.cyan'],
  ['blue', '#014598', 'printerDetailInfo.filament.color.blue'],
  ['teal', '#00766E', 'printerDetailInfo.filament.color.teal'],
  ['darkGreen', '#006940', 'printerDetailInfo.filament.color.darkGreen'],
  ['green', '#70C140', 'printerDetailInfo.filament.color.green'],
  ['beige', '#B5A37B', 'printerDetailInfo.filament.color.beige'],
  ['yellow', '#FFF968', 'printerDetailInfo.filament.color.yellow'],
  ['amber', '#FEA000', 'printerDetailInfo.filament.color.amber'],
  ['orange', '#FA6418', 'printerDetailInfo.filament.color.orange'],
  ['gray', '#808080', 'printerDetailInfo.filament.color.gray'],
  ['darkGray', '#4B4B4B', 'printerDetailInfo.filament.color.darkGray'],
  ['black', '#000000', 'printerDetailInfo.filament.color.black'],
  ['white', '#FFFFFF', 'printerDetailInfo.filament.color.white'],
  ['lightGray', '#E1E1E1', 'printerDetailInfo.filament.color.lightGray'],
  ['silver', '#A0A0A0', 'printerDetailInfo.filament.color.silver'],
] as const;

export type FilamentColor = (typeof FILAMENT_COLORS)[number];

export type PreviewFilaments = ReadonlyMap<
  string,
  {
    tool: string;
    colors: ReadonlyMap<string, FilamentColor[0]>;
  }
>;

const PREVIEW = Symbol('preview filament colors');

/** Demo defaults belong to the story, and are never written to storage. */
export const provideFilamentPreview = (preview: PreviewFilaments): void => {
  setContext(PREVIEW, preview);
};

export const filamentPreview = (): PreviewFilaments | undefined =>
  getContext<PreviewFilaments | undefined>(PREVIEW);

const colors = new SvelteMap<string, FilamentColor | null>();
const keyOf = (device: string, tool: string): string =>
  `prusa.prusa-connect.filament-color:${JSON.stringify([device, tool])}`;

export const filamentColorOf = (
  device: string,
  tool: string,
  preview?: PreviewFilaments,
): FilamentColor | undefined => {
  const key = keyOf(device, tool);
  if (colors.has(key)) return colors.get(key) ?? undefined;
  const saved = readRemembered(browserStorage(), key);
  const name = saved ?? preview?.get(device)?.colors.get(tool);
  return FILAMENT_COLORS.find(([id]) => id === name);
};

export const setFilamentColor = (
  device: string,
  tool: string,
  color?: FilamentColor,
): void => {
  const key = keyOf(device, tool);
  colors.set(key, color ?? null);
  writeRemembered(browserStorage(), key, color?.[0] ?? '');
};
