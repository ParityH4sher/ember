import { listApps, registerApp, registerApps, unregisterApp } from './apps/registry';
import { staticApps } from './apps/static';
import type { AppDefinition } from './apps/types';
import { widgetRegistry } from './widgets/registry';
import { launcherWidget } from './launcher/widget';

// The composition root: the one module that names the built-in widgets and
// apps. The root layout imports it, so they are registered before any page
// renders. Widgets and apps never import each other; they meet in the
// registries. Apps that depend on the session or the API are registered at
// runtime by the root layout.

widgetRegistry.register(launcherWidget);
registerApps(staticApps);

/** For scripts loaded at runtime, which cannot import the app's modules.
    Definitions are validated like any other; see docs/app-launcher.md. */
export interface EmberGlobal {
  apps: {
    register(app: AppDefinition, options?: { replace?: boolean }): () => void;
    unregister(id: string): boolean;
    list(): readonly AppDefinition[];
  };
}
declare global {
  interface Window { ember?: EmberGlobal }
}
if (typeof window !== 'undefined')
  window.ember = { ...window.ember, apps: { register: registerApp, unregister: unregisterApp, list: listApps } };
