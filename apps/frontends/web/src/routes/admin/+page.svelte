<script lang="ts">
  import { onMount } from 'svelte';
  import Badge from '../../lib/ui/Badge.svelte';
  import Card from '../../lib/ui/Card.svelte';
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import ServiceLinks from '../../lib/ServiceLinks.svelte';
  import { getHealth, listAgents, listPlugins, type AgentList, type Health, type Plugin } from '../../lib/api';
  import { failure, settle, type Loaded } from '../../lib/load';
  import { auth } from '../../lib/session.svelte';

  // Read-only overview of the instance for admins. The API enforces roles;
  // this page only hides what a user could not change anyway.
  let health = $state.raw<Loaded<Health>>(), plugins = $state.raw<Loaded<Plugin[]>>();
  let agents = $state.raw<Loaded<AgentList>>();

  onMount(() => {
    if (!auth.admin) return;
    settle(getHealth(), v => (health = v));
    settle(listPlugins().then(r => r.plugins), v => (plugins = v));
    settle(listAgents(), v => (agents = v));
  });
</script>

<PageHeader title="Admin" lede="This Ember instance: status, plugins, agents and who may sign in." />

{#if !auth.admin}
  <Card title="Admins only" id="denied">
    <p>Your role is <strong>user</strong>. Ask an Ember admin to add you to an admin group in
      {auth.providerName}, then sign in again.</p>
    <p><a href="/">Back to the dashboard</a></p>
  </Card>
{:else}
  <div class="grid">
    <Card title="Instance" id="instance">
      <dl>
        <dt>Core API</dt>
        <dd>
          {#if health === undefined}<span class="muted">checking…</span>
          {:else if failure(health)}<Badge tone="danger">unavailable</Badge> <span class="muted">{health.error}</span>
          {:else}<Badge tone="ok">{health.status}</Badge> version {health.version}{/if}
        </dd>
        <dt>Sign-in</dt>
        <dd>{#if auth.session?.enabled}<Badge tone="ok">OpenID Connect</Badge>{:else}<Badge tone="warn">off</Badge>{/if}</dd>
        <dt>Project chat</dt>
        <dd>
          {#if agents === undefined}<span class="muted">checking…</span>
          {:else if failure(agents)}<span class="muted">{agents.error}</span>
          {:else if agents.enabled}<Badge tone="ok">on</Badge> {agents.agents.length} agent{agents.agents.length === 1 ? '' : 's'}
          {:else}<Badge>off</Badge>{/if}
        </dd>
      </dl>
      {#if !auth.session?.enabled}
        <p class="warn">Sign-in is off, so every visitor is an admin. Keep this instance on loopback or a trusted network.</p>
      {/if}
    </Card>

    <Card title="Access" id="access">
      <p>Ember has no user directory. People and groups live in {auth.session?.enabled ? auth.providerName : 'the identity provider'}
        (in Authentik: <strong>Directory → Users</strong> and <strong>Groups</strong>):</p>
      <ul>
        <li>Members of the configured <code>adminGroups</code> get the <strong>admin</strong> role.</li>
        <li>Members of <code>allowedGroups</code> (or everyone, if unset) get the <strong>user</strong> role.</li>
        <li>Role changes apply at the next sign-in. Sessions are not renewed, so a removed admin keeps the role until their session expires.</li>
      </ul>
    </Card>
  </div>

  <Card title="Plugins" id="plugins">
    {#snippet actions()}<a href="/plugins/">Configure →</a>{/snippet}
    {#if plugins === undefined}
      <p class="muted">Loading plugins…</p>
    {:else if failure(plugins)}
      <p role="alert">Unable to load plugins: {plugins.error}</p>
    {:else if !plugins.length}
      <p class="muted">No plugins are installed.</p>
    {:else}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Plugin</th><th scope="col">Version</th><th scope="col">Status</th><th scope="col">Capabilities</th></tr></thead>
          <tbody>
            {#each plugins as p (p.id)}
              <tr>
                <td>{p.name} <span class="muted">{p.id}</span></td>
                <td>{p.version}</td>
                <td>
                  <Badge tone={p.active ? 'ok' : 'neutral'}>{p.active ? 'active' : 'inactive'}</Badge>
                  {#if !p.configured}<Badge tone="warn">needs configuration</Badge>{/if}
                </td>
                <td>{p.capabilities.join(', ') || '—'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Card>

  <div class="grid">
    <Card title="Agents" id="agents">
      {#snippet actions()}<a href="/admin/ai/">Agents & models →</a>{/snippet}
      {#if agents === undefined}
        <p class="muted">Loading agents…</p>
      {:else if failure(agents)}
        <p role="alert">Unable to load agents: {agents.error}</p>
      {:else}
        <ul>
          {#each agents.agents as a (a.id)}<li>{a.name} <span class="muted">{a.id}{a.model ? ` · ${a.model}` : ''}</span></li>
          {:else}<li class="muted">No agents yet. Add one under Agents & models.</li>{/each}
        </ul>
      {/if}
    </Card>
    <Card title="Shared services" id="services"><ServiceLinks /></Card>
  </div>
{/if}

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr)); gap: var(--space); align-items: start; margin-bottom: var(--space) }
  :global(#plugins) { margin-bottom: var(--space) }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: .5rem 1rem; margin: 0 }
  dt { color: var(--muted) }
  dd { margin: 0; overflow-wrap: anywhere }
  .warn { color: var(--warn) }
  ul { margin: 0; padding-left: 1.2rem }
  .table { overflow-x: auto }
  table { width: 100%; border-collapse: collapse }
  th, td { text-align: left; padding: .45rem .6rem; border-top: 1px solid var(--border); vertical-align: top }
  th { color: var(--muted); font-weight: 600; border-top: 0 }
  td .muted { font-size: .85rem }
</style>
