// The path prefix the app is served under, and the two conversions between an
// address inside the app and the address the browser shows.
//
// The core serves the app at the origin root. A reverse proxy can serve it
// under a prefix instead, such as the Home Assistant Supervisor's Ingress at
// `/api/hassio_ingress/<token>/`. The core then names the prefix in the entry
// document's `<base>` element, and only there (internal/transport/http/httpapi/appprefix.go).
// Every route, link and request in the app is written from the root, as
// `/printers/abc` or `/api/v1/devices`, and is moved under the prefix here when
// it reaches the browser.

/**
 * The prefix from the document's `<base href>`, without its trailing slash,
 * or `''` at the root. A document with no `<base>` element, such as the Vite
 * dev server's or a test's, is at the root.
 */
export const basePathOf = (
  doc: Pick<Document, 'querySelector'> | undefined,
): string => {
  const href = doc?.querySelector('base')?.getAttribute('href') ?? '';
  // The core writes a path. Anything else, such as a full URL some other host
  // wrote, is not a prefix this app can put in front of its own paths.
  if (!href.startsWith('/') || href.startsWith('//')) return '';

  return href.replace(/\/+$/u, '');
};

let cached: string | undefined;

/** The prefix of this page. Read once: the `<base>` element never changes. */
export const basePath = (): string => {
  cached ??= basePathOf(globalThis.document);

  return cached;
};

/** Whether an address is a path from the origin root, as `/printers/abc` is and `//host/x` is not. */
const rooted = (address: string): boolean =>
  address.startsWith('/') && !address.startsWith('//');

/**
 * The address the browser uses for a path inside the app: `/printers/abc`
 * under the prefix. A path that already carries the prefix, a full URL and a
 * relative address are returned as they are.
 */
export const withBase = (
  address: string,
  base: string = basePath(),
): string => {
  if (base === '' || !rooted(address)) return address;
  if (address === base || address.startsWith(`${base}/`)) return address;

  return `${base}${address}`;
};

/**
 * The path inside the app for a path the browser shows: `/printers/abc` for
 * `<prefix>/printers/abc`. A path outside the prefix is returned as it is.
 */
export const withoutBase = (
  path: string,
  base: string = basePath(),
): string => {
  if (base === '') return path;
  if (path === base) return '/';
  if (path.startsWith(`${base}/`)) return path.slice(base.length);

  return path;
};

/**
 * Moves a link the app wrote from the root under the prefix, at the moment the
 * pointer or the keyboard reaches it. A click the router takes needs nothing,
 * but the browser also opens a link itself: in a new tab on a middle click, or
 * copied from the context menu. It reads the `href` attribute then, and a link
 * from the root would leave the app for the proxy's own page.
 *
 * Returns the call that stops it. Nothing is done at the root.
 */
export const keepLinksUnderBase = (
  target: Pick<Document, 'addEventListener' | 'removeEventListener'>,
  base: string = basePath(),
): (() => void) => {
  if (base === '') return () => undefined;

  const move = (event: Event): void => {
    if (!(event.target instanceof Element)) return;
    const anchor = event.target.closest('a[href]');
    if (anchor === null) return;
    const href = anchor.getAttribute('href') ?? '';
    const moved = withBase(href, base);
    if (moved !== href) anchor.setAttribute('href', moved);
  };

  const events = [
    'pointerover',
    'pointerdown',
    'focusin',
    'contextmenu',
  ] as const;
  for (const name of events) target.addEventListener(name, move, true);

  return () => {
    for (const name of events) target.removeEventListener(name, move, true);
  };
};
