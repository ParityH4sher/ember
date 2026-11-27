import type { AppDefinition } from '../apps/types';

// Filtering and ordering for the launcher. Pure functions; the component
// memoizes them with $derived, and each app's search terms are normalized
// once and cached, so typing stays cheap with hundreds of apps.

interface Terms { name: string; words: string[]; keywords: string[]; category: string }
const cache = new WeakMap<AppDefinition, Terms>();

/** Lower case without accents, so "cafe" finds "Café". */
export const normalize = (s: string) => s.normalize('NFKD').replace(/[̀-ͯ]/g, '').toLowerCase();

function terms(app: AppDefinition): Terms {
  let t = cache.get(app);
  if (!t) {
    const name = normalize(app.name);
    cache.set(app, t = {
      name, words: name.split(/[^\p{L}\p{N}]+/u).filter(Boolean),
      keywords: (app.keywords ?? []).map(normalize), category: normalize(app.category ?? '')
    });
  }
  return t;
}

function tokenScore(t: Terms, token: string): number {
  if (t.name === token) return 100;
  if (t.name.startsWith(token)) return 80;
  if (t.words.some(w => w.startsWith(token))) return 60;
  if (t.name.includes(token)) return 40;
  if (t.keywords.some(k => k.startsWith(token))) return 30;
  if (t.keywords.some(k => k.includes(token))) return 20;
  if (t.category.startsWith(token)) return 10;
  return 0;
}

/** 0 when the app does not match; otherwise higher is better. Every word of
    the query must match the name, a keyword or the category. */
export function matchScore(app: AppDefinition, query: string): number {
  const tokens = normalize(query).split(/\s+/).filter(Boolean);
  if (!tokens.length) return 0;
  const t = terms(app);
  let total = 0;
  for (const token of tokens) {
    const score = tokenScore(t, token);
    if (!score) return 0;
    total += score;
  }
  return total;
}

export const byName = (a: AppDefinition, b: AppDefinition) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id);

/** Matching apps: pinned first, then best match, then by name. */
export function searchApps(apps: readonly AppDefinition[], query: string, pinned: readonly string[] = []): AppDefinition[] {
  const pins = new Set(pinned);
  const hits: { app: AppDefinition; score: number; pinned: boolean }[] = [];
  for (const app of apps) {
    const score = matchScore(app, query);
    if (score) hits.push({ app, score, pinned: pins.has(app.id) });
  }
  hits.sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.score - a.score || byName(a.app, b.app));
  return hits.map(h => h.app);
}

/** Apps sorted by name with pinned ones first; optionally one category only. */
export function sortApps(apps: readonly AppDefinition[], pinned: readonly string[] = [], category?: string): AppDefinition[] {
  const pins = new Set(pinned);
  return apps.filter(a => category === undefined || a.category === category)
    .sort((a, b) => Number(pins.has(b.id)) - Number(pins.has(a.id)) || byName(a, b));
}

/** Registered apps for a list of ids, in that order; unknown ids are skipped. */
export function appsById(apps: readonly AppDefinition[], ids: readonly string[], limit = Infinity): AppDefinition[] {
  const byId = new Map(apps.map(a => [a.id, a]));
  const out: AppDefinition[] = [];
  for (const id of ids) {
    const app = byId.get(id);
    if (app) out.push(app);
    if (out.length >= limit) break;
  }
  return out;
}

/** Categories that at least one app uses, sorted. */
export const categoriesOf = (apps: readonly AppDefinition[]): string[] =>
  [...new Set(apps.flatMap(a => a.category ? [a.category] : []))].sort((a, b) => a.localeCompare(b));
