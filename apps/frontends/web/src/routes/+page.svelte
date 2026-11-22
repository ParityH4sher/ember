<script lang="ts">
  import { onMount } from 'svelte';
  import { replaceState } from '$app/navigation';
  import {
    createProject, getSession, listProjects, loadNotes, saveNote, signInUrl, signOut, SignInRequired,
    type Notes, type Project, type Session
  } from '../lib/api';
  import { blocks } from '../lib/links';
  import '../notes.css';

  let projects = $state<Project[]>([]), project = $state(''), path = $state('');
  let notes = $state.raw<Notes>(Object.create(null));
  let editing = $state(false), busy = $state(false), offline = $state(false);
  let draft = $state(''), filter = $state(''), message = $state(''), retry = $state(false);
  let session = $state.raw<Session | null>(null), signedOut = $state(false);
  let dirty = $derived(editing && draft !== (notes[path] ?? ''));
  let paths = $derived(Object.keys(notes).sort());
  let references = $derived(paths.filter(p => p !== path &&
    [...blocks(notes[p] ?? '', p, notes)].some(([,parts]) => parts.some(t => t.path === path))));

  const query = (dest: string) => '?' + new URLSearchParams({project, note: dest});
  const leave = () => !dirty || confirm('Discard unsaved changes?');

  async function action(fn: () => Promise<void>) {
    if (busy) return;
    busy = true;
    try { await fn(); } catch (error) {
      if (error instanceof SignInRequired) { signedOut = true; retry = false; message = ''; }
      else message = error instanceof Error ? error.message : String(error);
    }
    finally { busy = false; }
  }
  // Sign-in and sign-out are full-page navigations through the API.
  function signIn() {
    if (!leave()) return;
    editing = false;
    location.assign(signInUrl(location.pathname + location.search));
  }
  async function logout() {
    if (!leave()) return;
    editing = false;
    const target = await signOut();
    if (target) location.assign(target); else location.reload();
  }
  function openNote(dest: string) {
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
  async function select(id: string, dest = 'index.md') {
    const data = await loadNotes(id);
    project = id;
    notes = Object.assign(Object.create(null), data.notes);
    filter = '';
    openNote(dest);
  }
  async function load() {
    try {
      retry = false;
      session = await getSession();
      signedOut = Boolean(session?.enabled && !session.authenticated);
      if (signedOut) return;
      projects = (await listProjects()).projects;
      if (!projects.length) { message = 'Create a project to start your linked notes.'; return; }
      const params = new URLSearchParams(location.search);
      await select(projects.find(p => p.id === params.get('project'))?.id ?? projects[0].id,
        params.get('note') || 'index.md');
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
  onMount(() => {
    const network = () => { offline = !navigator.onLine; };
    const unload = (e: BeforeUnloadEvent) => { if (dirty) { e.preventDefault(); e.returnValue = ''; } };
    network();
    window.addEventListener('online', network);
    window.addEventListener('offline', network);
    window.addEventListener('beforeunload', unload);
    action(load);
    return () => {
      window.removeEventListener('online', network);
      window.removeEventListener('offline', network);
      window.removeEventListener('beforeunload', unload);
    };
  });
</script>

<svelte:head><title>Project notes · Ember</title></svelte:head>
{#snippet link(label: string, dest: string)}
  <a href={query(dest)} onclick={(e) => follow(e, dest)}>{label}</a>
{/snippet}
<header><a href="/" data-sveltekit-reload>Ember</a> / Project notes
  {#if session?.authenticated}
    <span id="account">{session.user?.name || session.user?.email || session.user?.subject}
      <button id="sign-out" onclick={() => action(logout)}>Sign out</button></span>
  {/if}
</header>
{#if offline}<p id="offline" role="status">Offline. Browse notes already loaded in this window. Reconnect to load projects or save edits.</p>{/if}
<main inert={busy}>
  <aside>
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
    {/if}
  </aside>
  <section class="page">
    <p id="message" role="status">{message}</p>
    {#if signedOut}
      <p id="signed-out">You are signed out. <button id="sign-in" onclick={signIn}>Sign in</button></p>
    {/if}
    {#if retry}<button id="retry" onclick={() => action(load)}>Retry connection</button>{/if}
    {#if project}
      <div class="toolbar">
        <strong id="path">{path}</strong>
        {#if editing}
          <button id="save" onclick={() => action(save)}>Save</button>
          <button onclick={() => { if (leave()) openNote(Object.hasOwn(notes,path) ? path : 'index.md'); }}>Cancel</button>
        {:else}<button id="edit" onclick={() => { draft = notes[path] ?? ''; editing = true; }}>Edit</button>{/if}
      </div>
      {#if editing}
        <textarea id="editor" aria-label="Markdown source" bind:value={draft}></textarea>
      {:else}
        <article id="preview">
          {#each [...blocks(notes[path] ?? '', path, notes)] as [tag, parts]}
            <svelte:element this={tag}>{#each parts as part}{#if part.path}{@render link(part.text,part.path)}{:else}{part.text}{/if}{/each}</svelte:element>
          {/each}
        </article>
      {/if}
      <h2>Backlinks</h2><nav id="backlinks" aria-label="Notes linking here">
        {#each references as ref}{@render link(ref, ref)}{:else}No notes link here yet.{/each}
      </nav>
    {/if}
  </section>
</main>
