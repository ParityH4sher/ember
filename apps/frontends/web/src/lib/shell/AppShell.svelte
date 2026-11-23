<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import Icon, { type IconName } from '../ui/Icon.svelte';
  import UserMenu from './UserMenu.svelte';
  import { auth, signIn } from '../session.svelte';

  let { children }: { children: Snippet } = $props();

  interface Item { href: string; label: string; icon: IconName; admin?: boolean }
  const sections: { label: string; items: Item[] }[] = [
    { label: 'Workspace', items: [
      { href: '/', label: 'Dashboard', icon: 'home' },
      { href: '/apps/notes/', label: 'Project notes', icon: 'notes' },
      { href: '/plugins/', label: 'Plugins', icon: 'plugin' }
    ] },
    { label: 'You', items: [
      { href: '/account/', label: 'Profile & account', icon: 'user' },
      { href: '/settings/', label: 'Settings', icon: 'settings' }
    ] },
    { label: 'Instance', items: [{ href: '/admin/', label: 'Admin', icon: 'shield', admin: true }] }
  ];

  let drawer = $state(false);
  const current = (href: string) => href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
  // Navigating closes the drawer on narrow screens.
  $effect(() => { void page.url.pathname; drawer = false; });
</script>

<a class="skip" href="#content">Skip to content</a>
<div class="shell" class:drawer>
  <header class="topbar">
    <button class="menu-button" aria-label="Menu" aria-expanded={drawer} aria-controls="sidebar"
      onclick={() => (drawer = !drawer)}><Icon name="menu" /></button>
    <a class="brand" href="/"><span class="mark" aria-hidden="true">◆</span> Ember</a>
    <UserMenu />
  </header>
  <nav id="sidebar" aria-label="Main">
    {#each sections as section}
      {@const items = section.items.filter(i => !i.admin || auth.admin)}
      {#if items.length}
        <h2>{section.label}</h2>
        <ul>
          {#each items as item}
            <li><a href={item.href} aria-current={current(item.href) ? 'page' : undefined}>
              <Icon name={item.icon} /> {item.label}</a></li>
          {/each}
        </ul>
      {/if}
    {/each}
  </nav>
  <button class="scrim" aria-label="Close menu" tabindex="-1" onclick={() => (drawer = false)}></button>
  <main id="content" tabindex="-1">
    {#if auth.expired}
      <div class="expired" role="alert">
        <p><strong>Your session has ended.</strong> Copy any unsaved text before you sign in again;
          Ember cannot save it without a session.</p>
        <button class="primary" onclick={() => signIn()}>Sign in again</button>
      </div>
    {/if}
    {@render children()}
  </main>
</div>

<style>
  .skip { position: absolute; left: -999px; top: .5rem; z-index: 50; background: var(--surface); padding: .4rem .8rem }
  .skip:focus { left: .5rem }
  .shell {
    display: grid; min-height: 100vh;
    grid-template: "top top" auto "nav main" 1fr / 15rem minmax(0, 1fr)
  }
  .topbar {
    grid-area: top; position: sticky; top: 0; z-index: 10; display: flex; align-items: center; gap: .6rem;
    padding: .5rem var(--space); background: var(--surface); border-bottom: 1px solid var(--border)
  }
  .brand { margin-right: auto; font-weight: 750; font-size: 1.1rem; color: inherit; text-decoration: none }
  .mark { color: var(--accent) }
  .menu-button { display: none; border: 0; background: none; padding: .3rem }
  nav {
    grid-area: nav; position: sticky; top: 3.1rem; align-self: start; height: calc(100vh - 3.1rem);
    overflow-y: auto; box-sizing: border-box; padding: .8rem; border-right: 1px solid var(--border); background: var(--surface)
  }
  nav h2 { font-size: .75rem; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 1rem .6rem .3rem }
  nav h2:first-child { margin-top: .2rem }
  ul { list-style: none; margin: 0; padding: 0 }
  nav a {
    display: flex; align-items: center; gap: .6rem; padding: .45rem .6rem; border-radius: .3rem;
    color: inherit; text-decoration: none
  }
  nav a:hover { background: var(--surface-2) }
  nav a[aria-current] { background: var(--surface-2); font-weight: 650; box-shadow: inset 3px 0 var(--accent) }
  main { grid-area: main; padding: var(--space); min-width: 0; outline: none }
  .scrim { display: none }
  .expired {
    display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; margin-bottom: var(--space);
    padding: .7rem 1rem; border-radius: var(--radius); background: var(--warn-bg); border: 1px solid var(--warn)
  }
  .expired p { margin: 0; flex: 1 1 20rem }
  @media (max-width: 52rem) {
    .shell { grid-template: "top" auto "main" 1fr / minmax(0, 1fr) }
    .menu-button { display: inline-flex }
    nav {
      position: fixed; top: 3.1rem; left: 0; bottom: 0; height: auto; width: min(17rem, 85vw); z-index: 30;
      transform: translateX(-100%); transition: transform .15s; visibility: hidden
    }
    .drawer nav { transform: none; visibility: visible }
    .drawer .scrim {
      display: block; position: fixed; inset: 3.1rem 0 0; z-index: 25; border: 0; border-radius: 0;
      background: rgb(0 0 0 / .4)
    }
  }
  @media (prefers-reduced-motion: reduce) { nav { transition: none } }
</style>
