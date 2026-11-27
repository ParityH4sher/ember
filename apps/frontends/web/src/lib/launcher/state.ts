import { persisted } from '../persisted';
import { isRecord, validId } from '../registry';

// Pinned and recently launched apps. Shared by every launcher instance and,
// through the storage event, by every window of this browser.
//
// Stale entries: an app can be pinned but not registered right now, for
// example while a plugin that registers it at runtime is still loading. Such
// entries are hidden, not deleted. `seen` records when each app was last
// registered; entries for apps not seen for STALE_AFTER are dropped.

export interface LauncherState {
  version: 1;
  /** In the order they were pinned. */
  pinned: string[];
  /** Most recent first. */
  recent: string[];
  /** App id → when it was last seen registered (ms since epoch). */
  seen: Record<string, number>;
}

export const STORAGE_KEY = 'ember:launcher:v1';
export const RECENT_MAX = 12;
export const STALE_AFTER = 30 * 24 * 3600_000;
const SEEN_REFRESH = 24 * 3600_000;

export const emptyState = (): LauncherState => ({ version: 1, pinned: [], recent: [], seen: {} });

const ids = (v: unknown, max = 200): string[] => Array.isArray(v) ? [...new Set(v.filter(validId))].slice(0, max) : [];

export function parseLauncherState(raw: unknown): LauncherState {
  if (!isRecord(raw) || raw.version !== 1) return emptyState();
  const seen: Record<string, number> = {};
  if (isRecord(raw.seen))
    for (const [id, t] of Object.entries(raw.seen)) if (validId(id) && Number.isFinite(t)) seen[id] = t as number;
  return { version: 1, pinned: ids(raw.pinned), recent: ids(raw.recent, RECENT_MAX), seen };
}

export function togglePin(s: LauncherState, id: string, now: number): LauncherState {
  const pinned = s.pinned.includes(id) ? s.pinned.filter(p => p !== id) : [...s.pinned, id];
  return { ...s, pinned, seen: { ...s.seen, [id]: now } };
}

export function recordLaunch(s: LauncherState, id: string, now: number): LauncherState {
  return { ...s, recent: [id, ...s.recent.filter(r => r !== id)].slice(0, RECENT_MAX), seen: { ...s.seen, [id]: now } };
}

/** Refreshes `seen` for registered apps (at most daily) and drops entries
    for apps unseen for STALE_AFTER. Returns null when nothing changed, so a
    caller can run it on every registry change without writing storage. */
export function reconcile(s: LauncherState, registered: ReadonlySet<string>, now: number): LauncherState | null {
  const keep = (id: string) => registered.has(id) || now - (s.seen[id] ?? -Infinity) < STALE_AFTER;
  const pinned = s.pinned.filter(keep), recent = s.recent.filter(keep);
  const used = new Set([...pinned, ...recent]);
  let changed = pinned.length !== s.pinned.length || recent.length !== s.recent.length;
  const seen: Record<string, number> = {};
  for (const id of used) {
    const last = s.seen[id];
    if (registered.has(id) && (last === undefined || now - last > SEEN_REFRESH)) { seen[id] = now; changed = true; }
    else if (last !== undefined) seen[id] = last;
  }
  if (Object.keys(s.seen).some(id => !used.has(id))) changed = true;
  return changed ? { version: 1, pinned, recent, seen } : null;
}

export const launcherState = persisted(STORAGE_KEY, parseLauncherState);
