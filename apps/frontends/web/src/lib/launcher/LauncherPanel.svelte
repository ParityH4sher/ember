<script lang="ts">
  import { tick } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import AppIcon from '../apps/AppIcon.svelte';
  import { appRegistry } from '../apps/registry';
  import { describeTarget } from '../apps/launch';
  import type { AppDefinition } from '../apps/types';
  import type { WidgetContext } from '../widgets/types';
  import { appsById, categoriesOf, searchApps, sortApps } from './search';
  import { launcherState, recordLaunch, reconcile, togglePin } from './state';
  import { moveFocus, type GridPos } from './keyboard';
  import type { LauncherConfig } from './config';

  // The launcher itself: search, category tabs, pinned/recent/all sections and
  // the per-app menu. Launcher.svelte places it inline or in a popover.
  let { config, setConfig, context, shortcut, onclose, onlaunch }: {
    config: LauncherConfig;
    setConfig: (change: Partial<LauncherConfig>) => void;
    context: WidgetContext;
    /** The shortcut in aria-keyshortcuts form, if one is set. */
    shortcut?: string;
    /** Set when the launcher is in a popover or dialog: Escape on an empty search closes it. */
    onclose?: () => void;
    onlaunch?: () => void;
  } = $props();

  interface Section { key: string; label: string; apps: AppDefinition[] }

  const uid = $props.id();
  let query = $state(''), status = $state('');
  let active = $state<GridPos>({ section: 0, index: 0 });
  let root = $state<HTMLElement>(), search = $state<HTMLInputElement>(), infoDialog = $state<HTMLDialogElement>();
  let menu = $state<{ app: AppDefinition; x: number; y: number; from: HTMLElement } | null>(null);
  let info = $state<AppDefinition | null>(null);

  let apps = $derived($appRegistry);
  let pins = $derived($launcherState.pinned);
  let pinSet = $derived(new Set(pins));
  let categories = $derived(categoriesOf(apps));
  let tab = $derived(categories.includes(config.category) ? config.category : 'all');
  let searching = $derived(query.trim() !== '');
  let sections = $derived.by((): Section[] => {
    if (searching) return [{ key: 'results', label: 'Results', apps: searchApps(apps, query, pins) }];
    if (tab !== 'all') return [{ key: 'category', label: tab, apps: sortApps(apps, pins, tab) }];
    const out: Section[] = [];
    const pinned = appsById(apps, pins);
    if (pinned.length) out.push({ key: 'pinned', label: 'Pinned', apps: pinned });
    const recent = config.showRecent ? appsById(apps, $launcherState.recent, config.recentLimit) : [];
    if (recent.length) out.push({ key: 'recent', label: 'Recent', apps: recent });
    out.push({ key: 'all', label: 'All apps', apps: sortApps(apps) });
    return out;
  });
  let sizes = $derived(sections.map(s => s.apps.length));
  let total = $derived(sizes.reduce((a, b) => a + b, 0));
  let resultsText = $derived(!searching ? '' : sizes[0] ? `${sizes[0]} ${sizes[0] === 1 ? 'app' : 'apps'} found` : 'No apps found');
  let otherDisplays = $derived(context.displays?.list().filter(d => d.id !== context.displayId) ?? []);

  // Keep the roving tab stop on an existing item when the lists change.
  $effect(() => {
    const { section, index } = active;
    if (sizes[section] && index < sizes[section]) return;
    const first = Math.max(0, sizes.findIndex(n => n > 0));
    // Assign only on a real change: a new object would re-run this effect forever.
    if (section !== first || index !== 0) active = { section: first, index: 0 };
  });

  // Refresh "last seen" for registered apps and drop long-gone ones.
  $effect(() => {
    const next = reconcile($launcherState, new Set(apps.map(a => a.id)), Date.now());
    if (next) launcherState.set(next);
  });

  export function focusSearch() {
    search?.focus();
    search?.select();
  }

  const item = (pos: GridPos) => root?.querySelector<HTMLButtonElement>(`[data-pos="${pos.section}:${pos.index}"]`);
  /** Items on the first row of a section, measured from the layout. */
  function columns(section: number): number {
    const items = root?.querySelectorAll<HTMLElement>(`[data-section="${section}"] > li`) ?? [];
    let n = 0;
    for (const li of items) { if (li.offsetTop !== items[0].offsetTop) break; n++; }
    return Math.max(1, n);
  }
  function focusItem(pos: GridPos) {
    active = pos;
    item(pos)?.focus();
  }

  async function launch(app: AppDefinition) {
    launcherState.update(s => recordLaunch(s, app.id, Date.now()));
    if (await context.launch(app.id)) { query = ''; onlaunch?.(); }
    else status = `Could not open ${app.name}.`;
  }
  function pin(app: AppDefinition) {
    const was = pinSet.has(app.id);
    launcherState.update(s => togglePin(s, app.id, Date.now()));
    status = `${was ? 'Unpinned' : 'Pinned'} ${app.name}.`;
  }

  function searchKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      const first = sections.find(s => s.apps.length)?.apps[0];
      if (first) launch(first);
    } else if (e.key === 'ArrowDown' && total) {
      e.preventDefault();
      focusItem(active);
    } else if (e.key === 'Escape') {
      if (query) { e.preventDefault(); query = ''; }
      else if (onclose) { e.preventDefault(); onclose(); }
    }
  }
  function itemKey(e: KeyboardEvent, pos: GridPos, app: AppDefinition) {
    if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) {
      e.preventDefault();
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
      openMenu(app, r.left, r.bottom, e.currentTarget as HTMLElement);
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      if (query) query = '';
      else if (onclose) { onclose(); return; }
      search?.focus();
      return;
    }
    if (e.ctrlKey || e.altKey || e.metaKey) return;
    const next = moveFocus(e.key, pos, sizes, columns);
    if (!next) return;
    e.preventDefault();
    if (next === 'search') search?.focus();
    else focusItem(next);
  }
  function tabKey(e: KeyboardEvent) {
    const all = ['all', ...categories];
    const i = all.indexOf(tab);
    const j = e.key === 'ArrowRight' ? (i + 1) % all.length : e.key === 'ArrowLeft' ? (i - 1 + all.length) % all.length
      : e.key === 'Home' ? 0 : e.key === 'End' ? all.length - 1 : -1;
    if (j < 0) return;
    e.preventDefault();
    setConfig({ category: all[j] });
    tick().then(() => root?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')?.focus());
  }

  // Context menu: right-click, long-press on touch, or Shift+F10 / the Menu key.
  let press: { timer: ReturnType<typeof setTimeout>; x: number; y: number } | null = null, pressed = false;
  function pointerDown(e: PointerEvent, app: AppDefinition) {
    if (e.pointerType === 'mouse') return;
    const target = e.currentTarget as HTMLElement;
    pressed = false;
    press = { x: e.clientX, y: e.clientY, timer: setTimeout(() => { pressed = true; openMenu(app, e.clientX, e.clientY, target); }, 550) };
  }
  function pointerEnd(e: PointerEvent) {
    if (press && (e.type !== 'pointermove' || Math.hypot(e.clientX - press.x, e.clientY - press.y) > 10)) {
      clearTimeout(press.timer);
      press = null;
    }
  }
  function click(e: MouseEvent, app: AppDefinition, pos: GridPos) {
    active = pos;
    if (pressed) { pressed = false; e.preventDefault(); return; }  // the long-press already opened the menu
    launch(app);
  }
  async function openMenu(app: AppDefinition, x: number, y: number, from: HTMLElement) {
    if (menu?.app === app) return;
    const width = 210, height = 44 * (2 + otherDisplays.length);
    menu = { app, from, x: Math.max(4, Math.min(x, innerWidth - width - 4)), y: Math.max(4, Math.min(y, innerHeight - height - 4)) };
    await tick();
    root?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
  }
  function closeMenu(restore = true) {
    const from = menu?.from;
    menu = null;
    if (restore) from?.focus();
  }
  function menuKey(e: KeyboardEvent) {
    const items = [...(root?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeMenu(); }
    else if (e.key === 'Tab') closeMenu(false);
    else if (e.key === 'ArrowDown') { e.preventDefault(); items[(i + 1) % items.length]?.focus(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); items[(i - 1 + items.length) % items.length]?.focus(); }
    else if (e.key === 'Home') { e.preventDefault(); items[0]?.focus(); }
    else if (e.key === 'End') { e.preventDefault(); items[items.length - 1]?.focus(); }
  }
  function menuAction(fn: (app: AppDefinition) => void) {
    const app = menu!.app;
    closeMenu();
    fn(app);
  }
  function openOn(app: AppDefinition, displayId: string, label: string) {
    launcherState.update(s => recordLaunch(s, app.id, Date.now()));
    context.displays?.launchOn(displayId, app.id);
    status = `Opening ${app.name} on ${label}.`;
  }
  async function showInfo(app: AppDefinition) {
    info = app;
    await tick();
    infoDialog?.showModal();
  }
</script>

<svelte:window onpointerdown={e => { if (menu && !(e.target as Element).closest?.('[role="menu"]')) closeMenu(false); }} />

<div class="launcher" bind:this={root}>
  <div class="search">
    <Icon name="search" size={18} />
    <input bind:this={search} bind:value={query} type="search" placeholder="Search apps…" aria-label="Search apps"
      aria-controls="{uid}-apps" aria-describedby="{uid}-count" aria-keyshortcuts={shortcut} autocomplete="off"
      spellcheck="false" onkeydown={searchKey}>
  </div>
  <p id="{uid}-count" class="visually-hidden" role="status">{resultsText}</p>
  <p class="status muted" role="status">{status}</p>

  {#if categories.length && !searching}
    <div class="tabs" role="tablist" aria-label="App categories">
      {#each ['all', ...categories] as c (c)}
        <button role="tab" aria-selected={tab === c} aria-controls="{uid}-apps" tabindex={tab === c ? 0 : -1}
          onclick={() => setConfig({ category: c })} onkeydown={tabKey}>{c === 'all' ? 'All' : c}</button>
      {/each}
    </div>
  {/if}

  <div id="{uid}-apps" class="sections" role={categories.length && !searching ? 'tabpanel' : undefined}
    aria-label={categories.length && !searching ? (tab === 'all' ? 'All' : tab) : undefined}>
    {#if !apps.length}
      <p class="empty">No apps are installed yet. Apps appear here when Ember or a plugin registers them.</p>
    {:else if searching && !total}
      <p class="empty">No apps match “{query.trim()}”. <button onclick={() => { query = ''; search?.focus(); }}>Clear search</button></p>
    {:else}
      {#each sections as section, s (section.key)}
        <h4 id="{uid}-{section.key}">{section.label}</h4>
        <ul class="grid" data-section={s} aria-labelledby="{uid}-{section.key}">
          {#each section.apps as app, i (app.id)}
            {@const pinned = pinSet.has(app.id)}
            <li>
              <button class="app" data-pos="{s}:{i}" tabindex={active.section === s && active.index === i ? 0 : -1}
                aria-haspopup="menu" title={app.description}
                onclick={e => click(e, app, { section: s, index: i })}
                onkeydown={e => itemKey(e, { section: s, index: i }, app)}
                oncontextmenu={e => { e.preventDefault(); openMenu(app, e.clientX, e.clientY, e.currentTarget); }}
                onpointerdown={e => pointerDown(e, app)} onpointerup={pointerEnd} onpointercancel={pointerEnd}
                onpointermove={pointerEnd} onfocus={() => (active = { section: s, index: i })}>
                <AppIcon icon={app.icon} name={app.name} size={32} />
                <span class="name">{app.name}{#if pinned && section.key !== 'pinned'}<span class="visually-hidden">{' (pinned)'}</span>{/if}</span>
                {#if pinned && section.key !== 'pinned'}<span class="pin" aria-hidden="true"><Icon name="pin" size={12} /></span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      {/each}
    {/if}
  </div>

  {#if menu}
    {@const app = menu.app}
    <div class="menu" role="menu" aria-label="Actions for {app.name}" style:left="{menu.x}px" style:top="{menu.y}px"
      tabindex="-1" onkeydown={menuKey}
      onfocusout={e => { if (!(e.currentTarget as Node).contains(e.relatedTarget as Node | null)) closeMenu(false); }}>
      <button role="menuitem" tabindex="-1" onclick={() => menuAction(pin)}>
        <Icon name="pin" size={16} /> {pinSet.has(app.id) ? 'Unpin' : 'Pin to top'}</button>
      {#each otherDisplays as d (d.id)}
        <button role="menuitem" tabindex="-1" onclick={() => menuAction(a => openOn(a, d.id, d.label))}>
          <Icon name="external" size={16} /> Open on {d.label}</button>
      {/each}
      <button role="menuitem" tabindex="-1" onclick={() => menuAction(showInfo)}><Icon name="info" size={16} /> App info</button>
    </div>
  {/if}

  <!-- Escape closes only this dialog, not a popover or panel around the launcher. -->
  <dialog bind:this={infoDialog} aria-labelledby="{uid}-info-title" onclose={() => (info = null)}
    onkeydown={e => { if (e.key === 'Escape') e.stopPropagation(); }}>
    {#if info}
      <div class="info-head">
        <AppIcon icon={info.icon} name={info.name} size={40} />
        <h4 id="{uid}-info-title">{info.name}</h4>
      </div>
      {#if info.description}<p>{info.description}</p>{/if}
      <dl>
        {#if info.category}<dt>Category</dt><dd>{info.category}</dd>{/if}
        <dt>Opens</dt><dd>{describeTarget(info)}</dd>
        {#if info.keywords?.length}<dt>Keywords</dt><dd>{info.keywords.join(', ')}</dd>{/if}
        <dt>App id</dt><dd><code>{info.id}</code></dd>
        <dt>Pinned</dt><dd>{pinSet.has(info.id) ? 'Yes' : 'No'}</dd>
      </dl>
      <form method="dialog"><button class="primary">Close</button></form>
    {/if}
  </dialog>
</div>

<style>
  .launcher { display: flex; flex-direction: column; gap: .4rem; min-height: 100% }
  .search { position: relative; display: flex; align-items: center; color: var(--muted) }
  .search :global(svg) { position: absolute; left: .55rem; pointer-events: none }
  .search input { margin: 0; padding-left: 2rem }
  .status { margin: 0; font-size: .85rem }
  .status:empty { display: none }
  .tabs { display: flex; gap: .25rem; overflow-x: auto; scrollbar-width: thin; padding-bottom: .1rem }
  .tabs button { flex: none; border-radius: 1rem; padding: .15rem .7rem; font-size: .85rem; background: none; border-color: var(--border) }
  .tabs button[aria-selected="true"] { background: var(--surface-2); border-color: var(--accent); font-weight: 650 }
  h4 { margin: .4rem 0 .2rem; font-size: .75rem; text-transform: uppercase; letter-spacing: .06em; color: var(--muted) }
  .grid {
    list-style: none; margin: 0; padding: 0; display: grid; gap: .25rem;
    grid-template-columns: repeat(auto-fill, minmax(5.25rem, 1fr))
  }
  .app {
    position: relative; display: flex; flex-direction: column; align-items: center; gap: .3rem; width: 100%; height: 100%;
    box-sizing: border-box; padding: .5rem .25rem; border: 1px solid transparent; background: none; border-radius: var(--radius);
    color: var(--text); -webkit-touch-callout: none; user-select: none
  }
  .app:hover { background: var(--surface-2) }
  .app:focus-visible { background: var(--surface-2); border-color: var(--link) }
  .name {
    font-size: .8rem; line-height: 1.25; text-align: center; overflow-wrap: anywhere;
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden
  }
  .pin { position: absolute; top: .3rem; right: .35rem; color: var(--accent) }
  .empty { color: var(--muted); margin: .5rem 0 }
  .empty button { margin-left: .3rem }
  .menu {
    position: fixed; z-index: 40; min-width: 12rem; padding: .3rem; background: var(--surface);
    border: 1px solid var(--border); border-radius: var(--radius); box-shadow: 0 8px 24px rgb(0 0 0 / .3)
  }
  .menu button {
    display: flex; align-items: center; gap: .5rem; width: 100%; text-align: left; border: 0; background: none;
    padding: .45rem .5rem; border-radius: .3rem
  }
  .menu button:hover, .menu button:focus-visible { background: var(--surface-2) }
  dialog {
    max-width: min(26rem, 90vw); color: var(--text); background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius); padding: var(--space)
  }
  dialog::backdrop { background: rgb(0 0 0 / .45) }
  .info-head { display: flex; align-items: center; gap: .7rem }
  .info-head h4 { font-size: 1.15rem; text-transform: none; letter-spacing: 0; color: var(--text); margin: 0 }
  dl { display: grid; grid-template-columns: auto 1fr; gap: .25rem .8rem; margin: .8rem 0 }
  dt { color: var(--muted) }
  dd { margin: 0; overflow-wrap: anywhere }
  form { margin: 0; text-align: right }
</style>
