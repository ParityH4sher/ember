// A value kept in localStorage under a versioned key, exposed as a Svelte
// store. Other windows of the same browser see changes live through the
// `storage` event. Like lib/prefs.svelte.ts, storage may be unavailable
// (private windows, blocked site data, prerendering); the value then lives
// only in memory and writes report false.

export interface Persisted<T> {
  subscribe(run: (value: T) => void): () => void;
  get(): T;
  /** Stores the value; false when storage refused it (it still applies in memory). */
  set(value: T): boolean;
  update(fn: (value: T) => T): boolean;
}

function storage(): Storage | undefined {
  try { return typeof localStorage === 'undefined' ? undefined : localStorage; } catch { return undefined; }
}

/** `parse` turns any stored JSON (or null) into a valid value; it must not throw. */
export function persisted<T>(key: string, parse: (raw: unknown) => T): Persisted<T> {
  const listeners = new Set<(value: T) => void>();
  let value: T | undefined;
  let loaded = false;

  function read(): T {
    let raw: unknown = null;
    try { raw = JSON.parse(storage()?.getItem(key) ?? 'null'); } catch { /* corrupt: use defaults */ }
    return parse(raw);
  }
  function current(): T {
    if (!loaded) { value = read(); loaded = true; }
    return value as T;
  }
  function emit() { for (const run of listeners) run(value as T); }
  function onStorage(event: StorageEvent) {
    // key === null means another window cleared all site storage.
    if (event.key !== key && event.key !== null) return;
    value = read();
    loaded = true;
    emit();
  }

  const store: Persisted<T> = {
    subscribe(run) {
      if (!listeners.size && typeof window !== 'undefined') {
        // Re-read: another window may have written while nobody listened.
        loaded = false;
        window.addEventListener('storage', onStorage);
      }
      listeners.add(run);
      run(current());
      return () => {
        listeners.delete(run);
        if (!listeners.size && typeof window !== 'undefined') window.removeEventListener('storage', onStorage);
      };
    },
    get: current,
    set(next) {
      value = next;
      loaded = true;
      emit();
      try {
        const s = storage();
        if (!s) return false;
        s.setItem(key, JSON.stringify(next));
        return true;
      } catch {
        return false;
      }
    },
    update: fn => store.set(fn(current()))
  };
  return store;
}
