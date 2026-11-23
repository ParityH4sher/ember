<script lang="ts">
  let { label, value, detail, href, tone }: {
    label: string;
    /** undefined while loading. */
    value: string | number | undefined;
    detail?: string;
    href?: string;
    tone?: 'ok' | 'warn' | 'danger';
  } = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class="stat">
  <span class="label">{label}</span>
  <strong class={tone}>{value ?? '…'}</strong>
  {#if detail}<span class="detail">{detail}</span>{/if}
</svelte:element>

<style>
  .stat {
    display: flex; flex-direction: column; gap: .15rem; padding: var(--space);
    border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
    color: inherit; text-decoration: none; min-width: 0
  }
  a.stat:hover { border-color: var(--border-strong) }
  .label, .detail { color: var(--muted); font-size: .85rem }
  strong { font-size: 1.6rem; line-height: 1.2; overflow-wrap: anywhere }
  .ok { color: var(--ok) }
  .warn { color: var(--warn) }
  .danger { color: var(--danger) }
</style>
