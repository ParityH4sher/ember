<script lang="ts">
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import { prefs } from '../../lib/prefs.svelte';
  import { displayShortcut, parseShortcut } from '../../lib/launcher/keyboard';

  let shortcut = $derived(prefs.launcherShortcut ? parseShortcut(prefs.launcherShortcut) : null);

  // The desktop surface. The launcher circle itself is drawn by AppShell,
  // centred on the primary display, above this background.
</script>

<PageHeader title="Desktop" lede="A workspace surface. App windows will open here." />

<p class="hint muted">
  {#if prefs.launcherCircle}
    Use the circle in the middle of your primary display{#if shortcut}, or press <kbd>{displayShortcut(shortcut)}</kbd>,{/if}
    to open the launcher.
  {:else}
    {#if shortcut}Press <kbd>{displayShortcut(shortcut)}</kbd> to open the launcher.{/if}
    You can show the launcher circle again in <a href="/settings/#desktop">Settings</a>.
  {/if}
</p>

<style>
  .hint { margin: 0; max-width: 40rem }
  kbd { border: 1px solid var(--border-strong); border-radius: .25rem; padding: 0 .3rem; font: inherit; font-size: .9em }
</style>
