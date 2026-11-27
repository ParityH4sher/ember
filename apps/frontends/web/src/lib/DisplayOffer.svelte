<!-- Non-modal banner for multi-display offers. Every "open" button calls
     DisplayManager.openAll directly so it carries the click's user activation. -->
<script lang="ts">
  import type { DisplayManager } from './display-manager.svelte';
  let { displays }: { displays: DisplayManager } = $props();
  const plural = (n: number) => n === 1 ? '1 display' : `${n} displays`;
</script>

{#if displays.offer}
  {@const offer = displays.offer}
  <aside id="display-offer" role="status">
    {#if offer.kind === 'setup'}
      <p>Ember can open a workspace on each of your other displays.</p>
      <button onclick={() => displays.openAll()}>Use all displays</button>
      <button onclick={() => displays.dismiss()}>Not now</button>
    {:else if offer.kind === 'remaining'}
      <p>{plural(offer.count)} still without a window. The browser may have blocked pop-ups.</p>
      <button onclick={() => displays.openAll()}>Open on remaining displays</button>
      <button onclick={() => displays.dismiss()}>Dismiss</button>
    {:else if offer.kind === 'new'}
      <p>New display detected.</p>
      <button onclick={() => displays.openAll()}>Open window there</button>
      <button onclick={() => displays.dismiss()}>Dismiss</button>
    {:else if offer.kind === 'notice'}
      <p>{offer.text}</p>
      <button onclick={() => displays.dismiss()}>Dismiss</button>
    {/if}
  </aside>
{/if}

<style>
  aside { position: fixed; right: 1rem; bottom: 1rem; z-index: 10; max-width: 22rem; padding: .8rem 1rem;
    background: #192129; border: 1px solid #516274; border-radius: .4rem; box-shadow: 0 .3rem 1rem #0008; }
  p { margin: 0 0 .6rem; }
  button { cursor: pointer; }
  button + button { margin-left: .4rem; }
</style>
