<script lang="ts">
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import { prefs } from '../../lib/prefs.svelte';

  // The desktop surface. The launcher circle itself is drawn by AppShell,
  // centred on the primary display, above this background.
</script>

<PageHeader title="Desktop" lede="A workspace surface. App windows will open here." />

<p class="hint muted">
  {#if prefs.launcherCircle}
    Use the circle in the middle of your primary display, or press <kbd>Ctrl</kbd>+<kbd>K</kbd>, to open the launcher.
  {:else}
    Press <kbd>Ctrl</kbd>+<kbd>K</kbd> to open the launcher. You can show the launcher circle again in
    <a href="/settings/#desktop">Settings</a>.
  {/if}
</p>

<style>
  .hint { margin: 0; max-width: 40rem }
  kbd { border: 1px solid var(--border-strong); border-radius: .25rem; padding: 0 .3rem; font: inherit; font-size: .9em }
</style>
