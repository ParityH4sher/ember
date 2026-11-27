<script lang="ts">
  import Card from '../../lib/ui/Card.svelte';
  import PageHeader from '../../lib/ui/PageHeader.svelte';
  import { defaults, prefs, savePrefs, type Density, type Theme } from '../../lib/prefs.svelte';
  import { displayShortcut, formatShortcut, parseShortcut } from '../../lib/launcher/keyboard';

  // Display preferences only. They apply at once and stay in this browser.
  let status = $state('');

  const themes: { value: Theme; label: string; hint: string }[] = [
    { value: 'system', label: 'System', hint: 'Follow your device setting' },
    { value: 'light', label: 'Light', hint: 'Dark text on a light background' },
    { value: 'dark', label: 'Dark', hint: 'Light text on a dark background' }
  ];
  const densities: { value: Density; label: string; hint: string }[] = [
    { value: 'comfortable', label: 'Comfortable', hint: 'More space around content' },
    { value: 'compact', label: 'Compact', hint: 'Fit more on the screen' }
  ];

  // The launcher shortcut: typed as text, e.g. "Mod+K" or "Ctrl+Alt+Space".
  let shortcutDraft = $state(''), shortcutError = $state('');
  $effect(() => { shortcutDraft = prefs.launcherShortcut; });
  let shortcut = $derived(prefs.launcherShortcut ? parseShortcut(prefs.launcherShortcut) : null);
  function saveShortcut() {
    const text = shortcutDraft.trim();
    const parsed = text ? parseShortcut(text) : null;
    if (text && !parsed) {
      shortcutError = 'Use a key with Mod (Ctrl, or ⌘ on a Mac), Ctrl, Alt or Meta, e.g. Mod+K or Ctrl+Alt+Space.';
      return;
    }
    shortcutError = '';
    save({ launcherShortcut: parsed ? formatShortcut(parsed) : '' });
  }

  function save(next: Partial<typeof prefs>) {
    status = savePrefs(next) ? 'Saved in this browser.' : 'Applied, but this browser blocks storage, so it resets when you reload.';
  }
</script>

<PageHeader title="Settings" lede="Display preferences for this browser." />

<div class="stack">
  <Card title="Appearance" id="appearance">
    <fieldset>
      <legend>Theme</legend>
      {#each themes as t}
        <label class="option">
          <input type="radio" name="theme" value={t.value} checked={prefs.theme === t.value}
            onchange={() => save({ theme: t.value })}>
          <span>{t.label} <small class="muted">{t.hint}</small></span>
        </label>
      {/each}
    </fieldset>
    <fieldset>
      <legend>Density</legend>
      {#each densities as d}
        <label class="option">
          <input type="radio" name="density" value={d.value} checked={prefs.density === d.value}
            onchange={() => save({ density: d.value })}>
          <span>{d.label} <small class="muted">{d.hint}</small></span>
        </label>
      {/each}
    </fieldset>
    <div class="row">
      <button onclick={() => save({ ...defaults })}>Reset to defaults</button>
      <p role="status" class="muted">{status}</p>
    </div>
  </Card>

  <Card title="Desktop" id="desktop">
    <label class="option">
      <input type="checkbox" checked={prefs.launcherCircle}
        onchange={e => save({ launcherCircle: e.currentTarget.checked })}>
      <span>Show launcher circle <small class="muted">In the middle of the desktop on your primary display.
        The shortcut opens the launcher either way.</small></span>
    </label>
    <label for="launcher-shortcut">Launcher shortcut</label>
    <input id="launcher-shortcut" bind:value={shortcutDraft} onchange={saveShortcut} placeholder="Off" autocomplete="off"
      spellcheck="false" aria-describedby="launcher-shortcut-hint" aria-invalid={shortcutError ? true : undefined}>
    <p id="launcher-shortcut-hint" class="muted hint" class:error={shortcutError}>
      {#if shortcutError}{shortcutError}
      {:else}Opens the launcher from any page{#if shortcut}, currently <kbd>{displayShortcut(shortcut)}</kbd>{/if}.
        <code>Mod</code> is Ctrl, or ⌘ on a Mac. Leave empty to turn the shortcut off.{/if}
    </p>
  </Card>

  <Card title="Where settings are kept" id="storage">
    <p>Ember's API stores no personal preferences yet, so these settings are kept in this browser
      only. They do not follow you to other devices and are cleared with this site's data.</p>
    <p>Your name, email and groups come from your identity provider. See
      <a href="/account/">Profile & account</a>.</p>
  </Card>
</div>

<style>
  .stack { display: grid; gap: var(--space); max-width: 44rem }
  fieldset { border: 0; padding: 0; margin: 0 0 1rem }
  legend { font-weight: 650; margin-bottom: .3rem }
  .option { display: flex; align-items: baseline; font-weight: 400; padding: .25rem 0 }
  small { margin-left: .4rem }
  .row { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap }
  .row p { margin: 0 }
  .hint { font-size: .85rem; margin: 0 }
  .error { color: var(--danger) }
  kbd { border: 1px solid var(--border-strong); border-radius: .25rem; padding: 0 .3rem; font: inherit; font-size: .9em }
</style>
