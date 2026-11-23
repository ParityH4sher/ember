<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, id, actions, children }: {
    title?: string; id?: string; actions?: Snippet; children: Snippet
  } = $props();
</script>

<section class="card" {id} aria-labelledby={title && id ? `${id}-title` : undefined}>
  {#if title || actions}
    <div class="head">
      {#if title}<h2 id={id ? `${id}-title` : undefined}>{title}</h2>{/if}
      {@render actions?.()}
    </div>
  {/if}
  {@render children()}
</section>

<style>
  .card {
    padding: var(--space); border: 1px solid var(--border); border-radius: var(--radius);
    background: var(--surface); min-width: 0
  }
  .head { display: flex; align-items: center; gap: .5rem; margin-bottom: .6rem }
  h2 { font-size: 1.1rem; margin: 0; margin-right: auto }
  .card > :global(:last-child) { margin-bottom: 0 }
</style>
