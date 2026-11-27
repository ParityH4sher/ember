<!-- Contents of the Document Picture-in-Picture window: one note, read-only. -->
<script lang="ts">
  import NotePreview from './NotePreview.svelte';
  import type { Notes } from './api';

  let props: {
    path: string;
    notes: Notes;
    href: (dest: string) => string;
    follow: (event: MouseEvent, dest: string) => void;
  } = $props();
</script>

<div class="pinned">
  <strong>{props.path}</strong>
  <NotePreview {...props} />
</div>

<style>
  .pinned { padding: .8rem; }
  strong { display: block; margin-bottom: .5rem; overflow-wrap: anywhere; }
</style>
