<script lang="ts">
  import { onMount } from 'svelte';
  import { beforeNavigate, replaceState } from '$app/navigation';
  import {
    createChat, createProject, getChat, listAgents, listChats, listProjects, loadNotes, saveNote,
    SignInRequired,
    type Agent, type Chat, type ChatSummary, type Notes, type Project
  } from '../../../lib/api';
  import { blocks } from '../../../lib/links';
  import { setLeaveGuard } from '../../../lib/session.svelte';
  import { DisplayManager } from '../../../lib/display-manager.svelte';
  import DisplayOffer from '../../../lib/DisplayOffer.svelte';
  import DisplaySettings from '../../../lib/DisplaySettings.svelte';
  import NotePreview from '../../../lib/NotePreview.svelte';
  import PinnedNote from '../../../lib/PinnedNote.svelte';
  import ProjectChat from '../../../lib/ProjectChat.svelte';
  import { pinSupported, pinToTop } from '../../../lib/pin';

  let projects = $state<Project[]>([]), project = $state(''), path = $state('');
  let notes = $state.raw<Notes>(Object.create(null));
  let editing = $state(false), busy = $state(false), offline = $state(false);
  let draft = $state(''), filter = $state(''), message = $state(''), retry = $state(false);
  // The layout signs people in first; this turns true when the session ends while
  // the page is open (the shell then offers to sign in again).
  let signedOut = $state(false);
  let dirty = $derived(editing && draft !== (notes[path] ?? ''));
  let paths = $derived(Object.keys(notes).sort());
  let references = $derived(paths.filter(p => p !== path &&
    [...blocks(notes[p] ?? '', p, notes)].some(([,parts]) => parts.some(t => t.path === path))));

  // Project chat (lib/ProjectChat.svelte): a turn agent runs once per message
  // in the sandbox router; a session agent is one harness session per chat.
  let agents = $state.raw<Agent[]>([]), agentsEnabled = $state(false), agentChoice = $state('');
  let chats = $state.raw<ChatSummary[]>([]), chat = $state.raw<Chat | null>(null);

  const query = (dest: string) => '?' + new URLSearchParams({project, note: dest});
  const chatQuery = (id: string) => '?' + new URLSearchParams({project, chat: id});
  const leave = () => !dirty || confirm('Discard unsaved changes?');
  const href = (dest: string) => new URL(query(dest), location.href).href;
  // Multi-display: this Notes window opens a workspace on each other display.
  const displays = new DisplayManager();

  // "Pin to top": the pinned window shows one note and follows saves to it.
  let canPin = $state(false);
  let pinned = $state<{path: string; notes: Notes; href: typeof href; follow: typeof follow} | null>(null);
  $effect(() => { if (pinned) pinned.notes = notes; });
  async function pin() {
    const props = $state({path, notes, href, follow});
    await pinToTop(PinnedNote, props, `${path} · Ember`, () => { if (pinned === props) pinned = null; });
    pinned = props;
  }

  async function action(fn: () => Promise<void>) {
    if (busy) return;
    busy = true;
    try { await fn(); } catch (error) {
      if (error instanceof SignInRequired) {
        signedOut = true; retry = false; message = '';
      }
      else message = error instanceof Error ? error.message : String(error);
    }
    finally { busy = false; }
  }
  // Sign-in and sign-out are full-page navigations through the API; the user
  // menu asks this guard first. Once the visitor agrees to discard a draft,
  // close the editor so that leaving does not ask again.
  function confirmLeave() {
    if (!leave()) return false;
    editing = false;
    return true;
  }
  // Covers links to other pages of the app, back/forward, and unloading the
  // page; for an unload the browser asks with its own dialog.
  beforeNavigate(({ cancel, type }) => {
    if (!dirty) return;
    if (type === 'leave') cancel();
    else if (!confirm('Discard unsaved changes?')) cancel();
  });
  function openNote(dest: string) {
    chat = null;
    path = dest;
    replaceState(query(dest), {});
    editing = !Object.hasOwn(notes, path);
    draft = notes[path] ?? '';
    message = '';
  }
  function follow(e: MouseEvent, dest: string) {
    if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    if (leave()) openNote(dest);
  }
  const summary = (c: Chat): ChatSummary => ({
    id: c.id, title: c.title, agent: c.agent, created: c.created, updated: c.updated,
    messageCount: c.messages.length, pending: c.running ?? c.messages.some(m => m.status === 'pending')
  });
  function showChat(c: Chat) {
    chat = c;
    chats = [summary(c), ...chats.filter(s => s.id !== c.id)];
  }
  async function openChat(id: string) {
    const loaded = await getChat(project, id);
    editing = false;
    path = '';
    replaceState(chatQuery(id), {});
    message = '';
    chat = loaded;
  }
  function followChat(e: MouseEvent, id: string) {
    if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    if (leave()) action(() => openChat(id));
  }
  async function loadChats(id: string) {
    try { chats = (await listChats(id)).chats; }
    catch (error) { if (error instanceof SignInRequired) throw error; chats = []; }
  }
  async function select(id: string, dest = 'index.md', chatId: string | null = null) {
    const data = await loadNotes(id);
    project = id;
    notes = Object.assign(Object.create(null), data.notes);
    filter = '';
    await loadChats(id);
    if (chatId && chats.some(c => c.id === chatId)) await openChat(chatId);
    else openNote(dest);
  }
  async function loadAgents() {
    try {
      const list = await listAgents();
      agents = list.agents;
      agentsEnabled = list.enabled && list.agents.length > 0;
      agentChoice = list.agents[0]?.id ?? '';
    } catch (error) {
      if (error instanceof SignInRequired) throw error;
      agentsEnabled = false; // An older API without chat.
    }
  }
  async function load() {
    try {
      retry = false;
      projects = (await listProjects()).projects;
      await loadAgents();
      if (!projects.length) { message = 'Create a project to start your linked notes.'; return; }
      const params = new URLSearchParams(location.search);
      await select(projects.find(p => p.id === params.get('project'))?.id ?? projects[0].id,
        params.get('note') || 'index.md', params.get('chat'));
    } catch (error) { retry = true; throw error; }
  }
  async function create() {
    if (!leave()) return;
    const name = prompt('Project name:')?.trim();
    if (!name) return;
    const id = prompt('Project ID:', name.toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-|-$/g,'').slice(0,64))?.trim();
    if (!id) return;
    const p = await createProject({ id, name });
    projects = [...projects, p];
    await select(p.id);
  }
  function newNote() {
    if (!leave()) return;
    let p = prompt('Note path, e.g. decisions/material.md')?.trim();
    if (!p) return;
    if (!p.endsWith('.md')) p += '.md';
    if (p.split('/').some(s => !s || s.startsWith('.') || /[\\:\x00-\x1f]/.test(s))) {
      message = 'Use a relative Markdown path.'; return;
    }
    openNote(p);
  }
  async function save() {
    await saveNote(project, {path, content: draft, previous: notes[path] ?? null});
    notes = Object.assign(Object.create(null), notes, {[path]: draft});
    openNote(path);
    message = 'Saved.';
  }
  async function newChat() {
    if (!leave()) return;
    const created = await createChat(project, agentChoice || agents[0].id);
    showChat(created);
    await openChat(created.id);
  }
  onMount(() => {
    const network = () => { offline = !navigator.onLine; };
    network();
    window.addEventListener('online', network);
    window.addEventListener('offline', network);
    setLeaveGuard(confirmLeave);
    action(load);
    canPin = pinSupported();
    const stopDisplays = displays.start();
    return () => {
      stopDisplays();
      setLeaveGuard(null);
      window.removeEventListener('online', network);
      window.removeEventListener('offline', network);
    };
  });
