<script lang="ts">
  import { untrack } from 'svelte';
  import {
    getChat, interruptChat, listPermissions, openChatEvents, replyPermission, sendMessage, SignInRequired,
    type Agent, type Chat, type ChatEvent, type Permission, type PermissionDecision
  } from './api';
  import { message as errorMessage } from './load';

  // One project chat (ADR 015): the transcript, the agent's live text while it
  // runs, tool activity, permission requests, steering and stopping. Updates
  // arrive on the chat's event stream; a slow poll covers a broken stream.
  let { project, chat, agents, available, onupdate, onsignedout }: {
    project: string; chat: Chat; agents: Agent[];
    /** false when this Ember no longer offers the chat's agent; the transcript stays readable. */
    available: boolean;
    /** Called with every fresh copy of the chat. */
    onupdate: (chat: Chat) => void;
    onsignedout: () => void;
  } = $props();

  const POLL_MS = 5000;           // Fallback while the stream is down.
  const DISCONNECT_GRACE_MS = 2000; // The browser reconnects a dropped stream quickly; do not flash a notice.
  const LIVE_LIMIT = 16_000;      // Characters of live text kept on screen.
  const TOOL_LIMIT = 5;

  let permissions = $state.raw<Permission[]>([]);
  let live = $state(''), tools = $state.raw<{ id: number; label: string }[]>([]), toolCount = $state(0);
  let streaming = $state(false), stopped = false;
  let draft = $state(''), sending = $state(false), stopping = $state(false), replying = $state('');
  let status = $state(''), error = $state('');

  const agentName = (id: string) => agents.find(a => a.id === id)?.name ?? id;
  // A fresh copy of the same chat must not reopen the stream.
  let chatId = $derived(chat.id);
  let mode = $derived(chat.mode ?? agents.find(a => a.id === chat.agent)?.mode ?? 'turn');
  let running = $derived(chat.running ?? chat.messages.some(m => m.status === 'pending'));
  let canSteer = $derived(mode === 'session');
  let sendLabel = $derived(!running ? 'Send' : canSteer ? 'Send to running agent' : 'Waiting for answer…');

  const toolLabel: Record<string, string> = {
    'tool.called': 'Using a tool', 'tool.succeeded': 'Tool finished', 'tool.failed': 'Tool failed'
  };
  const runStatus: Record<string, string> = {
    'run.started': 'Agent is working.', 'run.succeeded': 'Agent finished.',
    'run.failed': 'Agent run failed.', 'run.interrupted': 'Agent run stopped.',
    'permission.asked': 'The agent asks for permission.'
  };

  function fail(e: unknown) {
    if (e instanceof SignInRequired) { stopped = true; onsignedout(); return; }
    error = errorMessage(e);
  }

  // Reloads the chat and its permission requests; drops answers for a chat
  // that is no longer shown.
  let loads = 0;
  async function refresh() {
    const proj = project, id = chat.id, n = ++loads;
    try {
      const [fresh, asked] = await Promise.all([getChat(proj, id), listPermissions(proj, id).catch(e => {
        if (e instanceof SignInRequired) throw e;
        return [] as Permission[]; // An older API without permissions.
      })]);
      if (n !== loads || proj !== project || id !== chat.id) return;
      permissions = asked;
      onupdate(fresh);
    } catch (e) { fail(e); }
  }

  function clearRun() { live = ''; tools = []; toolCount = 0; }

  function onEvent(event: ChatEvent) {
    if (event.type === 'text.delta') {
      live = (live + (event.delta ?? '')).slice(-LIVE_LIMIT);
      return;
    }
    if (event.type === 'run.started') clearRun();
    if (event.type in toolLabel) {
      toolCount++;
      tools = [...tools, { id: toolCount, label: toolLabel[event.type] + (event.delta ? `: ${event.delta}` : '') }]
        .slice(-TOOL_LIMIT);
    }
    // Keep the more specific result of a Stop or a permission reply from this page.
    if (event.type in runStatus && !(event.type === 'run.interrupted' && status === 'Stopped the agent.'))
      status = runStatus[event.type];
    if (['run.succeeded', 'run.failed', 'run.interrupted'].includes(event.type)) clearRun();
    void refresh();
  }

  // The event stream, reopened for each chat. connected, resync and every
  // event but text.delta reload the chat and its permissions.
  $effect(() => {
    const proj = project, id = chatId;
    stopped = false;
    clearRun();
    permissions = [];
    status = error = '';
    streaming = false;
    let grace: ReturnType<typeof setTimeout> | undefined;
    const close = openChatEvents(proj, id, {
      event: event => {
        clearTimeout(grace);
        grace = undefined;
        if (event.type === 'connected' || event.type === 'resync') {
          if (!streaming && status.startsWith('Live updates')) status = 'Live updates reconnected.';
          streaming = true;
          void refresh();
          return;
        }
        streaming = true;
        onEvent(event);
      },
      error: () => {
        if (stopped) { close(); return; }
        grace ??= setTimeout(() => {
          streaming = false;
          status = 'Live updates disconnected. Reconnecting…';
        }, DISCONNECT_GRACE_MS);
      }
    });
    untrack(refresh);
    return () => { clearTimeout(grace); close(); };
  });

  // Slow poll, only while the stream is down and a run may change the chat.
  $effect(() => {
    if (streaming || !running) return;
    const timer = setInterval(() => { if (!stopped) void refresh(); }, POLL_MS);
    return () => clearInterval(timer);
  });

  async function send() {
    const text = draft.trim();
    if (!text || sending || (running && !canSteer)) return;
    sending = true;
    error = '';
    try {
      const steering = running;
      onupdate(await sendMessage(project, chat.id, text));
      if (draft.trim() === text) draft = '';
      status = steering ? 'Message sent to the running agent.' : 'Message sent. Agent is working.';
    } catch (e) { fail(e); }
    finally { sending = false; }
  }

  async function stop() {
    if (stopping) return;
    stopping = true;
    error = '';
    try {
      status = await interruptChat(project, chat.id)
        ? 'Stopped the agent.' : 'The agent was not running, so nothing was stopped.';
      await refresh();
    } catch (e) { fail(e); }
    finally { stopping = false; }
  }

  async function reply(p: Permission, decision: PermissionDecision) {
    if (replying) return;
    replying = p.id;
    error = '';
    try {
      await replyPermission(project, chat.id, p.id, decision);
      permissions = permissions.filter(x => x.id !== p.id);
      status = decision === 'once' ? `Allowed ${p.action} once.` : `Rejected ${p.action}.`;
      await refresh();
    } catch (e) { fail(e); }
    finally { replying = ''; }
  }

  function composerKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); void send(); }
  }
