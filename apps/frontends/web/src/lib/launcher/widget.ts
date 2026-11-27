import Icon from '../ui/Icon.svelte';
import type { WidgetDefinition } from '../widgets/types';
import Launcher from './Launcher.svelte';
import LauncherSettings from './LauncherSettings.svelte';
import { defaultLauncherConfig, type LauncherConfig } from './config';

/** The app launcher, as a widget. It reads apps from the app registry and
    launches them through WidgetContext.launch; it knows no app in particular. */
export const launcherWidget: WidgetDefinition<LauncherConfig> = {
  id: 'ember.launcher',
  name: 'App launcher',
  icon: { component: Icon, props: { name: 'apps' } },
  description: 'Search, pin and open apps.',
  defaultSize: { cols: 2, rows: 3 },
  minSize: { cols: 1, rows: 1 },
  maxSize: { cols: 4, rows: 8 },
  defaultConfig: defaultLauncherConfig,
  component: Launcher,
  settingsComponent: LauncherSettings
};
