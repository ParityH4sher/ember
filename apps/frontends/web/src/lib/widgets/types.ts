import type { Component } from 'svelte';
import type { AppIcon } from '../apps/types';

/** A size in board grid cells. */
export interface WidgetSize { cols: number; rows: number }

/** A screen in a multi-display setup. */
export interface DisplayInfo { id: string; label: string }

/** Supplied by a multi-display feature when one is active; undefined otherwise.
    Ember has none yet, so widgets must treat it as optional. */
export interface DisplayCoordinator {
  /** All displays, including the current one. */
  list(): readonly DisplayInfo[];
  /** Asks the window on `displayId` to launch the app. */
  launchOn(displayId: string, appId: string): void;
}

/** What the host gives every widget instance. */
export interface WidgetContext {
  /** This placement of the widget; one widget can be placed several times. */
  instanceId: string;
  /** The display this board belongs to ('main' without multi-display). */
  displayId: string;
  /** The resolved colour scheme. Prefer the CSS tokens in app.css for styling. */
  theme: 'light' | 'dark';
  /** Launches a registered app by id; false if it is unknown or failed. */
  launch(appId: string): Promise<boolean>;
  displays?: DisplayCoordinator;
}

/** Props of a widget's component and of its settings component. */
export interface WidgetProps<C extends object = Record<string, unknown>> {
  /** This instance's configuration, always a complete C (defaults filled in). */
  config: C;
  /** Merges a change into the configuration and stores it. */
  setConfig(change: Partial<C>): void;
  context: WidgetContext;
}

export interface WidgetDefinition<C extends object = Record<string, unknown>> {
  /** Unique, stable id; it is stored in saved board layouts. */
  id: string;
  name: string;
  icon?: AppIcon;
  description: string;
  defaultSize: WidgetSize;
  minSize?: WidgetSize;
  maxSize?: WidgetSize;
  /** Configuration for a new instance; stored config is merged over it. */
  defaultConfig: C;
  component: Component<WidgetProps<C>>;
  /** Shown in the widget's settings panel, above the size controls. */
  settingsComponent?: Component<WidgetProps<C>>;
  /** Called when an instance appears on the board. */
  onMount?(context: WidgetContext): void;
  /** Called when an instance leaves the board (removed or the page closes). */
  onUnmount?(context: WidgetContext): void;
}

/** A widget definition of any config type, as stored in the registry. */
export type AnyWidget = WidgetDefinition<any>;
