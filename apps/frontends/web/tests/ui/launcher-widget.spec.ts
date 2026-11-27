import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';
import { expect, grace, install, signedIn, test } from './mock-api';

// The app launcher widget on the dashboard, against the built app. The
// launcher starts on the dashboard's widget board on a first visit.

test.beforeEach(({ api }) => signedIn(api));
// A widget that throws is contained by its error boundary; that must never happen here.
test.afterEach(async ({ page }) => {
  await expect(page.getByText('This widget stopped working.')).toHaveCount(0);
});

const launcher = (page: Page) => page.getByRole('region', { name: 'App launcher' });
const search = (page: Page) => launcher(page).getByRole('searchbox', { name: 'Search apps' });
const list = (page: Page, name: string) => launcher(page).getByRole('list', { name });
const appButton = (page: Page, section: string, name: string) => list(page, section).getByRole('button', { name, exact: true });

async function open(page: Page) {
  await page.goto('/');
  await expect(search(page)).toBeVisible();
  // Shared services are registered at runtime, after the API answers.
  await expect(appButton(page, 'All apps', 'Authentik')).toBeVisible();
}

test('shows built-in and runtime apps by category', async ({ page }) => {
  await open(page);
  await expect(list(page, 'All apps').getByRole('button')).toHaveText(['Authentik', 'Dashboard', 'Desktop', 'Plugins', 'Profile & account', 'Project notes', 'Settings']);
  await expect(launcher(page).getByRole('tab')).toHaveText(['All', 'Services', 'Workspace', 'You']);
  await expect(launcher(page).getByRole('tab', { name: 'All' })).toHaveAttribute('aria-selected', 'true');
  await launcher(page).getByRole('tab', { name: 'You' }).click();
  await expect(launcher(page).getByRole('tabpanel').getByRole('button')).toHaveText(['Profile & account', 'Settings']);
  // The selected tab is part of the widget's saved configuration.
  await page.reload();
  await expect(launcher(page).getByRole('tab', { name: 'You' })).toHaveAttribute('aria-selected', 'true');
  // No Admin app for a user.
  await expect(launcher(page).getByRole('button', { name: 'Admin' })).toHaveCount(0);
});

test('admins also get the Admin app, registered at runtime', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  await open(page);
  await expect(appButton(page, 'All apps', 'Admin')).toBeVisible();
});

test('search filters as you type, announces the count, and Enter opens the top result', async ({ page }) => {
  await open(page);
  await search(page).fill('pro');
  await expect(list(page, 'Results').getByRole('button')).toHaveText(['Profile & account', 'Project notes']);
  await expect(launcher(page).getByRole('status').filter({ hasText: 'found' })).toHaveText('2 apps found');
  await search(page).fill('wiki');
  await expect(list(page, 'Results').getByRole('button')).toHaveText(['Project notes']);
  await expect(launcher(page).getByText('1 app found')).toBeAttached();
  await search(page).press('Enter');
  await expect(page).toHaveURL('/apps/notes/');
});

test('shows an empty state without results; Escape clears the search', async ({ page }) => {
  await open(page);
  await search(page).fill('zzz');
  await expect(launcher(page).getByText('No apps match “zzz”.')).toBeVisible();
  await expect(launcher(page).getByText('No apps found')).toBeAttached();
  await search(page).press('Escape');
  await expect(search(page)).toHaveValue('');
  await expect(list(page, 'All apps')).toBeVisible();
  await search(page).fill('zzz');
  await launcher(page).getByRole('button', { name: 'Clear search' }).click();
  await expect(search(page)).toBeFocused();
  await expect(search(page)).toHaveValue('');
});

