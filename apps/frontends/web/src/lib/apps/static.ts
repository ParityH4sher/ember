import Icon, { type IconName } from '../ui/Icon.svelte';
import type { ServiceLink } from '../api';
import { httpUrl } from './registry';
import type { AppDefinition } from './types';

// Apps registered at startup (the root layout calls registerApps(staticApps)).
// Add an entry here for a new built-in page; plugins register theirs at
// runtime with registerApp(). See docs/app-launcher.md.

const icon = (name: IconName) => ({ component: Icon, props: { name } });

export const staticApps: readonly AppDefinition[] = [
  { id: 'ember.notes', name: 'Project notes', icon: icon('notes'), category: 'Workspace',
    description: 'Linked Markdown notes and project chat.', keywords: ['markdown', 'wiki', 'chat', 'projects'],
    launch: { type: 'internal', path: '/apps/notes/' } },
  { id: 'ember.plugins', name: 'Plugins', icon: icon('plugin'), category: 'Workspace',
    description: 'Run plugin operations and see their status.', keywords: ['integrations', 'extensions'],
    launch: { type: 'internal', path: '/plugins/' } },
  { id: 'ember.account', name: 'Profile & account', icon: icon('user'), category: 'You',
    description: 'Your identity and session.', keywords: ['profile', 'session', 'sign out'],
    launch: { type: 'internal', path: '/account/' } },
  { id: 'ember.settings', name: 'Settings', icon: icon('settings'), category: 'You',
    description: 'Theme and density for this browser.', keywords: ['preferences', 'theme', 'dark', 'light'],
    launch: { type: 'internal', path: '/settings/' } }
];

/** Registered at runtime for admins only; the API enforces the role. */
export const adminApp: AppDefinition = {
  id: 'ember.admin', name: 'Admin', icon: icon('shield'), category: 'Instance',
  description: 'Plugins, agents and access for this instance.', keywords: ['instance', 'operator'],
  launch: { type: 'internal', path: '/admin/' }
};

/** Shared services the operator configured (GET /v1/services), as launchable apps. */
export function serviceApps(services: readonly ServiceLink[]): AppDefinition[] {
  const used = new Set<string>();
  return services.filter(s => httpUrl(s.url)).map(s => {
    const base = 'service:' + (s.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 60) || 'link');
    let id = base;
    for (let n = 2; used.has(id); n++) id = `${base}-${n}`;
    used.add(id);
    return { id, name: s.name, icon: icon('external'), category: 'Services', description: `Opens ${new URL(s.url).host} in a new tab.`,
      keywords: ['service', new URL(s.url).host], launch: { type: 'url', url: s.url } };
  });
}
