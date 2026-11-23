<script lang="ts">
  import ActionForm from './ActionForm.svelte';
  import SchemaFields from './SchemaFields.svelte';
  import {
    configurePlugin, getPlugin, invokeOperation, listPluginOperations, operationPath, setPluginActivation,
    type Plugin, type PluginCatalog
  } from './api';
  import { readFields } from './schema';

  // Admins get settings and activation controls; users only run operations.
  // The API enforces roles; this only hides controls it would refuse.
  let { plugin, admin, onchange }: {
    plugin: Plugin; admin: boolean; onchange: (plugin: Plugin) => void
  } = $props();

  let catalog = $state.raw<PluginCatalog>(), catalogError = $state(''), loading = false;

  async function saveSettings(form: HTMLFormElement) {
    await configurePlugin(plugin.id, readFields(plugin.configurationSchema, form));
    onchange(await getPlugin(plugin.id));
    return 'Saved';
  }
  async function toggle() {
    onchange(await setPluginActivation(plugin.id, !plugin.active));
    return 'Updated';
  }
  // The catalog is loaded the first time the list is opened.
  async function loadOperations(event: Event & { currentTarget: HTMLDetailsElement }) {
    if (!event.currentTarget.open || loading) return;
    loading = true;
    catalogError = '';
    try { catalog = await listPluginOperations(plugin.id); }
    catch (error) { catalogError = error instanceof Error ? error.message : String(error); loading = false; }
  }
</script>

<!-- A changed plugin re-renders the card, which resets its forms. -->
{#key plugin}
  <article>
    <h3>{plugin.name}</h3>
    {#if plugin.description}<p>{plugin.description}</p>{/if}
    <p>{plugin.active ? 'Active' : 'Inactive'} · {plugin.configured ? 'Configured' : 'Needs configuration'} · {plugin.version}</p>
    {#if admin}
      <details>
        <summary>Settings</summary>
        <ActionForm title="Save settings" submit={saveSettings}>
          <SchemaFields schema={plugin.configurationSchema} values={plugin.configuration} configured={plugin.configured} />
        </ActionForm>
      </details>
      <ActionForm title={plugin.active ? 'Deactivate' : 'Activate'} disabled={!plugin.configured} submit={toggle} />
    {:else if !plugin.active}
      <p>An Ember admin must configure and activate this plugin before you can use it.</p>
    {/if}
    <details ontoggle={loadOperations}>
      <summary>Operations</summary>
      {#if catalogError}
        <p>{catalogError}</p>
      {:else if !catalog}
        <p>Loading…</p>
      {:else}
        {#each catalog.operations ?? [] as op}
          {@const path = operationPath(plugin.id, catalog.apiVersion, op.operationId)}
          <h4>{op.name}</h4>
          {#if op.description}<p>{op.description}</p>{/if}
          <!-- Use only the expected same-origin route, never a catalog-supplied destination. -->
          {#if op.method !== 'POST' || path !== op.path}
            <p>Unsupported operation route.</p>
          {:else}
            <ActionForm title="Run {op.name}" disabled={!plugin.active} submit={async form =>
              JSON.stringify((await invokeOperation(path, readFields(op.inputSchema, form))).data, null, 2) ?? ''}>
              <SchemaFields schema={op.inputSchema} />
            </ActionForm>
          {/if}
        {:else}
          <p>No operations advertised.</p>
        {/each}
      {/if}
    </details>
  </article>
{/key}

<style>
  article { border-top: 1px solid #34404c; padding: 1rem 0 }
  h3 { margin: 0 0 .4rem }
  details { margin: .8rem 0 }
  summary { cursor: pointer }
</style>
