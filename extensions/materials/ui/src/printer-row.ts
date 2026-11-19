// A printer as Core's own printer list draws it: its artwork, its model and
// its state with the tint the state wears. Core keeps the helpers behind its
// list private, so the two small mappings below follow
// packages/svelte/src/shell/printers/presentation.ts.
import {
  PRINTER_STATE_TONE,
  type PrinterState,
  printerStateOf,
} from '@prusa/connect-ui/domain';
import {
  type BadgeVariant,
  PRINTER_MODELS,
  type PrinterModel,
} from '@prusa/connect-ui/primitives';

const MODEL_BY_SPELLING: ReadonlyMap<string, PrinterModel> = new Map(
  PRINTER_MODELS.map((model) => [model.replaceAll('_', ''), model]),
);

/** The artwork a printer's own spelling of its model asks for. */
export const artworkOf = (model: string): PrinterModel =>
  MODEL_BY_SPELLING.get(model.toLowerCase().replace(/[^a-z0-9]/g, '')) ??
  'unknown';

/** A printer that is not connected reads OFFLINE, whatever it said last. */
export const stateOf = (connected: boolean, state: string): PrinterState =>
  connected ? printerStateOf(state === '' ? 'UNKNOWN' : state) : 'OFFLINE';

const BADGE: Record<(typeof PRINTER_STATE_TONE)[PrinterState], BadgeVariant> = {
  active: 'stateActive',
  busy: 'stateBusy',
  moving: 'stateMoving',
  ready: 'stateReady',
  finished: 'stateFinished',
  stopped: 'stateStopped',
  paused: 'statePaused',
  attention: 'stateAttention',
  fault: 'stateFault',
  idle: 'stateIdle',
  quiet: 'stateQuiet',
  offline: 'stateOffline',
};

/** The badge a state wears. */
export const stateBadge = (state: PrinterState): BadgeVariant =>
  BADGE[PRINTER_STATE_TONE[state]];
