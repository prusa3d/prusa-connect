import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import TableProbe from '../../test/table-probe.svelte';

const caption = 'Files stored on this printer';
const columns = ['Name', 'Size', 'Modified', 'Status'];
const rows = [
  ['firmware.bbl', '18.2 MB', '2026-08-12', 'Ready'],
  ['benchy.3mf', '4.1 MB', '2026-08-14', 'Printing'],
  ['bracket.stl', '820 KB', '2026-08-15', 'Queued'],
];
const total = '3 files';

const draw = (selected?: string) =>
  render(TableProbe, {
    props: selected
      ? { caption, columns, rows, total, selected }
      : { caption, columns, rows, total },
  });

describe('Table', () => {
  it('resizes the column and its rows from the header, then resets it', async () => {
    const { container } = await draw();
    const header = container.querySelector<HTMLTableCellElement>('thead th')!;
    const cell = container.querySelector<HTMLTableCellElement>('tbody th')!;
    const handle = page
      .getByRole('columnheader', { name: 'Name', exact: true })
      .getByRole('button', { name: 'Resize column' });
    const start = header.getBoundingClientRect().width;
    await expect.element(handle).toBeVisible();
    (handle.element() as HTMLButtonElement).focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(header.getBoundingClientRect().width).toBeCloseTo(start + 16);
    expect(cell.getBoundingClientRect().width).toBeCloseTo(start + 16);
    await userEvent.keyboard('{ArrowLeft}');
    expect(cell.getBoundingClientRect().width).toBeCloseTo(start);
    await userEvent.keyboard('{ArrowRight}{Enter}');
    expect(header.getBoundingClientRect().width).toBeCloseTo(start);
    await expectNoViolations(container);
  });

  it('lets a tick box in a narrow first column take its click', async () => {
    const { container } = await render(TableProbe, {
      props: {
        caption,
        columns: [...columns, 'Printer', 'Location', 'Notes'],
        rows: rows.map((row) => [
          ...row,
          'MK4S',
          'Shelf two',
          'For the bracket',
        ]),
        total,
        selectAll: 'Select every file',
      },
    });
    const tick = page.getByRole('checkbox', { name: 'Select every file' });
    const selectHandle = container.querySelector(
      'thead th:first-child > button[data-slot="table-column-resize"]',
    )!;
    await expect.element(page.elementLocator(selectHandle)).not.toBeVisible();
    await tick.click();
    await expect.element(tick).toBeChecked();

    const header = container.querySelector<HTMLTableCellElement>(
      'thead th:nth-child(2)',
    )!;
    const start = header.getBoundingClientRect().width;
    (
      page
        .getByRole('columnheader', { name: 'Name', exact: true })
        .getByRole('button', { name: 'Resize column' })
        .element() as HTMLButtonElement
    ).focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(header.getBoundingClientRect().width).toBeCloseTo(start + 16);
  });

  it('scrolls sideways in a box of its own', async () => {
    const { container } = await draw();

    const box = container.querySelector<HTMLElement>(
      '[data-slot="table-container"]',
    );
    const table = container.querySelector('[data-slot="table"]');
    expect(box?.contains(table ?? null)).toBe(true);
    // A table wider than the pane keeps its column widths and the box takes
    // the overflow, rather than every cell squeezing.
    expect(getComputedStyle(box!).overflowX).toBe('auto');
  });

  it('is announced by its caption', async () => {
    const { container } = await draw();

    await expect
      .element(page.getByRole('table', { name: caption }))
      .toBeInTheDocument();
    // The caption is the table's first child, which is where the parser keeps
    // it whatever the order of the markup, and it draws under the rows.
    const table = container.querySelector('[data-slot="table"]');
    expect(table?.firstElementChild?.tagName).toBe('CAPTION');
    expect(getComputedStyle(table!).captionSide).toBe('bottom');
  });

  it('says which cells every header heads', async () => {
    await draw();

    const headers = page.getByRole('columnheader').elements();
    expect(headers).toHaveLength(columns.length);
    for (const header of headers) {
      expect(header.getAttribute('scope')).toBe('col');
    }

    // A header inside the body names its row, and the caller's `scope` wins
    // over the `col` default.
    const rowHeaders = page.getByRole('rowheader').elements();
    expect(rowHeaders).toHaveLength(rows.length);
    for (const header of rowHeaders) {
      expect(header.getAttribute('scope')).toBe('row');
    }

    // Three data cells per body row, plus the footer's one spanning cell.
    expect(page.getByRole('cell').elements()).toHaveLength(
      rows.length * (columns.length - 1) + 1,
    );
  });

  it('fills a selected row and keeps the hover fill over it', async () => {
    const { container } = await draw('benchy.3mf');

    const body = container.querySelector('[data-slot="table-body"]');
    const plain = body?.children[0] as HTMLElement;
    const selected = body?.children[1] as HTMLElement;
    expect(selected.getAttribute('data-state')).toBe('selected');

    // `transition-colors` reports a fill part way through its interpolation,
    // and in another colour space, so comparing the strings would read a
    // change of notation as a change of colour.
    for (const row of [plain, selected]) row.style.transition = 'none';

    const fill = (row: HTMLElement): string =>
      getComputedStyle(row).backgroundColor;
    const resting = fill(plain);
    expect(fill(selected)).not.toBe(resting);

    // `state-selected:hover:bg-accent` is a compound selector, so a
    // selected row under the pointer still reads as hovered. A plain
    // `hover:bg-accent` ties with the selected rule on specificity and loses
    // on order.
    await page.getByRole('rowheader', { name: 'firmware.bbl' }).hover();
    let hovered = resting;
    // The pointer lands before the style pass runs, so the fill is waited for
    // rather than read once.
    await vi.waitFor(() => {
      hovered = fill(plain);
      expect(hovered).not.toBe(resting);
    });

    await page.getByRole('rowheader', { name: 'benchy.3mf' }).hover();
    await vi.waitFor(() => {
      expect(fill(selected)).toBe(hovered);
    });
  });

  it('draws no rule under the last row of the body', async () => {
    const { container } = await draw();

    const body = container.querySelector('[data-slot="table-body"]');
    const all = [...(body?.children ?? [])] as HTMLElement[];
    const last = all.at(-1)!;

    for (const row of all.slice(0, -1)) {
      expect(getComputedStyle(row).borderBottomWidth).not.toBe('0px');
    }
    // Without this the body would draw a line straight onto the footer's own.
    expect(getComputedStyle(last).borderBottomWidth).toBe('0px');
  });

  it('has nothing axe objects to', async () => {
    const { container } = await draw('benchy.3mf');

    await expectNoViolations(container);
  });
});
