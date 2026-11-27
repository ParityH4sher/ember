import type { IconName } from '../ui/Icon.svelte';

// The app's destinations, shown in the sidebar and in the launcher.
export interface NavItem { href: string; label: string; icon: IconName; admin?: boolean }
export interface NavSection { label: string; items: NavItem[] }

export const sections: NavSection[] = [
  { label: 'Workspace', items: [
    { href: '/', label: 'Dashboard', icon: 'home' },
    { href: '/desktop/', label: 'Desktop', icon: 'desktop' },
    { href: '/apps/notes/', label: 'Project notes', icon: 'notes' },
    { href: '/plugins/', label: 'Plugins', icon: 'plugin' }
  ] },
  { label: 'You', items: [
    { href: '/account/', label: 'Profile & account', icon: 'user' },
    { href: '/settings/', label: 'Settings', icon: 'settings' }
  ] },
  { label: 'Instance', items: [{ href: '/admin/', label: 'Admin', icon: 'shield', admin: true }] }
];

/** Sections with the items this person may see; empty sections dropped. */
export function visibleSections(admin: boolean): NavSection[] {
  return sections
    .map(s => ({ ...s, items: s.items.filter(i => !i.admin || admin) }))
    .filter(s => s.items.length);
}

/** The global launcher shortcut: Ctrl+K, or ⌘K on Apple keyboards. */
export function isLauncherShortcut(e: KeyboardEvent): boolean {
  return (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'k';
}
