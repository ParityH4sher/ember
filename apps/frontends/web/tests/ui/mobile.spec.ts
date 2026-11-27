import { expect, signedIn, test } from './mock-api';

// Runs in the "mobile" project (Pixel 7 viewport).
test('the sidebar is a drawer that closes after navigating', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/');
  const nav = page.getByRole('navigation', { name: 'Main' });
  const menu = page.getByRole('button', { name: 'Menu', exact: true });
  await expect(nav).toBeHidden();
  await menu.click();
  await expect(menu).toHaveAttribute('aria-expanded', 'true');
  await expect(nav).toBeVisible();
  await nav.getByRole('link', { name: 'Settings' }).click();
  await expect(page).toHaveURL('/settings/');
  await expect(nav).toBeHidden();
  // No horizontal scrolling at phone width.
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('the dashboard and its app launcher fit a phone', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/');
  const launcher = page.getByRole('region', { name: 'App launcher' });
  await expect(launcher.getByRole('button', { name: 'Authentik', exact: true })).toBeVisible();
  // A touch long-press opens the app menu instead of launching.
  await launcher.getByRole('button', { name: 'Settings', exact: true }).dispatchEvent('pointerdown', { pointerType: 'touch', isPrimary: true });
  await expect(launcher.getByRole('menu', { name: 'Actions for Settings' })).toBeInViewport();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('the sign-in page fits a phone', async ({ page, api: _ }) => {
  await page.goto('/login/');
  await expect(page.getByRole('button', { name: 'Sign in with Authentik' })).toBeInViewport();
});
