<script lang="ts">
  import Badge from '../../lib/ui/Badge.svelte';
  import Card from '../../lib/ui/Card.svelte';
  import Icon from '../../lib/ui/Icon.svelte';
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import { auth, logout } from '../../lib/session.svelte';
  import { message } from '../../lib/load';

  // Ember does not own accounts: everything here comes from the ID token the
  // identity provider issued at sign-in, as reported by GET /v1/auth/session.
  let busy = $state(false), error = $state('');
  let expires = $derived(auth.session?.expiresAt ? new Date(auth.session.expiresAt) : null);

  async function signOut() {
    busy = true;
    error = '';
    try { await logout(); } catch (e) { error = message(e); } finally { busy = false; }
  }
</script>

<PageHeader title="Profile & account" lede="Who you are signed in as, and how your account is managed." />

{#if !auth.session?.enabled}
  <Card title="Local mode" id="local">
    <p>Sign-in is not configured on this Ember instance, so there are no accounts. Everyone who can
      reach it has full admin rights; keep it on a trusted network.</p>
    <p class="muted">An operator can enable sign-in with Authentik; see the authentication guide in Ember's documentation.</p>
  </Card>
{:else}
  <div class="grid">
    <Card title="Profile" id="profile">
      <dl>
        <dt>Name</dt><dd>{auth.user?.name || '—'}</dd>
        <dt>Email</dt><dd>{auth.user?.email || '—'}</dd>
        <dt>Subject</dt><dd><code>{auth.user?.subject}</code></dd>
        <dt>Role</dt><dd><Badge tone={auth.admin ? 'warn' : 'neutral'}>{auth.admin ? 'admin' : 'user'}</Badge></dd>
        <dt>Groups</dt>
        <dd>
          {#each auth.user?.groups ?? [] as group}<Badge>{group}</Badge>{' '}{:else}<span class="muted">None reported</span>{/each}
        </dd>
      </dl>
      <p class="muted">These details come from {auth.providerName} when you sign in. Change them there;
        Ember picks up changes, including group and role changes, at your next sign-in.</p>
      {#if auth.accountUrl}
        <p><a href={auth.accountUrl} target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer">
          Manage your account at {auth.session?.provider?.name || 'your identity provider'}
          <Icon name="external" size={14} /><span class="visually-hidden"> (opens in a new tab)</span></a></p>
      {/if}
    </Card>

    <Card title="Session" id="session">
      <dl>
        <dt>Signed in with</dt><dd>{auth.session?.provider?.name ?? 'Single sign-on'} (OpenID Connect)</dd>
        <dt>Expires</dt>
        <dd>{#if expires}<time datetime={expires.toISOString()}>{expires.toLocaleString()}</time>{:else}—{/if}</dd>
      </dl>
      <p class="muted">Sessions are not renewed. Ember signs you out when this one expires.</p>
      <button onclick={signOut} disabled={busy}>{busy ? 'Signing out…' : 'Sign out'}</button>
      {#if error}<p role="alert" class="error">{error}</p>{/if}
    </Card>
  </div>
{/if}

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr)); gap: var(--space); align-items: start }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: .4rem 1rem; margin: 0 0 1rem }
  dt { color: var(--muted) }
  dd { margin: 0; overflow-wrap: anywhere }
  .error { color: var(--danger) }
</style>
