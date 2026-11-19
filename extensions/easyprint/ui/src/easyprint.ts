// What this module knows about EasyPrint: where it is served from, the page a
// person opens it on, and the address that starts the pairing.

/** The origin EasyPrint calls the server from, as module.yaml asks for it. */
export const EASYPRINT_ORIGIN = 'https://slicer.printables.com';

/** The Printables page a person opens EasyPrint on. */
export const EASYPRINT_PAGE = 'https://www.printables.com/slice';

/**
 * The address that opens EasyPrint with its form for a new server filled in.
 * `server` is this server's origin as the person's browser reaches it,
 * because EasyPrint calls the server from that same browser.
 */
export const pairHref = (server: string): string =>
  `${EASYPRINT_PAGE}?pair_connect_local=${encodeURIComponent(server)}`;

/** Whether a page opened on `protocol` reached the server over plain HTTP. */
export const isPlainHttp = (protocol: string): boolean => protocol === 'http:';
