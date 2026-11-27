---
sidebar_label: App launcher
sidebar_position: 6
---

# App launcher

There is one app launcher, with three ways to open it:

- the **launcher circle** in the middle of the desktop surface (`/desktop/`)
  on the primary display
- the **launcher shortcut**, **Ctrl+K** (**⌘K** on a Mac) by default, from
  any page: it focuses the search of a launcher widget shown on the page, and
  otherwise opens the launcher in the centre of the screen
- the **App launcher** [widget](widgets.md) on the dashboard, shown in place
  or as a compact button

All three show the same launcher UI (`LauncherPanel`): every registered app
in a grid you can search, with category tabs, pinned apps and recently used
apps. They read the same **app registry**, which is separate from the widget
registry, and share pinned and recent apps. Registering an app needs no
change to the launcher, and an app registered while the page is open appears
at once.

Code: `apps/frontends/web/src/lib/apps/` (registry, launch targets, built-in
apps), `src/lib/launcher/` (the launcher UI and widget) and
`src/lib/desktop/` (the circle, its primary-display election), wired together
in `src/lib/shell/AppShell.svelte`.

The installed app starts on the desktop surface: the web app manifest's
`start_url` is `./desktop/`, which resolves to `/desktop/` next to the
manifest. The dashboard (`/`) stays one launch away as the built-in
**Dashboard** app, and the brand link in the top bar still goes there.

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

### At startup (built-in pages)

Ember's own pages are listed once, in `src/lib/shell/nav.ts`. The sidebar
renders that list, and `src/lib/apps/static.ts` turns each entry into an app
(section → category, `href` → an `internal` launch). `src/lib/setup.ts`
registers them before any page renders. To add a page, add one entry:

```ts
{ id: 'ember.notes', href: '/apps/notes/', label: 'Project notes', icon: 'notes',
  description: 'Linked Markdown notes and project chat.', keywords: ['markdown', 'wiki'] },
```

The `id` is stored in pinned and recent lists, so keep it stable. Entries with
`admin: true` are registered at runtime, for admins only.

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
- the `admin: true` pages from `nav.ts` (**Admin**), for admins only (the API still enforces the role)

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
  category. A widget saves its selected tab; the circle's launcher keeps it
  until the page reloads.
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
| **Ctrl+K** / **⌘K** (configurable) | Any page | With a launcher widget shown in place (grid mode) on the page, focus its search. Otherwise open the launcher (in the centre, or from the circle on the desktop) with the search focused; press again to close. |
| Type | Search box | Filter. |
| **Enter** | Search box | Open the top result. |
| **↓** | Search box | Move to the first app. |
| **← → ↑ ↓** | App grid | Move between apps; up and down move by rows and between sections; **↑** on the first row returns to search. |
| **Home** / **End** | App grid | First or last app of the section. |
| **Enter** / **Space** | App | Open it. |
| **Shift+F10** / **Menu** | App | Open the app menu (**↑ ↓**, **Home**, **End**, **Escape**). |
| **Escape** | Search or grid | Clear the search; with an empty search, close the launcher (circle or compact popover). An open app menu or App info closes first. |
| **← →**, **Home**, **End** | Category tabs | Change tab. |

Only one app is a tab stop at a time (roving tabindex), so **Tab** leaves the
grid in one step.

### The launcher shortcut

There is one shortcut for the whole app, set in **Settings › Desktop ›
Launcher shortcut** and stored with the other display preferences. The
default is `Mod+K`: **Ctrl+K**, or **⌘K** on Apple keyboards (`Mod` accepts
either key). It was chosen over Alt+Shift+L because:

- Ctrl/⌘+K is the usual command-palette shortcut (GitHub, Slack, Linear and
  many editors), so people try it first.
- In the installed app's own window no browser shortcut competes. In a
  browser tab the page takes it first; the browser's own Ctrl+K (search the
  web) stays available from the address bar.
- Alt+Shift alone switches the input language on Windows for many people, and
  Alt+Shift+L types "Ò" on macOS.

