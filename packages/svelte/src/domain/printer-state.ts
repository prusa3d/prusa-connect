// Moved from pull request 185, packages/domain/src/printer-state.ts, 2026-09-11

/**
 * The state words this library has rules for. The SDK types a device's state
 * as an open string, because an adapter declares its own set (ADR-20260831-c7b) and
 * `GET /api/v1/vocabularies/device-states` publishes what one server
 * installed. The list below is `ConnectState` in
 * `legacy/printer/model/printer.go`, the set a Prusa printer reports, which
 * `frontend/src/types/index.ts` also draws. A word outside it folds to
 * `UNKNOWN` through `printerStateOf`, so a server whose adapter names a state
 * this library has no rule for still renders, in the quiet tier.
 */
export const PRINTER_STATES = [
  'UNKNOWN',
  'OFFLINE',
  'IDLE',
  'READY',
  'PRINTING',
  'BUSY',
  'PAUSED',
  'STOPPED',
  'FINISHED',
  'ERROR',
  'ATTENTION',
  'MANIPULATING',
] as const;

export type PrinterState = (typeof PRINTER_STATES)[number];

const KNOWN_STATES: ReadonlySet<string> = new Set(PRINTER_STATES);

/** Whether a word off the wire is one this library has rules for. */
export const isPrinterState = (value: string): value is PrinterState =>
  KNOWN_STATES.has(value);

/** The wire value as a known state, `UNKNOWN` for a word this library has no rule for. */
export const printerStateOf = (value: string): PrinterState =>
  isPrinterState(value) ? value : 'UNKNOWN';

/** The state a print's recorded outcome files it under. */
const STATE_OF_OUTCOME: Readonly<Record<string, PrinterState>> = {
  finished: 'FINISHED',
  failed: 'ERROR',
  cancelled: 'STOPPED',
};

/** The states a print can end in by itself, which an unknown outcome keeps. */
const ENDED_STATES: ReadonlySet<PrinterState> = new Set([
  'FINISHED',
  'ERROR',
  'STOPPED',
]);

/**
 * The state one print reads under, on every page that lists prints. A print
 * still running reads the machine's word. An ended one is filed by its
 * recorded outcome, since the state it was last seen in may be the idle,
 * ready or printing the printer passed through. With no outcome, `unknown`
 * (what the core writes when a driver named none) or a word outside the
 * table, an ended state still says how it ended and anything else is
 * `UNKNOWN`.
 */
export const printStateOf = (print: {
  state: string;
  ended: boolean;
  outcome?: string | undefined;
}): PrinterState => {
  const state = printerStateOf(print.state);
  if (!print.ended) return state;
  const filed =
    print.outcome !== undefined &&
    Object.hasOwn(STATE_OF_OUTCOME, print.outcome)
      ? STATE_OF_OUTCOME[print.outcome]
      : undefined;
  return filed ?? (ENDED_STATES.has(state) ? state : 'UNKNOWN');
};

/**
 * The catalogue key each state reads. The English line behind `state.MOVING`
 * is "Moving", a deliberate rewording of `MANIPULATING` and not a
 * transcription, and `state.ATTENTION` is one word so it fits the badge's
 * fixed width beside every other state. `stateLabel` in `src/device/refusal.ts`
 * is what turns a wire value into words (frontend-i18n.md rule 1).
 */
export const PRINTER_STATE_LABEL_KEY: Record<PrinterState, string> = {
  UNKNOWN: 'state.UNKNOWN',
  OFFLINE: 'state.OFFLINE',
  IDLE: 'state.IDLE',
  READY: 'state.READY',
  PRINTING: 'state.PRINTING',
  BUSY: 'state.BUSY',
  PAUSED: 'state.PAUSED',
  STOPPED: 'state.STOPPED',
  FINISHED: 'state.FINISHED',
  ERROR: 'state.ERROR',
  ATTENTION: 'state.ATTENTION',
  MANIPULATING: 'state.MANIPULATING',
};

/**
 * The fleet sort, worst first. It follows Prusa Connect's order: a finished or
 * stopped printer ranks below the ones doing work and above the idle ones.
 */
export const PRINTER_STATE_SEVERITY: Record<PrinterState, number> = {
  ERROR: 0,
  ATTENTION: 1,
  PAUSED: 1,
  OFFLINE: 2,
  UNKNOWN: 2,
  PRINTING: 3,
  BUSY: 3,
  MANIPULATING: 3,
  STOPPED: 4,
  FINISHED: 4,
  IDLE: 5,
  READY: 5,
};

/** The `--state-*` tint a state takes, as src/styles/tokens.css names them. */
export type PrinterStateTone =
  | 'active'
  | 'busy'
  | 'moving'
  | 'ready'
  | 'finished'
  | 'stopped'
  | 'paused'
  | 'idle'
  | 'attention'
  | 'fault'
  | 'quiet'
  | 'offline';

export const PRINTER_STATE_TONE: Record<PrinterState, PrinterStateTone> = {
  ERROR: 'fault',
  ATTENTION: 'attention',
  PRINTING: 'active',
  BUSY: 'busy',
  MANIPULATING: 'moving',
  IDLE: 'idle',
  READY: 'ready',
  FINISHED: 'finished',
  STOPPED: 'stopped',
  PAUSED: 'paused',
  OFFLINE: 'offline',
  UNKNOWN: 'quiet',
};

export type PrinterSeverityTier =
  'critical' | 'attention' | 'not-reporting' | 'normal';

/**
 * The tiers a fleet filter offers, worst first: the order a farm reads them
 * in, and the order any control listing them draws them in.
 */
export const SEVERITY_FILTER_TIERS: readonly PrinterSeverityTier[] = [
  'critical',
  'attention',
  'not-reporting',
  'normal',
];

/**
 * The catalogue key each tier reads. The English behind `severity.notReporting`
 * is "Silent": short enough for a 320px row, where the fuller phrasing would
 * not fit.
 */
export const SEVERITY_FILTER_LABEL_KEY: Record<PrinterSeverityTier, string> = {
  critical: 'severity.critical',
  attention: 'severity.attention',
  'not-reporting': 'severity.notReporting',
  normal: 'severity.normal',
};

export const PRINTER_STATE_SEVERITY_TIER: Record<
  PrinterState,
  PrinterSeverityTier
> = {
  ERROR: 'critical',
  ATTENTION: 'attention',
  PAUSED: 'attention',
  OFFLINE: 'not-reporting',
  UNKNOWN: 'not-reporting',
  PRINTING: 'normal',
  BUSY: 'normal',
  MANIPULATING: 'normal',
  STOPPED: 'normal',
  FINISHED: 'normal',
  IDLE: 'normal',
  READY: 'normal',
};

/** `muted` marks a progress bar whose value is frozen, so it stops implying "running" once stalled. */
export const PRINTER_STATE_PROGRESS_TONE: Record<
  PrinterState,
  'primary' | 'muted' | 'success'
> = {
  FINISHED: 'success',
  ERROR: 'muted',
  PAUSED: 'muted',
  STOPPED: 'muted',
  ATTENTION: 'muted',
  PRINTING: 'primary',
  BUSY: 'primary',
  MANIPULATING: 'primary',
  IDLE: 'primary',
  READY: 'primary',
  OFFLINE: 'primary',
  UNKNOWN: 'primary',
};
