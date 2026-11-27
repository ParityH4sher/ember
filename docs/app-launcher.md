---
sidebar_label: App launcher
sidebar_position: 6
---

# App launcher

The app launcher is a [widget](widgets.md) on the dashboard. It shows every
registered app in a grid you can search, with category tabs, pinned apps and
recently used apps. It reads apps from the **app registry**, which is separate
from the widget registry. Registering an app needs no change to the launcher,
and an app registered while the page is open appears at once.

Code: `apps/frontends/web/src/lib/apps/` (registry, launch targets, static
list) and `src/lib/launcher/` (the widget).

## Registering apps

An app definition:

```ts
interface AppDefinition {
  id: string;             // unique and stable: letters, digits, . _ : -
  name: string;
  icon?: AppIcon;         // image URL or data:image URI, a component, or { component, props }
  description?: string;   // shown as a tooltip and in App info
  category?: string;      // becomes a tab
  keywords?: string[];    // extra search terms
  launch: LaunchTarget;
}
```

Without an icon, the launcher shows the app's initial.

### Launch targets

| Type | Shape | Behaviour |
| --- | --- | --- |
| `internal` | `{ type: 'internal', path: '/apps/notes/' }` | A page of the web app. The dashboard opens it with client-side navigation. The path must start with `/` (not `//`). |
| `url` | `{ type: 'url', url: 'https://…', newWindow?: boolean }` | An http(s) URL without credentials. Opens in a new tab without a referrer, or in the current tab with `newWindow: false`. |
| `custom` | `{ type: 'custom', run: app => … }` | Any function, sync or async. If it throws, the launcher reports "Could not open …" and logs a warning. |

### At startup (static list)

Built-in apps are listed in `src/lib/apps/static.ts` (`staticApps`) and
registered by `src/lib/setup.ts` before any page renders:

```ts
export const staticApps: readonly AppDefinition[] = [
  { id: 'ember.notes', name: 'Project notes', icon: icon('notes'), category: 'Workspace',
    keywords: ['markdown', 'wiki'], launch: { type: 'internal', path: '/apps/notes/' } },
  // …
];
```

### At runtime

Code in the web app imports the registry:

```ts
import { registerApp, unregisterApp } from '../lib/apps/registry';

const undo = registerApp({
  id: 'workshop.kiln', name: 'Kiln monitor', category: 'Workshop', keywords: ['ceramics'],
  launch: { type: 'url', url: 'https://kiln.example.org/' }
});
// later
undo();                       // or unregisterApp('workshop.kiln')
```

The root layout uses this to register apps that depend on the session or the
API:

- one app per shared service from `GET /v1/services`, under **Services**
- **Admin**, for admins only (the API still enforces the role)

A script that is not bundled with the app can use the same registry through
`window.ember.apps.register(app)`, `.unregister(id)` and `.list()`.

### Validation

Definitions are checked when registered: id format, a name, a known launch
type with a safe path or URL, icon form and `keywords` type. A bad definition
is never registered. It logs a warning that names the app and every problem,
for example:

```text
[ember] Ignored app "test.bad": "name" is required; a url launch needs an http(s) "url" without credentials.
```

A second app with an id that is already registered is ignored with a warning,
unless it is registered with `{ replace: true }`. Registration never throws.

## Using the launcher

- **Search** filters by name, keywords and category as you type, ignoring
  case and accents. Every word must match. Results are ordered pinned first,
  then by match quality: exact name, name prefix, word prefix, then keyword
  matches. Screen readers hear the number of results.
- **Enter** in the search box opens the top result.
- **Categories:** when any app has a category, tabs show *All* plus each
  category. The selected tab is saved with the widget.
- **Pinned** apps show first, in the order they were pinned. **Recent** lists
  the last apps opened from the launcher, newest first.
- **App menu:** right-click, long-press on touch screens, or press
  **Shift+F10** or the **Menu** key on an app. It offers **Pin to top** or
  **Unpin**, **Open on *display*** (only when a multi-display feature is
  active, which Ember does not have yet), and **App info** (description,
  category, where it opens, keywords and id).

