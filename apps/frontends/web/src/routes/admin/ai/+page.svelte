<script lang="ts">
  import { onMount } from 'svelte';
  import Badge from '../../../lib/ui/Badge.svelte';
  import Card from '../../../lib/ui/Card.svelte';
  import PageHeader from '../../../lib/ui/PageHeader.svelte';
  import {
    deleteAgent, deleteModel, getAICatalog, listGatewayModels, saveAgent, saveModel,
    type AICatalog, type CatalogAgent, type CatalogModel
  } from '../../../lib/api';
  import { failure, message, settle, type Loaded } from '../../../lib/load';
  import { auth } from '../../../lib/session.svelte';

  // Admins offer the model gateway's models under a display name and add the
  // agents that use them (ADR 015). Providers and their keys are configured in
  // the gateway (LiteLLM), never here. The API enforces the admin role.
  let catalog = $state.raw<Loaded<AICatalog>>();
  let gatewayModels = $state.raw<Loaded<string[]>>();
  const loadGateway = () => { gatewayModels = undefined; settle(listGatewayModels(), m => (gatewayModels = m)); };
  const reload = () => settle(getAICatalog(), v => {
    catalog = v;
    if (!failure(v) && v.gateway.configured && gatewayModels === undefined) loadGateway();
  });
  onMount(() => { if (auth.admin) reload(); });

  const harnessLabel: Record<string, string> = {
    'claude-code': 'Claude Code', codex: 'Codex CLI', opencode: 'OpenCode', custom: 'Custom image'
  };
  /** Harnesses that run as long-lived sessions on a worker instead of as a router agent. */
  const onWorker = (harness: string) => harness === 'opencode';

  /** Suggests an ID from a name: lowercase letters, digits and hyphens. */
  function slug(name: string): string {
    const s = name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63);
    return /^[a-z]/.test(s) ? s : s ? `x-${s}`.slice(0, 63) : '';
  }

  // --- Models --------------------------------------------------------------
  const blankModel = () => ({ id: '', name: '' });
  let model = $state(blankModel());
  let editingModel = $state<CatalogModel | null>(null);
  let modelStatus = $state('');
  let modelForm = $state<HTMLElement>();
  let displayName = $state<HTMLInputElement>();

  function editModel(m: CatalogModel) { editingModel = m; model = { ...m }; modelStatus = ''; }
  function newModel() { editingModel = null; model = blankModel(); modelStatus = ''; }
  /** Prefills the form with a gateway model; the admin names it and adds it. */
  function offer(id: string) {
    editingModel = null;
    model = { id, name: id };
    modelStatus = '';
    modelForm?.scrollIntoView({ block: 'nearest' });
    displayName?.focus();
    displayName?.select();
  }

  async function submitModel(event: SubmitEvent) {
    event.preventDefault();
    const id = editingModel?.id ?? model.id.trim();
    modelStatus = 'Saving…';
    try {
      const saved = await saveModel(id, { name: model.name.trim() });
      modelStatus = `Saved ${saved.name}.`;
      editingModel = null;
      model = blankModel();
      reload();
    } catch (error) { modelStatus = message(error); }
  }

  async function removeModel(m: CatalogModel) {
    if (!confirm(`Delete model “${m.name}”?`)) return;
    try { await deleteModel(m.id); modelStatus = `Deleted ${m.name}.`; reload(); }
    catch (error) { modelStatus = message(error); }
  }

  // --- Agents --------------------------------------------------------------
  const blankAgent = () => ({
    id: '', name: '', harness: 'claude-code', routerAgent: '', worker: '', models: [] as string[], defaultModel: '', enabled: true
  });
  let agent = $state(blankAgent());
  let editingAgent = $state<CatalogAgent | null>(null);
  let agentStatus = $state('');

  function editAgent(a: CatalogAgent) {
    editingAgent = a;
    agent = { ...a, routerAgent: a.routerAgent ?? '', worker: a.worker ?? '', models: [...a.models], defaultModel: a.defaultModel ?? '' };
    agentStatus = '';
  }
  function newAgent() { editingAgent = null; agent = blankAgent(); agentStatus = ''; }

  async function submitAgent(event: SubmitEvent) {
    event.preventDefault();
    const id = editingAgent?.id ?? (agent.id || slug(agent.name));
    const defaultModel = agent.models.includes(agent.defaultModel) ? agent.defaultModel : (agent.models[0] ?? '');
    agentStatus = 'Saving…';
    try {
      const saved = await saveAgent(id, {
        name: agent.name.trim(), harness: agent.harness,
        // A worker agent has no router agent, and a router agent no worker.
        ...(onWorker(agent.harness) ? { worker: agent.worker } : { routerAgent: agent.routerAgent.trim() }),
        models: agent.models, defaultModel, enabled: agent.enabled
      });
      agentStatus = `Saved ${saved.name}.`;
      editingAgent = null;
      agent = blankAgent();
      reload();
    } catch (error) { agentStatus = message(error); }
  }

  async function removeAgent(a: CatalogAgent) {
    if (!confirm(`Delete agent “${a.name}”? Existing chats stay readable but cannot continue.`)) return;
    try { await deleteAgent(a.id); agentStatus = `Deleted ${a.name}.`; reload(); }
    catch (error) { agentStatus = message(error); }
  }

  const modelName = (c: AICatalog, id?: string) => (id && c.models.find(m => m.id === id)?.name) || id || '—';
  const workersFor = (c: AICatalog, harness: string) => c.workers.filter(w => w.harness === harness);
