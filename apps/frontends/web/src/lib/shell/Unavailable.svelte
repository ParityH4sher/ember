<script lang="ts">
  import { loadSession } from '../session.svelte';

  // Shown instead of the app while the API answers 503 for the session: its
  // session store (Redis) or the identity provider is down. Nothing is signed
  // out; the session cookie is kept, so retrying is enough once it recovers.
  let busy = $state(false);
  async function retry() {
    busy = true;
    try { await loadSession(); } finally { busy = false; }
  }
</script>

<svelte:head><title>Sign-in unavailable · Ember</title></svelte:head>
<main>
  <div class="panel" role="alert">
    <h1>Sign-in is temporarily unavailable</h1>
    <p class="muted">Ember cannot check your session right now. You are still signed in; nothing was lost.
      Try again in a moment, or tell your Ember operator if this continues.</p>
    <button class="primary" onclick={retry} disabled={busy}>{busy ? 'Checking…' : 'Try again'}</button>
  </div>
</main>

<style>
  main { display: grid; place-items: center; min-height: 100vh; padding: 1rem; box-sizing: border-box }
  .panel {
    width: min(28rem, 100%); box-sizing: border-box; text-align: center; padding: 2rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: .8rem
  }
  h1 { font-size: 1.3rem; margin-bottom: .8rem }
  p { margin: 0 0 1.4rem }
</style>
