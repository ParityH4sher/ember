<script lang="ts">
  import { onMount } from 'svelte';
  import Card from '../../lib/ui/Card.svelte';
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import PluginCard from '../../lib/PluginCard.svelte';
  import { listPlugins, type Plugin } from '../../lib/api';
  import { failure, settle, type Loaded } from '../../lib/load';
  import { auth } from '../../lib/session.svelte';

  let plugins = $state.raw<Loaded<Plugin[]>>();

  onMount(() => settle(listPlugins().then(r => r.plugins), v => (plugins = v)));

  function changed(updated: Plugin) {
    if (Array.isArray(plugins)) plugins = plugins.map(p => p.id === updated.id ? updated : p);
  }
</script>

<PageHeader title="Plugins"
  lede={auth.admin ? 'Configure, activate and run the plugins on this Ember instance.' : 'Run operations of the plugins an admin has activated.'} />

{#if auth.admin}
  <p class="note" role="note">Settings are local to this Ember instance and reset when the API restarts. Use test credentials only.</p>
{/if}

<Card>
  {#if plugins === undefined}
    <p class="muted">Loading plugins…</p>
  {:else if failure(plugins)}
    <p role="alert">Unable to load plugins: {plugins.error}</p>
  {:else}
    {#each plugins as plugin (plugin.id)}
      <PluginCard {plugin} admin={auth.admin} onchange={changed} />
    {:else}
      <p class="muted">No plugins are installed.</p>
    {/each}
  {/if}
</Card>

<style>
  .note { padding: .6rem .9rem; border-radius: var(--radius); background: var(--warn-bg); margin: 0 0 var(--space) }
</style>
