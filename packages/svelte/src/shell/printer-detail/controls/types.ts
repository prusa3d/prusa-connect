import type { Snippet } from 'svelte';

import type { Device } from '../../../domain';
import type { CommandsSource } from '../commands/types';
import type { CommandPort, TelemetrySource } from '../types';

/**
 * The Controls tab's own port. A screen takes a port and not a store
 * (ADR-20260917-j7s), so the panel reads the record, the live readings and the command
 * seam it is handed and reaches for nothing else.
 */
export interface PrinterControlsPanelProps {
  /** The record, once it has arrived. Until then the panel draws its own wait. */
  device?: Device | undefined;
  telemetry: TelemetrySource;
  commands: CommandPort;
  loading?: boolean | undefined;
  /**
   * Draws the material of one tool, handed its id and the chip to fall back
   * on. An extension that assigns materials supplies it; absent, the chip is
   * the host's own.
   */
  material?: Snippet<[string, Snippet]> | undefined;
  advanced?:
    | {
        commands: CommandsSource;
        requested: boolean;
        highlight?: string | undefined;
      }
    | undefined;
}
