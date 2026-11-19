// What a capability verb is called in words.
import { t } from '../../../i18n';

const ELSEWHERE: ReadonlySet<string> = new Set([
  'core.command.create_folder',
  'core.command.delete_file',
  'core.command.delete_folder',
  'core.command.dialog_action',
  'core.command.force_gcode',
  'core.command.get_status',
  'core.command.send_file_info',
  'core.command.send_gcode',
  'core.command.send_info',
  'core.command.send_job_info',
  'core.command.send_state_info',
  'core.command.send_transfer_info',
  'core.command.start_connect_download',
  'core.command.start_encrypted_download',
  'core.command.start_inline_download',
  'core.command.start_print',
  'core.command.stop_transfer',
]);

/** Whether this catalogue offers the verb at all. */
export const isOfferedHere = (verb: string): boolean => !ELSEWHERE.has(verb);

export const COMMAND_GROUPS = [
  'print',
  'state',
  'movement',
  'heat',
  'chamber',
  'calibration',
  'filament',
  'system',
  'other',
] as const;

export type CommandGroup = (typeof COMMAND_GROUPS)[number];

export const GROUP_LABEL_KEY: Readonly<Record<CommandGroup, string>> = {
  calibration: 'printerDetail.commands.group.calibration',
  chamber: 'printerDetail.commands.group.chamber',
  filament: 'printerDetail.commands.group.filament',
  heat: 'printerDetail.commands.group.heat',
  movement: 'printerDetail.commands.group.movement',
  other: 'printerDetail.commands.group.other',
  print: 'printerDetail.commands.group.print',
  state: 'printerDetail.commands.group.state',
  system: 'printerDetail.commands.group.system',
};

/** Every verb this catalogue draws, and the heading it sits under. */
export const VERB_GROUP: Readonly<Record<string, CommandGroup>> = {
  'core.command.beep': 'system',
  'core.command.cancel_object': 'print',
  'core.command.cancel_printer_ready': 'state',
  'core.command.disable_steppers': 'movement',
  'core.command.factory_reset': 'system',
  'core.command.flash': 'system',
  'core.command.home': 'movement',
  'core.command.load_filament': 'filament',
  'core.command.mesh_bed_leveling': 'calibration',
  'core.command.move': 'movement',
  'core.command.move_e': 'movement',
  'core.command.move_z': 'movement',
  'core.command.pause_print': 'print',
  'core.command.reset': 'system',
  'core.command.reset_printer': 'system',
  'core.command.resume_print': 'print',
  'core.command.set_addon_power': 'system',
  'core.command.set_bed_temperature': 'heat',
  'core.command.set_chamber_fan_target': 'chamber',
  'core.command.set_chamber_led_intensity': 'chamber',
  'core.command.set_chamber_target_temp': 'heat',
  'core.command.set_enclosure_enabled': 'chamber',
  'core.command.set_enclosure_postprint': 'chamber',
  'core.command.set_enclosure_postprint_filtration_time': 'chamber',
  'core.command.set_enclosure_printing_filtration': 'chamber',
  'core.command.set_flow': 'print',
  'core.command.set_heatbed_temperature': 'heat',
  'core.command.set_hostname': 'system',
  'core.command.set_idle': 'state',
  'core.command.set_nozzle_temperature': 'heat',
  'core.command.set_printer_ready': 'state',
  'core.command.set_speed': 'print',
  'core.command.set_token': 'system',
  'core.command.set_tool_hardened': 'system',
  'core.command.set_tool_high_flow': 'system',
  'core.command.set_tool_nozzle_diameter': 'system',
  'core.command.stop_print': 'print',
  'core.command.uncancel_object': 'print',
  'core.command.unload_filament': 'filament',
  'core.command.upgrade': 'system',
  'core.command.z_power_move': 'movement',
};

/** The order a group's verbs read in. */
const READING_ORDER: readonly string[] = [
  'core.command.pause_print',
  'core.command.resume_print',
  'core.command.stop_print',
  'core.command.cancel_object',
  'core.command.set_speed',
  'core.command.set_flow',
  'core.command.set_printer_ready',
  'core.command.cancel_printer_ready',
  'core.command.set_idle',
  'core.command.home',
  'core.command.disable_steppers',
  'core.command.move_z',
  'core.command.set_nozzle_temperature',
  'core.command.set_bed_temperature',
  'core.command.set_chamber_target_temp',
  'core.command.set_chamber_fan_target',
  'core.command.set_chamber_led_intensity',
  'core.command.mesh_bed_leveling',
  'core.command.load_filament',
  'core.command.unload_filament',
  'core.command.reset_printer',
  'core.command.set_hostname',
  'core.command.set_tool_hardened',
  'core.command.upgrade',
  'core.command.flash',
];

/** The two verbs drawn in the outlined red. */
const HALTS_THE_MACHINE: ReadonlySet<string> = new Set([
  'core.command.reset_printer',
  'core.command.stop_print',
]);

const RANK: ReadonlyMap<string, number> = new Map(
  READING_ORDER.map((verb, at) => [verb, at]),
);

/** The verb's own tail, which is what both catalogue keys are built on. */
const tailOf = (verb: string): string => verb.slice(verb.lastIndexOf('.') + 1);

const camel = (tail: string): string =>
  tail
    .split('_')
    .map((word, at) =>
      at === 0 ? word : word.charAt(0).toUpperCase() + word.slice(1),
    )
    .join('');

/** Both keys are the verb's tail in camel case under their own family. */
const keyFor = (family: string, verb: string): string | undefined =>
  verb in VERB_GROUP
    ? `printerDetail.commands.${family}.${camel(tailOf(verb))}`
    : undefined;

export const labelKeyOf = (verb: string): string | undefined =>
  keyFor('verb', verb);

export const descriptionKeyOf = (verb: string): string | undefined =>
  keyFor('detail', verb);

/** Whether the row's control carries the warning colour. */
export const haltsTheMachine = (verb: string): boolean =>
  HALTS_THE_MACHINE.has(verb);

/** Where a verb sits inside its group. */
export const rankOf = (verb: string): number =>
  RANK.get(verb) ?? Number.MAX_SAFE_INTEGER;

/** Which heading a verb sits under. One this file never heard of lands last. */
export const groupOf = (verb: string): CommandGroup =>
  VERB_GROUP[verb] ?? 'other';

/** A verb this file has never heard of. */
export const spellOut = (verb: string): string => {
  const tail = tailOf(verb).replace(/_/g, ' ');

  return tail.charAt(0).toUpperCase() + tail.slice(1);
};

/** The verb's name as a person reads it. */
export const verbText = (verb: string, compact = false): string => {
  if (compact && verb === 'core.command.disable_steppers')
    return t('printerDetailControls.disableSteppers.label');
  const key = labelKeyOf(verb);

  return key === undefined ? spellOut(verb) : t(key);
};
