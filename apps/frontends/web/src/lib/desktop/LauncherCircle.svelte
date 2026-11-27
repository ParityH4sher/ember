<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte';
  import { trackPrimaryDisplay, type PrimaryDisplayTracker } from './primary-display';
  import { isLauncherShortcut } from '../shell/nav';

  interface Props {
    /** Launcher contents, rendered in the panel. */
    children: Snippet;
    /**
     * This window shows the desktop surface and the "show launcher circle"
     * setting is on. When false there is no circle, but the shortcut still
     * opens the launcher.
     */
    enabled?: boolean;
    /** Circle diameter as a CSS length; defaults to `--launcher-circle-size`. */
    size?: string;
    /** Accessible name of the circle button. */
    label?: string;
    /** Listen for the global launcher shortcut (Ctrl+K / ⌘K). */
    shortcut?: boolean;
    open?: boolean;
  }
  let {
    children, enabled = true, size, label = 'Open app launcher', shortcut = true, open = $bindable(false)
  }: Props = $props();

  const panelId = 'app-launcher';
  let primary = $state(false);
  let button = $state<HTMLButtonElement>();
  let panel = $state<HTMLElement>();
  let tracker: PrimaryDisplayTracker | undefined;
  // Where focus returns on close when there is no circle to return to.
  let opener: HTMLElement | null = null;
  let showCircle = $derived(enabled && primary);

  onMount(() => {
    tracker = trackPrimaryDisplay(show => { primary = show; }, enabled);
    return () => tracker?.stop();
  });
  $effect(() => { tracker?.setEligible(enabled); });

  async function show() {
    const active = document.activeElement;
    opener = active instanceof HTMLElement && active !== document.body ? active : null;
    open = true;
    await tick();
    const first = panel?.querySelector<HTMLElement>('a[href], button:not(:disabled), input, select, textarea, [tabindex]:not([tabindex="-1"])');
    (first ?? panel)?.focus();
  }
  /** Closes the launcher; `refocus` returns focus to the circle (or the opener). */
  function hide(refocus: boolean) {
    if (!open) return;
    open = false;
    if (refocus) (button ?? opener)?.focus();
    opener = null;
  }
  /** Focuses the circle (when shown) and opens the launcher. */
  export function activate() {
    button?.focus();
    void show();
  }

  function onKeydown(e: KeyboardEvent) {
    if (open && e.key === 'Escape') { e.preventDefault(); hide(true); return; }
    if (shortcut && isLauncherShortcut(e)) {
      e.preventDefault();
      if (open) hide(true); else activate();
    }
  }
  function onPointerdown(e: PointerEvent) {
    const target = e.target as Node;
    if (!open || panel?.contains(target) || button?.contains(target)) return;
    hide(false);
    // Focus returns to the circle, unless the click landed on a control that
    // took focus (a link, button or field). Focus moves after pointerdown.
    setTimeout(() => {
      const active = document.activeElement;
      if (!(active instanceof HTMLElement) || active === document.body || active.tabIndex < 0)
        (button ?? opener)?.focus();
    });
  }
  function onFocusout(e: FocusEvent) {
    const next = e.relatedTarget as Node | null;
    if (next && !panel?.contains(next) && next !== button) hide(false);
  }
</script>

<svelte:window onkeydown={onKeydown} />
<svelte:document onpointerdown={onPointerdown} />

{#if showCircle}
  <div class="circle-layer" style:--launcher-circle-size={size}>
    <button
      bind:this={button}
      class="circle"
      type="button"
      aria-label={label}
      aria-expanded={open}
      aria-controls={open ? panelId : undefined}
      aria-haspopup="dialog"
      aria-keyshortcuts="Control+K Meta+K"
      onclick={() => (open ? hide(true) : show())}
    >
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        {#each [6, 12, 18] as y}{#each [6, 12, 18] as x}<circle cx={x} cy={y} r="1.9" />{/each}{/each}
      </svg>
    </button>
  </div>
{/if}

{#if open}
  <div bind:this={panel} id={panelId} class="panel" role="dialog" aria-label="App launcher" tabindex="-1" onfocusout={onFocusout}>
    {@render children()}
  </div>
{/if}

<style>
  /* Covers the viewport to centre the circle but lets clicks through;
     only the button itself receives pointer events. */
  .circle-layer {
    position: fixed; inset: 0; display: grid; place-items: center;
    pointer-events: none; z-index: var(--z-launcher-circle)
  }
  .circle {
    pointer-events: auto; display: grid; place-items: center; padding: 0;
    inline-size: var(--launcher-circle-size); block-size: var(--launcher-circle-size);
    border: 0; border-radius: 50%; background: var(--accent); color: var(--accent-text);
    box-shadow: var(--shadow); transition: filter .15s, transform .15s
  }
  .circle svg { inline-size: 42%; block-size: 42%; fill: currentColor }
  .circle:hover { filter: brightness(1.12); transform: scale(1.05) }
  .circle:active, .circle[aria-expanded='true'] { filter: brightness(.88); transform: scale(.96) }
  .circle:focus-visible { outline: 3px solid var(--link); outline-offset: 4px }

  .panel {
    position: fixed; top: 50%; left: 50%; translate: -50% -50%; z-index: var(--z-dialog);
    box-sizing: border-box; inline-size: min(26rem, 100vw - 2rem); max-block-size: min(34rem, 100dvh - 2rem);
    overflow: auto; padding: 1rem; border: 1px solid var(--border); border-radius: 1rem;
    background: var(--surface); color: var(--text); box-shadow: var(--shadow);
    animation: expand .16s ease-out
  }
  .panel:focus { outline: none }
  @keyframes expand { from { opacity: 0; transform: scale(.6) } }
  @media (prefers-reduced-motion: reduce) {
    .circle, .panel { transition: none; animation: none }
    .circle:hover, .circle:active, .circle[aria-expanded='true'] { transform: none }
  }
</style>
