import type { IconName } from '../ui/Icon.svelte';

// The app's own pages: the one list behind both the sidebar and the built-in
// apps in the app registry (lib/apps/static.ts derives them from it). The
// launcher reads only the app registry, so plugins and shared services appear
// next to these.
export interface NavItem {
  /** App registry id; stored in pinned and recent lists, so keep it stable. */
  id: string;
  href: string;
  label: string;
  icon: IconName;
  description?: string;
  keywords?: string[];
  /** Shown to admins only; the API still enforces the role. */
  admin?: boolean;
}
export interface NavSection { label: string; items: NavItem[] }

export const sections: NavSection[] = [
  { label: 'Workspace', items: [
    { id: 'ember.dashboard', href: '/', label: 'Dashboard', icon: 'home',
      description: 'Status, projects, services and your widgets.', keywords: ['home', 'start', 'widgets'] },
    { id: 'ember.desktop', href: '/desktop/', label: 'Desktop', icon: 'desktop',
      description: 'The desktop surface with the launcher circle.', keywords: ['circle', 'workspace'] },
    { id: 'ember.notes', href: '/apps/notes/', label: 'Project notes', icon: 'notes',
      description: 'Linked Markdown notes and project chat.', keywords: ['markdown', 'wiki', 'chat', 'projects'] },
    { id: 'ember.plugins', href: '/plugins/', label: 'Plugins', icon: 'plugin',
      description: 'Run plugin operations and see their status.', keywords: ['integrations', 'extensions'] }
  ] },
  { label: 'You', items: [
    { id: 'ember.account', href: '/account/', label: 'Profile & account', icon: 'user',
      description: 'Your identity and session.', keywords: ['profile', 'session', 'sign out'] },
    { id: 'ember.settings', href: '/settings/', label: 'Settings', icon: 'settings',
      description: 'Theme, density, launcher circle and shortcut.', keywords: ['preferences', 'theme', 'dark', 'light', 'shortcut'] }
  ] },
  { label: 'Instance', items: [
    { id: 'ember.admin', href: '/admin/', label: 'Admin', icon: 'shield', admin: true,
      description: 'Plugins, agents and access for this instance.', keywords: ['instance', 'operator'] },
    { id: 'ember.admin.ai', href: '/admin/ai/', label: 'Agents & models', icon: 'agent', admin: true,
      description: 'Model providers, models and agents for project chat.',
      keywords: ['ai', 'llm', 'claude', 'codex', 'openai', 'ollama', 'provider', 'api key'] }
  ] }
];

/** Sections with the items this person may see; empty sections dropped. */
export function visibleSections(admin: boolean): NavSection[] {
  return sections
    .map(s => ({ ...s, items: s.items.filter(i => !i.admin || admin) }))
    .filter(s => s.items.length);
}
