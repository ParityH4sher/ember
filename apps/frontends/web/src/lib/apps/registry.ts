import { createRegistry, isRecord, nonEmpty, validId, type Registry, type Warn } from '../registry';
import type { AppDefinition } from './types';

/** A same-origin path such as '/apps/notes/', never '//host' or a backslash path. */
export const internalPath = (v: unknown): v is string =>
  typeof v === 'string' && v.startsWith('/') && !v.startsWith('//') && !v.includes('\\') && !/[\x00-\x1f\x7f]/.test(v);

/** An http(s) URL without credentials. */
export function httpUrl(v: unknown): v is string {
  if (typeof v !== 'string') return false;
  try {
    const url = new URL(v);
    return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password;
  } catch {
    return false;
  }
}

/** Image icons: same-origin paths, http(s) URLs or data:image URIs. */
const imageIcon = (v: string) => internalPath(v) || httpUrl(v) || /^data:image\/[a-z0-9.+-]+[;,]/i.test(v);

export function validateApp(entry: unknown): string[] {
  if (!isRecord(entry)) return ['an app definition must be an object'];
  const problems: string[] = [];
  const { id, name, icon, description, category, keywords, launch } = entry;
  if (!validId(id)) problems.push('"id" must be a non-empty string of letters, digits, ".", "_", ":" or "-"');
  if (!nonEmpty(name)) problems.push('"name" is required');
  if (icon !== undefined) {
    const ok = typeof icon === 'string' ? imageIcon(icon)
      : typeof icon === 'function' || (isRecord(icon) && typeof icon.component === 'function');
    if (!ok) problems.push('"icon" must be an image URL, a data:image URI or a component');
  }
  if (description !== undefined && typeof description !== 'string') problems.push('"description" must be a string');
  if (category !== undefined && !nonEmpty(category)) problems.push('"category" must be a non-empty string');
  if (keywords !== undefined && !(Array.isArray(keywords) && keywords.every(k => typeof k === 'string')))
    problems.push('"keywords" must be an array of strings');
  if (!isRecord(launch)) problems.push('"launch" is required');
  else if (launch.type === 'internal') {
    if (!internalPath(launch.path)) problems.push('an internal launch needs a "path" starting with "/"');
  } else if (launch.type === 'url') {
    if (!httpUrl(launch.url)) problems.push('a url launch needs an http(s) "url" without credentials');
  } else if (launch.type === 'custom') {
    if (typeof launch.run !== 'function') problems.push('a custom launch needs a "run" function');
  } else problems.push('"launch.type" must be "internal", "url" or "custom"');
  return problems;
}

export type AppRegistry = Registry<AppDefinition>;

export const createAppRegistry = (warn?: Warn): AppRegistry => createRegistry<AppDefinition>('app', validateApp, warn);

/** The apps the launcher shows. */
export const appRegistry = createAppRegistry();

/** Registers an app; returns a function that unregisters it. Bad definitions log a warning. */
export const registerApp = (app: AppDefinition, options?: { replace?: boolean }) => appRegistry.register(app, options);
/** Registers several apps; returns one function that unregisters them all. */
export function registerApps(apps: readonly AppDefinition[]): () => void {
  const undo = apps.map(app => registerApp(app));
  return () => undo.forEach(fn => fn());
}
export const unregisterApp = (id: string) => appRegistry.unregister(id);
export const getApp = (id: string) => appRegistry.get(id);
export const listApps = () => appRegistry.list();
