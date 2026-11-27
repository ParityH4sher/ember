<!-- Rendered Markdown note with vault-local links. Shared by the Notes page and the pinned window. -->
<script lang="ts">
  import { blocks } from './links';
  import type { Notes } from './api';

  let { path, notes, href, follow }: {
    path: string;
    notes: Notes;
    href: (dest: string) => string;
    follow: (event: MouseEvent, dest: string) => void;
  } = $props();
</script>

<article id="preview">
  {#each [...blocks(notes[path] ?? '', path, notes)] as [tag, parts]}
    <svelte:element this={tag}>{#each parts as part}{#if part.path}<a href={href(part.path)} onclick={e => follow(e, part.path!)}>{part.text}</a>{:else}{part.text}{/if}{/each}</svelte:element>
  {/each}
</article>
