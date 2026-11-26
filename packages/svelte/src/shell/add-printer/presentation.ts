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
