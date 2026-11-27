// Display preferences for this browser. The API stores no per-user data, so
// these live in localStorage and are not synced between devices. Storage can
// be unavailable (private windows, blocked site data); defaults apply then.

import { DEFAULT_SHORTCUT, formatShortcut, parseShortcut } from './launcher/keyboard';

export type Theme = 'system' | 'light' | 'dark';
export type Density = 'comfortable' | 'compact';
export interface Prefs {
  theme: Theme; density: Density;
  /** Show the central launcher circle on the desktop. The shortcut works either way. */
  launcherCircle: boolean;
  /** Opens the launcher from any page, e.g. "Mod+K" (Ctrl+K, ⌘K on Apple); '' turns it off. */
  launcherShortcut: string;
}

const KEY = 'ember:prefs';
export const defaults: Prefs = { theme: 'system', density: 'comfortable', launcherCircle: true, launcherShortcut: DEFAULT_SHORTCUT };
const themes: Theme[] = ['system', 'light', 'dark'];
const densities: Density[] = ['comfortable', 'compact'];

function read(): Prefs {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(KEY) ?? 'null');
    if (!raw || typeof raw !== 'object') return { ...defaults };
    const { theme, density, launcherCircle, launcherShortcut } = raw as Record<string, unknown>;
    const shortcut = typeof launcherShortcut === 'string' ? launcherShortcut.trim() : null;
    const parsed = shortcut ? parseShortcut(shortcut) : null;
    return {
      theme: themes.includes(theme as Theme) ? theme as Theme : defaults.theme,
      density: densities.includes(density as Density) ? density as Density : defaults.density,
      launcherCircle: typeof launcherCircle === 'boolean' ? launcherCircle : defaults.launcherCircle,
      launcherShortcut: shortcut === '' ? '' : parsed ? formatShortcut(parsed) : defaults.launcherShortcut
    };
  } catch {
    return { ...defaults };
  }
}

export const prefs = $state<Prefs>({ ...defaults });

function apply() {
  document.documentElement.dataset.theme = prefs.theme;
  document.documentElement.dataset.density = prefs.density;
}

/** Loads stored preferences; call once in the browser. */
export function loadPrefs() {
  Object.assign(prefs, read());
  apply();
}

/** Applies and stores; returns false when storage refused the write. */
export function savePrefs(next: Partial<Prefs>): boolean {
  Object.assign(prefs, next);
  apply();
  try {
    localStorage.setItem(KEY, JSON.stringify(prefs));
    return true;
  } catch {
    return false;
  }
}
