<script lang="ts">
  import type { WidgetProps } from '../widgets/types';
  import { formatShortcut, parseShortcut } from './keyboard';
  import { launcherConfig, type LauncherConfig } from './config';
  import { RECENT_MAX } from './state';

  let { config: stored, setConfig }: WidgetProps<LauncherConfig> = $props();
  const uid = $props.id();
  let config = $derived(launcherConfig(stored));
  let shortcutDraft = $state('');
  $effect(() => { shortcutDraft = config.shortcut; });
  let shortcutError = $state('');

  function saveShortcut() {
    const text = shortcutDraft.trim();
    if (!text) { shortcutError = ''; setConfig({ shortcut: '' }); return; }
    const parsed = parseShortcut(text);
    if (!parsed) {
      shortcutError = 'Use a key with Ctrl, Alt or Meta, e.g. Alt+Shift+L or Ctrl+Alt+Space.';
      return;
    }
    shortcutError = '';
    shortcutDraft = formatShortcut(parsed);
    setConfig({ shortcut: shortcutDraft });
  }
</script>

<fieldset>
  <legend>Display</legend>
  <label class="option"><input type="radio" name="{uid}-mode" checked={config.mode === 'grid'}
    onchange={() => setConfig({ mode: 'grid' })}> Grid <small class="muted">Apps shown in place</small></label>
  <label class="option"><input type="radio" name="{uid}-mode" checked={config.mode === 'compact'}
    onchange={() => setConfig({ mode: 'compact' })}> Compact <small class="muted">One button that opens the launcher</small></label>
</fieldset>

<label for="{uid}-shortcut">Keyboard shortcut</label>
<input id="{uid}-shortcut" bind:value={shortcutDraft} onchange={saveShortcut} placeholder="Off"
  aria-describedby="{uid}-shortcut-hint" aria-invalid={shortcutError ? true : undefined}>
<small id="{uid}-shortcut-hint" class:error={shortcutError} class="muted">
  {shortcutError || 'Opens or focuses the launcher from anywhere on the page. Leave empty to turn it off.'}</small>

<label class="option"><input type="checkbox" checked={config.showRecent}
  onchange={e => setConfig({ showRecent: e.currentTarget.checked })}> Show recently used apps</label>
{#if config.showRecent}
  <label for="{uid}-recent">Recent apps to show</label>
  <input id="{uid}-recent" type="number" min="1" max={RECENT_MAX} value={config.recentLimit}
    onchange={e => setConfig({ recentLimit: e.currentTarget.valueAsNumber })}>
{/if}

<style>
  fieldset { border: 0; padding: 0; margin: 0 }
  legend { font-weight: 650; margin-bottom: .2rem }
  .option { display: flex; align-items: baseline; gap: .1rem; font-weight: 400; padding: .2rem 0 }
  small { display: block; font-size: .8rem; margin-left: .3rem }
  label + input + small { margin: -.2rem 0 .4rem }
  .option small { display: inline }
  .error { color: var(--danger) }
</style>
