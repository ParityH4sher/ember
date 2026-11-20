# OpenCode terminal shortcuts

`config/opencode/cli.json` provides a starter set of OpenCode TUI keybindings:

| Shortcut | Action |
| --- | --- |
| `Ctrl+N` | Create a new session |
| `Ctrl+W` | Close the current session tab |
| `Ctrl+Tab` | Switch to the next open session tab |
| `Ctrl+Shift+Tab` | Switch to the previous open session tab |

OpenCode CLI settings are user-level, not project-local. To use these shortcuts,
merge the `keybinds` object from that file into
`~/.config/opencode/cli.json` (or
`$XDG_CONFIG_HOME/opencode/cli.json` if `XDG_CONFIG_HOME` is set). Preserve
any other settings already in your CLI config. OpenCode reloads valid changes
while the TUI is running.

`Ctrl+W` replaces the default input-editor binding for deleting the previous
word, and `Ctrl+N` also serves as “next” in some dialogs/autocomplete menus.
