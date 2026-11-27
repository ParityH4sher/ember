import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';
import { expect, install, signedIn, test } from './mock-api';

// The central launcher circle on the desktop surface (lib/desktop).

test.beforeEach(({ api }) => signedIn(api));

const circle = (page: Page) => page.getByRole('button', { name: 'Open app launcher' });
const launcher = (page: Page) => page.getByRole('dialog', { name: 'App launcher' });

/** Stubs the Window Management API with permission granted. */
async function screens(page: Page, count: number, isPrimary: boolean) {
  await page.addInitScript(([count, isPrimary]) => {
    const details = Object.assign(new EventTarget(), {
      screens: Array.from({ length: count }, () => ({})),
      currentScreen: { isPrimary }
    });
    (window as unknown as { screenDetails: typeof details }).screenDetails = details;
    (window as unknown as { getScreenDetails: () => Promise<typeof details> }).getScreenDetails = async () => details;
    const query = navigator.permissions.query.bind(navigator.permissions);
    navigator.permissions.query = (d: PermissionDescriptor) => (d.name as string) === 'window-management'
      ? Promise.resolve(Object.assign(new EventTarget(), { state: 'granted' }) as PermissionStatus)
      : query(d);
  }, [count, isPrimary] as const);
}
/** Changes the stubbed screens and fires `type` on ScreenDetails. */
function change(page: Page, type: string, next: { count?: number; isPrimary?: boolean }) {
  return page.evaluate(([type, next]) => {
    const d = (window as unknown as { screenDetails: { screens: object[]; currentScreen: { isPrimary: boolean } } & EventTarget }).screenDetails;
    if (next.count !== undefined) d.screens = Array.from({ length: next.count }, () => ({}));
    if (next.isPrimary !== undefined) d.currentScreen = { isPrimary: next.isPrimary };
    d.dispatchEvent(new Event(type));
  }, [type, next] as const);
}

test('without multi-display support the circle is centred in the single window', async ({ page }) => {
  await page.goto('/desktop/');
  const box = await circle(page).boundingBox();
  const view = page.viewportSize()!;
  expect(box).not.toBeNull();
  expect(Math.abs(box!.x + box!.width / 2 - view.width / 2)).toBeLessThan(1);
  expect(Math.abs(box!.y + box!.height / 2 - view.height / 2)).toBeLessThan(1);
  expect(box!.width).toBeGreaterThanOrEqual(56);
  expect(box!.width).toBeLessThanOrEqual(96);
});

