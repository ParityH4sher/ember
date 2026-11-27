// Launcher widgets shown in place on the page (grid mode). The app-wide
// launcher shortcut (AppShell) asks these first: if one is visible, the
// shortcut focuses its search instead of opening the centred launcher.

/** Focuses the launcher's search; returns false when it cannot (e.g. it is hidden). */
export type FocusInline = () => boolean;

const inline = new Set<FocusInline>();

/** Registers a launcher shown in place; returns a function that unregisters it. */
export function registerInlineLauncher(focus: FocusInline): () => void {
  inline.add(focus);
  return () => { inline.delete(focus); };
}

/** Focuses the first in-place launcher that can take focus, in registration
    order; returns false when there is none. */
export function focusInlineLauncher(): boolean {
  for (const focus of inline) if (focus()) return true;
  return false;
}
