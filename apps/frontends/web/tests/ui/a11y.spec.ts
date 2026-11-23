import AxeBuilder from '@axe-core/playwright';
import { expect, grace, menuButton, signedIn, test } from './mock-api';

// Automated WCAG 2.1 A/AA checks with axe-core on every page, in both themes.
// Axe finds a subset of problems; keyboard and screen-reader checks are
// separate tests or manual.
const pages = ['/login/', '/', '/apps/notes/', '/plugins/', '/settings/', '/account/', '/admin/', '/no-such-page/'];

for (const theme of ['dark', 'light'] as const) {
  for (const path of pages) {
    test(`${path} has no axe violations (${theme})`, async ({ page, api }) => {
      if (path !== '/login/') signedIn(api, grace, 'admin');
      await page.addInitScript(t => localStorage.setItem('ember:prefs', JSON.stringify({ theme: t })), theme);
      await page.goto(path);
      await expect(page.locator('h1')).toBeVisible();
      await page.waitForLoadState('networkidle');
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
      expect(results.violations.map(v => `${v.id}: ${v.nodes.map(n => n.target.join(' ')).join(', ')}`)).toEqual([]);
    });
  }
}

test('the user menu works from the keyboard', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  await menuButton(page).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeHidden();
  await expect(menuButton(page)).toBeFocused();
});

test('the skip link moves focus to the content', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('#content')).toBeFocused();
});
