import { expect, test } from '@playwright/test';
import type { AppDefinition } from '../../src/lib/apps/types';
import { appsById, categoriesOf, matchScore, searchApps, sortApps } from '../../src/lib/launcher/search';
import {
  emptyState, parseLauncherState, reconcile, recordLaunch, RECENT_MAX, STALE_AFTER, togglePin
} from '../../src/lib/launcher/state';
import { formatShortcut, matchesShortcut, moveFocus, parseShortcut } from '../../src/lib/launcher/keyboard';
import { launcherConfig } from '../../src/lib/launcher/config';

const app = (id: string, name: string, extra: Partial<AppDefinition> = {}): AppDefinition =>
  ({ id, name, launch: { type: 'internal', path: `/${id}/` }, ...extra });

const apps = [
  app('notes', 'Project notes', { category: 'Workspace', keywords: ['markdown', 'wiki'] }),
  app('plugins', 'Plugins', { category: 'Workspace', keywords: ['integrations'] }),
  app('settings', 'Settings', { category: 'You', keywords: ['theme', 'dark'] }),
  app('cafe', 'Café orders'),
  app('notifier', 'Notifier')
];
const ids = (list: AppDefinition[]) => list.map(a => a.id);

test.describe('search', () => {
  test('matches names, words, keywords and categories, case- and accent-insensitively', () => {
    expect(ids(searchApps(apps, 'NOTES'))).toEqual(['notes']);
    expect(ids(searchApps(apps, 'wiki'))).toEqual(['notes']);
    expect(ids(searchApps(apps, 'cafe'))).toEqual(['cafe']);
    expect(ids(searchApps(apps, 'you'))).toEqual(['settings']);
    expect(searchApps(apps, 'zzz')).toEqual([]);
    expect(searchApps(apps, '   ')).toEqual([]);
  });

  test('every word must match', () => {
    expect(ids(searchApps(apps, 'project markdown'))).toEqual(['notes']);
    expect(searchApps(apps, 'project theme')).toEqual([]);
  });

  test('ranks name prefixes above word starts above keywords', () => {
    // "not": "Notifier" starts with it; "Project notes" has a word starting with it.
    expect(ids(searchApps(apps, 'not'))).toEqual(['notifier', 'notes']);
    expect(matchScore(apps[3], 'café orders')).toBeGreaterThan(matchScore(apps[3], 'orders'));
    const keywordOnly = app('k', 'Zeta', { keywords: ['plugins'] });
    expect(ids(searchApps([keywordOnly, ...apps], 'plugins'))).toEqual(['plugins', 'k']);
  });

  test('pinned matches come first', () => {
    expect(ids(searchApps(apps, 'not', ['notes']))).toEqual(['notes', 'notifier']);
  });

  test('sortApps sorts by name, pinned first, optionally one category', () => {
    expect(ids(sortApps(apps))).toEqual(['cafe', 'notifier', 'plugins', 'notes', 'settings']);
    expect(ids(sortApps(apps, ['settings']))).toEqual(['settings', 'cafe', 'notifier', 'plugins', 'notes']);
    expect(ids(sortApps(apps, ['plugins'], 'Workspace'))).toEqual(['plugins', 'notes']);
  });

  test('appsById keeps order, skips unknown ids and honours a limit', () => {
    expect(ids(appsById(apps, ['settings', 'gone', 'notes']))).toEqual(['settings', 'notes']);
    expect(ids(appsById(apps, ['settings', 'gone', 'notes', 'cafe'], 2))).toEqual(['settings', 'notes']);
  });

  test('categoriesOf lists used categories once, sorted', () => {
    expect(categoriesOf(apps)).toEqual(['Workspace', 'You']);
    expect(categoriesOf([apps[3]])).toEqual([]);
  });

  test('stays fast with many apps', () => {
    const many = Array.from({ length: 2000 }, (_, i) => app(`app-${i}`, `Application ${i}`, { keywords: [`tag${i % 50}`] }));
    searchApps(many, 'warm'); // fills the term cache
    const start = performance.now();
    for (const q of ['a', 'ap', 'app', 'appl', 'application 1', 'tag4']) searchApps(many, q);
    expect(performance.now() - start).toBeLessThan(250);
  });
});

