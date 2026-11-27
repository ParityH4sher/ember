import { DEFAULT_SHORTCUT } from './keyboard';
import { RECENT_MAX } from './state';

export interface LauncherConfig {
  /** 'grid' shows the launcher in place; 'compact' is one button that opens it as a popover. */
  mode: 'grid' | 'compact';
  /** Opens or focuses the launcher from anywhere on the page; '' turns it off. */
  shortcut: string;
  showRecent: boolean;
  /** How many recent apps to show (1–12). */
  recentLimit: number;
  /** The selected category tab, or 'all'. */
  category: string;
}

export const defaultLauncherConfig: LauncherConfig = {
  mode: 'grid', shortcut: DEFAULT_SHORTCUT, showRecent: true, recentLimit: 6, category: 'all'
};

/** Stored configuration can be anything; fall back field by field. */
export function launcherConfig(c: Partial<Record<keyof LauncherConfig, unknown>>): LauncherConfig {
  const d = defaultLauncherConfig;
  const limit = Number(c.recentLimit);
  return {
    mode: c.mode === 'compact' ? 'compact' : 'grid',
    shortcut: typeof c.shortcut === 'string' ? c.shortcut : d.shortcut,
    showRecent: typeof c.showRecent === 'boolean' ? c.showRecent : d.showRecent,
    recentLimit: Number.isInteger(limit) ? Math.min(Math.max(limit, 1), RECENT_MAX) : d.recentLimit,
    category: typeof c.category === 'string' && c.category ? c.category : d.category
  };
}
