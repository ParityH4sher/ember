<script lang="ts">
  import { onMount } from 'svelte';
  import Account from '../lib/Account.svelte';
  import PluginCard from '../lib/PluginCard.svelte';
  import {
    getHealth, getSession, listPlugins, listServices, signInUrl,
    type Health, type Plugin, type ServiceLink, type Session
  } from '../lib/api';

  // Each is undefined while loading. health is null when the API is unreachable;
  // services and plugins hold the error message when they could not be loaded.
  let health = $state.raw<Health | null>(), session = $state.raw<Session | null>(null);
  let services = $state.raw<ServiceLink[] | string>(), plugins = $state.raw<Plugin[] | string>();
  let signedOut = $derived(Boolean(session?.enabled && !session.authenticated));
  // Without sign-in every caller is an admin.
  let admin = $derived(!session?.enabled || session.role === 'admin');

  const failure = (error: unknown) => error instanceof Error ? error.message : String(error);

  // Only http(s) links without credentials are shown.
  function serviceUrl(raw: string): string | null {
    try {
      const url = new URL(raw);
      return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password ? url.href : null;
    } catch {
      return null;
    }
  }

  onMount(async () => {
    getHealth().then(h => { health = h; }, () => { health = null; });
    session = await getSession();
    if (signedOut) return;
    listServices().then(r => { services = r.services; }, e => { services = failure(e); });
    listPlugins().then(r => { plugins = r.plugins; }, e => { plugins = failure(e); });
  });
</script>

<svelte:head><title>Ember</title></svelte:head>
{#snippet signInFirst(what: string)}
  <p><a href={signInUrl('/')}>Sign in</a> to view {what}.</p>
{/snippet}
<header>Ember <Account {session} /></header>
<main>
  <h1>Ember</h1>
  <p class="lede">A self-hosted workspace for turning ideas into engineered artifacts.</p>
  <section class="card" aria-live="polite">
    <strong>Core API:</strong>
    <span id="status" class:ok={health}>
      {health === undefined ? 'checking…' : health ? `${health.status} (version ${health.version})` : 'unavailable'}
    </span>
    <p><a href="/apps/notes/">Open project notes →</a></p>
  </section>
  <section class="card">
    <h2>Shared services</h2>
    {#if signedOut}
      {@render signInFirst('services')}
    {:else if services === undefined}
      <p>Loading services…</p>
    {:else if typeof services === 'string'}
      <p>Unable to load services: {services}</p>
    {:else}
      {#each services as service}
        {@const url = serviceUrl(service.url)}
        {#if url}<p><a href={url} referrerpolicy="no-referrer">Open {service.name} →</a></p>{/if}
      {:else}
        <p>No services configured.</p>
      {/each}
    {/if}
  </section>
  <section class="card">
    <h2>Plugins</h2>
    <p>Settings are local to this Ember instance and reset on API restart. Use test credentials only.</p>
    {#if signedOut}
      {@render signInFirst('plugins')}
    {:else if plugins === undefined}
      <p>Loading plugins…</p>
    {:else if typeof plugins === 'string'}
      <p>Unable to load plugins: {plugins}</p>
    {:else}
      {#each plugins as plugin (plugin.id)}
        <PluginCard {plugin} {admin}
          onchange={updated => { if (Array.isArray(plugins)) plugins = plugins.map(p => p.id === updated.id ? updated : p); }} />
      {:else}
        <p>No plugins configured.</p>
      {/each}
    {/if}
  </section>
</main>

<style>
  main {max-width:56rem;margin:auto;padding:1.2rem}
  h1 {font-size:2rem;margin:.5rem 0 0}
  .lede {color:#9da7b3}
  .card {margin-top:1.2rem;padding:1.2rem;border:1px solid #34404c;border-radius:.4rem;background:#192129}
  .card h2 {font-size:1.2rem;margin-top:0}
  #status {color:#f0b44c}
  #status.ok {color:#3fb950}
</style>
