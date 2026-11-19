// Which of Unload and Unassign a tool offers, and why the rules refuse them,
// for the Tools tab and for a spool row on a material's page alike. The
// module's reasons decide; this only picks the button and its text.
import { sendCommand } from '@prusa/connect-ui/data';
import type { Translate } from '@prusa/connect-ui/i18n';
import type { ModuleHost } from '@prusa/connect-ui/slots';

import type { PrinterTools, Tool } from '../api';
import { reasonText } from './reasons';
import { circleOf } from './tool-view';

export interface ToolOffer {
  /** Unload, on the print head a connected printer would unload. */
  unload: boolean;
  /** Unassign, on a slot or on any tool of a printer that is not connected. */
  unassign: boolean;
  /** The rules or the person's role refuse Unload and Unassign. */
  refused: boolean;
  /** Why a change is refused, each reason once. */
  reasons: readonly string[];
}

/**
 * The printer unloads its active head, or its one head when it has no
 * other tool. A slot unloads through the MMU's own button.
 */
const unloadable = (printer: PrinterTools, tool: Tool): boolean =>
  tool.kind !== 'slot' && (tool.active || printer.tools.length === 1);

/**
 * What the tool offers. `assign` adds the reason Assign is refused, for a
 * view that also draws Assign.
 */
export const toolOffer = (
  t: Translate,
  printer: PrinterTools,
  tool: Tool,
  assign = false,
): ToolOffer => {
  const reported = tool.reported?.abbreviation ?? '';
  const unload =
    printer.connected && unloadable(printer, tool) && circleOf(tool) !== '';
  const unassign =
    tool.assignment !== null && (!printer.connected || tool.kind === 'slot');
  const why = (action: 'assign' | 'unassign'): string | undefined =>
    reasonText(t, tool[action], action, printer.state, reported);
  const unloadWhy =
    tool.unassign === 'printing'
      ? t('materials.tools.unloadPrinting')
      : why('unassign');

  return {
    unload,
    unassign,
    refused: !printer.may_operate || tool.unassign !== '',
    reasons: printer.may_operate
      ? [
          ...new Set(
            [
              assign && printer.connected ? why('assign') : undefined,
              unload ? unloadWhy : undefined,
              unassign ? why('unassign') : undefined,
            ].filter((r) => r !== undefined),
          ),
        ]
      : [t('materials.printer.notOperator')],
  };
};

/**
 * Asks the printer to unload the filament, as the Controls tab does. A
 * refusal throws with the printer's code and its English message.
 */
export const unloadFilament = async (
  host: ModuleHost,
  device: string,
): Promise<void> => {
  const result = await sendCommand(
    host.client,
    device,
    'core.command.unload_filament',
  );
  if (!result.ok) {
    throw Object.assign(
      new Error(
        result.machine_reason ?? host.t('materials.tools.unloadRefused'),
      ),
      { code: result.fault_code ?? undefined },
    );
  }
};
