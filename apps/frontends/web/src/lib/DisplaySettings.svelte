<!-- Header control for multi-display settings and manual recovery. -->
<script lang="ts">
  import type { DisplayManager } from './display-manager.svelte';
  let { displays }: { displays: DisplayManager } = $props();
</script>

{#if displays.supported}
  <button id="displays-button" popovertarget="displays-settings">Displays</button>
  <div id="displays-settings" popover>
    <label><input type="checkbox" checked={displays.startup}
      onchange={e => displays.setStartup(e.currentTarget.checked)}> Open on all displays at startup</label>
    <button id="reopen-displays" onclick={() => displays.openAll()}>Reopen on all displays</button>
    <p>Moves open workspaces back to their screens and opens missing ones.</p>
  </div>
{/if}

<style>
  [popover] { margin: 3.5rem 1rem auto auto; max-width: 20rem; padding: 1rem; color: inherit;
    background: #192129; border: 1px solid #516274; border-radius: .4rem; -webkit-app-region: no-drag; }
  label { display: flex; gap: .5rem; align-items: center; font-weight: 400; }
  label input { width: auto; margin: 0; }
  p { margin: .5rem 0 0; font-size: .875rem; color: #9fb0c0; }
</style>
