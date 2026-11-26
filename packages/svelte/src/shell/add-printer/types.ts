/** The four steps of adding a printer, in order. */
export type AddPrinterStep = 'prepare' | 'printer' | 'waiting' | 'connected';

/**
 * How a new printer joins this server. `approval` waits for a person to
 * approve the code each printer shows; `automatic` approves every printer that
 * asks.
 */
export type PrinterAdmission = 'approval' | 'automatic';

/**
 * A printer that has asked to join and is waiting for a person to admit it.
 *
 * `code` and `fingerprint` are what the printer shows on its own display.
 * They are the pair an approval is checked against, because the name beside
 * them was chosen by whoever set the printer up and proves nothing.
 *
 * There is no field for when the printer asked. `Registration` in the core's
 * own schema carries the code, the fingerprint, the connector, the model, the
 * firmware and the serial, and no timestamp, so a row that showed one would be
 * showing a fact the server does not have.
 */
export interface PendingPrinter {
  code: string;
  fingerprint: string;
  /** What the printer is, as a person reads it, when it said. */
  model?: string | undefined;
  /**
   * The spelling the card draws the printer by: the model's own spelling in
   * the host's vocabulary, when the host knows the model.
   */
  icon?: string | undefined;
  /**
   * The model as the printer spells it, which the icon looks up in the
   * device-model vocabulary when it has no drawing of its own.
   */
  reported?: string | undefined;
  firmware?: string | undefined;
  serial?: string | undefined;
}

/** A printer that was let in and has not dialled in yet. */
export interface JoiningPrinter {
  code: string;
  model?: string | undefined;
  icon?: string | undefined;
  serial?: string | undefined;
}

/** A printer that was let in and is now connected. */
export interface JoinedPrinter {
  deviceId: string;
  /** Its name in this server, which is the model and serial until renamed. */
  name: string;
  model?: string | undefined;
  icon?: string | undefined;
  /**
   * The model as the printer spells it, which the icon looks up in the
   * device-model vocabulary when it has no drawing of its own.
   */
  reported?: string | undefined;
  /** Its page. */
  href: string;
}

/** Why the server turned a printer away at the door. */
export type RefusedReason = 'unknown_credentials' | 'not_admitted';

/** A printer the server turned away while this screen waited. */
export interface RefusedPrinter {
  reason: RefusedReason;
  model?: string | undefined;
  firmware?: string | undefined;
}

/** Where a printer points itself, which is what the generated file carries. */
export interface AutoConnect {
  host: string;
  port: number;
  tls: boolean;
  /**
   * True where the printer must trust the connect.der on its USB stick: the
   * listener's certificate is signed by this deployment's own authority.
   */
  customCert?: boolean;
  wsUrl: string;
  registerUrl: string;
}

/**
 * Where a pairing at the printer's own screen stands. `waiting` is the
 * question on the printer's screen, `approved` a person who pressed yes while
 * the printer registers, and `admitted` the printer let in. The other three
 * end the pairing.
 */
export type PairingState =
  'waiting' | 'approved' | 'admitted' | 'refused' | 'expired' | 'withdrawn';

/** A pairing this server sent to a printer, as the screen draws it. */
export interface PrinterPairingView {
  /** The code the printer shows, which the person checks before pressing yes. */
  code: string;
  /** The address the pairing went to, as host:port. */
  printer: string;
  state: PairingState;
}

/** What a person asks for when they pair a printer at its screen. */
export interface PairingRequest {
  /** The printer's address on the local network, as typed. */
  printer: string;
  /** This server's address, which the printer stores when the person says yes. */
  connect: { host: string; port: number; tls: boolean };
}

/**
 * What the screen reads, and what it hands back. A callback says what the
 * reader did and never how to do it, so a product on another core hands in an
 * object of its own and decides for itself what an approval performs.
 */
export interface AddPrinterScreenProps {
  connection: AutoConnect;
  /**
   * How a new printer joins, as the server says. Undefined until it has
   * answered: the screen does not guess which steps to show.
   */
  admission: PrinterAdmission | undefined;
  /** Every printer waiting for a decision, whoever started it. */
  pending: readonly PendingPrinter[];
  /** The printers let in since the person began, not yet connected. */
  joining: readonly JoiningPrinter[];
  /**
   * The printers let in since the person began that have dialled in. A host
   * keeps one here once it has arrived, even if it drops again.
   */
  joined: readonly JoinedPrinter[];
  /** The last printer turned away since the person began. */
  refused: RefusedPrinter | undefined;
  /**
   * The certificate of this server's own authority, which a printer that
   * trusts it loads as connect.der. Undefined while it is read, null when the
   * server has none to give.
   */
  authority?: Blob | null | undefined;
  /** The documentation address, ending in a slash, for the certificates page. */
  docsHref?: string | undefined;
  /** Where the trail leads back to. */
  printersHref: string;
  /** The last refusal of a request, as it was thrown. */
  error?: unknown;
  /**
   * How long the waiting step waits before it opens the help, in
   * milliseconds. A printer that asks shows up within seconds.
   */
  helpAfterMs?: number | undefined;
  /** Which step opens. The screen holds the step from there on. */
  initialStep?: AddPrinterStep | undefined;
  /**
   * The person began to add a printer: the screen opened, or they chose to
   * add another one. The host counts approvals, arrivals and refusals from
   * this moment, so a printer added earlier is not shown as the one just
   * added. A printer that registers before the waiting step opens, as one on
   * a server that approves at once does, still counts.
   */
  onBegin: () => void;
  onApprove: (code: string) => void;
  onReject: (code: string) => void;
  /**
   * The person asked a printer on the network to pair at its own screen. A
   * host that cannot pair printers leaves it out, and the screen then offers
   * only the settings file.
   */
  onPair?: ((request: PairingRequest) => void) | undefined;
  /** The person withdrew the pairing, which takes the question off the printer. */
  onCancelPairing?: (() => void) | undefined;
  /** The person wants to pair again after a pairing ended. */
  onPairAgain?: (() => void) | undefined;
  /** The pairing this screen started, until it ends and the person moves on. */
  pairing?: PrinterPairingView | undefined;
  /** Whether a pairing request is on its way to the printer. */
  pairingBusy?: boolean | undefined;
  /** Why the last pairing request failed, as it was thrown. */
  pairingError?: unknown;
}
