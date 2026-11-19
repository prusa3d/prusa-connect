import type { Device } from '../../domain';

type Tree = NonNullable<Device['tools']>;
type Tool = NonNullable<Tree['tools'][string]>;

/** One row of the table: a head, or a slot of the feeder on a head. */
interface Row {
  id: string;
  slot: boolean;
  /** Null is a tool the printer has switched off. */
  tool: Tool | null;
}

// Ids are whole numbers joined by dots, and order part by part as numbers,
// so 0.10 comes after 0.2 and a head comes before its slots.
export const compareIds = (left: string, right: string): number => {
  const a = left.split('.').map(Number);
  const b = right.split('.').map(Number);

  for (let i = 0; i < Math.min(a.length, b.length); i += 1) {
    const step = (a[i] ?? 0) - (b[i] ?? 0);
    if (step !== 0) return step;
  }

  return a.length - b.length;
};

// A slot is listed under its head while the head's feeder covers it. A slot
// the printer still holds under a feeder it switched off is left out.
const covered = (tree: Tree, id: string): boolean => {
  const at = id.lastIndexOf('.');
  if (at < 0) return true;

  const parent = tree.tools[id.slice(0, at)];
  const index = Number(id.slice(at + 1));

  return (
    parent !== null &&
    parent !== undefined &&
    index < (parent.feeder?.slot_count ?? 0) &&
    covered(tree, id.slice(0, at))
  );
};

export const toolRowsOf = (tools: Device['tools']): Row[] =>
  tools === undefined
    ? []
    : Object.keys(tools.tools)
        .filter((id) => covered(tools, id))
        .sort(compareIds)
        .map((id) => ({
          id,
          slot: id.includes('.'),
          tool: tools.tools[id] ?? null,
        }));