</script>

<svelte:head><title>Project notes · Ember</title></svelte:head>
{#snippet link(label: string, dest: string)}
  <a href={query(dest)} onclick={(e) => follow(e, dest)}>{label}</a>
{/snippet}
{#if offline}<p id="offline" role="status">Offline. Browse notes already loaded in this window. Reconnect to load projects or save edits.</p>{/if}
<div class="notes" inert={busy}>
  <aside>
    <DisplaySettings {displays} />
    <label for="project">Project</label>
    <select id="project" value={project} onchange={e => {
      const id = e.currentTarget.value; e.currentTarget.value = project;
      if (leave()) action(() => select(id));
    }}>
      {#each projects as p (p.id)}<option value={p.id} selected={project === p.id}>{p.name} ({p.id})</option>{/each}
    </select>
    <button id="create" onclick={() => action(create)}>Create project</button>
    {#if project}
      <h2>Notes</h2><input type="search" aria-label="Find a note" placeholder="Find a note…" bind:value={filter}>
      <nav aria-label="Project notes">
        {#each paths.filter(p => p.toLowerCase().includes(filter.toLowerCase())) as p}{@render link(p,p)}{/each}
      </nav>
      <button onclick={newNote}>New note</button>
      {#if agentsEnabled || chats.length}
        <h2>Chats</h2>
        <nav id="chats" aria-label="Project chats">
          {#each chats as c (c.id)}
            <a href={chatQuery(c.id)} onclick={(e) => followChat(e, c.id)}
              aria-current={chat?.id === c.id ? 'page' : undefined}>{c.title || 'New chat'}{c.pending ? ' …' : ''}</a>
          {:else}<span class="hint">No chats yet.</span>{/each}
        </nav>
        {#if agentsEnabled}
          {#if agents.length > 1}
            <label for="agent">Agent</label>
            <select id="agent" bind:value={agentChoice}>
              {#each agents as a (a.id)}<option value={a.id}>{a.name}{a.model ? ` (${a.model})` : ''}</option>{/each}
            </select>
          {/if}
          <button id="new-chat" onclick={() => action(newChat)}>New chat</button>
        {/if}
      {/if}
    {/if}
  </aside>
  <section class="page">
    <p id="message" role="status">{message}</p>
    {#if retry}<button id="retry" onclick={() => action(load)}>Retry connection</button>{/if}
    {#if project && chat}
      <!-- Keyed so that a draft or live text never carries over to another chat. -->
      {#key `${project}/${chat.id}`}
        <ProjectChat {project} {chat} {agents} available={agentsEnabled && agents.some(a => a.id === chat?.agent)}
          onupdate={showChat} onsignedout={() => { signedOut = true; }} />
      {/key}
    {:else if project}
      <div class="toolbar">
        <strong id="path">{path}</strong>
        {#if editing}
          <button id="save" onclick={() => action(save)}>Save</button>
          <button onclick={() => { if (leave()) openNote(Object.hasOwn(notes,path) ? path : 'index.md'); }}>Cancel</button>
        {:else}<button id="edit" onclick={() => { draft = notes[path] ?? ''; editing = true; }}>Edit</button>
          {#if canPin}<button id="pin" onclick={() => action(pin)} title="Keep this note in a small window above other windows">Pin to top</button>{/if}
        {/if}
      </div>
      {#if editing}
        <textarea id="editor" aria-label="Markdown source" bind:value={draft}></textarea>
      {:else}
        <NotePreview {path} {notes} {href} {follow} />
      {/if}
      <h2>Backlinks</h2><nav id="backlinks" aria-label="Notes linking here">
        {#each references as ref}{@render link(ref, ref)}{:else}No notes link here yet.{/each}
      </nav>
    {/if}
  </section>
</div>
<DisplayOffer {displays} />

<style>
  aside {padding:var(--space)}
  #offline {margin:0 0 var(--space);padding:.6rem 1.2rem;background:var(--warn-bg);border-radius:var(--radius)}
  .toolbar,aside {background:var(--surface);border:1px solid var(--border)}
  .notes {display:grid;grid-template-columns:minmax(14rem,20rem) 1fr;gap:var(--space)}
  aside {align-self:start;border-radius:.4rem}
  textarea {height:55vh}
  nav a {display:block;overflow-wrap:anywhere;padding:.15rem 0}
  .toolbar {display:flex;align-items:center;gap:.5rem;padding:.5rem;border-radius:var(--radius)}
  .toolbar strong {margin-right:auto;overflow-wrap:anywhere}
  .page {min-width:0}
  /* The preview is NotePreview's article, shared with the pinned window. */
  :global(article#preview) {min-height:10rem;white-space:pre-wrap;overflow-wrap:anywhere}
  :global(article#preview pre) {overflow:auto;background:var(--surface-2);padding:.5rem}
  #message:empty {display:none}
  @media(max-width:62rem){.notes{grid-template-columns:1fr}}
  nav a[aria-current] {font-weight:650}
  .hint {color:var(--muted)}
</style>
