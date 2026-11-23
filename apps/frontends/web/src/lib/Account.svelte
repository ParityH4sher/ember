<script lang="ts">
  import { signInUrl, signOut, type Session } from './api';

  // Shows the optional OpenID Connect sign-in state. The API owns the session;
  // the browser only learns whether it exists, never a token. Sign-in and
  // sign-out are full-page navigations through the API.
  let { session, confirmLeave = () => true }: {
    session: Session | null;
    /** Asked before leaving the page; false keeps the visitor here. */
    confirmLeave?: () => boolean;
  } = $props();

  let busy = $state(false), error = $state('');
  let user = $derived(session?.user);

  function signIn() {
    if (confirmLeave()) location.assign(signInUrl(location.pathname + location.search));
  }
  async function logout() {
    if (!confirmLeave()) return;
    busy = true;
    error = '';
    try {
      // End the identity provider session too when it supports RP-initiated logout.
      const target = await signOut();
      if (target) location.assign(target); else location.reload();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      busy = false;
    }
  }
</script>

{#if session?.authenticated}
  <span class="account">Signed in as {user?.name || user?.email || user?.subject}
    ({session.role === 'admin' ? 'admin' : 'user'})
    <button id="sign-out" onclick={logout} disabled={busy}>Sign out</button>
    {#if error}<span role="status">{error}</span>{/if}
  </span>
{:else if session?.enabled}
  <span class="account"><button onclick={signIn}>Sign in</button></span>
{/if}

<style>
  .account { float: right; overflow-wrap: anywhere }
  button { margin-left: .5rem; padding: .1rem .5rem }
</style>