test.describe('pinned and recent state', () => {
  test('pins in order and unpins', () => {
    let s = togglePin(emptyState(), 'a', 1);
    s = togglePin(s, 'b', 2);
    expect(s.pinned).toEqual(['a', 'b']);
    s = togglePin(s, 'a', 3);
    expect(s.pinned).toEqual(['b']);
  });

  test('recent is most-recent-first, without duplicates, capped', () => {
    let s = emptyState();
    for (const id of ['a', 'b', 'a', 'c']) s = recordLaunch(s, id, 1);
    expect(s.recent).toEqual(['c', 'a', 'b']);
    for (let i = 0; i < 20; i++) s = recordLaunch(s, `x${i}`, 1);
    expect(s.recent).toHaveLength(RECENT_MAX);
    expect(s.recent[0]).toBe('x19');
  });

  test('parsing tolerates anything stored', () => {
    for (const raw of [null, 42, 'x', [], { version: 2, pinned: ['a'] }]) expect(parseLauncherState(raw)).toEqual(emptyState());
    expect(parseLauncherState({ version: 1, pinned: ['a', 'a', 7, 'b c', 'b'], recent: 'no', seen: { a: 5, b: 'x' } }))
      .toEqual({ version: 1, pinned: ['a', 'b'], recent: [], seen: { a: 5 } });
  });

  test('unregistered apps stay hidden-but-kept until stale, then are dropped', () => {
    const day = 24 * 3600_000;
    let s = togglePin(emptyState(), 'late-plugin', 0);
    s = recordLaunch(s, 'late-plugin', 0);
    // Not registered yet (a plugin still loading): kept.
    expect(reconcile(s, new Set(), day * 2)).toBeNull();
    // Gone for longer than STALE_AFTER: dropped from both lists.
    const pruned = reconcile(s, new Set(), STALE_AFTER + 1);
    expect(pruned).toEqual({ version: 1, pinned: [], recent: [], seen: {} });
  });

  test('registered apps refresh their last-seen time at most daily', () => {
    const day = 24 * 3600_000;
    const s = togglePin(emptyState(), 'a', 0);
    expect(reconcile(s, new Set(['a']), day / 2)).toBeNull();
    expect(reconcile(s, new Set(['a']), day * 2)?.seen).toEqual({ a: day * 2 });
  });
});

test.describe('keyboard navigation', () => {
  // Sections of 5, 0 (empty, skipped) and 3 items; 3 columns everywhere.
  const sizes = [5, 0, 3];
  const cols = () => 3;
  const move = (key: string, section: number, index: number) => moveFocus(key, { section, index }, sizes, cols);

  test('left and right move linearly, across sections', () => {
    expect(move('ArrowRight', 0, 0)).toEqual({ section: 0, index: 1 });
    expect(move('ArrowRight', 0, 4)).toEqual({ section: 2, index: 0 });
    expect(move('ArrowLeft', 2, 0)).toEqual({ section: 0, index: 4 });
    expect(move('ArrowLeft', 0, 0)).toBeNull();
    expect(move('ArrowRight', 2, 2)).toBeNull();
  });

  test('up and down move by rows, into the next section and back to search', () => {
    expect(move('ArrowDown', 0, 1)).toEqual({ section: 0, index: 4 });
    // Third column of row 1 has nothing below it in row 2: last item.
    expect(move('ArrowDown', 0, 2)).toEqual({ section: 0, index: 4 });
    expect(move('ArrowDown', 0, 4)).toEqual({ section: 2, index: 1 });
    expect(move('ArrowDown', 2, 1)).toBeNull();
    expect(move('ArrowUp', 2, 1)).toEqual({ section: 0, index: 4 });
    expect(move('ArrowUp', 2, 2)).toEqual({ section: 0, index: 4 });
    expect(move('ArrowUp', 2, 0)).toEqual({ section: 0, index: 3 });
    expect(move('ArrowUp', 0, 4)).toEqual({ section: 0, index: 1 });
    expect(move('ArrowUp', 0, 1)).toBe('search');
  });

  test('Home and End stay in the section; other keys do nothing', () => {
    expect(move('Home', 0, 3)).toEqual({ section: 0, index: 0 });
    expect(move('End', 0, 0)).toEqual({ section: 0, index: 4 });
    expect(move('a', 0, 0)).toBeNull();
  });
});

test.describe('shortcuts', () => {
  test('parses, normalizes and formats', () => {
    expect(parseShortcut('Alt+Shift+L')).toEqual({ code: 'KeyL', ctrl: false, alt: true, shift: true, meta: false });
    expect(formatShortcut(parseShortcut(' ctrl + alt + space ')!)).toBe('Ctrl+Alt+Space');
    expect(formatShortcut(parseShortcut('cmd+k')!)).toBe('Meta+K');
    expect(formatShortcut(parseShortcut('Option+1')!)).toBe('Alt+1');
    expect(parseShortcut('F2')?.code).toBe('F2');
  });

  test('rejects shortcuts that would take over typing, and nonsense', () => {
    for (const bad of ['', 'L', 'Shift+L', 'Hyper+L', 'Alt+', 'Alt+Shift+Tab', 'Ctrl+Alt+Shift+Meta+Super+L'])
      expect(parseShortcut(bad), bad).toBeNull();
  });

  test('matches physical keys with exact modifiers', () => {
    const s = parseShortcut('Alt+Shift+L')!;
    const e = { code: 'KeyL', ctrlKey: false, altKey: true, shiftKey: true, metaKey: false };
    expect(matchesShortcut(e, s)).toBe(true);
    expect(matchesShortcut({ ...e, ctrlKey: true }, s)).toBe(false);
    expect(matchesShortcut({ ...e, code: 'KeyK' }, s)).toBe(false);
  });
});

test('launcher config falls back field by field', () => {
  expect(launcherConfig({ mode: 'compact', recentLimit: 99, shortcut: 7, showRecent: 'yes', category: '' }))
    .toEqual({ mode: 'compact', shortcut: 'Alt+Shift+L', showRecent: true, recentLimit: 12, category: 'all' });
});
