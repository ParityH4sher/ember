import { RECENT_MAX } from './state';

// Configuration of one launcher widget. The keyboard shortcut is not here:
// there is one shortcut for the whole app (Settings › Desktop, stored in
// lib/prefs.svelte.ts), and it opens the shell's launcher.

export interface LauncherConfig {
  /** 'grid' shows the launcher in place; 'compact' is one button that opens it as a popover. */
  mode: 'grid' | 'compact';
  showRecent: boolean;
  /** How many recent apps to show (1–12). */
  recentLimit: number;
  /** The selected category tab, or 'all'. */
  category: string;
}

export const defaultLauncherConfig: LauncherConfig = { mode: 'grid', showRecent: true, recentLimit: 6, category: 'all' };

/** Stored configuration can be anything; fall back field by field. Unknown
    fields, such as the `shortcut` older versions stored, are ignored. */
export function launcherConfig(c: Partial<Record<keyof LauncherConfig, unknown>>): LauncherConfig {
  const d = defaultLauncherConfig;
  const limit = Number(c.recentLimit);
  return {
    mode: c.mode === 'compact' ? 'compact' : 'grid',
    showRecent: typeof c.showRecent === 'boolean' ? c.showRecent : d.showRecent,
    recentLimit: Number.isInteger(limit) ? Math.min(Math.max(limit, 1), RECENT_MAX) : d.recentLimit,
    category: typeof c.category === 'string' && c.category ? c.category : d.category
  };
}
