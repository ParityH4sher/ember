<script lang="ts">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import AppShell from '../lib/shell/AppShell.svelte';
  import { auth, loadSession, loginPath } from '../lib/session.svelte';
  import Unavailable from '../lib/shell/Unavailable.svelte';
  import { loadPrefs } from '../lib/prefs.svelte';
  import { listServices } from '../lib/api';
  import { registerApps } from '../lib/apps/registry';
  import { adminApps, serviceApps } from '../lib/apps/static';
  import '../lib/setup';
  import '../app.css';

  let { children }: { children: Snippet } = $props();

  // The sign-in page stands alone; every other page needs the session first.
  let bare = $derived(page.url.pathname === '/login/');

  onMount(() => {
    loadPrefs();
    loadSession();
  });

  // Apps registered at runtime: they appear in the launcher without a reload.
  let ready = $derived(auth.loaded && !auth.signedOut && !auth.unavailable);
  $effect(() => {
    if (!ready) return;
    let undo = () => {};
    let live = true;
    listServices().then(r => { if (live) undo = registerApps(serviceApps(r.services)); }, () => {});
    return () => { live = false; undo(); };
  });
  $effect(() => { if (ready && auth.admin) return registerApps(adminApps); });

  // Auth gate. The API enforces sign-in; this sends people to the sign-in page
  // instead of showing pages full of 401 errors.
  $effect(() => {
    if (auth.loaded && auth.signedOut && !bare)
      goto(loginPath(page.url.pathname + page.url.search), { replaceState: true });
  });
</script>

{#if bare}
  {@render children()}
{:else if auth.unavailable}
  <Unavailable />
{:else if !auth.loaded || auth.signedOut}
  <p class="loading" role="status">Loading Ember…</p>
{:else}
  <AppShell>{@render children()}</AppShell>
{/if}

<style>
  .loading { display: grid; place-items: center; min-height: 100vh; margin: 0; color: var(--muted) }
</style>
