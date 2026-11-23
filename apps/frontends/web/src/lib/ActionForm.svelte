<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, submit, disabled = false, children }: {
    title: string;
    /** Runs on submit; the result or the error message becomes the status. */
    submit: (form: HTMLFormElement) => Promise<string>;
    disabled?: boolean;
    children?: Snippet;
  } = $props();

  let working = $state(false), status = $state('');

  async function onsubmit(event: SubmitEvent & { currentTarget: HTMLFormElement }) {
    event.preventDefault();
    working = true;
    status = 'Working…';
    try { status = await submit(event.currentTarget); }
    catch (error) { status = error instanceof Error ? error.message : String(error); }
    finally { working = false; }
  }
</script>

<form {onsubmit}>
  {@render children?.()}
  <button disabled={disabled || working}>{title}</button>
  <pre role="status">{status}</pre>
</form>

<style>
  pre { white-space: pre-wrap; overflow-wrap: anywhere; margin: .4rem 0 }
  pre:empty { display: none }
</style>
