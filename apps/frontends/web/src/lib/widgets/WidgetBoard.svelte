<script lang="ts">
  import { tick } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import { resolvedTheme } from '../theme.svelte';
  import { browserOpeners, launchById, type Openers } from '../apps/launch';
  import { widgetRegistry } from './registry';
  import { addPlacement, boardLayout, changePlacement, removePlacement } from './layout';
  import WidgetFrame from './WidgetFrame.svelte';
  import type { DisplayCoordinator, WidgetContext } from './types';

  // A board of widgets: the host side of the widget contract. It lists the
  // registered widgets, stores what is placed (per display), and gives each
  // instance its config and context. It knows no widget in particular.
  let { displayId = 'main', initial = [], openers = browserOpeners, displays, title = 'Widgets' }: {
    displayId?: string;
    /** Widget ids placed on a first visit. */
    initial?: string[];
    openers?: Openers;
    displays?: DisplayCoordinator;
    title?: string;
  } = $props();

  let layout = $derived(boardLayout(displayId, initial));
  let adding = $state(false), status = $state('');
  let addButton = $state<HTMLButtonElement>(), board = $state<HTMLElement>();

  let theme = $derived(resolvedTheme());
  const launch = (appId: string) => launchById(appId, openers);
  const context = (instanceId: string): WidgetContext => ({ instanceId, displayId, theme, launch, displays });

  async function add(widgetId: string, name: string) {
    adding = false;
    layout.update(l => addPlacement(l, widgetId));
    status = `Added ${name}.`;
    await tick();
    board?.querySelector<HTMLElement>('.widget:last-child h3')?.scrollIntoView({ block: 'nearest' });
  }
  function remove(id: string, name: string) {
    layout.update(l => removePlacement(l, id));
    status = `Removed ${name}.`;
    addButton?.focus();
  }
  function closeMenu(event: Event) {
    if (adding && !(event.target as Element).closest?.('.add')) adding = false;
  }
</script>

<svelte:window onclick={closeMenu} onkeydown={e => { if (e.key === 'Escape' && adding) { adding = false; addButton?.focus(); } }} />

<section class="board-section" aria-labelledby="board-{displayId}-title">
  <div class="head">
    <h2 id="board-{displayId}-title">{title}</h2>
    <p role="status" class="muted status">{status}</p>
    <div class="add">
      <button bind:this={addButton} aria-expanded={adding} aria-controls="board-{displayId}-catalog"
        onclick={() => (adding = !adding)}><Icon name="plus" size={16} /> Add widget</button>
      {#if adding}
        <ul class="catalog" id="board-{displayId}-catalog" aria-label="Available widgets">
          {#each $widgetRegistry as w (w.id)}
            <li><button onclick={() => add(w.id, w.name)}><strong>{w.name}</strong>
              <span class="muted">{w.description}</span></button></li>
          {:else}
            <li class="muted">No widgets are installed.</li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
  <div class="board" bind:this={board}>
    {#each $layout.placements as placement (placement.id)}
      {@const widget = $widgetRegistry.find(w => w.id === placement.widget)}
      <WidgetFrame {placement} {widget} context={context(placement.id)}
        onremove={() => remove(placement.id, widget?.name ?? 'widget')}
        onchange={change => layout.update(l => changePlacement(l, placement.id, change))} />
    {:else}
      <p class="muted empty">No widgets here yet. Use <strong>Add widget</strong> to place one.</p>
    {/each}
  </div>
</section>

<style>
  .board-section { margin-top: var(--space) }
  .head { display: flex; align-items: center; gap: .8rem; margin-bottom: .6rem; flex-wrap: wrap }
  h2 { font-size: 1.1rem; margin: 0 }
  .status { margin: 0 auto 0 0; font-size: .9rem }
  .add { position: relative }
  .add > button { display: inline-flex; align-items: center; gap: .3rem }
  .catalog {
    position: absolute; right: 0; top: calc(100% + .3rem); z-index: 20; width: min(20rem, 80vw);
    list-style: none; margin: 0; padding: .3rem; background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius); box-shadow: 0 8px 24px rgb(0 0 0 / .25)
  }
  .catalog button {
    display: grid; width: 100%; text-align: left; border: 0; background: none; padding: .45rem .5rem; border-radius: .3rem
  }
  .catalog button:hover { background: var(--surface-2) }
  .catalog span { font-size: .85rem }
  .board {
    display: grid; gap: var(--space); grid-template-columns: repeat(4, minmax(0, 1fr));
    grid-auto-rows: minmax(7.5rem, auto); grid-auto-flow: row dense
  }
  .empty { grid-column: 1 / -1; margin: 0 }
  @media (max-width: 75rem) { .board { grid-template-columns: repeat(2, minmax(0, 1fr)) } }
  @media (max-width: 40rem) { .board { grid-template-columns: minmax(0, 1fr) } }
</style>
