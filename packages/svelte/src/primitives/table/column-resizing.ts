const HANDLE = 'button[data-slot="table-column-resize"]';
const MIN_WIDTH = 72;
const KEY_STEP = 16;

interface Column {
  head: HTMLTableCellElement;
  start: number;
  element: HTMLTableColElement;
  width: number;
}

export function resizeTableColumns(table: HTMLTableElement): () => void {
  let group: HTMLTableColElement | undefined;
  let columns: Column[] = [];
  let drag:
    | {
        pointer: number;
        column: Column;
        x: number;
        width: number;
        direction: number;
      }
    | undefined;

  const getHeaders = () => {
    if (table.querySelector(':scope > colgroup:not([data-resized-columns])'))
      return [];
    const row = table.tHead?.rows[0];
    if (!row || table.tHead?.rows.length !== 1) return [];
    const cells = [...row.cells];
    if (cells.some((cell) => cell.colSpan !== 1 || cell.rowSpan !== 1))
      return [];
    return cells.filter((cell) => getComputedStyle(cell).display !== 'none');
  };

  function clearWidths() {
    drag = undefined;
    group?.remove();
    group = undefined;
    columns = [];
    table.removeAttribute('data-column-widths');
    table.style.removeProperty('--table-resized-width');
  }

  function syncHandles() {
    const heads = getHeaders();
    const headingVisible = (table.tHead?.getClientRects().length ?? 0) > 0;
    if (
      columns.length > 0 &&
      (!headingVisible ||
        columns.length !== heads.length ||
        columns.some((column, index) => column.head !== heads[index]))
    )
      clearWidths();
    let hasHandle = false;
    for (const head of table.tHead?.querySelectorAll<HTMLTableCellElement>(
      'th',
    ) ?? []) {
      const handle = head.querySelector<HTMLButtonElement>(HANDLE);
      if (handle) {
        const index = heads.indexOf(head);
        handle.hidden =
          !headingVisible ||
          index < 0 ||
          index === heads.length - 1 ||
          head.classList.contains('sr-only');
        hasHandle ||= !handle.hidden;
      }
    }
    if (hasHandle) {
      const last = heads.at(-1);
      const label = last?.querySelector(':scope > span[id]') ?? last;
      const text = label?.textContent.trim();
      table.dataset.columnInsets =
        text &&
        !last?.classList.contains('sr-only') &&
        text !== label?.querySelector('.sr-only')?.textContent.trim()
          ? 'all'
          : 'interior';
    } else {
      table.removeAttribute('data-column-insets');
    }
  }

  function freezeWidths() {
    if (group) return;
    const heads = getHeaders();
    if (heads.length < 2) return;
    const body = table.tBodies[0]?.rows[0];
    const bodyCells = body
      ? [...body.cells].filter(
          (cell) => getComputedStyle(cell).display !== 'none',
        )
      : [];
    const bodyMatches =
      bodyCells.length === heads.length &&
      bodyCells.every((cell) => cell.colSpan === 1);
    const measured = bodyMatches ? bodyCells : heads;
    const sized = heads.map((head, index) => ({
      head,
      width: (measured[index] ?? head).getBoundingClientRect().width,
    }));
    if (sized.some(({ width }) => width < 2)) return;

    const nextGroup = document.createElement('colgroup');
    nextGroup.dataset.resizedColumns = '';
    columns = sized.map(({ head, width }) => {
      const element = document.createElement('col');
      nextGroup.append(element);
      return { head, start: width, width, element };
    });
    group = nextGroup;
    table.insertBefore(group, table.caption?.nextSibling ?? table.firstChild);
    table.dataset.columnWidths = '';
    applyWidths();
  }

  function applyWidths() {
    let total = 0;
    for (const column of columns) {
      total += column.width;
      column.element.style.setProperty(
        '--table-column-width',
        `${String(column.width)}px`,
      );
    }
    table.style.setProperty('--table-resized-width', `${String(total)}px`);
  }

  function setWidth(column: Column, width: number) {
    column.width = Math.max(Math.min(column.start, MIN_WIDTH), width);
    applyWidths();
  }

  function readHandle(event: Event) {
    const handle = event.target;
    if (!(handle instanceof HTMLButtonElement) || !handle.matches(HANDLE))
      return;
    if (handle.closest('table') !== table) return;
    return handle;
  }

  function startDrag(event: PointerEvent) {
    const handle = readHandle(event);
    if (!handle || event.button !== 0 || drag) return;
    freezeWidths();
    const column = columns.find((entry) => entry.head === handle.parentElement);
    if (!column) return;
    handle.focus({ preventScroll: true });
    handle.setPointerCapture(event.pointerId);
    event.preventDefault();
    drag = {
      pointer: event.pointerId,
      column,
      x: event.clientX,
      width: column.width,
      direction: getComputedStyle(table).direction === 'rtl' ? -1 : 1,
    };
  }

  function moveDrag(event: PointerEvent) {
    if (drag?.pointer !== event.pointerId) return;
    setWidth(
      drag.column,
      drag.width + (event.clientX - drag.x) * drag.direction,
    );
  }

  function endDrag(event: PointerEvent) {
    if (drag?.pointer !== event.pointerId) return;
    if (event.type === 'pointercancel' && columns.includes(drag.column)) {
      setWidth(drag.column, drag.width);
    }
    drag = undefined;
    if (columns.every((column) => column.width === column.start)) clearWidths();
  }

  function reset(event: Event) {
    const handle = readHandle(event);
    const column = columns.find(
      (entry) => entry.head === handle?.parentElement,
    );
    if (!column) return;
    setWidth(column, column.start);
    if (columns.every((entry) => entry.width === entry.start)) clearWidths();
  }

  function keydown(event: KeyboardEvent) {
    const handle = readHandle(event);
    if (!handle) return;
    if (event.key === 'Enter') {
      event.preventDefault();
      reset(event);
      return;
    }
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
    event.preventDefault();
    freezeWidths();
    const column = columns.find((entry) => entry.head === handle.parentElement);
    if (!column) return;
    const step = event.key === 'ArrowRight' ? KEY_STEP : -KEY_STEP;
    setWidth(
      column,
      column.width +
        (getComputedStyle(table).direction === 'rtl' ? -step : step),
    );
  }

  const events = new AbortController();
  const options = { signal: events.signal };
  table.addEventListener('pointerdown', startDrag, options);
  table.addEventListener('pointermove', moveDrag, options);
  table.addEventListener('pointerup', endDrag, options);
  table.addEventListener('pointercancel', endDrag, options);
  table.addEventListener('lostpointercapture', endDrag, options);
  table.addEventListener('dblclick', reset, options);
  table.addEventListener('keydown', keydown, options);

  const changes = new MutationObserver((records) => {
    if (
      records.some(
        ({ target }) => target === table || table.tHead?.contains(target),
      )
    )
      syncHandles();
  });
  changes.observe(table, { childList: true, subtree: true });
  const size = new ResizeObserver(syncHandles);
  if (table.parentElement) size.observe(table.parentElement);
  syncHandles();

  return () => {
    events.abort();
    changes.disconnect();
    size.disconnect();
    clearWidths();
    table.removeAttribute('data-column-insets');
  };
}
