<script lang="ts">
  import type { Snippet } from 'svelte';

  let { tone = 'neutral', children }: { tone?: 'neutral' | 'ok' | 'warn' | 'danger'; children: Snippet } = $props();
</script>

<span class="badge {tone}">{@render children()}</span>

<style>
  .badge {
    display: inline-block; padding: 0 .5rem; border-radius: 1rem; font-size: .8rem; font-weight: 650;
    border: 1px solid currentColor; color: var(--muted); white-space: nowrap
  }
  .ok { color: var(--ok) }
  .warn { color: var(--warn) }
  .danger { color: var(--danger) }
</style>
