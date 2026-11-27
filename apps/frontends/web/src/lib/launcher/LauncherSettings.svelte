<script lang="ts">
  import type { WidgetProps } from '../widgets/types';
  import { launcherConfig, type LauncherConfig } from './config';
  import { RECENT_MAX } from './state';

  let { config: stored, setConfig }: WidgetProps<LauncherConfig> = $props();
  const uid = $props.id();
  let config = $derived(launcherConfig(stored));
</script>

<fieldset>
  <legend>Display</legend>
  <label class="option"><input type="radio" name="{uid}-mode" checked={config.mode === 'grid'}
    onchange={() => setConfig({ mode: 'grid' })}> Grid <small class="muted">Apps shown in place</small></label>
  <label class="option"><input type="radio" name="{uid}-mode" checked={config.mode === 'compact'}
    onchange={() => setConfig({ mode: 'compact' })}> Compact <small class="muted">One button that opens the launcher</small></label>
</fieldset>

<label class="option"><input type="checkbox" checked={config.showRecent}
  onchange={e => setConfig({ showRecent: e.currentTarget.checked })}> Show recently used apps</label>
{#if config.showRecent}
  <label for="{uid}-recent">Recent apps to show</label>
  <input id="{uid}-recent" type="number" min="1" max={RECENT_MAX} value={config.recentLimit}
    onchange={e => setConfig({ recentLimit: e.currentTarget.valueAsNumber })}>
{/if}
<p class="muted hint">The keyboard shortcut that opens the launcher is set for the whole app in
  <a href="/settings/#desktop">Settings</a>.</p>

<style>
  fieldset { border: 0; padding: 0; margin: 0 }
  legend { font-weight: 650; margin-bottom: .2rem }
  .option { display: flex; align-items: baseline; gap: .1rem; font-weight: 400; padding: .2rem 0 }
  small { font-size: .8rem; margin-left: .3rem }
  .hint { font-size: .85rem; margin: .2rem 0 0 }
</style>
