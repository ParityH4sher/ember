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

test('the sign-in page fits a phone', async ({ page, api: _ }) => {
  await page.goto('/login/');
  await expect(page.getByRole('button', { name: 'Sign in with Authentik' })).toBeInViewport();
});

test('the launcher circle scales down and its launcher fits a phone', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/desktop/');
  const circle = page.getByRole('button', { name: 'Open app launcher' });
  const box = (await circle.boundingBox())!;
  expect(box.width).toBeGreaterThanOrEqual(56);
  expect(box.width).toBeLessThan(80);
  await circle.tap();
  const panel = (await page.getByRole('dialog', { name: 'App launcher' }).boundingBox())!;
  expect(panel.x).toBeGreaterThanOrEqual(0);
  expect(panel.x + panel.width).toBeLessThanOrEqual(page.viewportSize()!.width);
});
