// Keyboard handling for the launcher: moving through sectioned grids and the
// configurable shortcut that opens or focuses it.

export interface GridPos { section: number; index: number }

/** The position after pressing `key`, 'search' to leave the grid upwards, or
    null when the key does not move. `sizes` are the item counts per section;
    `columns(section)` is how many items fit on one row of that section. */
export function moveFocus(key: string, pos: GridPos, sizes: readonly number[], columns: (section: number) => number): GridPos | 'search' | null {
  const { section: s, index: i } = pos;
  const n = sizes[s] ?? 0;
  const cols = Math.max(1, columns(s));
  const next = (from: number, step: 1 | -1) => {
    for (let k = from + step; k >= 0 && k < sizes.length; k += step) if (sizes[k] > 0) return k;
    return -1;
  };
  switch (key) {
    case 'ArrowRight': {
      if (i + 1 < n) return { section: s, index: i + 1 };
      const t = next(s, 1);
      return t < 0 ? null : { section: t, index: 0 };
    }
    case 'ArrowLeft': {
      if (i > 0) return { section: s, index: i - 1 };
      const t = next(s, -1);
      return t < 0 ? null : { section: t, index: sizes[t] - 1 };
    }
    case 'ArrowDown': {
      if (i + cols < n) return { section: s, index: i + cols };
      // On a row above the last, partial row: go to the last item.
      if (Math.floor(i / cols) < Math.floor((n - 1) / cols)) return { section: s, index: n - 1 };
      const t = next(s, 1);
      return t < 0 ? null : { section: t, index: Math.min(i % cols, sizes[t] - 1) };
    }
    case 'ArrowUp': {
      if (i - cols >= 0) return { section: s, index: i - cols };
      const t = next(s, -1);
      if (t < 0) return 'search';
      const m = sizes[t], c = Math.max(1, columns(t));
      return { section: t, index: Math.min(Math.floor((m - 1) / c) * c + (i % cols), m - 1) };
    }
    case 'Home': return { section: s, index: 0 };
    case 'End': return { section: s, index: n - 1 };
    default: return null;
  }
}

/** `mod` is Ctrl, or ⌘ (Meta) on Apple keyboards: either key matches. */
export interface Shortcut { code: string; mod: boolean; ctrl: boolean; alt: boolean; shift: boolean; meta: boolean }

/** Ctrl+K, or ⌘K on Apple keyboards: the usual command-palette shortcut. */
export const DEFAULT_SHORTCUT = 'Mod+K';

const named: Record<string, string> = {
  space: 'Space', enter: 'Enter', '/': 'Slash', '.': 'Period', ',': 'Comma', ';': 'Semicolon',
  "'": 'Quote', '[': 'BracketLeft', ']': 'BracketRight', '-': 'Minus', '=': 'Equal', '`': 'Backquote'
};

/** Parses e.g. "Mod+K", "Alt+Shift+L" or "Ctrl+Alt+Space". "Mod" means
    Ctrl, or ⌘ on Apple keyboards. Matching uses physical key codes, so it
    works with any keyboard layout and with Option on macOS. A shortcut needs
    Mod, Ctrl, Alt or Meta, except F1–F12, so it never takes over plain
    typing. Returns null for anything else. */
export function parseShortcut(text: string): Shortcut | null {
  const parts = text.split('+').map(p => p.trim()).filter(Boolean);
  if (!parts.length || parts.length > 5) return null;
  const s: Shortcut = { code: '', mod: false, ctrl: false, alt: false, shift: false, meta: false };
  for (const part of parts.slice(0, -1)) {
    const m = part.toLowerCase();
    if (m === 'mod') s.mod = true;
    else if (m === 'ctrl' || m === 'control') s.ctrl = true;
    else if (m === 'alt' || m === 'option') s.alt = true;
    else if (m === 'shift') s.shift = true;
    else if (m === 'meta' || m === 'cmd' || m === 'command' || m === 'super') s.meta = true;
    else return null;
  }
  const key = parts[parts.length - 1];
  if (/^[a-z]$/i.test(key)) s.code = 'Key' + key.toUpperCase();
  else if (/^[0-9]$/.test(key)) s.code = 'Digit' + key;
  else if (/^f([1-9]|1[0-2])$/i.test(key)) s.code = key.toUpperCase();
  else if (named[key.toLowerCase()]) s.code = named[key.toLowerCase()];
  else if (/^(Key[A-Z]|Digit[0-9])$/.test(key)) s.code = key;
  else return null;
  if (s.mod && (s.ctrl || s.meta)) return null;
  if (!s.mod && !s.ctrl && !s.alt && !s.meta && !/^F\d+$/.test(s.code)) return null;
  return s;
}

const keyName = (s: Shortcut) => s.code.replace(/^Key|^Digit/, '');

/** The stored form, e.g. "Mod+K" or "Ctrl+Alt+Space". */
export function formatShortcut(s: Shortcut): string {
  return [s.mod && 'Mod', s.ctrl && 'Ctrl', s.alt && 'Alt', s.shift && 'Shift', s.meta && 'Meta', keyName(s)].filter(Boolean).join('+');
}

/** Apple keyboards: macOS, iOS and iPadOS. */
export const isApple = () => typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform || navigator.userAgent);

/** What to show people, e.g. "Ctrl+K", or "⌘K" on Apple keyboards. */
export function displayShortcut(s: Shortcut, apple = isApple()): string {
  if (apple) return [s.ctrl && '⌃', s.alt && '⌥', s.shift && '⇧', (s.mod || s.meta) && '⌘', keyName(s)].filter(Boolean).join('');
  return [(s.mod || s.ctrl) && 'Ctrl', s.alt && 'Alt', s.shift && 'Shift', s.meta && 'Meta', keyName(s)].filter(Boolean).join('+');
}

/** The value for aria-keyshortcuts; "Mod" lists both the Ctrl and the ⌘ form. */
export function ariaShortcut(s: Shortcut): string {
  const one = (ctrl: boolean, meta: boolean) =>
    [ctrl && 'Control', s.alt && 'Alt', s.shift && 'Shift', meta && 'Meta', keyName(s)].filter(Boolean).join('+');
  return s.mod ? `${one(true, false)} ${one(false, true)}` : one(s.ctrl, s.meta);
}

export function matchesShortcut(e: Pick<KeyboardEvent, 'code' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'metaKey'>, s: Shortcut): boolean {
  if (e.code !== s.code || e.altKey !== s.alt || e.shiftKey !== s.shift) return false;
  // Mod: exactly one of Ctrl and ⌘, whichever the keyboard uses.
  if (s.mod) return e.ctrlKey !== e.metaKey;
  return e.ctrlKey === s.ctrl && e.metaKey === s.meta;
}
