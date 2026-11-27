// Shared pieces of the multi-display feature. Window Management
// (getScreenDetails) is Chromium-only; every caller must cope with `api()`
// returning null. See docs/multi-display.md for the behaviour this implements.
import { resolve } from '$app/paths';

// Local types: the API is not in TypeScript's DOM library.
export interface ScreenDetailed extends Screen {
  readonly availLeft: number; readonly availTop: number;
  readonly left: number; readonly top: number;
  readonly isPrimary: boolean; readonly label: string;
}
export interface ScreenDetails extends EventTarget {
  readonly screens: readonly ScreenDetailed[];
  readonly currentScreen: ScreenDetailed;
}
interface WindowManagement { getScreenDetails(): Promise<ScreenDetails> }

/** The Window Management API, or null when this browser lacks it. */
export function api(): WindowManagement | null {
  return 'getScreenDetails' in window ? window as unknown as WindowManagement : null;
}

/** 'granted' | 'prompt' | 'denied', or null if the permission is unknown here. */
export async function permission(): Promise<PermissionState | null> {
  try {
    return (await navigator.permissions.query({ name: 'window-management' as PermissionName })).state;
  } catch { return null; }
}

/** Whether more than one screen is attached; readable without the permission. */
export const extended = () => (screen as Screen & { isExtended?: boolean }).isExtended === true;

/** Stable identity for a screen: its label plus its position in the screen layout. */
export const screenKey = (s: ScreenDetailed) => `${s.label || 'screen'}@${s.left},${s.top}`;
/** Window name for a screen's workspace, so the browser can reuse an open one. */
export const windowName = (key: string) => 'ember-display-' + key.replace(/[^\w@,.-]/g, '_');
const slash = (path: string) => path.replace(/\/?$/, '/');
/** The Notes page and the workspace page, with the app's base path. */
export const notesPath = () => slash(resolve('/apps/notes'));
export const workspacePath = () => slash(resolve('/display'));
export const workspaceUrl = (key: string) => `${workspacePath()}?${new URLSearchParams({ display: key })}`;

/** Move and resize a window to fill a screen's available area. Browsers ignore
 * this for windows that were not opened by script; callers treat it as best effort. */
export function fit(w: Window, s: ScreenDetailed) {
  try {
    w.moveTo(s.availLeft, s.availTop);
    w.resizeTo(s.availWidth, s.availHeight);
  } catch { /* not permitted for this window */ }
}

/** Open (or reuse, when the name is still known) the workspace window for a screen. */
export function openWorkspace(s: ScreenDetailed): Window | null {
  const key = screenKey(s);
  const features = `popup,left=${s.availLeft},top=${s.availTop},width=${s.availWidth},height=${s.availHeight}`;
  return window.open(workspaceUrl(key), windowName(key), features);
}

// Messages between the app's windows. Workspaces announce their screen on load
// and leave on pagehide; any window can ask the others to identify themselves.
// 'arrange' asks workspaces to move back onto their own screen.
export type Message =
  | { type: 'query' }
  | { type: 'arrange' }
  | { type: 'notes' }
  | { type: 'workspace'; display: string }
  | { type: 'bye'; display: string };
export const channel = () => new BroadcastChannel('ember-displays');

const setting = 'ember.displays.openAtStartup';
const dismissed = 'ember.displays.promptDismissed';
function read(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}
function write(key: string, value: string) {
  try { localStorage.setItem(key, value); } catch { /* storage unavailable: keep defaults */ }
}
/** "Open on all displays at startup" (default on). */
export const openAtStartup = () => read(setting) !== 'false';
export const setOpenAtStartup = (on: boolean) => write(setting, String(on));
/** Whether the first-run "Use all displays" offer was dismissed. */
export const promptDismissed = () => read(dismissed) === 'true';
export const dismissPrompt = () => write(dismissed, 'true');