Write a shortcut as modifiers and a key, e.g. `Mod+K`, `Ctrl+Alt+Space` or
`Alt+Shift+L`. It matches physical keys, so it works with any keyboard layout.
A shortcut must include Mod, Ctrl, Alt or Meta (or be F1–F12), so it cannot
take over plain typing. Leave the field empty to turn the shortcut off; the
circle and the widget still work.

What the shortcut does depends on the page:

- **A launcher widget is shown in place** (grid mode) and visible: the
  shortcut scrolls it into view if needed and focuses its search. Nothing
  opens on top of the launcher that is already there. With several, the
  first one placed wins. A widget hidden with CSS, or on a board that is not
  shown, does not count.
- **Otherwise** (no widget, or only compact ones): the shortcut opens the
  centred launcher, from the circle on the desktop surface, and pressing it
  again closes it.

Grid-mode widgets register themselves in `src/lib/launcher/inline.ts`;
`LauncherCircle` asks them first (its `intercept` prop) before it opens.

## Configuration

Open **Configure App launcher** on its title bar:

| Option | Default | Meaning |
| --- | --- | --- |
| Display | Grid | *Grid* shows the launcher in place. *Compact* shows one button that opens it as a popover, for a narrow, taskbar-like slot. |
| Show recently used apps | On | Shows or hides the Recent section. |
| Recent apps to show | 6 | From 1 to 12. |
| Width and height | 2 × 3 | In board cells, from 1 × 1 to 4 × 8. |

Each placement keeps its own configuration. Placing two launchers, one grid
and one compact, works. The keyboard shortcut is not a widget setting (see
above); a grid-mode widget lists it on its search box
(`aria-keyshortcuts`).

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

Ember detects the primary display and makes sure exactly one window shows
the launcher circle: `src/lib/desktop/primary-display.ts` (Window Management
API, when permission was already granted) and `election.ts` (windows announce
themselves on the `ember:multi-display` BroadcastChannel and elect one). It
does not yet send launches between windows. The widget context has an
optional `displays` coordinator for that, which nothing provides yet:

```ts
interface DisplayCoordinator {
  list(): readonly { id: string; label: string }[];   // all displays, including this one
  launchOn(displayId: string, appId: string): void;  // ask that display's window to launch
}
```

When the multi-display work passes one to `WidgetBoard` (`displays` prop), and
to the circle's launcher in `AppShell`, the app menu shows **Open on
*display*** for every other display. It should build on the channel and
election above rather than add a second mechanism. Boards already store
layouts per display (`displayId` prop), so each screen can have its own
launcher configuration.

## Tests

- `tests/unit/registry.spec.ts`: register, unregister, duplicates,
  replacement, validation, and each launch type.
- `tests/unit/launcher.spec.ts`: search and ranking, pinned and recent
  ordering, stale entries, grid navigation, shortcut parsing (including
  `Mod`), and which in-place launcher takes the shortcut.
- `tests/ui/launcher-widget.spec.ts`: the widget in the built app. It covers
  search and Enter, keyboard navigation, the context menu by mouse, keyboard
  and touch, App info, pinning synced across two windows, recents, runtime
  and invalid apps, and 250 apps. It also covers the shortcut focusing the
  widget's search on the dashboard (and opening the centred launcher when the
  widget is compact), the shortcut opening the same launcher with the same
  pins on other pages, setting the shortcut in
  Settings, configuring, removing and re-adding the widget, and axe checks in
  both themes.
- `tests/ui/launcher-circle.spec.ts`: the circle on `/desktop/`: centring,
  the primary-display election across windows, the shortcut, Escape order
  (search, app menu, App info, then the launcher), menu placement inside the
  centred panel, and axe checks with the launcher open.
- `src/lib/desktop/election.test.ts` (`npm test`): the election rules.
- `tests/ui/pages.spec.ts`: the manifest's `start_url` resolves to the
  desktop surface, and the Dashboard opens from the launcher there.
