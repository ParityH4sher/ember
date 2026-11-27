import Icon from '../ui/Icon.svelte';
import type { ServiceLink } from '../api';
import { sections, type NavItem } from '../shell/nav';
import { httpUrl } from './registry';
import type { AppDefinition } from './types';

// Built-in apps: Ember's own pages, derived from the sidebar's list
// (shell/nav.ts) so there is one list to maintain. setup.ts registers them
// at startup; plugins register theirs at runtime with registerApp(). See
// docs/app-launcher.md.

const fromNav = (item: NavItem, category: string): AppDefinition => ({
  id: item.id, name: item.label, icon: { component: Icon, props: { name: item.icon } }, category,
  description: item.description, keywords: item.keywords, launch: { type: 'internal', path: item.href }
});

const all = sections.flatMap(s => s.items.map(item => ({ item, app: fromNav(item, s.label) })));

/** Pages everyone may open. */
export const staticApps: readonly AppDefinition[] = all.filter(a => !a.item.admin).map(a => a.app);

/** Admin-only pages; the root layout registers them at runtime for admins (the API enforces the role). */
export const adminApps: readonly AppDefinition[] = all.filter(a => a.item.admin).map(a => a.app);

/** Shared services the operator configured (GET /v1/services), as launchable apps. */
export function serviceApps(services: readonly ServiceLink[]): AppDefinition[] {
  const used = new Set<string>();
  return services.filter(s => httpUrl(s.url)).map(s => {
    const base = 'service:' + (s.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 60) || 'link');
    let id = base;
    for (let n = 2; used.has(id); n++) id = `${base}-${n}`;
    used.add(id);
    return { id, name: s.name, icon: { component: Icon, props: { name: 'external' } }, category: 'Services',
      description: `Opens ${new URL(s.url).host} in a new tab.`, keywords: ['service', new URL(s.url).host],
      launch: { type: 'url', url: s.url } };
  });
}
