<!--
  Plain workspace shown on additional displays. It is intentionally blank:
  the window only claims its screen. It announces itself to the other Ember
  windows, moves back onto its screen when asked, and turns into the Notes
  window when the app is launched while no Notes window is open.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { replaceState } from '$app/navigation';
  import * as d from './displays';

  interface LaunchQueue { setConsumer(consumer: (params: { targetURL?: string }) => void): void }

  onMount(() => {
    let display = new URLSearchParams(location.search).get('display') ?? '';
    let details: d.ScreenDetails | null = null;
    let notesSeen = false;
    // Identity for duplicate detection: after a relaunch, Notes may open a
    // second workspace for a screen before the first one has announced itself.
    const id = crypto.randomUUID();
    const since = performance.timeOrigin;
    let duplicate = false;
    const channel = typeof BroadcastChannel === 'undefined' ? null : d.channel();
    const send = (message: d.Message) => channel?.postMessage(message);
    const announce = () => { if (display && !duplicate) send({ type: 'workspace', display, id, since }); };
    const place = () => {
      const target = details?.screens.find(s => d.screenKey(s) === display);
      if (target) d.fit(window, target);
      // If the browser kept this window elsewhere, say where it really is, so
      // Notes sees the intended screen as still without a window.
      setTimeout(moved, 500);
    };
    // If the user drags this window to another screen, it now belongs there.
    const moved = () => {
      const key = details && d.screenKey(details.currentScreen);
      if (!key || key === display) return;
      if (display) send({ type: 'bye', display });
      display = key;
      window.name = d.windowName(key);
      replaceState(d.workspaceUrl(key), {});
      announce();
    };
    const leave = () => { if (display && !duplicate) send({ type: 'bye', display }); };

    if (channel) channel.onmessage = ({ data }: MessageEvent<d.Message>) => {
      if (data.type === 'query') announce();
      else if (data.type === 'workspace' && data.display === display && data.id !== id) {
        // Another workspace claims this screen. The newer one closes without a
        // 'bye' (the screen stays covered); the older one answers so the newer sees it.
        if (d.yields({ id, since }, data)) { duplicate = true; window.close(); }
        else announce();
      }
      else if (data.type === 'notes') notesSeen = true;
      else if (data.type === 'arrange') place();
    };
    announce();
    addEventListener('pagehide', leave);

    // Only fit when the permission is already granted: a workspace never prompts.
    void d.permission().then(async state => {
      if (state !== 'granted' || !d.api()) return;
      try { details = await d.api()!.getScreenDetails(); } catch { return; }
      place();
      details.addEventListener('currentscreenchange', moved);
    });

    // launch_handler "focus-existing" may hand an app launch to this window.
    // With no Notes window left, become one instead of leaving the user on a blank screen.
    const queue = (window as Window & { launchQueue?: LaunchQueue }).launchQueue;
    queue?.setConsumer(async params => {
      notesSeen = false;
      send({ type: 'query' });
      await new Promise(resolve => setTimeout(resolve, 300));
      if (notesSeen) return;
      const target = params.targetURL && !new URL(params.targetURL).pathname.startsWith(d.workspacePath())
        ? params.targetURL : d.notesPath();
      leave();
      location.assign(target);
    });

    return () => {
      removeEventListener('pagehide', leave);
      details?.removeEventListener('currentscreenchange', moved);
      leave();
      channel?.close();
    };
  });
</script>

<svelte:head><title>Workspace · Ember</title></svelte:head>
<main class="workspace" aria-label="Ember workspace"></main>

<style>
  :global(html) { color-scheme: dark; background: #10151b; }
  :global(body) { margin: 0; }
  /* Blank and full-bleed. In a window-controls-overlay window the whole
     surface is the drag handle, so the window can still be moved. */
  .workspace { position: fixed; inset: 0; background: #10151b; -webkit-app-region: drag; }
</style>
