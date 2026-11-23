<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './ui/Icon.svelte';
  import { listServices, type ServiceLink } from './api';
  import { failure, settle, type Loaded } from './load';
  import { safeExternalUrl } from './url';

  // Shared services the operator configured. Only http(s) links without
  // credentials are shown (safeExternalUrl), and they open without a referrer.
  let services = $state.raw<Loaded<ServiceLink[]>>();

  onMount(() => settle(listServices().then(r => r.services), v => (services = v)));
</script>

{#if services === undefined}
  <p class="muted">Loading services…</p>
{:else if failure(services)}
  <p role="alert">Unable to load services: {services.error}</p>
{:else}
  <ul>
    {#each services as service}
      {@const url = safeExternalUrl(service.url)}
      {#if url}
        <li><a href={url} referrerpolicy="no-referrer" target="_blank" rel="noopener noreferrer">
          {service.name} <Icon name="external" size={14} /><span class="visually-hidden"> (opens in a new tab)</span></a></li>
      {/if}
    {:else}
      <li class="muted">No services configured.</li>
    {/each}
  </ul>
{/if}

<style>
  ul { list-style: none; margin: 0; padding: 0 }
  li { padding: .35rem 0; border-top: 1px solid var(--border) }
  li:first-child { border-top: 0 }
</style>
