---
sidebar_label: Always on top
sidebar_position: 9
---

# Keeping Ember windows on top

A web app cannot pin its own window above other windows. No web API does this
for an installed app window. You have two options: a small floating window that
Ember opens itself (in some browsers), or your operating system's window
manager.

## In Ember: Pin to top (Document Picture-in-Picture)

When you view a note, **Pin to top** in the toolbar opens that note in a small
window that always stays on top. This uses
[Document Picture-in-Picture](https://developer.mozilla.org/en-US/docs/Web/API/Document_Picture-in-Picture_API).

- It floats a **separate small window**, not the main Notes window. The pinned
  window is read-only and shows the note as it was when pinned, updated
  whenever you save it. Links in it open in the main window.
- It needs a click. The browser rejects the request without a user gesture.
- Only one pinned window exists at a time. Pinning another note replaces it.
  Closing Notes closes it too.
- Support: Chrome and Edge 116 and later. Chrome's compatibility table also
  lists recent Firefox, which has not been tested with Ember. Safari is not
  supported. In other browsers the button does not appear.
- The pinned document starts unstyled. Ember copies the page's stylesheets into
  it once, when it opens.

## Windows: PowerToys Always On Top

1. Install [Microsoft PowerToys](https://learn.microsoft.com/windows/powertoys/install)
   from the Microsoft Store or with `winget install Microsoft.PowerToys`.
2. Open PowerToys, select **Always On Top** and make sure it is enabled.
3. Focus the Ember window and press <kbd>Win</kbd>+<kbd>Ctrl</kbd>+<kbd>T</kbd>.
   A border marks the pinned window. Press the shortcut again to unpin it.

The shortcut, border colour and excluded apps can be changed on the same
settings page.

## Linux

Most window managers have this built in:

- **GNOME:** right-click the title bar (or press <kbd>Alt</kbd>+<kbd>Space</kbd>)
  and choose **Always on Top**.
- **KDE Plasma:** right-click the title bar → **More Actions → Keep Above
  Others**, or press <kbd>Alt</kbd>+<kbd>F3</kbd>. You can assign a shortcut in
  *System Settings → Shortcuts → KWin* ("Keep Window Above Others").
- **Xfce, Cinnamon, MATE and most others:** the title bar's right-click menu
  has **Always on Top**.
- **Tiling compositors** (Sway, Hyprland, i3): make the window floating, then
  use the compositor's pin or sticky rule. Check your configuration.

With Window Controls Overlay, Ember's top bar is the title bar, but the right-click
menu on that title bar area still comes from the window manager. If it does
not appear, use <kbd>Alt</kbd>+<kbd>Space</kbd> on GNOME or
<kbd>Alt</kbd>+<kbd>F3</kbd> on KDE.

## macOS

macOS has no built-in way to keep a window on top. You need a third-party
window utility. Several free and paid ones exist. Ember does not recommend a
particular one. On macOS, **Pin to top** is the only option that needs no
extra software.

## Managed and kiosk setups

Administrators can launch Ember and set up its permissions in advance:

- **Launch as an app window:** `chrome --app=https://<host>/apps/notes/` (or
  `msedge --app=…`) opens a window without browser chrome. For a locked-down
  single screen, use `--kiosk`. Kiosk mode is full-screen, so always-on-top no
  longer matters there.
- **Pre-grant window management:** Chrome's enterprise policy
  `WindowManagementAllowedForUrls` grants the `window-management` permission
  to the listed sites. The first-run **Use all displays** prompt then never
  appears.
- **Allow pop-ups:** `PopupsAllowedForUrls` lets Ember open the workspace
  windows at launch without a click.
  See [multi-display](multi-display.md#browser-limits).
- Microsoft Edge has policies with the same names.

Check the names against the current
[Chrome Enterprise policy list](https://chromeenterprise.google/policies/)
before you deploy. The list did not render during review, so the two names
above are unconfirmed.
