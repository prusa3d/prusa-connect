// The core's own cards, registered through the path a module's card takes
// (ADR-20261008-sww rule 17), so the page draws both the same way.
import type { Component } from 'svelte';

import type { CardEditorProps, CardProps, SlotRegistry } from '../../../slots';
import CameraCard from './camera-tile.svelte';
import ClockCard from './clock-tile.svelte';
import { CORE_CARDS } from './declarations';
import FinishingCard from './finishing-tile.svelte';
import IssuesCard from './issues-tile.svelte';
import NoteCard from './note-tile.svelte';
import PrinterCard from './printer-tile.svelte';
import PrintersCard from './printers-tile.svelte';
import SummaryCard from './summary-tile.svelte';
import TemperatureSettings from './temperature-settings.svelte';
import TemperatureCard from './temperature-tile.svelte';

const COMPONENTS: Readonly<Record<string, Component<CardProps>>> = {
  printer: PrinterCard,
  printers: PrintersCard,
  summary: SummaryCard,
  finishing: FinishingCard,
  temperature: TemperatureCard,
  camera: CameraCard,
  note: NoteCard,
  clock: ClockCard,
  issues: IssuesCard,
};

/** The cards whose settings depend on the printer picked. */
const EDITORS: Readonly<Record<string, Component<CardEditorProps>>> = {
  temperature: TemperatureSettings,
};

/** The registries that already hold the core's cards. */
const registered = new WeakSet<SlotRegistry>();

/**
 * Registers every core card with the registry, once. The dashboard chunk
 * calls it when it first loads, so the cards' code is never in the bundle a
 * page without a dashboard loads. A declaration with no component is a
 * mistake in this file, and the registry's own check refuses a bad one.
 */
export const registerCoreCards = (registry: SlotRegistry): void => {
  if (registered.has(registry)) return;
  registered.add(registry);
  for (const declaration of CORE_CARDS) {
    const component = COMPONENTS[declaration.id];
    if (component === undefined) {
      throw new Error(`the core card ${declaration.id} has no component`);
    }
    const editor = EDITORS[declaration.id];
    registry.registerCoreCard(
      declaration,
      component,
      editor === undefined ? undefined : { editor },
    );
  }
};

export { CORE_CARDS } from './declarations';
export { default as UnknownCard } from './unknown-tile.svelte';
