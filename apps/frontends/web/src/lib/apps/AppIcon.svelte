<script lang="ts">
  import type { AppIcon } from './types';

  // Decorative: the app's name is always shown or announced next to it.
  let { icon, name, size = 32 }: { icon?: AppIcon; name: string; size?: number } = $props();
  let broken = $state(false);
  $effect(() => { void icon; broken = false; });
  let initial = $derived([...name.trim()][0]?.toUpperCase() ?? '?');
</script>

{#if typeof icon === 'string' && !broken}
  <img src={icon} alt="" width={size} height={size} loading="lazy" draggable="false" onerror={() => (broken = true)}>
{:else if typeof icon === 'function'}
  {@const Glyph = icon}
  <Glyph {size} />
{:else if icon && typeof icon === 'object'}
  {@const Glyph = icon.component}
  <Glyph {...icon.props} {size} />
{:else}
  <span class="monogram" aria-hidden="true" style:width="{size}px" style:height="{size}px"
    style:font-size="{size * .5}px">{initial}</span>
{/if}

<style>
  img { object-fit: contain; border-radius: .25rem }
  .monogram {
    display: inline-grid; place-items: center; border-radius: .35rem; font-weight: 700;
    background: var(--accent); color: var(--accent-text)
  }
</style>
