import type { ModuleHost } from '@prusa/connect-ui/slots';

import type { MaterialsService, PrinterTools, Spool, Tool } from '../api';
import { follow, type SpoolChange, type ToolChange } from '../live';

function printerMaterials(
  host: ModuleHost,
  service: MaterialsService,
  deviceId: string,
) {
  let printer = $state.raw<PrinterTools | undefined>(undefined);
  let gone = $state(false);
  let fault = $state.raw<unknown>(undefined);
  let spools = $state.raw<ReadonlyMap<number, Spool>>(new Map());

  /** The spools on the tools, read from their materials' spool lists. */
  const readSpools = async (
    tools: readonly Tool[],
  ): Promise<Map<number, Spool>> => {
    const on = tools.flatMap(({ assignment: a }) =>
      a?.spool_id === undefined
        ? []
        : [{ uuid: a.material_uuid, id: a.spool_id }],
    );
    const read = await Promise.all(
      [...new Set(on.map((o) => o.uuid))].map(async (uuid) => {
        const ids = on.filter((o) => o.uuid === uuid).map((o) => o.id);
        const all: Spool[] = [];
        let offset: number | undefined;
        do {
          const page = await service.spools(uuid, offset, following.signal);
          all.push(...page.spools);
          offset = page.next;
        } while (
          offset !== undefined &&
          !ids.every((id) => all.some((s) => s.id === id))
        );
        return all.filter((s) => ids.includes(s.id));
      }),
    );
    return new Map(read.flat().map((s) => [s.id, s]));
  };

  /** Bumped by each read, so an older answer that lands late is dropped. */
  let reads = 0;
  /** Ends the event stream, and the reads, once the printer is gone. */
  const following = new AbortController();

  const read = async (): Promise<void> => {
    if (following.signal.aborted) return;
    reads += 1;
    const mine = reads;
    try {
      const answer = await service.tools(deviceId, following.signal);
      if (mine !== reads) return;
      printer = answer;
      fault = undefined;
      // The weight is a detail: a failed read leaves it away and keeps the rows.
      const weighed = await readSpools(answer.tools).catch(() => undefined);
      if (mine === reads) spools = weighed ?? new Map();
    } catch (thrown) {
      if (mine !== reads) return;
      if ((thrown as { code?: unknown }).code === 'materials/NOT_FOUND') {
        gone = true;
        following.abort();
      } else {
        fault = thrown;
      }
    }
  };

  /** How long a burst of telemetry settles before the tools are read again. */
  const SETTLE_MS = 300;
  /**
   * How often the tools are read while the page is in view: the event stream
   * does not tell of a nozzle the printer reports changed.
   */
  const POLL_MS = 15_000;
  let settle: ReturnType<typeof setTimeout> | undefined;

  /** Reads the tools again once a burst of changes settles. */
  const soon = (): void => {
    if (host.signal.aborted || gone) return;
    clearTimeout(settle);
    settle = setTimeout(() => void read(), SETTLE_MS);
  };

  /**
   * Whether a change this module told of is one this printer shows: a change
   * on one of its tools, a change of a spool on one, or a lost place in the
   * stream.
   */
  const ours = (change: ToolChange | SpoolChange | undefined): boolean =>
    change === undefined ||
    ('device_id' in change
      ? change.device_id === deviceId
      : spools.has(change.spool_id));

  void read().then(() => {
    if (gone || following.signal.aborted) return;
    follow(host.client, following.signal, ['tool', 'spool'], (_, change) => {
      if (ours(change)) soon();
    });
  });
  const poll = setInterval(() => {
    if (!document.hidden && !gone) void read();
  }, POLL_MS);
  const stop = (): void => {
    reads += 1;
    clearInterval(poll);
    clearTimeout(settle);
    following.abort();
    host.signal.removeEventListener('abort', stop);
  };
  host.signal.addEventListener('abort', stop, { once: true });

  return {
    get printer() {
      return printer;
    },
    get spools() {
      return spools;
    },
    get gone() {
      return gone;
    },
    get fault() {
      return fault;
    },
    read,
    soon,
    stop,
  };
}

type PrinterMaterials = ReturnType<typeof printerMaterials>;
interface Held {
  state: PrinterMaterials;
  users: number;
}
const sessions = new WeakMap<ModuleHost, Map<string, Held>>();

export function holdPrinterMaterials(
  host: ModuleHost,
  service: MaterialsService,
  deviceId: string,
) {
  let printers = sessions.get(host);
  if (printers === undefined) {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- the registry tracks mounts outside Svelte reactivity.
    printers = new Map();
    sessions.set(host, printers);
  }
  const held = printers.get(deviceId) ?? {
    state: printerMaterials(host, service, deviceId),
    users: 0,
  };
  printers.set(deviceId, held);
  held.users += 1;

  return {
    state: held.state,
    release() {
      held.users -= 1;
      if (held.users === 0) {
        held.state.stop();
        printers.delete(deviceId);
      }
    },
  };
}
