import { elect, primaryClaim, type Claim, type ScreenSnapshot } from './election';

/** Channel shared by every Ember window on this origin. */
export const MULTI_DISPLAY_CHANNEL = 'ember:multi-display';

type Message =
  | { type: 'hello'; claim: Claim }
  | { type: 'claim'; claim: Claim }
  | { type: 'bye'; id: string };

interface ScreenDetails extends EventTarget, ScreenSnapshot {}
type WindowWithScreens = Window & { getScreenDetails?: () => Promise<ScreenDetails> };

const HEARTBEAT_MS = 2000;
const EXPIRE_MS = 5000;
// Wait briefly for other windows to answer before the first decision, so a
// newly opened window does not flash a circle that an older window owns.
const SETTLE_MS = 150;

/**
 * Reads ScreenDetails only when permission is already granted: the launcher
 * must never trigger a permission prompt on page load.
 */
async function screenDetails(win: WindowWithScreens): Promise<ScreenDetails | null> {
  if (typeof win.getScreenDetails !== 'function') return null;
  try {
    const status = await navigator.permissions.query({ name: 'window-management' as PermissionName });
    if (status.state !== 'granted') return null;
    return await win.getScreenDetails();
  } catch {
    return null;
  }
}

export interface PrimaryDisplayTracker {
  /**
   * Whether this window wants the circle at all (desktop surface shown, setting
   * on). Windows that do not want it never win, so another window can.
   */
  setEligible(eligible: boolean): void;
  /** Stops tracking and releases this window's claim. */
  stop(): void;
}

/**
 * Tracks whether this window should show the launcher circle and calls
 * `onChange` whenever that answer changes.
 */
export function trackPrimaryDisplay(onChange: (show: boolean) => void, eligible = true): PrimaryDisplayTracker {
  const win = window as WindowWithScreens;
  const self: Claim = { id: crypto.randomUUID(), since: Date.now(), primary: eligible, verified: false };
  const peers = new Map<string, { claim: Claim; seen: number }>();
  const channel = typeof BroadcastChannel === 'function' ? new BroadcastChannel(MULTI_DISPLAY_CHANNEL) : null;
  let details: ScreenDetails | null = null;
  let permission: PermissionStatus | null = null;
  let settled = false, stopped = false, shown: boolean | null = null;

  const post = (message: Message) => channel?.postMessage(message);
  const decide = () => {
    if (stopped || !settled) return;
    const now = Date.now();
    for (const [id, peer] of peers) if (now - peer.seen > EXPIRE_MS) peers.delete(id);
    const show = elect([self, ...[...peers.values()].map(p => p.claim)]) === self.id;
    if (show !== shown) { shown = show; onChange(show); }
  };
  const evaluate = () => {
    const display = primaryClaim(details);
    self.verified = display.verified;
    self.primary = display.primary && eligible;
    post({ type: 'claim', claim: { ...self } });
    decide();
  };
  const attach = async () => {
    details?.removeEventListener('screenschange', evaluate);
    details?.removeEventListener('currentscreenchange', evaluate);
    details = await screenDetails(win);
    if (stopped) return;
    details?.addEventListener('screenschange', evaluate);
    details?.addEventListener('currentscreenchange', evaluate);
    evaluate();
  };

  if (channel) channel.onmessage = ({ data }: MessageEvent<Message>) => {
    if (data.type === 'bye') peers.delete(data.id);
    else {
      peers.set(data.claim.id, { claim: data.claim, seen: Date.now() });
      if (data.type === 'hello') post({ type: 'claim', claim: { ...self } });
    }
    decide();
  };
  const heartbeat = setInterval(evaluate, HEARTBEAT_MS);
  const settle = setTimeout(() => { settled = true; decide(); }, channel ? SETTLE_MS : 0);
  const leave = () => post({ type: 'bye', id: self.id });
  window.addEventListener('pagehide', leave);

  post({ type: 'hello', claim: { ...self } });
  void attach();
  // Re-evaluate if the user grants or revokes window-management permission.
  navigator.permissions?.query({ name: 'window-management' as PermissionName }).then(status => {
    if (stopped) return;
    permission = status;
    permission.addEventListener('change', attach);
  }, () => {});

  const stop = () => {
    if (stopped) return;
    stopped = true;
    clearInterval(heartbeat);
    clearTimeout(settle);
    window.removeEventListener('pagehide', leave);
    permission?.removeEventListener('change', attach);
    details?.removeEventListener('screenschange', evaluate);
    details?.removeEventListener('currentscreenchange', evaluate);
    leave();
    channel?.close();
  };
  return {
    setEligible(next) { if (next !== eligible) { eligible = next; evaluate(); } },
    stop
  };
}
