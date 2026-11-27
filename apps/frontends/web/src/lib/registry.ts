// A small keyed registry shared by the widget and app registries. It follows
// Svelte's store contract (`subscribe` calls back at once and on every change),
// so a component can read `$registry` and see entries registered at runtime.
// Invalid or duplicate entries are logged and ignored; they never throw, so
// one bad plugin cannot stop the page from rendering.

export interface Registered { id: string }

export interface Registry<T extends Registered> {
  /** Adds an entry; returns a function that removes exactly that entry again.
      Invalid entries and duplicate ids are ignored with a warning, unless
      `replace` is set for an intentional replacement. */
  register(entry: T, options?: { replace?: boolean }): () => void;
  /** Removes an entry by id; false when there was none. */
  unregister(id: string): boolean;
  get(id: string): T | undefined;
  /** Entries in registration order. The array is replaced, never mutated. */
  list(): readonly T[];
  subscribe(run: (entries: readonly T[]) => void): () => void;
}

export type Warn = (message: string) => void;

const defaultWarn: Warn = message => console.warn(message);

/** Problems with an entry, as readable phrases; empty when it is valid. */
export type Validator = (entry: unknown) => string[];

export function describeId(entry: unknown): string {
  try {
    const id = entry && typeof entry === 'object' ? (entry as Registered).id : undefined;
    return typeof id === 'string' && id ? `"${id.slice(0, 80)}"` : '(no id)';
  } catch {
    return '(unreadable id)';
  }
}

export function createRegistry<T extends Registered>(kind: string, validate: Validator, warn: Warn = defaultWarn): Registry<T> {
  const entries = new Map<string, T>();
  const listeners = new Set<(entries: readonly T[]) => void>();
  let snapshot: readonly T[] = Object.freeze([]);
  const noop = () => {};

  function changed() {
    snapshot = Object.freeze([...entries.values()]);
    for (const run of listeners) run(snapshot);
  }

  function remove(id: string, only?: T): boolean {
    const current = entries.get(id);
    if (!current || (only && current !== only)) return false;
    entries.delete(id);
    changed();
    return true;
  }

  return {
    register(entry, options) {
      let problems: string[];
      try { problems = validate(entry); } catch (error) { problems = [`validation failed: ${error}`]; }
      if (problems.length) {
        warn(`[ember] Ignored ${kind} ${describeId(entry)}: ${problems.join('; ')}.`);
        return noop;
      }
      const existing = entries.get(entry.id);
      // Registering the same object twice (e.g. a module set up twice) is harmless.
      if (existing === entry) return () => { remove(entry.id, entry); };
      if (existing && !options?.replace) {
        warn(`[ember] Ignored ${kind} "${entry.id}": a ${kind} with this id is already registered. ` +
          `Use a unique id, or pass { replace: true } to replace it.`);
        return noop;
      }
      entries.set(entry.id, entry);
      changed();
      return () => { remove(entry.id, entry); };
    },
    unregister: id => remove(id),
    get: id => entries.get(id),
    list: () => snapshot,
    subscribe(run) {
      listeners.add(run);
      run(snapshot);
      return () => { listeners.delete(run); };
    }
  };
}

// Validation helpers shared by the registries.
export const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
export const nonEmpty = (v: unknown): v is string => typeof v === 'string' && v.trim() !== '';
/** Ids are stable keys in storage and URLs: letters, digits, '.', '_', ':', '-'. */
export const validId = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(v);
