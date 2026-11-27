import type { Component } from 'svelte';

/** An app icon: an image URL or data URI, a component (receives `size` in
    pixels), or a component with fixed props, e.g. `{ component: Icon, props: { name: 'notes' } }`.
    Without an icon the launcher shows the app's initial. */
export type AppIcon =
  | string
  | Component<{ size?: number }>
  | { component: Component<any>; props?: Record<string, unknown> };

/** What happens when an app is launched. */
export type LaunchTarget =
  /** A route of this web app, e.g. '/apps/notes/'. Opens in the current window. */
  | { type: 'internal'; path: string }
  /** An http(s) URL. Opens in a new tab unless `newWindow` is false. */
  | { type: 'url'; url: string; newWindow?: boolean }
  /** Anything else. Errors are caught and logged. */
  | { type: 'custom'; run: (app: AppDefinition) => void | Promise<void> };

export interface AppDefinition {
  /** Unique, stable id; it is stored in pinned and recent lists. */
  id: string;
  name: string;
  icon?: AppIcon;
  description?: string;
  /** Groups apps under a tab in the launcher. */
  category?: string;
  /** Extra search terms. */
  keywords?: string[];
  launch: LaunchTarget;
}
