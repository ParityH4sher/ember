import { prefs } from './prefs.svelte';

// The colour scheme in effect: the Settings choice, or the system's when it
// is "system". For code that cannot use the CSS tokens in app.css.
//
// The system preference is read once at module load and kept current by a
// listener, so reading it (often inside $derived) never writes state.

const query = typeof matchMedia === 'undefined' ? null : matchMedia('(prefers-color-scheme: dark)');
const system = $state({ dark: query?.matches ?? true });
query?.addEventListener('change', () => { system.dark = query.matches; });

/** Reactive when read in a component, $derived or effect. */
export function resolvedTheme(): 'light' | 'dark' {
  return prefs.theme === 'system' ? (system.dark ? 'dark' : 'light') : prefs.theme;
}
