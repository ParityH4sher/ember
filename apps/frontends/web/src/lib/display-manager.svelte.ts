// Multi-display coordination for the Notes window. The primary Notes window
// (holder of a Web Lock) owns setup: it opens one blank workspace window per
// additional screen and offers to open more when screens are added.
import * as d from './displays';

export type Offer =
  | { kind: 'setup' }
  | { kind: 'remaining' | 'new'; count: number }
  | { kind: 'notice'; text: string };

const wait = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

export class DisplayManager {
  /** Window Management and BroadcastChannel are both available. */
  supported = $state(false);
  /** The banner currently offered to the user, if any. */
  offer = $state<Offer | null>(null);
  /** "Open on all displays at startup" setting. */
  startup = $state(true);

  #details: d.ScreenDetails | null = null;
  #live = new Set<string>();
  #channel: BroadcastChannel | null = null;

  /** Join the window group and, as primary, run startup setup. Returns cleanup. */
  start(): () => void {
    this.startup = d.openAtStartup();
    // A workspace window that became the Notes window keeps its old name;
    // drop it so reopening that screen's workspace creates a new window.
    if (window.name.startsWith('ember-display-')) window.name = '';
    if (!d.api() || typeof BroadcastChannel === 'undefined') return () => {};
    this.supported = true;
    const channel = this.#channel = d.channel();
    channel.onmessage = ({ data }: MessageEvent<d.Message>) => {
      if (data.type === 'query') channel.postMessage({ type: 'notes' } satisfies d.Message);
      else if (data.type === 'workspace') { this.#live.add(data.display); this.#recount(); }
      else if (data.type === 'bye') { this.#live.delete(data.display); this.#gap(); }
    };
    channel.postMessage({ type: 'query' } satisfies d.Message);

    // The lock is released when this window closes, which promotes the next
    // waiting Notes window. Setup runs once, when the lock is first held.
    const stop = new AbortController();
    let release = () => {};
    const held = new Promise<void>(resolve => { release = resolve; });
    const setup = async () => { await this.#launch(stop.signal); await held; };
    if (navigator.locks) navigator.locks.request('ember-displays-primary', { signal: stop.signal }, setup).catch(() => {});
    else void this.#launch(stop.signal);
    return () => { stop.abort(); release(); channel.close(); this.#channel = null; };
  }

  async #launch(signal: AbortSignal) {
    // Let already open workspaces answer the query. After a relaunch they load
    // alongside this window, so give them time; late ones still dedupe themselves.
    await wait(1000);
    if (signal.aborted) return;
    const state = await d.permission();
    if (state === 'granted') {
      if (!await this.#loadDetails()) return;
      // Without a user gesture the popup blocker usually stops these windows;
      // #openMissing then offers a one-click "Open on remaining displays".
      if (this.startup && this.#details!.screens.length > 1) this.#openMissing();
    } else if (state !== 'denied' && this.startup && d.extended() && !d.promptDismissed()) {
      this.offer = { kind: 'setup' };
    }
  }

  async #loadDetails(): Promise<boolean> {
    if (this.#details) return true;
    try { this.#details = await d.api()!.getScreenDetails(); } catch { return false; }
    this.#details.addEventListener('screenschange', () => {
      const count = this.#missing().length;
      if (count) this.offer = { kind: 'new', count };
      else this.#recount();
    });
    this.#details.addEventListener('currentscreenchange', () => this.#recount());
    return true;
  }

  /** Screens other than this window's that have no live workspace. */
  #missing(): d.ScreenDetailed[] {
    const details = this.#details;
    if (!details) return [];
    const current = d.screenKey(details.currentScreen);
    return details.screens.filter(s => d.screenKey(s) !== current && !this.#live.has(d.screenKey(s)));
  }

  #openMissing() {
    d.fit(window, this.#details!.currentScreen); // best effort; see docs/multi-display.md
    let blocked = 0;
    for (const s of this.#missing()) {
      if (d.openWorkspace(s)) this.#live.add(d.screenKey(s)); else blocked++;
    }
    this.offer = blocked ? { kind: 'remaining', count: blocked } : null;
  }

  /** A workspace closed or left its screen: offer to fill the gap (one click). */
  #gap() {
    if (this.offer?.kind === 'setup' || !this.startup || !this.#details) return this.#recount();
    const count = this.#missing().length;
    if (count) this.offer = { kind: 'remaining', count };
    else this.#recount();
  }

  /** Keep an open "remaining"/"new" banner accurate; close it when nothing is missing. */
  #recount() {
    if (this.offer?.kind !== 'remaining' && this.offer?.kind !== 'new') return;
    const count = this.#missing().length;
    this.offer = count ? { ...this.offer, count } : null;
  }

  /** Click handler for every "open" action. Call it directly from the click so
   * the permission request and window.open receive the user activation. */
  async openAll() {
    this.offer = null;
    if (!await this.#loadDetails()) {
      this.offer = { kind: 'notice', text: 'Ember cannot use your other displays. Allow "Window management" in site settings.' };
      return;
    }
    if (this.#details!.screens.length < 2) {
      this.offer = { kind: 'notice', text: 'Only one display is connected.' };
      return;
    }
    this.#channel?.postMessage({ type: 'arrange' } satisfies d.Message);
    this.#openMissing();
  }

  dismiss() {
    if (this.offer?.kind === 'setup') d.dismissPrompt();
    this.offer = null;
  }

  setStartup(on: boolean) {
    this.startup = on;
    d.setOpenAtStartup(on);
  }
}
