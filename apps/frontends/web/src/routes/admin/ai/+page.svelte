<script lang="ts">
  import { onMount } from 'svelte';
  import Badge from '../../../lib/ui/Badge.svelte';
  import Card from '../../../lib/ui/Card.svelte';
  import PageHeader from '../../../lib/ui/PageHeader.svelte';
  import {
    deleteAgent, deleteModel, deleteProvider, discoverModels, getAICatalog, saveAgent, saveModel, saveProvider,
    type AICatalog, type CatalogAgent, type CatalogModel, type ModelProvider, type ProviderKind
  } from '../../../lib/api';
  import { failure, message, settle, type Loaded } from '../../../lib/load';
  import { auth } from '../../../lib/session.svelte';

  // Admins add model providers (with their keys), the models people may use
  // and the agents that run them. The API enforces the admin role and never
  // sends a key back.
  let catalog = $state.raw<Loaded<AICatalog>>();
  const reload = () => settle(getAICatalog(), v => (catalog = v));
  onMount(() => { if (auth.admin) reload(); });

  const kindLabel: Record<ProviderKind, string> = {
    anthropic: 'Anthropic (Claude)', openai: 'OpenAI (GPT, Codex)', ollama: 'Ollama', 'openai-compatible': 'OpenAI-compatible server'
  };
  const urlHint: Record<ProviderKind, string> = {
    anthropic: 'https://api.anthropic.com/v1 (leave empty)', openai: 'https://api.openai.com/v1 (leave empty)',
    ollama: 'http://ollama.lan:11434', 'openai-compatible': 'http://llm.lan:8000/v1'
  };
  const harnessLabel: Record<string, string> = {
    'claude-code': 'Claude Code', codex: 'Codex CLI', opencode: 'OpenCode', custom: 'Custom image'
  };
  const hosted = (k: ProviderKind) => k === 'anthropic' || k === 'openai';

  /** Suggests an ID from a name: lowercase letters, digits and hyphens. */
  function slug(name: string): string {
    const s = name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63);
    return /^[a-z]/.test(s) ? s : s ? `x-${s}`.slice(0, 63) : '';
  }

  // --- Providers -----------------------------------------------------------
  const blankProvider = () => ({ id: '', name: '', kind: 'ollama' as ProviderKind, baseURL: '', local: true, apiKey: '', clearApiKey: false });
  let provider = $state(blankProvider());
  let editingProvider = $state<ModelProvider | null>(null);
  let providerStatus = $state('');
  let discovered = $state<{ provider: string; models: string[] } | null>(null);
  let discoverStatus = $state('');

  function editProvider(p: ModelProvider) {
    editingProvider = p;
    provider = { id: p.id, name: p.name, kind: p.kind, baseURL: p.baseURL, local: p.local, apiKey: '', clearApiKey: false };
    providerStatus = '';
  }
  function newProvider() { editingProvider = null; provider = blankProvider(); providerStatus = ''; }

  async function submitProvider(event: SubmitEvent) {
    event.preventDefault();
    const id = editingProvider?.id ?? (provider.id || slug(provider.name));
    providerStatus = 'Saving…';
    try {
      const saved = await saveProvider(id, {
        name: provider.name.trim(), kind: provider.kind, baseURL: provider.baseURL.trim(),
        local: hosted(provider.kind) ? false : provider.local,
        ...(provider.apiKey && { apiKey: provider.apiKey }),
        ...(provider.clearApiKey && { clearApiKey: true })
      });
      providerStatus = `Saved ${saved.name}.` + (editingProvider?.hasAPIKey && !saved.hasAPIKey && !provider.clearApiKey
        ? ' The address changed, so the stored key was removed: enter it again.' : '');
      editingProvider = null;
      provider = blankProvider();
      reload();
    } catch (error) { providerStatus = message(error); }
  }

  async function removeProvider(p: ModelProvider) {
    if (!confirm(`Delete provider “${p.name}” and its stored key?`)) return;
    try { await deleteProvider(p.id); providerStatus = `Deleted ${p.name}.`; reload(); }
    catch (error) { providerStatus = message(error); }
  }

  async function discover(p: ModelProvider) {
    discoverStatus = `Asking ${p.name} for its models…`;
    discovered = null;
    try {
      const models = await discoverModels(p.id);
      discovered = { provider: p.id, models };
      discoverStatus = models.length ? `${p.name} offers ${models.length} model${models.length === 1 ? '' : 's'}.` : `${p.name} reported no models.`;
    } catch (error) { discoverStatus = message(error); }
  }

  // --- Models --------------------------------------------------------------
  const blankModel = () => ({ id: '', name: '', provider: '', upstream: '' });
  let model = $state(blankModel());
  let editingModel = $state<CatalogModel | null>(null);
  let modelStatus = $state('');
  let modelForm = $state<HTMLElement>();

  function editModel(m: CatalogModel) { editingModel = m; model = { ...m }; modelStatus = ''; }
  function newModel() { editingModel = null; model = blankModel(); modelStatus = ''; }
  function useDiscovered(providerId: string, upstream: string) {
    editingModel = null;
    model = { id: slug(upstream), name: upstream, provider: providerId, upstream };
    modelStatus = '';
    modelForm?.scrollIntoView({ block: 'nearest' });
  }

  async function submitModel(event: SubmitEvent) {
    event.preventDefault();
    const id = editingModel?.id ?? (model.id || slug(model.name));
    modelStatus = 'Saving…';
    try {
      const saved = await saveModel(id, { name: model.name.trim(), provider: model.provider, upstream: model.upstream.trim() });
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
  const blankAgent = () => ({ id: '', name: '', harness: 'claude-code', routerAgent: '', models: [] as string[], defaultModel: '', enabled: true });
  let agent = $state(blankAgent());
  let editingAgent = $state<CatalogAgent | null>(null);
  let agentStatus = $state('');

  function editAgent(a: CatalogAgent) {
    editingAgent = a;
    agent = { ...a, models: [...a.models], defaultModel: a.defaultModel ?? '' };
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
        name: agent.name.trim(), harness: agent.harness, routerAgent: agent.routerAgent.trim(),
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

  const providerName = (c: AICatalog, id: string) => c.providers.find(p => p.id === id)?.name ?? id;
  const modelName = (c: AICatalog, id?: string) => (id && c.models.find(m => m.id === id)?.name) || id || '—';
</script>

<PageHeader title="Agents & models"
  lede="Connect model providers such as Claude, OpenAI/Codex and Ollama, choose the models people may use, and add the agents that run them." />

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
    <p class="note" role="note">Project chat is off on this instance: the operator has not connected the sandbox router. You can prepare
      providers, models and agents now; agents appear in chat once it is on.</p>
  {/if}

  <!-- Providers -->
  <Card title="Model providers" id="providers">
    <p class="muted">Keys are stored by the Ember API only. They are never shown again and never handed to an agent.</p>
    {#if c.providers.length}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Provider</th><th scope="col">Kind</th><th scope="col">Address</th><th scope="col">Status</th><th scope="col"><span class="sr">Actions</span></th></tr></thead>
          <tbody>
            {#each c.providers as p (p.id)}
              <tr>
                <td>{p.name} <span class="muted">{p.id}</span></td>
                <td>{kindLabel[p.kind] ?? p.kind}</td>
                <td class="url">{p.baseURL}</td>
                <td>
                  <Badge tone={p.local ? 'ok' : 'neutral'}>{p.local ? 'local' : 'hosted'}</Badge>
                  {#if p.hasAPIKey}<Badge tone="ok">key set</Badge>{:else if hosted(p.kind)}<Badge tone="warn">needs key</Badge>{/if}
                </td>
                <td class="actions">
                  <button type="button" onclick={() => discover(p)}>Fetch models</button>
                  <button type="button" onclick={() => editProvider(p)}>Edit</button>
                  <button type="button" class="danger" onclick={() => removeProvider(p)}>Delete</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="muted">No providers yet.</p>
    {/if}

    {#if discoverStatus}<p role="status">{discoverStatus}</p>{/if}
    {#if discovered?.models.length}
      <ul class="chips" aria-label="Models offered by the provider">
        {#each discovered.models as upstream (upstream)}
          {@const added = c.models.some(m => m.provider === discovered?.provider && m.upstream === upstream)}
          <li><button type="button" disabled={added} onclick={() => useDiscovered(discovered!.provider, upstream)}>
            {upstream}{added ? ' ✓' : ' +'}</button></li>
        {/each}
      </ul>
    {/if}

    <form class="editor" onsubmit={submitProvider} aria-labelledby="provider-form-title">
      <h3 id="provider-form-title">{editingProvider ? `Edit ${editingProvider.name}` : 'Add a provider'}</h3>
      <div class="fields">
        <label>Name <input required maxlength="80" bind:value={provider.name} placeholder="Claude, Home Ollama…" /></label>
        <label>Kind
          <select bind:value={provider.kind} onchange={() => { provider.local = provider.kind === 'ollama'; }}>
            {#each c.providerKinds as k (k)}<option value={k}>{kindLabel[k] ?? k}</option>{/each}
          </select>
        </label>
        {#if !editingProvider}
          <label>ID <input maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={provider.id} placeholder={slug(provider.name) || 'from the name'} /></label>
        {/if}
        <label>Base URL <input type="url" bind:value={provider.baseURL} required={!hosted(provider.kind)} placeholder={urlHint[provider.kind]} /></label>
        <label>API key
          <input type="password" autocomplete="off" bind:value={provider.apiKey} disabled={provider.clearApiKey}
            placeholder={editingProvider?.hasAPIKey ? 'Stored; type to replace' : provider.kind === 'ollama' ? 'Usually none' : 'Paste the key'} />
        </label>
      </div>
      {#if !hosted(provider.kind)}
        <label class="check"><input type="checkbox" bind:checked={provider.local} /> Runs on my network (allows plain http)</label>
      {/if}
      {#if editingProvider?.hasAPIKey}
        <label class="check"><input type="checkbox" bind:checked={provider.clearApiKey} /> Remove the stored key</label>
      {/if}
      {#if hosted(provider.kind)}
        <p class="muted small">Use an API key from the provider's console. Claude and ChatGPT subscriptions sign in inside the
          agent's harness instead; check that your plan allows that before relying on it.</p>
      {/if}
      <div class="buttons">
        <button class="primary">{editingProvider ? 'Save provider' : 'Add provider'}</button>
        {#if editingProvider}<button type="button" onclick={newProvider}>Cancel</button>{/if}
      </div>
      {#if providerStatus}<p role="status">{providerStatus}</p>{/if}
    </form>
  </Card>

  <!-- Models -->
  <Card title="Models" id="models">
    <p class="muted">Each model is an alias people and agents see, pointing at one model of a provider.</p>
    {#if c.models.length}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Model</th><th scope="col">Provider</th><th scope="col">Provider's model ID</th><th scope="col"><span class="sr">Actions</span></th></tr></thead>
          <tbody>
            {#each c.models as m (m.id)}
              <tr>
                <td>{m.name} <span class="muted">{m.id}</span></td>
                <td>{providerName(c, m.provider)}</td>
                <td><code>{m.upstream}</code></td>
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
      <p class="muted">No models yet. Add a provider, then use <strong>Fetch models</strong> or type the provider's model ID.</p>
    {/if}

    <form class="editor" onsubmit={submitModel} bind:this={modelForm} aria-labelledby="model-form-title">
      <h3 id="model-form-title">{editingModel ? `Edit ${editingModel.name}` : 'Add a model'}</h3>
      <div class="fields">
        <label>Name <input required maxlength="80" bind:value={model.name} placeholder="Claude Sonnet, Qwen 3 8B…" /></label>
        <div>
          <label for="model-provider">Provider</label>
          <select id="model-provider" required bind:value={model.provider}>
            <option value="" disabled>Choose…</option>
            {#each c.providers as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </div>
        <label>Provider's model ID <input required maxlength="200" bind:value={model.upstream} placeholder="qwen3:8b" list="discovered-models" /></label>
        {#if !editingModel}
          <label>ID <input maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={model.id} placeholder={slug(model.name) || 'from the name'} /></label>
        {/if}
      </div>
      <datalist id="discovered-models">
        {#each discovered?.provider === model.provider ? discovered.models : [] as d (d)}<option value={d}></option>{/each}
      </datalist>
      <div class="buttons">
        <button class="primary" disabled={!c.providers.length}>{editingModel ? 'Save model' : 'Add model'}</button>
        {#if editingModel}<button type="button" onclick={newModel}>Cancel</button>{/if}
      </div>
      {#if modelStatus}<p role="status">{modelStatus}</p>{/if}
    </form>
  </Card>

  <!-- Agents -->
  <Card title="Agents" id="agents">
    <p class="muted">An agent is a harness image the operator registered in the sandbox router (for example Claude Code or Codex),
      with the models it may use. Each chat message names the agent's default model to it.</p>
    {#if c.operatorAgents.length}
      <p class="small">From the operator's agents file (edit on the server):
        {#each c.operatorAgents as a, i (a.id)}{i ? ', ' : ''}<strong>{a.name}</strong> <span class="muted">{a.id}</span>{/each}</p>
    {/if}
    {#if c.agents.length}
      <div class="table">
        <table>
          <thead><tr><th scope="col">Agent</th><th scope="col">Harness</th><th scope="col">Router agent</th><th scope="col">Models</th><th scope="col">Status</th><th scope="col"><span class="sr">Actions</span></th></tr></thead>
          <tbody>
            {#each c.agents as a (a.id)}
              <tr>
                <td>{a.name} <span class="muted">{a.id}</span></td>
                <td>{harnessLabel[a.harness] ?? a.harness}</td>
                <td><code>{a.routerAgent}</code></td>
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
        <label>Router agent
          <input required maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={agent.routerAgent} placeholder="claude-code" />
        </label>
        {#if !editingAgent}
          <label>ID <input maxlength="63" pattern="[a-z][a-z0-9\-]*" bind:value={agent.id} placeholder={slug(agent.name) || 'from the name'} /></label>
        {/if}
      </div>
      <p class="muted small">The router agent is the name the image has in the sandbox router's policy file on the server.</p>
      <fieldset>
        <legend>Models this agent may use</legend>
        {#each c.models as m (m.id)}
          <label class="check"><input type="checkbox" value={m.id} bind:group={agent.models} /> {m.name}
            <span class="muted">{providerName(c, m.provider)}</span></label>
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
  :global(#providers), :global(#models) { margin-bottom: var(--space) }
  .table { overflow-x: auto; margin-bottom: .8rem }
  table { width: 100%; border-collapse: collapse }
  th, td { text-align: left; padding: .45rem .6rem; border-top: 1px solid var(--border); vertical-align: top }
  th { color: var(--muted); font-weight: 600; border-top: 0 }
  td .muted { font-size: .85rem }
  .url { overflow-wrap: anywhere; font-size: .9rem }
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