### Keyboard

| Key | Where | Action |
| --- | --- | --- |
| **Alt+Shift+L** (configurable) | Anywhere on the page | Focus the launcher's search; in compact mode, open it first. |
| Type | Search box | Filter. |
| **Enter** | Search box | Open the top result. |
| **↓** | Search box | Move to the first app. |
| **← → ↑ ↓** | App grid | Move between apps; up and down move by rows and between sections; **↑** on the first row returns to search. |
| **Home** / **End** | App grid | First or last app of the section. |
| **Enter** / **Space** | App | Open it. |
| **Shift+F10** / **Menu** | App | Open the app menu (**↑ ↓**, **Home**, **End**, **Escape**). |
| **Escape** | Search or grid | Clear the search; with an empty search, close the compact popover. |
| **← →**, **Home**, **End** | Category tabs | Change tab. |

Only one app is a tab stop at a time (roving tabindex), so **Tab** leaves the
grid in one step. The shortcut matches physical keys, so it works with any
keyboard layout and with Option on macOS. Alt+Shift+L was chosen because no
common browser binds it by default and desktops rarely do. On macOS it
replaces typing "Ò" while the page has focus. If it clashes with your system
or an input method, change it. A shortcut must include Ctrl, Alt or
Meta (or be F1–F12), so it cannot take over plain typing. With several
launchers on one page, the first one claims the shortcut.

## Configuration

Open **Configure App launcher** on its title bar:

| Option | Default | Meaning |
| --- | --- | --- |
| Display | Grid | *Grid* shows the launcher in place. *Compact* shows one button that opens it as a popover, for a narrow, taskbar-like slot. |
| Keyboard shortcut | `Alt+Shift+L` | For example `Ctrl+Alt+Space` or `Meta+K`. Leave it empty to turn it off. |
| Show recently used apps | On | Shows or hides the Recent section. |
| Recent apps to show | 6 | From 1 to 12. |
| Width and height | 2 × 3 | In board cells, from 1 × 1 to 4 × 8. |

Each placement keeps its own configuration. Placing two launchers, one grid
and one compact, works.

## Persistence and sync

Pinned and recent apps are shared by all launchers. They are stored in
`localStorage` under `ember:launcher:v1`, alongside the
[display preferences](https://github.com/ParityH4sher/ember/blob/main/apps/frontends/README.md). Ember's API stores no
per-user data yet, so they stay in one browser. Every open window of that
browser updates live when another one pins, unpins or opens an app (through
the `storage` event).

An app can be pinned but not registered at the moment, for example while a
service list is loading or a plugin is turned off. It is hidden, not
forgotten. The launcher records when each pinned or recent app was last seen
registered. Entries for apps not seen for 30 days are removed.

## Multi-display

Ember has no multi-display coordination yet (no window-per-screen,
no `BroadcastChannel` protocol). The widget context has an optional
`displays` coordinator for it:

```ts
interface DisplayCoordinator {
  list(): readonly { id: string; label: string }[];   // all displays, including this one
  launchOn(displayId: string, appId: string): void;  // ask that display's window to launch
}
```

When a future feature passes one to `WidgetBoard` (`displays` prop), the app
menu shows **Open on *display*** for every other display. Boards already store
layouts per display (`displayId` prop), so each screen can have its own
launcher configuration.

## Tests

- `tests/unit/registry.spec.ts`: register, unregister, duplicates,
  replacement, validation, and each launch type.
- `tests/unit/launcher.spec.ts`: search and ranking, pinned and recent
  ordering, stale entries, grid navigation, and shortcut parsing.
- `tests/ui/launcher.spec.ts`: the widget in the built app. It covers search
  and Enter, keyboard navigation, the shortcut, the context menu by mouse,
  keyboard and touch, App info, pinning synced across two windows, recents,
  runtime and invalid apps, and 250 apps. It also configures, removes and
  re-adds the widget and runs axe checks in both themes.
