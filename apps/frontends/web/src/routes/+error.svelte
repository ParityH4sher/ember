<script lang="ts">
  import { page } from '$app/state';
  import Card from '../lib/ui/Card.svelte';
  import PageHeader from '../lib/ui/PageHeader.svelte';

  // Rendered inside the shell for unknown paths (the web container serves the
  // 404.html fallback for them) and for errors while loading a page.
  let missing = $derived(page.status === 404);
</script>

<PageHeader title={missing ? 'Page not found' : 'Something went wrong'} />
<Card>
  {#if missing}
    <p>There is no page at <code>{page.url.pathname}</code>. It may have moved, or the link is mistyped.</p>
  {:else}
    <p>Ember could not show this page ({page.status}){page.error?.message ? `: ${page.error.message}` : '.'}</p>
  {/if}
  <p><a href="/">Go to the dashboard</a></p>
</Card>
