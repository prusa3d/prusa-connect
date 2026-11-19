// The name a person reads for a printer tool.
import type { Translate } from '@prusa/connect-ui/i18n';

/** The label for one head, a head among several, or an MMU slot. */
export const toolLabel = (t: Translate, id: string, single = false): string => {
  const [head = '0', slot] = id.split('.');
  if (slot !== undefined)
    return t('materials.printer.slot', { n: Number(slot) + 1 });
  return single
    ? t('materials.tools.extruder')
    : t('materials.printer.tool', { n: Number(head) + 1 });
};
