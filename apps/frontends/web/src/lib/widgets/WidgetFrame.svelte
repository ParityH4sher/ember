<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import { clampSize } from './registry';
  import type { Placement } from './layout';
  import type { AnyWidget, WidgetContext, WidgetSize } from './types';

  // One placed widget: title bar, settings panel and the widget itself. A
  // widget that throws while rendering is contained here (svelte:boundary) so
  // the rest of the board keeps working.
  let { placement, widget, context, onremove, onchange }: {
    placement: Placement;
    /** undefined when no registered widget has placement.widget as its id. */
    widget: AnyWidget | undefined;
    context: WidgetContext;
    onremove: () => void;
    onchange: (change: { size?: WidgetSize; config?: Record<string, unknown> }) => void;
  } = $props();

  let settings = $state(false);
  let name = $derived(widget?.name ?? 'Unavailable widget');
  let config = $derived(widget ? { ...widget.defaultConfig, ...placement.config } : {});
  let size = $derived(widget ? clampSize(widget, placement.size ?? widget.defaultSize) : { cols: 1, rows: 1 });
  let limits = $derived(widget ? { min: widget.minSize ?? { cols: 1, rows: 1 }, max: widget.maxSize ?? { cols: 4, rows: 8 } } : undefined);
  const setConfig = (change: Record<string, unknown>) => onchange({ config: change });
  const uid = $props.id();

  // Lifecycle hooks run once per instance, and again only if the widget's
  // definition is replaced. A failing hook is logged, never fatal.
  $effect(() => {
    const w = widget;
    if (!w) return;
    const ctx = untrack(() => context);
    const call = (hook: 'onMount' | 'onUnmount') => {
      try { w[hook]?.(ctx); } catch (error) { console.warn(`[ember] Widget "${w.id}" ${hook} failed:`, error); }
    };
    call('onMount');
    return () => call('onUnmount');
  });

  function resize(axis: 'cols' | 'rows', value: number) {
    if (widget) onchange({ size: clampSize(widget, { ...size, [axis]: value }) });
  }
</script>

<section class="widget" class:wide={size.cols > 2} style:--cols={size.cols} style:--rows={size.rows}
  aria-labelledby="{uid}-title" data-widget={placement.widget}>
  <header>
    <h3 id="{uid}-title">{name}</h3>
    {#if widget}
      <button class="tool" aria-label="Configure {name}" aria-expanded={settings} aria-controls="{uid}-settings"
        onclick={() => (settings = !settings)}><Icon name="settings" size={16} /></button>
    {/if}
    <button class="tool" aria-label="Remove {name}" onclick={onremove}><Icon name="close" size={16} /></button>
  </header>

  {#if widget && settings}
    {@const Settings = widget.settingsComponent}
    <div class="body settings" id="{uid}-settings">
      {#if Settings}<Settings {config} {setConfig} {context} />{/if}
      {#if limits}
        <fieldset class="size">
          <legend>Size</legend>
          <label>Width (columns)
            <input type="number" min={limits.min.cols} max={Math.min(limits.max.cols, 4)} value={size.cols}
              onchange={e => resize('cols', e.currentTarget.valueAsNumber)}></label>
          <label>Height (rows)
            <input type="number" min={limits.min.rows} max={limits.max.rows} value={size.rows}
              onchange={e => resize('rows', e.currentTarget.valueAsNumber)}></label>
        </fieldset>
      {/if}
      <button onclick={() => (settings = false)}>Done</button>
    </div>
  {:else if widget}
    {@const Body = widget.component}
    <div class="body">
      <svelte:boundary onerror={error => console.warn(`[ember] Widget "${widget.id}" failed:`, error)}>
        <Body {config} {setConfig} {context} />
        {#snippet failed(_error, reset)}
          <p role="alert">This widget stopped working. <button onclick={reset}>Try again</button></p>
        {/snippet}
      </svelte:boundary>
    </div>
  {:else}
    <p class="body muted">The widget “{placement.widget}” is not installed. Remove it, or enable the plugin that provides it.</p>
  {/if}
</section>

<style>
  .widget {
    grid-column: span var(--cols); grid-row: span var(--rows); display: flex; flex-direction: column; min-width: 0; min-height: 0;
    border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface)
  }
  header { display: flex; align-items: center; gap: .25rem; padding: .3rem .4rem .3rem .8rem; border-bottom: 1px solid var(--border) }
  h3 { font-size: .9rem; margin: 0 auto 0 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
  .tool { display: inline-grid; place-items: center; border: 0; background: none; padding: .3rem; color: var(--muted) }
  .tool:hover { background: var(--surface-2); color: var(--text) }
  .body { flex: 1; min-height: 0; overflow: auto; padding: .6rem .8rem; margin: 0 }
  .settings { display: grid; gap: .6rem; align-content: start }
  .size { display: flex; gap: .8rem; border: 0; padding: 0; margin: 0 }
  .size legend { font-weight: 650; margin-bottom: .2rem }
  .size label { font-weight: 400; flex: 1 }
  .settings > button { justify-self: start }
  /* The board is 4 columns wide, 2 on medium screens and 1 on narrow ones
     (breakpoints shared with WidgetBoard.svelte). */
  @media (max-width: 75rem) { .wide { grid-column: 1 / -1 } }
  @media (max-width: 40rem) { .widget { grid-column: 1 / -1 } }
</style>
