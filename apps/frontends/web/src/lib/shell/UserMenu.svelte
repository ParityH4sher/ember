<script lang="ts">
  import Icon from '../ui/Icon.svelte';
  import { auth, logout, signIn } from '../session.svelte';

  let open = $state(false), busy = $state(false), error = $state('');
  let menu = $state<HTMLDetailsElement>();
  let initials = $derived(auth.displayName.split(/[\s@.]+/).filter(Boolean).slice(0, 2)
    .map(w => w[0]?.toUpperCase()).join('') || '?');

  async function signOut() {
    busy = true;
    error = '';
    try { await logout(); }
    catch (e) { error = e instanceof Error ? e.message : String(e); }
    finally { busy = false; }
  }
  // Close when focus or a click leaves the menu, and on Escape.
  function outside(event: Event) {
    if (open && menu && !menu.contains(event.target as Node)) open = false;
  }
  function key(event: KeyboardEvent) {
    if (event.key === 'Escape' && open) { open = false; menu?.querySelector('summary')?.focus(); }
  }
</script>

<svelte:window onclick={outside} onfocusin={outside} onkeydown={key} />

{#if auth.session?.authenticated}
  <details class="user-menu" bind:open bind:this={menu}>
    <!-- The visible name is part of the accessible name (WCAG 2.5.3). -->
    <summary>
      <span class="avatar" aria-hidden="true">{initials}</span>
      <span class="visually-hidden">{'Account menu for '}</span><span class="name">{auth.displayName}</span>
    </summary>
    <div class="panel">
      <p class="who"><strong>{auth.displayName}</strong>
        {#if auth.user?.email && auth.user.email !== auth.displayName}<br><span class="muted">{auth.user.email}</span>{/if}
        <br><span class="muted">Role: {auth.admin ? 'admin' : 'user'}</span></p>
      <a href="/account/" onclick={() => (open = false)}><Icon name="user" size={18} /> Profile & account</a>
      <a href="/settings/" onclick={() => (open = false)}><Icon name="settings" size={18} /> Settings</a>
      <button onclick={signOut} disabled={busy}><Icon name="logout" size={18} /> {busy ? 'Signing out…' : 'Sign out'}</button>
      {#if error}<p role="alert" class="error">{error}</p>{/if}
    </div>
  </details>
{:else if auth.signedOut}
  <button class="primary" onclick={() => signIn()}>Sign in</button>
{:else if auth.loaded && !auth.session?.enabled}
  <span class="muted local" title="Sign-in is not configured on this Ember instance">Local mode</span>
{/if}

<style>
  .user-menu { position: relative }
  summary { list-style: none; display: flex; align-items: center; gap: .5rem; padding: .2rem; border-radius: 2rem }
  summary::-webkit-details-marker { display: none }
  summary:hover { background: var(--surface-2) }
  .avatar {
    display: grid; place-items: center; width: 2rem; height: 2rem; border-radius: 50%;
    background: var(--accent); color: var(--accent-text); font-weight: 700; font-size: .8rem
  }
  .name { max-width: 12rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding-right: .4rem }
  .panel {
    position: absolute; right: 0; top: calc(100% + .4rem); z-index: 20; min-width: 15rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
    padding: .4rem; box-shadow: 0 8px 24px rgb(0 0 0 / .25)
  }
  .who { margin: .3rem .5rem .6rem; overflow-wrap: anywhere }
  .panel a, .panel button {
    display: flex; align-items: center; gap: .5rem; width: 100%; box-sizing: border-box;
    padding: .45rem .5rem; border: 0; border-radius: .3rem; background: none;
    color: inherit; text-decoration: none; text-align: left
  }
  .panel a:hover, .panel button:hover:not(:disabled) { background: var(--surface-2) }
  .error { color: var(--danger); margin: .4rem .5rem }
  .local { font-size: .9rem }
  @media (max-width: 40rem) {
    .name { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); padding: 0 }
  }
</style>
