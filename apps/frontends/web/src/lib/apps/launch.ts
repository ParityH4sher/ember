import type { AppDefinition } from './types';

/** How the host page opens things. The page supplies these, so that apps and
    widgets never navigate by themselves. */
export interface Openers {
  /** Opens a route of this web app (client-side navigation). */
  openInternal(path: string): void | Promise<void>;
  /** Opens an external URL; `newWindow` false means the current tab. */
  openUrl(url: string, newWindow: boolean): void;
}

/** Browser defaults; the dashboard passes SvelteKit's goto for internal paths. */
export const browserOpeners: Openers = {
  openInternal: path => location.assign(path),
  openUrl(url, newWindow) {
    if (newWindow) window.open(url, '_blank', 'noopener,noreferrer');
    else location.assign(url);
  }
};

/** Launches an app; resolves false, with a warning, if launching failed. */
export async function launchApp(app: AppDefinition, openers: Openers, warn = (m: string) => console.warn(m)): Promise<boolean> {
  try {
    const target = app.launch;
    if (target.type === 'internal') await openers.openInternal(target.path);
    else if (target.type === 'url') openers.openUrl(target.url, target.newWindow !== false);
    else await target.run(app);
    return true;
  } catch (error) {
    warn(`[ember] Could not launch app "${app.id}": ${error instanceof Error ? error.message : error}`);
    return false;
  }
}

/** A short, readable description of where an app opens. */
export function describeTarget(app: AppDefinition): string {
  const target = app.launch;
  if (target.type === 'internal') return `Ember page ${target.path}`;
  if (target.type === 'url') return `${target.url} (${target.newWindow === false ? 'this tab' : 'new tab'})`;
  return 'Runs an action';
}