test('arrow keys move through the grid and Enter launches', async ({ page }) => {
  await open(page);
  await search(page).focus();
  await page.keyboard.press('ArrowDown');
  await expect(appButton(page, 'All apps', 'Authentik')).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(appButton(page, 'All apps', 'Dashboard')).toBeFocused();
  await page.keyboard.press('End');
  await expect(appButton(page, 'All apps', 'Settings')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(appButton(page, 'All apps', 'Authentik')).toBeFocused();
  // Only the active item is a tab stop.
  await expect(list(page, 'All apps').locator('button[tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('ArrowUp');
  await expect(search(page)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  for (let i = 0; i < 5; i++) await page.keyboard.press('ArrowRight');
  await expect(appButton(page, 'All apps', 'Project notes')).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL('/apps/notes/');
});

test('with a launcher shown in place, the shortcut focuses its search instead of opening the overlay', async ({ page }) => {
  await open(page);
  const shell = page.getByRole('dialog', { name: 'App launcher' });
  await expect(search(page)).toHaveAttribute('aria-keyshortcuts', 'Control+K Meta+K');
  for (const key of ['Control+KeyK', 'Meta+KeyK']) {
    await page.getByRole('heading', { name: 'Dashboard' }).click();
    await expect(search(page)).not.toBeFocused();
    await page.keyboard.press(key);
    await expect(search(page)).toBeFocused();
    await expect(shell).toHaveCount(0);
  }
  // In compact mode the widget is a button, not shown in place: the shortcut opens the overlay.
  await page.getByRole('button', { name: 'Configure App launcher' }).click();
  await page.getByRole('radio', { name: /Compact/ }).check();
  await page.getByRole('button', { name: 'Done' }).click();
  await page.getByRole('heading', { name: 'Dashboard' }).click();
  await page.keyboard.press('Control+KeyK');
  await expect(shell.getByRole('searchbox', { name: 'Search apps' })).toBeFocused();
  // Pressed again, the shortcut closes it.
  await page.keyboard.press('Control+KeyK');
  await expect(shell).toHaveCount(0);
});

test('on pages without a launcher widget the shortcut opens the same launcher, with the same apps and pins', async ({ page }) => {
  await open(page);
  // Pin in the widget...
  await appButton(page, 'All apps', 'Settings').click({ button: 'right' });
  await launcher(page).getByRole('menuitem', { name: 'Pin to top' }).click();
  // ...and the shell's launcher (Ctrl+K by default) shows it: one registry, one state.
  await page.goto('/plugins/');
  await expect(page.getByRole('heading', { name: 'Plugins', exact: true })).toBeVisible();
  await page.keyboard.press('Control+KeyK');
  const shell = page.getByRole('dialog', { name: 'App launcher' });
  const shellSearch = shell.getByRole('searchbox', { name: 'Search apps' });
  await expect(shellSearch).toBeFocused();
  await expect(shellSearch).toHaveAttribute('aria-keyshortcuts', 'Control+K Meta+K');
  await expect(shell.getByRole('list', { name: 'Pinned' }).getByRole('button')).toHaveText(['Settings']);
  await expect(shell.getByRole('list', { name: 'All apps' }).getByRole('button', { name: 'Authentik', exact: true })).toBeVisible();
  // Escape with a query clears it; with an empty search it closes the launcher.
  await shellSearch.fill('zzz');
  await page.keyboard.press('Escape');
  await expect(shellSearch).toHaveValue('');
  await expect(shell).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(shell).toHaveCount(0);
  // ⌘K works too, and Enter opens the top result.
  await page.keyboard.press('Meta+KeyK');
  await shellSearch.fill('profile');
  await shellSearch.press('Enter');
  await expect(page).toHaveURL('/account/');
  await expect(shell).toHaveCount(0);
});

test('the launcher shortcut is set once, in Settings', async ({ page }) => {
  await page.goto('/settings/');
  const field = page.getByRole('textbox', { name: 'Launcher shortcut' });
  await expect(field).toHaveValue('Mod+K');
  await expect(page.locator('#launcher-shortcut-hint kbd')).toHaveText('Ctrl+K');
  await field.fill('shift+k');
  await field.press('Tab');
  await expect(field).toHaveAttribute('aria-invalid', 'true');
  await field.fill('ctrl+alt+k');
  await field.press('Tab');
  await expect(field).toHaveValue('Ctrl+Alt+K');
  await expect(page.getByText('Saved in this browser.')).toBeVisible();

  // The new shortcut focuses the launcher widget on the dashboard...
  await page.goto('/');
  await expect(search(page)).toBeVisible();
  const shell = page.getByRole('dialog', { name: 'App launcher' });
  await page.keyboard.press('Control+KeyK');
  await expect(search(page)).not.toBeFocused();
  await expect(shell).toHaveCount(0);
  await page.keyboard.press('Control+Alt+KeyK');
  await expect(search(page)).toBeFocused();
  await expect(search(page)).toHaveAttribute('aria-keyshortcuts', 'Control+Alt+K');
  await expect(shell).toHaveCount(0);
  // ...and opens the centred launcher elsewhere.
  await page.goto('/plugins/');
  await expect(page.getByRole('heading', { name: 'Plugins', exact: true })).toBeVisible();
  await page.keyboard.press('Control+KeyK');
  await expect(shell).toHaveCount(0);
  await page.keyboard.press('Control+Alt+KeyK');
  await expect(shell).toBeVisible();
  await page.keyboard.press('Escape');

  // Empty turns it off.
  await page.goto('/settings/');
  await field.fill('');
  await field.press('Tab');
  await page.goto('/');
  await expect(search(page)).toBeVisible();
  await expect(search(page)).not.toHaveAttribute('aria-keyshortcuts');
  await page.keyboard.press('Control+Alt+KeyK');
  await expect(search(page)).not.toBeFocused();
  await page.goto('/plugins/');
  await expect(page.getByRole('heading', { name: 'Plugins', exact: true })).toBeVisible();
  await page.keyboard.press('Control+Alt+KeyK');
  await expect(shell).toHaveCount(0);
});

test('pinning from the context menu puts apps first, persists and syncs to other windows', async ({ page, api }) => {
  await open(page);
  const other = await page.context().newPage();
  await install(other, api);
  await open(other);

  await appButton(page, 'All apps', 'Settings').click({ button: 'right' });
  const menu = launcher(page).getByRole('menu', { name: 'Actions for Settings' });
  await expect(menu.getByRole('menuitem', { name: 'Pin to top' })).toBeFocused();
  // No other display is available, so there is no "Open on" item.
  await expect(menu.getByRole('menuitem')).toHaveText(['Pin to top', 'App info']);
  await menu.getByRole('menuitem', { name: 'Pin to top' }).click();
  await expect(list(page, 'Pinned').getByRole('button')).toHaveText(['Settings']);
  await expect(appButton(page, 'All apps', 'Settings (pinned)')).toBeVisible();

  // The other window updates without a reload.
  await expect(list(other, 'Pinned').getByRole('button')).toHaveText(['Settings']);
  // Pinned apps come first in search results too.
  await search(page).fill('s');
  await expect(list(page, 'Results').getByRole('button').first()).toHaveAccessibleName('Settings (pinned)');

  // Unpinning in the other window updates this one.
  await appButton(other, 'Pinned', 'Settings').click({ button: 'right' });
  await launcher(other).getByRole('menuitem', { name: 'Unpin' }).click();
  await search(page).fill('');
  await expect(list(page, 'Pinned')).toHaveCount(0);

  await appButton(page, 'All apps', 'Plugins').click({ button: 'right' });
  await launcher(page).getByRole('menuitem', { name: 'Pin to top' }).click();
  await page.reload();
  await expect(list(page, 'Pinned').getByRole('button')).toHaveText(['Plugins']);
});

test('the context menu and app info work from the keyboard', async ({ page }) => {
  await open(page);
  await appButton(page, 'All apps', 'Settings').focus();
  await page.keyboard.press('Shift+F10');
  const menu = launcher(page).getByRole('menu');
  await expect(menu.getByRole('menuitem', { name: 'Pin to top' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(appButton(page, 'All apps', 'Settings')).toBeFocused();

  await page.keyboard.press('Shift+F10');
  await page.keyboard.press('ArrowDown');
  await expect(menu.getByRole('menuitem', { name: 'App info' })).toBeFocused();
  await page.keyboard.press('Enter');
  const info = page.getByRole('dialog', { name: 'Settings' });
  await expect(info).toBeVisible();
  await expect(info.getByText('Ember page /settings/')).toBeVisible();
  await expect(info.getByText('ember.settings')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(info).toBeHidden();
  await expect(appButton(page, 'All apps', 'Settings')).toBeFocused();
});

test('a touch long-press opens the context menu instead of launching', async ({ page }) => {
  await open(page);
  const button = appButton(page, 'All apps', 'Plugins');
  await button.dispatchEvent('pointerdown', { pointerType: 'touch', clientX: 10, clientY: 10, isPrimary: true });
  await expect(launcher(page).getByRole('menu', { name: 'Actions for Plugins' })).toBeVisible();
  await button.dispatchEvent('pointerup', { pointerType: 'touch' });
  await button.dispatchEvent('click');
  await expect(page).toHaveURL('/');
});

test('recently launched apps are listed', async ({ page }) => {
  await open(page);
  await expect(list(page, 'Recent')).toHaveCount(0);
  await appButton(page, 'All apps', 'Plugins').click();
  await expect(page).toHaveURL('/plugins/');
  await page.goto('/');
  await search(page).fill('settings');
  await search(page).press('Enter');
  await expect(page).toHaveURL('/settings/');
  await page.goto('/');
  await expect(list(page, 'Recent').getByRole('button')).toHaveText(['Settings', 'Plugins']);
});

test('apps registered at runtime appear without a reload; bad ones only warn', async ({ page }) => {
  const warnings: string[] = [], errors: string[] = [];
  page.on('console', m => { if (m.type() === 'warning') warnings.push(m.text()); });
  page.on('pageerror', e => errors.push(e.message));
  await open(page);

  await page.evaluate(() => {
    window.ember!.apps.register({
      id: 'test.kiln', name: 'Kiln monitor', category: 'Workshop', keywords: ['ceramics'],
      launch: { type: 'custom', run: () => { document.title = 'kiln launched'; } }
    });
    window.ember!.apps.register({ id: 'test.bad', name: '', launch: { type: 'url', url: 'javascript:alert(1)' } } as never);
    window.ember!.apps.register({ id: 'ember.notes', name: 'Duplicate', launch: { type: 'internal', path: '/' } });
  });
  await expect(appButton(page, 'All apps', 'Kiln monitor')).toBeVisible();
  await expect(launcher(page).getByRole('tab', { name: 'Workshop' })).toBeVisible();
  await expect(appButton(page, 'All apps', 'Duplicate')).toHaveCount(0);
  expect(warnings.some(w => w.includes('Ignored app "test.bad"') && w.includes('"name" is required'))).toBe(true);
  expect(warnings.some(w => w.includes('Ignored app "ember.notes"') && w.includes('already registered'))).toBe(true);

  await search(page).fill('ceramics');
  await search(page).press('Enter');
  await expect(page).toHaveTitle('kiln launched');

  await page.evaluate(() => window.ember!.apps.unregister('test.kiln'));
  await search(page).fill('');
  await expect(appButton(page, 'All apps', 'Kiln monitor')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('stays responsive with 250 registered apps', async ({ page }) => {
  await open(page);
  await page.evaluate(() => {
    for (let i = 0; i < 250; i++)
      window.ember!.apps.register({ id: `bulk.${i}`, name: `Bulk app ${i}`, category: `Group ${i % 8}`, keywords: [`tag${i % 25}`],
        launch: { type: 'internal', path: '/' } });
  });
  await expect(list(page, 'All apps').getByRole('button')).toHaveCount(257);
  const started = Date.now();
  for (const q of ['b', 'bu', 'bulk', 'bulk app 1', 'bulk app 12']) await search(page).fill(q);
  // The exact name first, then names with a word starting "12", then substring and keyword (tag12) matches.
  const results = list(page, 'Results').getByRole('button');
  await expect(results).toHaveCount(20);
  expect(Date.now() - started).toBeLessThan(2000);
  expect((await results.locator('.name').allInnerTexts()).slice(0, 11)).toEqual(['Bulk app 12', 'Bulk app 120', 'Bulk app 121',
    'Bulk app 122', 'Bulk app 123', 'Bulk app 124', 'Bulk app 125', 'Bulk app 126', 'Bulk app 127', 'Bulk app 128', 'Bulk app 129']);
  await search(page).press('ArrowDown');
  await page.keyboard.press('End');
  await expect(results.last()).toBeFocused();
});

test('pinned apps that are not registered are hidden', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('ember:launcher:v1', JSON.stringify({
    version: 1, pinned: ['plugin.gone', 'ember.settings'], recent: ['plugin.gone'], seen: { 'plugin.gone': Date.now() }
  })));
  await open(page);
  await expect(list(page, 'Pinned').getByRole('button')).toHaveText(['Settings']);
  await expect(list(page, 'Recent')).toHaveCount(0);
});

test('the launcher is configured, removed and added back like any widget', async ({ page }) => {
  await open(page);
  // Compact mode. The shortcut is no longer a widget setting.
  await page.getByRole('button', { name: 'Configure App launcher' }).click();
  await expect(page.getByRole('textbox', { name: 'Keyboard shortcut' })).toHaveCount(0);
  await page.getByRole('radio', { name: /Compact/ }).check();
  await page.getByRole('button', { name: 'Done' }).click();

  const toggle = launcher(page).getByRole('button', { name: 'App launcher', exact: true });
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(search(page)).toHaveCount(0);
  await toggle.click();
  await expect(launcher(page).getByRole('dialog', { name: 'App launcher' })).toBeVisible();
  await expect(search(page)).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'App launcher' })).toHaveCount(0);
  await expect(toggle).toBeFocused();
  await toggle.click();
  await search(page).fill('plugins');
  await search(page).press('Enter');
  await expect(page).toHaveURL('/plugins/');

  // Remove it; the board remembers.
  await page.goto('/');
  await page.getByRole('button', { name: 'Remove App launcher' }).click();
  await expect(launcher(page)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Add widget' })).toBeFocused();
  await page.reload();
  await expect(page.getByText('No widgets here yet.')).toBeVisible();

  // Add it back: a new instance with the default configuration.
  await page.getByRole('button', { name: 'Add widget' }).click();
  await page.getByRole('list', { name: 'Available widgets' }).getByRole('button', { name: /App launcher/ }).click();
  await expect(search(page)).toBeVisible();
});

test('the widget size can be changed within its limits', async ({ page }) => {
  await open(page);
  const frame = page.locator('[data-widget="ember.launcher"]');
  const before = (await frame.boundingBox())!;
  await page.getByRole('button', { name: 'Configure App launcher' }).click();
  await page.getByRole('spinbutton', { name: 'Width (columns)' }).fill('9');
  await page.getByRole('spinbutton', { name: 'Width (columns)' }).press('Tab');
  await expect(page.getByRole('spinbutton', { name: 'Width (columns)' })).toHaveValue('4');
  await page.getByRole('button', { name: 'Done' }).click();
  expect((await frame.boundingBox())!.width).toBeGreaterThan(before.width * 1.5);
});

test('a placed widget that is not installed can be removed', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('ember:widgets:v1:main', JSON.stringify({
    version: 1, placements: [{ id: 'gone-1', widget: 'plugin.gone', config: {} }]
  })));
  await page.goto('/');
  await expect(page.getByText('The widget “plugin.gone” is not installed.')).toBeVisible();
  await page.getByRole('button', { name: 'Remove Unavailable widget' }).click();
  await expect(page.getByText('No widgets here yet.')).toBeVisible();
});

for (const theme of ['dark', 'light'] as const) {
  test(`open menus and results have no axe violations (${theme})`, async ({ page }) => {
    await page.addInitScript(t => localStorage.setItem('ember:prefs', JSON.stringify({ theme: t })), theme);
    await open(page);
    await search(page).fill('s');
    await appButton(page, 'Results', 'Settings').click({ button: 'right' });
    await expect(launcher(page).getByRole('menu')).toBeVisible();
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    expect(results.violations.map(v => `${v.id}: ${v.nodes.map(n => n.target.join(' ')).join(', ')}`)).toEqual([]);
  });
}
