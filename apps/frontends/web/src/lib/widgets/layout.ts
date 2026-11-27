import { persisted, type Persisted } from '../persisted';
import { isRecord, validId } from '../registry';
import type { WidgetSize } from './types';

// Which widgets a board shows, with each instance's size and configuration.
// Stored per display under a versioned key, so a future multi-display setup
// can give each screen its own layout. A placement whose widget is no longer
// registered is kept (the board offers to remove it) in case a plugin that
// provides it loads later.

export interface Placement {
  /** Instance id, unique within the board. */
  id: string;
  /** Widget definition id. */
  widget: string;
  /** Omitted until resized: the widget's defaultSize applies. */
  size?: WidgetSize;
  config: Record<string, unknown>;
}
export interface BoardLayout { version: 1; placements: Placement[] }

export const layoutKey = (displayId: string) => `ember:widgets:v1:${displayId}`;

const size = (v: unknown): WidgetSize | undefined =>
  isRecord(v) && Number.isInteger(v.cols) && Number.isInteger(v.rows) && (v.cols as number) > 0 && (v.rows as number) > 0
    ? { cols: v.cols as number, rows: v.rows as number } : undefined;

/** Any stored JSON (or null for a first visit) to a valid layout. */
export function parseLayout(raw: unknown, initial: readonly string[]): BoardLayout {
  if (!isRecord(raw) || raw.version !== 1 || !Array.isArray(raw.placements))
    return { version: 1, placements: initial.map((widget, i) => ({ id: `${widget}-${i + 1}`, widget, config: {} })) };
  const seen = new Set<string>();
  const placements: Placement[] = [];
  for (const p of raw.placements) {
    if (!isRecord(p) || !validId(p.id) || !validId(p.widget) || seen.has(p.id)) continue;
    seen.add(p.id);
    placements.push({ id: p.id, widget: p.widget, size: size(p.size), config: isRecord(p.config) ? p.config : {} });
  }
  return { version: 1, placements };
}

const boards = new Map<string, Persisted<BoardLayout>>();
/** The stored layout of a display's board. `initial` lists widget ids for a first visit. */
export function boardLayout(displayId: string, initial: readonly string[] = []): Persisted<BoardLayout> {
  let board = boards.get(displayId);
  if (!board) boards.set(displayId, board = persisted(layoutKey(displayId), raw => parseLayout(raw, initial)));
  return board;
}

const newId = (widget: string, layout: BoardLayout) => {
  for (let n = 1; ; n++) {
    const id = `${widget}-${n}`;
    if (!layout.placements.some(p => p.id === id)) return id;
  }
};

export const addPlacement = (layout: BoardLayout, widget: string, config: Record<string, unknown> = {}): BoardLayout =>
  ({ ...layout, placements: [...layout.placements, { id: newId(widget, layout), widget, config }] });

export const removePlacement = (layout: BoardLayout, id: string): BoardLayout =>
  ({ ...layout, placements: layout.placements.filter(p => p.id !== id) });

export const changePlacement = (layout: BoardLayout, id: string, change: { size?: WidgetSize; config?: Record<string, unknown> }): BoardLayout =>
  ({ ...layout, placements: layout.placements.map(p => p.id !== id ? p : {
    ...p, ...(change.size && { size: change.size }), ...(change.config && { config: { ...p.config, ...change.config } })
  }) });
