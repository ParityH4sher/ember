<script lang="ts">
  import { tick } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import type { WidgetProps } from '../widgets/types';
  import LauncherPanel from './LauncherPanel.svelte';
  import { launcherConfig, type LauncherConfig } from './config';

  // The launcher widget. In grid mode the launcher is shown in place; in
  // compact mode a single button opens it as a popover (for a narrow,
  // taskbar-like slot). The app-wide shortcut opens the shell's launcher
  // (AppShell), not a particular widget.
  let { config: stored, setConfig, context }: WidgetProps<LauncherConfig> = $props();

  const uid = $props.id();
  let config = $derived(launcherConfig(stored));
  let open = $state(false);
  let panel = $state<LauncherPanel>(), toggle = $state<HTMLButtonElement>(), root = $state<HTMLElement>();
  let compact = $derived(config.mode === 'compact');

  // The popover is fixed to the viewport, so the widget's scrolling body does
  // not clip it; it follows the button on scroll and resize.
  let at = $state({ left: 0, top: 0 });
  function place() {
    const r = toggle?.getBoundingClientRect();
    if (!r) return;
    const width = Math.min(384, innerWidth * .85);
    at = { left: Math.max(8, Math.min(r.left, innerWidth - width - 8)), top: r.bottom + 6 };
  }
  async function reveal() {
    place();
    open = true;
    await tick();
    panel?.focusSearch();
  }
  function close() {
    open = false;
    toggle?.focus();
  }
  function outside(e: Event) {
    if (open && root && !root.contains(e.target as Node) && !(e.target as Element).closest?.('dialog')) open = false;
  }
</script>

<svelte:window onpointerdown={outside} onresize={() => open && place()} onscrollcapture={() => open && place()} />

<div class="widget-launcher" class:compact bind:this={root}>
  {#if compact}
    <button bind:this={toggle} class="toggle" aria-label="App launcher" aria-expanded={open} aria-controls="{uid}-popover"
      title="App launcher" onclick={() => (open ? close() : reveal())}><Icon name="apps" size={22} /></button>
    {#if open}
      <div class="popover" id="{uid}-popover" role="dialog" aria-label="App launcher" style:left="{at.left}px" style:top="{at.top}px">
        <LauncherPanel bind:this={panel} {config} {setConfig} {context} onclose={close} onlaunch={() => (open = false)} />
      </div>
    {/if}
  {:else}
    <LauncherPanel bind:this={panel} {config} {setConfig} {context} />
  {/if}
</div>

<style>
  .widget-launcher { position: relative; height: 100% }
  .toggle { display: inline-grid; place-items: center; padding: .45rem; border-radius: var(--radius) }
  .toggle[aria-expanded="true"] { border-color: var(--accent) }
  .popover {
    position: fixed; z-index: 30; width: min(24rem, 85vw); max-height: min(30rem, 70vh);
    overflow: auto; padding: .6rem; background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius); box-shadow: 0 8px 24px rgb(0 0 0 / .3)
  }
</style>
