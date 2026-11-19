/**
 * Starts at most `size` jobs at once, in the order they asked. Taking a slot
 * hands over the job's start and returns the one way to give the slot back:
 * called before the job started it leaves the queue, called after it frees the
 * slot for the next in line. A second call does nothing. With `boundMs`, a
 * job still running after that long frees its slot on its own and `stall` is
 * called, so the job can cancel what it started; giving it back later does
 * nothing either.
 */
export const slots = (
  size: number,
  boundMs?: number,
): ((start: () => void, stall?: () => void) => () => void) => {
  let busy = 0;
  const queue: (() => void)[] = [];

  const next = (): void => {
    while (busy < size) {
      const start = queue.shift();
      if (start === undefined) return;
      busy += 1;
      start();
    }
  };

  return (start, stall) => {
    let state: 'queued' | 'running' | 'done' = 'queued';
    let bound: ReturnType<typeof setTimeout> | undefined;
    const release = (): void => {
      const was = state;
      state = 'done';
      clearTimeout(bound);
      if (was === 'queued') queue.splice(queue.indexOf(run), 1);
      if (was === 'running') {
        busy -= 1;
        next();
      }
    };
    const run = (): void => {
      state = 'running';
      if (boundMs !== undefined)
        bound = setTimeout(() => {
          release();
          stall?.();
        }, boundMs);
      start();
    };
    queue.push(run);
    next();

    return release;
  };
};

/**
 * One queue for every thumbnail on the page. The page is served over
 * HTTP/1.1, where a browser keeps six connections to the server, and a
 * picture of a file on a printer waits on a printer that runs one command at
 * a time and can be busy for fifteen seconds. A screen of those would hold all
 * six and leave the page's own reads queued behind them in the browser. Two
 * leaves four for the page. A picture from the server's own disk answers in
 * milliseconds, so it loses nothing by sharing the queue, and no caller has to
 * say which kind it draws. Page-wide on purpose: the connection limit it
 * guards is page-wide too. Only a mounted thumbnail in a browser takes a slot,
 * so a server render never touches it.
 *
 * A picture that has not answered after 20 s, past the 15 s a busy printer
 * takes, gives its slot to the next one and its request is cancelled, so a
 * stalled printer never holds more than the two.
 */
export const takeThumbnailSlot = slots(2, 20_000);
