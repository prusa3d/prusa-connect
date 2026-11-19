type SortValue = string | number | undefined;

const collator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base',
});

const missing = (value: SortValue): boolean =>
  value === undefined ||
  value === '' ||
  (typeof value === 'number' && !Number.isFinite(value));

export function compareValues(
  a: SortValue,
  b: SortValue,
  order: 'asc' | 'desc',
): number {
  if (missing(a) || missing(b)) return Number(missing(a)) - Number(missing(b));

  const difference =
    typeof a === 'number' && typeof b === 'number'
      ? a - b
      : collator.compare(String(a), String(b));
  return order === 'asc' ? difference : -difference;
}

export function compareFiles(
  a: { folder: boolean; size: number; modified?: string | undefined },
  b: typeof a,
  nameA: string,
  nameB: string,
  sort?: { field: string; order: 'asc' | 'desc' },
): number {
  const group = Number(b.folder) - Number(a.folder);
  if (group !== 0) return group;

  if (sort?.field === 'size' && !a.folder) {
    return (
      compareValues(a.size, b.size, sort.order) || nameA.localeCompare(nameB)
    );
  }
  if (sort?.field === 'modified') {
    return (
      compareValues(
        Date.parse(a.modified ?? ''),
        Date.parse(b.modified ?? ''),
        sort.order,
      ) || nameA.localeCompare(nameB)
    );
  }
  return sort?.field === 'name'
    ? compareValues(nameA, nameB, sort.order)
    : nameA.localeCompare(nameB);
}
