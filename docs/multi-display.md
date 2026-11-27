---
sidebar_label: Multi-display
sidebar_position: 8
---

# Opening Ember Notes on every display

The installed Notes app (`/apps/notes/`) can fill every connected display. The
window you launch shows Notes. Each additional display gets a **workspace**:
a plain, intentionally blank window at `/apps/notes/display/?display=<id>`
that claims the screen. Per-display content is not implemented yet. The
workspace route exists so you can add it later.

Status: implemented and type-checked only. It has not been tested in a browser
with real multiple monitors. See [verification](#verification).

## Browser support

The feature uses the [Window Management API](https://developer.mozilla.org/en-US/docs/Web/API/Window_Management_API)
(`window.getScreenDetails()`, permission `window-management`). Only Chromium
browsers (Chrome, Edge) provide it. In Firefox, Safari, on a single display,
or with the permission denied, Notes behaves as before: one window, no prompt
and no errors.

## What happens at launch

1. Notes checks for the API and reads the permission with
   `navigator.permissions.query({name: "window-management"})`.
2. **Not yet granted:** when `screen.isExtended` reports more than one
   display, a small banner offers **Use all displays**. The click requests the
   permission and opens the workspaces. **Not now** hides the banner for good.
   It is still available under **Displays → Reopen on all displays**.
3. **Already granted:** Notes tries to open a workspace on every other display
   right away, sized to that screen's `availLeft/availTop/availWidth/availHeight`.
4. Every `window.open()` that returns `null` counts as blocked. The banner then
   offers **Open on remaining displays (n)**. Nothing retries on its own.

### Browser limits

These come from current documentation (November 2026) and determine how many
clicks you need:

- **Pop-ups need a user gesture.** Granting `window-management` does not lift
  the pop-up blocker. At launch there is no gesture, so the automatic step in 3
  is normally blocked, and one click on the banner finishes the job. To open
  workspaces with no click at all, allow pop-ups for the Ember site
  (site settings → *Pop-ups and redirects*) or set the
  [policies below](#managed-setups).
- **One window per click on some versions.** MDN states that "a separate user
  gesture event is required for each `Window.open()` call". With three or more
  displays, you may need one click per workspace. The banner's count drops as
  each window opens. Some Chrome versions allow several pop-ups per click once
  the permission is granted. The code opens as many as the browser allows.
- **The permission prompt can use up the click.** If answering the prompt takes
  longer than the browser's activation window (a few seconds), the windows are
  blocked and the banner offers the next click.
- **The launched window is not moved.** CSSOM View lets `moveTo()`/`resizeTo()`
  act only on windows a script opened. Notes calls them on its own window, but
  the browser normally ignores the calls. Maximise that window yourself. The
  workspaces were opened by script, so they fit themselves to their screen.
- **App window or pop-up.** The documentation does not say whether
  `window.open()` from an installed app opens an app-styled window or a plain
  pop-up. Either works.

## How windows cooperate

- **Names:** each workspace's window name comes from its screen's label and
  position (`ember-display-<label>@<left>,<top>`). The browser reuses a window
  with that name when it can still find it.
- **Duplicates:** all Ember windows share the `BroadcastChannel`
  `ember-displays`. Workspaces announce their screen when they open, answer a
  query, and say goodbye on `pagehide`. Before opening anything, Notes skips
  any screen that already has a live workspace. After a relaunch, the browser
  may not know the old window names, so the announcements are what prevent
  duplicates. The manifest's `"launch_handler": {"client_mode": "focus-existing"}`
  focuses the most recently used Ember window instead of opening a new one.
- **Primary:** the Notes window holding the Web Lock `ember-displays-primary`
  runs the startup setup and watches for display changes. Other Notes windows
  wait for the lock.
- **When the primary closes:** workspaces stay open. Another open Notes window
  takes the lock and becomes primary. It offers to open any workspaces that
  are missing. If only workspaces remain and you launch the app, the focused
  workspace becomes the Notes window. If a Notes window is still open but a
  workspace was focused, that workspace stays blank. Switch to the Notes window.
- **Display plugged in:** `screenschange` shows **New display detected → Open
  window there**. Opening needs a click.
- **Display removed:** nothing closes. The operating system moves orphaned
  windows to a remaining display, where they keep working.
- **Window dragged to another display:** on `currentscreenchange`, the
  workspace takes on that display's identity.

## Settings

Open **Displays** in the Notes header:

- **Open on all displays at startup** (default on). This is stored in this
  browser's `localStorage` (`ember.displays.openAtStartup`). If it is off,
  Notes neither opens workspaces nor shows the first-run banner.
- **Reopen on all displays** moves open workspaces back to their own screens
  and opens missing ones.

The **Displays** button only appears in browsers that support the API.

## Title bar

The manifest lists `window-controls-overlay` first in `display_override`. In
an installed desktop window, the Notes header becomes the title bar. Drag the
header to move the window. The header's padding uses the `titlebar-area-*`
environment variables to stay clear of the window buttons. A workspace can be
dragged from anywhere in it.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| Banner keeps saying *n displays still without a window* | Pop-ups are blocked or each window needs its own click. Click again, or allow pop-ups for the site. |
| No banner and no **Displays** button | Browser without Window Management (Firefox, Safari), or the site is not HTTPS/localhost. |
| No banner but **Displays** is shown | Permission denied, banner dismissed, setting off, or only one display. Re-allow *Window management* in site settings and use **Reopen on all displays**. |
| Workspace on the wrong screen | Use **Displays → Reopen on all displays**. If labels or positions changed, the old window counts as a different screen. Close it. |
| Duplicate workspaces | A workspace that never sent its announcement (for example, it was frozen or discarded) is not counted. Close the extra windows. Relaunching after that should not duplicate them. |
| Launching shows a blank window | `focus-existing` focused a workspace while a Notes window was open. Switch to the Notes window. |

## Verification

Done: source review against the Window Management, Launch Handler and Document
Picture-in-Picture documentation. Not done: running the feature with two or
more physical displays, testing on Edge, or checking whether a single click
opens several windows on your Chrome version. Record results in the
[implementation log](progress/implementation-log.md).
