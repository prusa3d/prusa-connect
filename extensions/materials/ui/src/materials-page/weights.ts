import { formatNumber } from '@prusa/connect-ui/i18n';

import type { Spool } from '../api';

/** Grams as the kilograms a person reads, one decimal at most. */
export const kilograms = (grams: number): string =>
  formatNumber(grams / 1000, { maximumFractionDigits: 1 });

/**
 * A weight as typed, or undefined for an empty field. The field is text
 * with a decimal keyboard, because a number field in Safari under a
 * comma-decimal region answers '' for "742,5"; the comma is read here.
 */
export const typedWeight = (typed: string): number | undefined => {
  const value = typed.trim().replace(',', '.');

  return value === '' ? undefined : Number(value);
};

/**
 * A spool added from a tag whose printer reported no full weight, and whose
 * weight nobody typed yet: its 0 g is unknown, not empty.
 */
export const unweighed = (spool: Spool | undefined): boolean =>
  spool?.source === 'printer' &&
  spool.full_weight_g == null &&
  spool.remaining_weight_g === 0;