</script>

<PageHeader title="Agents & models"
  lede="Offer the model gateway's models under names people recognise, and add the agents that use them." />

<p><a href="/admin/">← Admin</a></p>

{#if !auth.admin}
  <Card title="Admins only" id="denied">
    <p>Only Ember admins can configure agents and models.</p>
  </Card>
{:else if catalog === undefined}
  <p class="muted">Loading…</p>
{:else if failure(catalog)}
  <p role="alert">Unable to load the agents and models: {catalog.error}</p>
{:else}
  {@const c = catalog}
  {#if !c.persisted}
    <p class="note" role="note">This API has no catalog file (<code>EMBER_AI_CATALOG_FILE</code>), so these settings reset when it restarts.</p>
  {/if}
  {#if !c.chatEnabled}
    <p class="note" role="note">Project chat is off on this instance: the operator has not connected a sandbox router or worker.
      You can prepare models and agents now; agents appear in chat once it is on.</p>
  {/if}

  <!-- Gateway models -->
  <Card title="Gateway models" id="gateway">
    {#if !c.gateway.configured}
      <p class="muted" id="gateway-note">Model providers (Claude, OpenAI, Ollama…) and their keys are configured in the LiteLLM
        model gateway, not in Ember; see <code>docs/models.md</code>. This instance has no gateway connected
        (<code>EMBER_MODEL_GATEWAY_URL</code>), so add models below by their gateway name.</p>
    {:else if gatewayModels === undefined}
      <p class="muted">Asking the gateway for its models…</p>
    {:else if failure(gatewayModels)}
      <p role="alert">Unable to list the gateway's models: {gatewayModels.error}</p>
      <button type="button" onclick={loadGateway}>Try again</button>
    {:else if gatewayModels.length}
      <p class="muted">Providers and keys are configured in the gateway (see <code>docs/models.md</code>). Offer one of its
        models to people and agents:</p>
      <ul class="chips" aria-label="Models the gateway offers">
        {#each gatewayModels as id (id)}
          {@const added = c.models.some(m => m.id === id)}
          <li><button type="button" disabled={added} aria-label={added ? `${id}, already offered` : `Offer ${id}`}
            onclick={() => offer(id)}>{id}{added ? ' ✓' : ' +'}</button></li>
        {/each}
      </ul>
    {:else}
      <p class="muted">The gateway reports no models. Add a provider in its configuration (see <code>docs/models.md</code>).</p>
    {/if}
  </Card>

  <!-- Models -->
  <Card title="Models" id="models">
    <p class="muted">Each model is a display name people and agents see for one gateway model.</p>
    {#if c.models.length}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Model</th><th scope="col">Gateway model</th><th scope="col"><span class="sr">Actions</span></th></tr></thead>
          <tbody>
            {#each c.models as m (m.id)}
              <tr>
                <td>{m.name}</td>
                <td><code>{m.id}</code></td>
                <td class="actions">
                  <button type="button" onclick={() => editModel(m)}>Edit</button>
                  <button type="button" class="danger" onclick={() => removeModel(m)}>Delete</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="muted">No models yet.{#if c.gateway.configured} Pick one of the gateway's models above.{/if}</p>
    {/if}

    <form class="editor" onsubmit={submitModel} bind:this={modelForm} aria-labelledby="model-form-title">
      <h3 id="model-form-title">{editingModel ? `Edit ${editingModel.name}` : 'Add a model'}</h3>
      <div class="fields">
        {#if editingModel}
          <p class="small">Gateway model <code>{editingModel.id}</code></p>
        {:else}
          <label>Gateway model <input required maxlength="200" bind:value={model.id} placeholder="claude-sonnet, qwen3-8b…"
            list="gateway-model-names" /></label>
        {/if}
        <label>Display name <input required maxlength="80" bind:value={model.name} bind:this={displayName}
          placeholder="Claude Sonnet, Qwen 3 8B…" /></label>
      </div>
      <datalist id="gateway-model-names">
        {#each Array.isArray(gatewayModels) ? gatewayModels : [] as d (d)}<option value={d}></option>{/each}
      </datalist>
      <div class="buttons">
        <button class="primary">{editingModel ? 'Save model' : 'Add model'}</button>
        {#if editingModel}<button type="button" onclick={newModel}>Cancel</button>{/if}
      </div>
      {#if modelStatus}<p role="status">{modelStatus}</p>{/if}
    </form>
  </Card>

  <!-- Agents -->
  <Card title="Agents" id="agents">
    <p class="muted">An agent is either a harness image the operator registered in the sandbox router (for example Claude Code
      or Codex), which runs once per message, or an OpenCode session on one of the operator's workers, which keeps working
      between messages and can be steered or stopped.</p>
    {#if c.operatorAgents.length}
      <p class="small">From the operator's agents file (edit on the server):
        {#each c.operatorAgents as a, i (a.id)}{i ? ', ' : ''}<strong>{a.name}</strong> <span class="muted">{a.id}</span>{/each}</p>
    {/if}
    {#if c.agents.length}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Agent</th><th scope="col">Harness</th><th scope="col">Runs on</th><th scope="col">Models</th><th scope="col">Status</th><th scope="col"><span class="sr">Actions</span></th></tr></thead>
          <tbody>
            {#each c.agents as a (a.id)}
              <tr>
                <td>{a.name} <span class="muted">{a.id}</span></td>
                <td>{harnessLabel[a.harness] ?? a.harness}</td>
                <td>{#if a.worker}worker <code>{a.worker}</code>{:else}router agent <code>{a.routerAgent}</code>{/if}</td>
                <td>{#if a.models.length}<strong>{modelName(c, a.defaultModel)}</strong>{#if a.models.length > 1}<span class="muted"> +{a.models.length - 1}</span>{/if}{:else}<span class="muted">none</span>{/if}</td>
                <td><Badge tone={a.enabled ? 'ok' : 'neutral'}>{a.enabled ? 'on' : 'off'}</Badge></td>
                <td class="actions">
                  <button type="button" onclick={() => editAgent(a)}>Edit</button>
                  <button type="button" class="danger" onclick={() => removeAgent(a)}>Delete</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="muted">No agents added here yet.</p>
    {/if}

    <form class="editor" onsubmit={submitAgent} aria-labelledby="agent-form-title">
      <h3 id="agent-form-title">{editingAgent ? `Edit ${editingAgent.name}` : 'Add an agent'}</h3>
      <div class="fields">
        <label>Name <input required maxlength="80" bind:value={agent.name} placeholder="Claude Code, Codex…" /></label>
        <label>Harness
          <select bind:value={agent.harness}>
            {#each c.harnesses as h (h)}<option value={h}>{harnessLabel[h] ?? h}</option>{/each}
          </select>
        </label>
        {#if onWorker(agent.harness)}
          {@const workers = workersFor(c, agent.harness)}
          <label>Worker
            <select required bind:value={agent.worker} aria-describedby="agent-runs-hint">
              <option value="" disabled>{workers.length ? 'Choose…' : 'No worker configured'}</option>
              {#each workers as w (w.id)}<option value={w.id}>{w.id}</option>{/each}
            </select>
          </label>
        {:else}
          <label>Router agent
            <input required maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={agent.routerAgent} placeholder="claude-code"
              aria-describedby="agent-runs-hint" />
          </label>
        {/if}
        {#if !editingAgent}
          <label>ID <input maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={agent.id} placeholder={slug(agent.name) || 'from the name'} /></label>
        {/if}
      </div>
      <p class="muted small" id="agent-runs-hint">
        {#if onWorker(agent.harness)}A worker is an OpenCode server from the operator's agents file; its own
          <code>opencode.json</code> sets the model it uses.
        {:else}The router agent is the name the image has in the sandbox router's policy file on the server.{/if}
      </p>
      <fieldset>
        <legend>Models this agent may use</legend>
        {#each c.models as m (m.id)}
          <label class="check"><input type="checkbox" value={m.id} bind:group={agent.models} /> {m.name}
            <span class="muted">{m.id}</span></label>
        {:else}
          <p class="muted small">No models yet: an agent without one must bring its own.</p>
        {/each}
      </fieldset>
      {#if agent.models.length > 1}
        <label>Default model
          <select bind:value={agent.defaultModel}>
            {#each agent.models as id (id)}<option value={id}>{modelName(c, id)}</option>{/each}
          </select>
        </label>
      {/if}
      <label class="check"><input type="checkbox" bind:checked={agent.enabled} /> Offer this agent in project chat</label>
      <div class="buttons">
        <button class="primary">{editingAgent ? 'Save agent' : 'Add agent'}</button>
        {#if editingAgent}<button type="button" onclick={newAgent}>Cancel</button>{/if}
      </div>
      {#if agentStatus}<p role="status">{agentStatus}</p>{/if}
    </form>
  </Card>
{/if}

<style>
  .note { padding: .6rem .9rem; border-radius: var(--radius); background: var(--warn-bg); margin: 0 0 var(--space) }
  :global(#gateway), :global(#models) { margin-bottom: var(--space) }
  .table { overflow-x: auto; margin-bottom: .8rem }
  table { width: 100%; border-collapse: collapse }
  th, td { text-align: left; padding: .45rem .6rem; border-top: 1px solid var(--border); vertical-align: top }
  th { color: var(--muted); font-weight: 600; border-top: 0 }
  td .muted { font-size: .85rem }
  .actions { white-space: nowrap; text-align: right }
  .actions button { margin-left: .3rem }
  .danger { color: var(--danger) }
  .chips { display: flex; flex-wrap: wrap; gap: .4rem; list-style: none; padding: 0; margin: 0 0 .8rem }
  .chips button { font-size: .85rem; padding: .15rem .55rem }
  .editor { border-top: 1px solid var(--border); padding-top: .8rem }
  h3 { font-size: 1rem; margin: 0 0 .4rem }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr)); gap: 0 var(--space) }
  label.check { display: flex; align-items: center; font-weight: 400; margin: .3rem 0; gap: .3rem }
  fieldset { border: 1px solid var(--border); border-radius: var(--radius); margin: .6rem 0; padding: .4rem .8rem }
  legend { font-weight: 650; padding: 0 .3rem }
  .buttons { display: flex; gap: .5rem; margin-top: .6rem }
  .small { font-size: .85rem }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap }
</style>
