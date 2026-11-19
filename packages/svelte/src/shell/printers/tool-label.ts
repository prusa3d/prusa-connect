import { t } from '../../i18n';

// The printer counts tools and slots from 0 on the wire and in G-code. A
// person reads them from 1, so every label goes through these two.

/** The name of the head with this index, such as `Tool 1` for head 0. */
export const toolLabel = (index: number | string): string =>
  t('printerDetailInfo.tools.toolNumber', { n: Number(index) + 1 });

/** The name of a slot of a feeder by its index on the head. */
export const slotLabel = (index: number | string): string =>
  t('printerDetailInfo.tools.slotNumber', { n: Number(index) + 1 });
