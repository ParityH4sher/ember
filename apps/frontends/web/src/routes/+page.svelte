<script lang="ts">
  import { onMount } from 'svelte';
  import Card from '../lib/ui/Card.svelte';
  import PageHeader from '../lib/ui/PageHeader.svelte';
  import Stat from '../lib/ui/Stat.svelte';
  import ServiceLinks from '../lib/ServiceLinks.svelte';
  import WidgetBoard from '../lib/widgets/WidgetBoard.svelte';
  import { browserOpeners, type Openers } from '../lib/apps/launch';
  import { goto } from '$app/navigation';
  import {
    getHealth, listAgents, listPlugins, listProjects,
    type AgentList, type Health, type Plugin, type Project
  } from '../lib/api';
  import { failure, settle, type Loaded } from '../lib/load';
  import { auth } from '../lib/session.svelte';

  let health = $state.raw<Loaded<Health>>(), projects = $state.raw<Loaded<Project[]>>();
  let plugins = $state.raw<Loaded<Plugin[]>>(), agents = $state.raw<Loaded<AgentList>>();
  let firstName = $derived(auth.user?.name?.split(' ')[0]);

  const count = <T,>(v: Loaded<T[]>) => v === undefined ? undefined : failure(v) ? '—' : v.length;
  let activePlugins = $derived(Array.isArray(plugins) ? plugins.filter(p => p.active).length : undefined);
  let needsSetup = $derived(Array.isArray(plugins) ? plugins.filter(p => !p.configured).length : 0);

  // Apps open Ember pages with client-side navigation, other sites in a new tab.
  const openers: Openers = { ...browserOpeners, openInternal: path => goto(path) };

  onMount(() => {
    settle(getHealth(), v => (health = v));
    settle(listProjects().then(r => r.projects), v => (projects = v));
    settle(listPlugins().then(r => r.plugins), v => (plugins = v));
    settle(listAgents(), v => (agents = v));
  });
</script>

<PageHeader title="Dashboard" lede={firstName ? `Welcome back, ${firstName}.` : 'A self-hosted workspace for turning ideas into engineered artifacts.'} />

<div class="stats">
  <Stat label="Core API" href={auth.admin ? '/admin/' : undefined}
    value={health === undefined ? undefined : failure(health) ? 'Unavailable' : health.status}
    tone={health === undefined ? undefined : failure(health) ? 'danger' : 'ok'}
    detail={health && !failure(health) ? `Version ${health.version}` : undefined} />
  <Stat label="Projects" value={count(projects)} href="/apps/notes/" detail="Linked notes and chats" />
  <Stat label="Active plugins" href="/plugins/"
    value={plugins === undefined ? undefined : failure(plugins) ? '—' : `${activePlugins} / ${plugins.length}`}
    detail={needsSetup ? `${needsSetup} need configuration` : undefined} tone={needsSetup ? 'warn' : undefined} />
  <Stat label="Agents" value={agents === undefined ? undefined : failure(agents) ? '—' : agents.enabled ? agents.agents.length : 'Off'}
    detail={agents && !failure(agents) && !agents.enabled ? 'Chat runs are disabled' : 'Available for project chat'} />
</div>

<div class="grid">
  <Card title="Projects" id="projects">
    {#snippet actions()}<a href="/apps/notes/">Open notes →</a>{/snippet}
    {#if projects === undefined}
      <p class="muted">Loading projects…</p>
    {:else if failure(projects)}
      <p role="alert">Unable to load projects: {projects.error}</p>
    {:else}
      <ul class="list">
        {#each projects.slice(0, 8) as p (p.id)}
          <li><a href={'/apps/notes/?' + new URLSearchParams({ project: p.id })}>{p.name}</a>
            <span class="muted">{p.id}</span></li>
        {:else}
          <li class="muted">No projects yet. <a href="/apps/notes/">Create one in Project notes.</a></li>
        {/each}
      </ul>
      {#if projects.length > 8}<p class="muted">and {projects.length - 8} more</p>{/if}
    {/if}
  </Card>

  <Card title="Shared services" id="services">
    <ServiceLinks />
  </Card>

  <Card title="Get started" id="start">
    <ul class="list">
      <li><a href="/apps/notes/">Write linked notes</a> <span class="muted">Markdown with wiki links and backlinks</span></li>
      <li><a href="/plugins/">Run plugin operations</a> <span class="muted">Forms built from each plugin's schema</span></li>
      <li><a href="/settings/">Choose a theme</a> <span class="muted">Light, dark or your system's</span></li>
      {#if auth.admin}<li><a href="/admin/">Review the instance</a> <span class="muted">Plugins, agents and access</span></li>{/if}
    </ul>
  </Card>
</div>

<WidgetBoard initial={['ember.launcher']} {openers} />

<style>
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(9.5rem, 1fr)); gap: var(--space); margin-bottom: var(--space) }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(19rem, 1fr)); gap: var(--space); align-items: start }
  .list { list-style: none; margin: 0; padding: 0 }
  .list li { display: flex; flex-wrap: wrap; gap: .2rem .6rem; align-items: baseline; padding: .35rem 0; border-top: 1px solid var(--border) }
  .list li:first-child { border-top: 0 }
  .list .muted { font-size: .85rem }
</style>
