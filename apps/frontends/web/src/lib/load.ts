// A value a page loads from the API: undefined while loading, the value, or
// a Failed carrying the error message when the request failed.
export class Failed {
  constructor(readonly error: string) {}
}
export type Loaded<T> = T | Failed | undefined;

export const message = (error: unknown) => error instanceof Error ? error.message : String(error);

export const failure = (value: unknown): value is Failed => value instanceof Failed;

/** Runs a request and reports its value or a Failed. */
export function settle<T>(promise: Promise<T>, set: (value: T | Failed) => void): void {
  promise.then(set, error => set(new Failed(message(error))));
}
