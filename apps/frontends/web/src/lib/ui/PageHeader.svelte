<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, lede, actions }: { title: string; lede?: string; actions?: Snippet } = $props();
</script>

<svelte:head><title>{title === 'Dashboard' ? 'Ember' : `${title} · Ember`}</title></svelte:head>
<div class="page-header">
  <div>
    <h1>{title}</h1>
    {#if lede}<p class="muted">{lede}</p>{/if}
  </div>
  {@render actions?.()}
</div>

<style>
  .page-header { display: flex; align-items: flex-end; gap: 1rem; flex-wrap: wrap; margin-bottom: var(--space) }
  .page-header > div { margin-right: auto }
  p { margin: .2rem 0 0 }
</style>
