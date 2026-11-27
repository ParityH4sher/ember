// Pure election rules for the multi-display channel. Kept free of DOM and
// Svelte imports so `node --test` can run it directly.

/** What one window says about itself on the multi-display channel. */
export interface Claim {
  /** Random per-window id. */
  id: string;
  /** When the window started; the oldest window wins ties. */
  since: number;
  /** The window believes it is on the primary display. */
  primary: boolean;
  /** `primary` came from the Window Management API rather than the fallback. */
  verified: boolean;
}

/** Orders claims: verified before fallback, then oldest, then lowest id. */
export function compareClaims(a: Claim, b: Claim): number {
  if (a.verified !== b.verified) return a.verified ? -1 : 1;
  if (a.since !== b.since) return a.since - b.since;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

/**
 * Returns the id of the single window that should show the launcher circle,
 * or null when no window claims the primary display. Every window runs this
 * on the same set of claims, so all of them agree on one winner.
 */
export function elect(claims: Iterable<Claim>): string | null {
  let best: Claim | null = null;
  for (const claim of claims) {
    if (claim.primary && (!best || compareClaims(claim, best) < 0)) best = claim;
  }
  return best?.id ?? null;
}

/** Minimal shape of the Window Management API's ScreenDetails we rely on. */
export interface ScreenSnapshot {
  screens: ArrayLike<unknown>;
  currentScreen: { isPrimary: boolean };
}

/**
 * Decides whether this window's display is primary. `details` is null when the
 * API is unsupported or permission was not granted; the window then counts as
 * primary, unverified. A single display is always primary.
 */
export function primaryClaim(details: ScreenSnapshot | null): Pick<Claim, 'primary' | 'verified'> {
  if (!details) return { primary: true, verified: false };
  if (details.screens.length <= 1) return { primary: true, verified: true };
  return { primary: details.currentScreen.isPrimary, verified: true };
}