test('the circle opens the launcher; Escape closes it and refocuses the circle', async ({ page }) => {
  await page.goto('/desktop/');
  await circle(page).click();
  await expect(launcher(page)).toBeVisible();
  await expect(circle(page)).toHaveAttribute('aria-expanded', 'true');
  // The same launcher as the widget: apps from the app registry, search first.
  await expect(launcher(page).getByRole('button', { name: 'Project notes', exact: true })).toBeVisible();
  await expect(launcher(page).getByRole('searchbox', { name: 'Search apps' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(launcher(page)).toBeHidden();
  await expect(circle(page)).toBeFocused();
});

test('clicking outside closes the launcher and refocuses the circle', async ({ page }) => {
  await page.goto('/desktop/');
  await circle(page).click();
  const view = page.viewportSize()!;
  await page.mouse.click(view.width - 10, view.height - 10);
  await expect(launcher(page)).toBeHidden();
  await expect(circle(page)).toBeFocused();
});

test('the circle does not block clicks outside its own bounds', async ({ page }) => {
  await page.goto('/desktop/');
  const box = (await circle(page).boundingBox())!;
  const hit = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.closest('.circle-layer') !== null,
    [box.x - 8, box.y + box.height / 2] as const);
  expect(hit).toBe(false);
});

test('the shortcut focuses the circle and opens the launcher, and works on other pages', async ({ page }) => {
  await page.goto('/desktop/');
  await expect(circle(page)).toBeVisible();
  await page.keyboard.press('Control+k');
  await expect(launcher(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(circle(page)).toBeFocused();

  await page.goto('/settings/');
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
  await expect(circle(page)).toHaveCount(0);
  await page.keyboard.press('Control+k');
  await expect(launcher(page)).toBeVisible();
  await launcher(page).getByRole('button', { name: 'Desktop', exact: true }).click();
  await expect(page).toHaveURL('/desktop/');
  await expect(circle(page)).toBeVisible();
});

test('the circle can be hidden in Settings; the shortcut still opens the launcher', async ({ page }) => {
  await page.goto('/settings/');
  await page.getByRole('checkbox', { name: /Show launcher circle/ }).uncheck();
  await page.goto('/desktop/');
  await expect(page.getByRole('heading', { name: 'Desktop' })).toBeVisible();
  await expect(circle(page)).toHaveCount(0);
  await page.keyboard.press('Control+k');
  await expect(launcher(page)).toBeVisible();
});

test('only the window on the primary display shows the circle, and it follows display changes', async ({ page }) => {
  await screens(page, 2, false);
  await page.goto('/desktop/');
  await expect(page.getByRole('heading', { name: 'Desktop' })).toBeVisible();
  await expect(circle(page)).toHaveCount(0);
  await change(page, 'currentscreenchange', { isPrimary: true });
  await expect(circle(page)).toBeVisible();
  await change(page, 'screenschange', { isPrimary: false });
  await expect(circle(page)).toHaveCount(0);
  await change(page, 'screenschange', { count: 1 });
  await expect(circle(page)).toBeVisible();
});

test('exactly one window shows the circle and hands it over when it closes', async ({ page, api }) => {
  await page.goto('/desktop/');
  await expect(circle(page)).toBeVisible();
  const second = await page.context().newPage();
  await install(second, api);
  await second.goto('/desktop/');
  await expect(second.getByRole('heading', { name: 'Desktop' })).toBeVisible();
  await second.waitForTimeout(500);
  await expect(circle(second)).toHaveCount(0);
  await expect(circle(page)).toBeVisible();

  // Leaving the desktop surface gives the circle up.
  await page.goto('/settings/');
  await expect(circle(second)).toBeVisible();
  await page.close();
  await expect(circle(second)).toBeVisible();
});

test('a verified primary window wins over a window without the API', async ({ page, api }) => {
  await page.goto('/desktop/');
  await expect(circle(page)).toBeVisible();
  const second = await page.context().newPage();
  await install(second, api);
  await screens(second, 2, true);
  await second.goto('/desktop/');
  await expect(circle(second)).toBeVisible();
  await expect(circle(page)).toHaveCount(0);
});

test('the circle opens the registry launcher: search, app menu and info do not close it early', async ({ page }) => {
  await page.goto('/desktop/');
  await circle(page).click();
  const search = launcher(page).getByRole('searchbox', { name: 'Search apps' });
  await search.fill('notes');
  await expect(launcher(page).getByRole('list', { name: 'Results' }).getByRole('button')).toHaveText(['Project notes']);
  // Escape clears the search first, then closes and refocuses the circle.
  await page.keyboard.press('Escape');
  await expect(search).toHaveValue('');
  await expect(launcher(page)).toBeVisible();
  // The app menu and App info close on Escape without closing the launcher.
  const settings = launcher(page).getByRole('list', { name: 'All apps' }).getByRole('button', { name: 'Settings', exact: true });
  await settings.click({ button: 'right' });
  const menu = launcher(page).getByRole('menu', { name: 'Actions for Settings' });
  await expect(menu).toBeInViewport();
  const box = (await menu.boundingBox())!, item = (await settings.boundingBox())!;
  // Placed at the pointer, not offset by the centred panel.
  expect(Math.abs(box.x - (item.x + item.width / 2))).toBeLessThan(item.width);
  await menu.getByRole('menuitem', { name: 'App info' }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeHidden();
  await expect(launcher(page)).toBeVisible();
  await search.focus();
  await page.keyboard.press('Escape');
  await expect(launcher(page)).toBeHidden();
  await expect(circle(page)).toBeFocused();
});

for (const theme of ['dark', 'light'] as const) {
  test(`the open circle launcher has no axe violations (${theme})`, async ({ page }) => {
    await page.addInitScript(t => localStorage.setItem('ember:prefs', JSON.stringify({ theme: t })), theme);
    await page.goto('/desktop/');
    await circle(page).click();
    await expect(launcher(page).getByRole('button', { name: 'Authentik', exact: true })).toBeVisible();
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    expect(results.violations.map(v => `${v.id}: ${v.nodes.map(n => n.target.join(' ')).join(', ')}`)).toEqual([]);
  });
}