</script>

<div class="toolbar">
  <strong id="chat-title">{chat.title || 'New chat'}</strong>
  <span class="agent">with {agentName(chat.agent)}</span>
  {#if running}
    <button id="stop" type="button" onclick={stop} disabled={stopping}>{stopping ? 'Stopping…' : 'Stop agent'}</button>
  {/if}
</div>
<p id="chat-status" class="status" role="status">{status}</p>
{#if error}<p id="chat-error" class="error" role="alert">{error}</p>{/if}

<ol id="transcript" aria-label="Chat messages">
  {#each chat.messages as m (m.id)}
    <li class={['msg', m.role, m.status]}>
      <span class="who">{m.role === 'user' ? 'You' : agentName(chat.agent)}</span>
      {#if m.status === 'pending'}
        {#if live}<p class="text live" id="live-text" aria-label="Live answer">{live}</p>{/if}
        <p class="working">{live ? 'Still writing…' : 'Agent is working…'}</p>
      {:else if m.status === 'failed'}<p class="error">{m.error}{#if m.run} <small>Run {m.run}</small>{/if}</p>
      {:else}<p class="text">{m.content}</p>{/if}
    </li>
  {:else}<li class="hint">Ask {agentName(chat.agent)} something about this project.{#if mode === 'turn'} Each message runs the agent once in a sandbox, without network access or project files.{/if}</li>{/each}
  {#if live && !chat.messages.some(m => m.status === 'pending')}
    <!-- Text from a run the transcript does not show yet. -->
    <li class="msg agent pending"><span class="who">{agentName(chat.agent)}</span>
      <p class="text live" id="live-text" aria-label="Live answer">{live}</p></li>
  {/if}
</ol>

{#if tools.length}
  <section class="activity" aria-labelledby="tool-activity-title">
    <h3 id="tool-activity-title">Tool activity <span class="muted">({toolCount} event{toolCount === 1 ? '' : 's'})</span></h3>
    <ul>{#each tools as t (t.id)}<li class={{ failed: t.label.startsWith('Tool failed') }}>{t.label}</li>{/each}</ul>
  </section>
{/if}

{#if permissions.length}
  <section class="permissions" aria-labelledby="permissions-title">
    <h3 id="permissions-title">The agent asks for permission</h3>
    <ul>
      {#each permissions as p (p.id)}
        <li>
          <p id="perm-{p.id}"><strong>{p.action}</strong>{#if p.resources.length}: <code>{p.resources.join(', ')}</code>{/if}</p>
          <div class="buttons">
            <button type="button" class="primary" aria-describedby="perm-{p.id}" disabled={!!replying}
              onclick={() => reply(p, 'once')}>Allow once</button>
            <button type="button" aria-describedby="perm-{p.id}" disabled={!!replying}
              onclick={() => reply(p, 'reject')}>Reject</button>
          </div>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if available}
  <form id="composer" onsubmit={(e) => { e.preventDefault(); void send(); }}>
    <textarea id="chat-input" aria-label="Message" aria-describedby="composer-hint"
      placeholder={running && canSteer ? 'Add to what the agent is doing… (Ctrl+Enter sends)' : 'Message the agent… (Ctrl+Enter sends)'}
      bind:value={draft} onkeydown={composerKey}></textarea>
    <span id="composer-hint" class="visually-hidden">
      {running && canSteer ? 'The agent is running; a message now steers it.' : running ? 'Wait for the answer before sending again.' : ''}
    </span>
    <button id="send" class="primary" disabled={sending || !draft.trim() || (running && !canSteer)}>{sendLabel}</button>
  </form>
{:else}
  <p class="hint">This chat's agent is not available on this Ember instance. The transcript stays readable.</p>
{/if}

<style>
  .toolbar {display:flex;align-items:center;gap:.5rem;padding:.5rem;border-radius:var(--radius);
    background:var(--surface);border:1px solid var(--border)}
  .toolbar strong {margin-right:auto;overflow-wrap:anywhere}
  .hint,.agent,.working,.msg .who {color:var(--muted)}
  .status {margin:.5rem 0 0;color:var(--muted)}
  .status:empty {display:none}
  .error {color:var(--danger)}
  #transcript {list-style:none;padding:0;margin:1rem 0;display:flex;flex-direction:column;gap:.6rem}
  .msg {padding:.5rem .7rem;border-radius:var(--radius);background:var(--surface);border:1px solid var(--border);max-width:52rem}
  .msg.user {background:var(--surface-2);align-self:flex-end}
  .msg .who {display:block;font-size:.8rem;font-weight:650}
  .msg p {margin:.2rem 0 0;white-space:pre-wrap;overflow-wrap:anywhere}
  .working {font-style:italic}
  h3 {font-size:1rem;margin:0 0 .3rem}
  .activity,.permissions {padding:.5rem .7rem;border-radius:var(--radius);border:1px solid var(--border);margin:0 0 1rem;max-width:52rem}
  .activity ul {margin:0;padding-left:1.2rem;font-size:.9rem}
  .activity li.failed {color:var(--danger)}
  .permissions {background:var(--warn-bg)}
  .permissions ul {list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:.5rem}
  .permissions p {margin:0;overflow-wrap:anywhere}
  .buttons {display:flex;gap:.5rem;margin-top:.3rem}
  #composer {display:flex;flex-direction:column;align-items:flex-end}
  #chat-input {height:6rem}
</style>
