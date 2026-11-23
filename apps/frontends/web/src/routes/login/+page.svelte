<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { auth, safeReturnTo, signIn } from '../../lib/session.svelte';

  // One way in: the API's OpenID Connect flow with the operator's identity
  // provider (Authentik by default; the API reports its name). The button is a
  // full-page navigation; the browser never handles a token.
  let returnTo = $state('/'), leaving = $state(false);

  onMount(() => { returnTo = safeReturnTo(page.url.searchParams.get('returnTo')); });

  // Already signed in (or sign-in is off): there is nothing to do here.
  $effect(() => {
    if (auth.session?.authenticated) goto(returnTo, { replaceState: true });
  });

  function start() {
    leaving = true;
    signIn(returnTo);
  }
</script>

<svelte:head><title>Sign in · Ember</title></svelte:head>
<main>
  <div class="panel">
    <p class="mark" aria-hidden="true">◆</p>
    <h1>Sign in to Ember</h1>
    {#if !auth.loaded}
      <p class="muted" role="status">Checking sign-in…</p>
    {:else if auth.unavailable}
      <p class="muted" role="alert">Sign-in is temporarily unavailable. Try again in a moment.</p>
      <button class="primary" onclick={() => location.reload()}>Try again</button>
    {:else if auth.session === null}
      <p class="muted" role="alert">Cannot reach Ember. Check your connection and reload this page.</p>
    {:else if !auth.session.enabled}
      <p class="muted">Sign-in is not configured on this Ember instance, so no account is needed.</p>
      <a class="go" href={returnTo}>Continue to Ember</a>
    {:else}
      <p class="muted">Your Ember account is managed by {auth.providerName}.</p>
      <button class="primary" onclick={start} disabled={leaving}>
        {leaving ? `Redirecting to ${auth.providerName}…` : `Sign in with ${auth.session.provider?.name || 'single sign-on'}`}
      </button>
    {/if}
  </div>
</main>

<style>
  main { display: grid; place-items: center; min-height: 100vh; padding: 1rem; box-sizing: border-box }
  .panel {
    width: min(24rem, 100%); box-sizing: border-box; text-align: center; padding: 2.5rem 2rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: .8rem
  }
  .mark { font-size: 2.4rem; line-height: 1; color: var(--accent); margin: 0 0 .8rem }
  h1 { margin-bottom: .6rem }
  p { margin: 0 0 1.6rem }
  button, .go { display: block; width: 100%; box-sizing: border-box; padding: .7rem 1rem; font-size: 1.05rem }
  .go { text-align: center }
</style>
