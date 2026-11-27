---
sidebar_label: Widgets
sidebar_position: 5
---

# Widgets

A widget is a self-contained panel that people place, configure, resize and
remove on a board. The dashboard (`/`) hosts the first board. The
[app launcher](app-launcher.md) is the first widget. The board knows no
particular widget, and widgets never import each other. They meet in the
widget registry, the context the board passes in, and the app registry.

The code is in `apps/frontends/web/src/lib/widgets/`:

| File | Role |
| --- | --- |
| `types.ts` | The contract: `WidgetDefinition`, `WidgetProps`, `WidgetContext`. |
| `registry.ts` | `widgetRegistry`, validation and size clamping. |
| `layout.ts` | The stored layout of each board (which instances, their size and config). |
| `WidgetBoard.svelte` | The host: the **Add widget** list, frames and context. |
| `WidgetFrame.svelte` | One instance: title bar, settings, lifecycle hooks and an error boundary. |

## The contract

```ts
interface WidgetDefinition<C extends object> {
  id: string;                  // unique and stable; stored in layouts
  name: string;
  icon?: AppIcon;              // same forms as an app icon
  description: string;         // shown in the Add widget list
  defaultSize: WidgetSize;     // { cols, rows } in board cells
  minSize?: WidgetSize;
  maxSize?: WidgetSize;
  defaultConfig: C;            // stored config is merged over it
  component: Component<WidgetProps<C>>;
  settingsComponent?: Component<WidgetProps<C>>;
  onMount?(context: WidgetContext): void;
  onUnmount?(context: WidgetContext): void;
}

interface WidgetProps<C> {
  config: C;                              // defaults filled in
  setConfig(change: Partial<C>): void;    // merges and stores
  context: WidgetContext;
}

interface WidgetContext {
  instanceId: string;          // this placement; a widget can be placed twice
  displayId: string;           // 'main' until there is multi-display support
  theme: 'light' | 'dark';     // resolved from the Settings page
  launch(appId: string): Promise<boolean>;
  displays?: DisplayCoordinator;  // only when a multi-display feature provides it
}
```

The board is four columns wide on large screens, two on medium ones and one
on phones. Widths over two columns span the full row on medium screens. Sizes
are clamped to `minSize`/`maxSize` and to four columns. Rows are at least
7.5rem high and grow with content; the frame scrolls its body.

The frame gives every widget a title bar with **Configure** and **Remove**
buttons. **Configure** shows the widget's `settingsComponent`, if any, then
width and height fields. A widget that throws while rendering is contained by
a `svelte:boundary` ("This widget stopped working", with **Try again**), and
a failing lifecycle hook is logged. Neither affects the rest of the page.

### Rules for widgets

- Keep state in `config` through `setConfig` when it should survive a reload,
  and in component state otherwise. Shared state that is not per instance
  belongs in its own store (the launcher's pinned and recent apps are an
  example).
- Use the colour tokens in `app.css`, not literal colours, so that both themes
  work. Use `context.theme` only for things CSS cannot do, such as a chart
  library's theme.
- Open apps with `context.launch(appId)`, never by navigating directly. The
  host decides how a page opens.
- Clean up in the component (`$effect` teardown) or in `onUnmount`. The
  hooks run once per instance: when it appears and when it is removed or the
  page closes.
- `context.displays` is undefined today. Show display-related actions only
  when it is set.

## Building and registering a widget

A minimal clock widget with one setting:

```svelte
<!-- src/lib/clock/Clock.svelte -->
<script lang="ts">
  import type { WidgetProps } from '../widgets/types';
  import type { ClockConfig } from './widget';

  let { config }: WidgetProps<ClockConfig> = $props();
  let now = $state(new Date());
  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 1000);
    return () => clearInterval(timer);
  });
</script>

<p class="time">{now.toLocaleTimeString([], { timeZone: config.timeZone })}</p>

<style>
  .time { font-size: 2rem; margin: 0; color: var(--text) }
</style>
```

```svelte
<!-- src/lib/clock/ClockSettings.svelte -->
<script lang="ts">
  import type { WidgetProps } from '../widgets/types';
  import type { ClockConfig } from './widget';

  let { config, setConfig }: WidgetProps<ClockConfig> = $props();
</script>

<label>Time zone
  <input value={config.timeZone} onchange={e => setConfig({ timeZone: e.currentTarget.value })}>
</label>
```

```ts
// src/lib/clock/widget.ts
import type { WidgetDefinition } from '../widgets/types';
import Clock from './Clock.svelte';
import ClockSettings from './ClockSettings.svelte';

export interface ClockConfig { timeZone: string }

export const clockWidget: WidgetDefinition<ClockConfig> = {
  id: 'example.clock',
  name: 'Clock',
  description: 'The time in a chosen time zone.',
  defaultSize: { cols: 1, rows: 1 },
  maxSize: { cols: 2, rows: 2 },
  defaultConfig: { timeZone: 'UTC' },
  component: Clock,
  settingsComponent: ClockSettings
};
```

Register it in the composition root, `src/lib/setup.ts`:

```ts
import { clockWidget } from './clock/widget';
widgetRegistry.register(clockWidget);
```

It then appears under **Add widget** on the dashboard. `register()` returns a
function that unregisters the widget again. An invalid definition, or a second
widget with an existing id, logs an `[ember] Ignored widget …` warning and is
ignored. Pass `{ replace: true }` to replace a widget on purpose. The registry
also provides `unregister(id)`, `get(id)`, `list()` and `subscribe()` (it is a
Svelte store).

Widgets are compiled into the web app. The browser loads no plugin code (see
[modularity](architecture/modularity.md)), so a third-party widget is a
reviewed source change, not a download.

## Storage

Each board's layout is stored in `localStorage` under
`ember:widgets:v1:<displayId>`. It holds the placements, with id, widget id,
size and config. A first visit places the board's initial widgets (the
dashboard places the launcher). After that, the stored layout wins, even
when it is empty. A placement whose widget is not registered stays on the board
as "not installed", with a **Remove** button, in case the widget is only
missing for a while. Like the [display preferences](https://github.com/ParityH4sher/ember/blob/main/apps/frontends/README.md),
layouts belong to one browser and do not follow people to other devices.
Other windows of the same browser pick up changes through the `storage` event.

## Tests

`apps/frontends/web/tests/unit/widgets.spec.ts` registers a dummy widget that
nothing else in Ember knows about. It covers validation, lifecycle hooks, size
clamping and layout changes. `tests/ui/launcher-widget.spec.ts` configures, resizes,
removes and adds back the launcher on the real board. It also covers a
placement whose widget is not installed.
