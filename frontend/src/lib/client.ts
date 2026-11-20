import { ApiError, PrusaConnect } from '@prusa/connect';
import { basePath } from '@prusa/connect-ui/router';

import { log } from './log';

const apiLog = log.scope('api');

/**
 * How long one request attempt may take. Thirty seconds: long enough for a
 * listing on a Raspberry Pi that is also serving a print, short enough that a
 * page waiting on a core that went away tells the person instead of hanging.
 */
const REQUEST_TIMEOUT_MS = 30_000;

/**
 * The error params whose values are machine identifiers (a verb, an
 * argument, a state), named in sdk/fault/paramkeys.go. `paramKeys` in
 * internal/app/diagnostics/logs.go is the same list and changes with this one.
 */
const IDENTIFIER_PARAMS: ReadonlySet<string> = new Set([
  'verb',
  'argument',
  'field',
  'state',
  'kind',
  'operation',
  'code',
  'reason_code',
  'machine_reason',
  'capability',
  'unit',
  'type',
  'action',
  'method',
  'op',
  'suggested_verb',
  'printer_model',
  'firmware',
  'version',
  'extension',
  'event',
  'device_id',
  'transfer_id',
  'command_id',
]);

/** The shape of an identifier, as the server checks it too. */
const IDENTIFIER = /^[A-Za-z0-9_.:/-]{1,64}$/u;

/**
 * An error's params as the log keeps them. "Copy browser details" never
 * reaches the server, so a value is kept here only for a listed key and in
 * the shape of an identifier; any other key is kept by name alone.
 */
export const paramsForLog = (
  params: Readonly<Record<string, unknown>>,
): Record<string, unknown> =>
  Object.fromEntries(
    Object.entries(params).map(([key, value]) => [
      key,
      IDENTIFIER_PARAMS.has(key) &&
      (typeof value !== 'string' || IDENTIFIER.test(value))
        ? value
        : null,
    ]),
  );

let refusalHandler: (() => void) | undefined;

/**
 * Sets what runs when any call of the app's client comes back 401. The client
 * exists before the session store, so the store registers itself here.
 */
export const onRefusal = (handler: () => void): void => {
  refusalHandler = handler;
};

/**
 * The one `PrusaConnect` this app builds (frontend-structure.md). Session
 * mode: the page the core serves holds no token, and the credential is the
 * HttpOnly cookie the login set, which the browser attaches to a same-origin
 * request on its own (ADR-20260911-4p6 rule 20, ADR-20260909-c7x rule 8).
 *
 * It takes no `url`, so every request goes to a relative path against the
 * page's own origin. A dev server points that at a core through the proxy in
 * vite.config.ts.
 *
 * `timeoutMs` is the app's own bound on one attempt. It is set here rather
 * than left to the SDK's default so that the number a page waits is a number
 * this app decided. The event stream and the camera routes are
 * not bounded by it: they are read as they arrive.
 *
 * `shareEvents` makes every tab of this browser read one event stream
 * (ADR-20261023-a52). A browser opens six connections to one host over plain HTTP,
 * and a stream per tab left none for any other request from the sixth tab
 * on. The pages of a module read through this client too, so they share it.
 */
export const client = new PrusaConnect({
  auth: { kind: 'session' },
  // The path prefix a reverse proxy serves the app under, or nothing at the
  // root. Every request then goes through the same proxy.
  url: basePath(),
  timeoutMs: REQUEST_TIMEOUT_MS,
  shareEvents: true,
  onUnauthorized: () => {
    refusalHandler?.();
  },
  onFailure: (failure, operation) => {
    apiLog.error(
      'request failed',
      failure instanceof ApiError
        ? {
            operation: operation.id,
            method: operation.method,
            status: failure.status,
            code: failure.code,
            traceId: failure.traceId,
            retryable: failure.retryable,
            params: paramsForLog(failure.params),
          }
        : {
            operation: operation.id,
            method: operation.method,
            attempts: failure.attempts,
          },
    );
  },
});
