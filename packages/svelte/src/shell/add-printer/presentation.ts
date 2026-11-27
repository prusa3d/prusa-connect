// The two values this screen derives: the file a printer is configured with,
// and the spacing that makes a fingerprint readable off a printer's display.
import type { AutoConnect } from './types';

/**
 * The file a printer reads from the root of a USB drive. It never carries a
 * token, not even an empty one: the printer gets its token from this server
 * when a person chooses Add Printer to Connect on it and the code is approved.
 * Wherever this server checks tokens, it refuses one it did not issue.
 */
export const buildIni = (connection: AutoConnect): string =>
  [
    '[service::connect]',
    `hostname = ${connection.host}`,
    `port = ${String(connection.port)}`,
    `tls = ${connection.tls ? '1' : '0'}`,
    ...(connection.tls && connection.customCert === true
      ? ['custom_cert = 1']
      : []),
  ].join('\n');

/**
 * A fingerprint in blocks of four, so the eye jumps block to block instead of
 * holding sixteen unbroken characters at once. This value is read against a
 * small printer display across a workshop and is never typed or searched, and
 * the same grouping is what an SSH host key and a hardware serial use.
 *
 * `(?=.)` inserts a space only before a block that has a character after it,
 * so a length that is not a multiple of four never ends with a trailing space.
 */
export const formatFingerprint = (fingerprint: string): string =>
  fingerprint.replaceAll(/(.{4})(?=.)/g, '$1 ');

/**
 * Whether a printer stores a host name whole. The firmware keeps 20
 * characters of it, and shortens only a name that ends in prusa3d.com before
 * it does: by `connect.prusa3d.com` to fit 35 characters, and by
 * `prusa3d.com` to fit 29. docs/src/content/docs/printer-configuration.md has
 * the table.
 */
export const storedHostFits = (host: string): boolean => {
  if (host.endsWith('connect.prusa3d.com')) return host.length <= 35;
  if (host.endsWith('prusa3d.com')) return host.length <= 29;

  return host.length <= 20;
};

/**
 * The models whose name a printer announces until somebody renames it. The
 * firmware names a new printer after its model (LAN_HOSTNAME_DEF in
 * src/common/config.h), and mDNS carries nothing else that says what the
 * printer is.
 */
const DEFAULT_NAMES: Readonly<Record<string, string>> = {
  'prusa-mini': 'MINI',
  'prusa-mk3-5': 'MK3.5',
  'prusa-mk4': 'MK4',
  'prusa-xl': 'XL',
  'prusa-ix': 'iX',
  'prusa-core-one': 'CORE One',
  'prusa-core-one-l': 'CORE One L',
};

/** The model a printer's announced name says, when it still has its default name. */
export const modelOfName = (name: string): string | undefined =>
  DEFAULT_NAMES[name.trim().toLowerCase()];

/**
 * The address the pairing request goes to: the IP alone on the printer's
 * default port 80, and with the port otherwise. An IPv6 address is bracketed.
 */
export const pairingAddress = (candidate: {
  address: string;
  port: number;
}): string => {
  const host = candidate.address.includes(':')
    ? `[${candidate.address}]`
    : candidate.address;

  return candidate.port === 80 ? host : `${host}:${String(candidate.port)}`;
};

/** The candidates whose name or address holds the search, ignoring case. */
export const matchingCandidates = <T extends { name: string; address: string }>(
  candidates: readonly T[],
  query: string,
): T[] => {
  const needle = query.trim().toLowerCase();
  if (needle === '') return [...candidates];

  return candidates.filter(
    (one) =>
      one.name.toLowerCase().includes(needle) ||
      one.address.toLowerCase().includes(needle),
  );
};
