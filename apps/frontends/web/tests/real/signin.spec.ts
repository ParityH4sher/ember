import { expect, test } from '@playwright/test';
import { env, requireDeployment, restartApi, signIn } from './env';

// Signs in through the real Authentik and uses the app as each role.
test.beforeEach(() => requireDeployment());

test.describe('as a user', () => {
  test.skip(() => !env.user || !env.password, 'Set EMBER_E2E_USER and EMBER_E2E_PASSWORD.');

  test('signs in, returns to the requested page, and sees their identity', async ({ page }) => {
    await signIn(page, env.user, env.password, '/account/');
    await expect(page).toHaveURL(/\/account\/$/);
    await expect(page.locator('#profile')).toContainText('user');
    await expect(page.locator('#session time')).toHaveAttribute('datetime', /\d{4}-\d\d-\d\dT/);
  });

  test('uses the dashboard, notes and plugins against the real API', async ({ page }) => {
    await signIn(page, env.user, env.password);
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByText('Version ')).toBeVisible();
    await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Project notes' }).click();
    await expect(page.getByRole('button', { name: 'Create project' })).toBeVisible();
    await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Plugins' }).click();
    await expect(page.getByRole('heading', { name: 'Plugins', level: 1 })).toBeVisible();
    await expect(page.getByText(/Unable to load/)).toHaveCount(0);
  });

  test('is not an admin', async ({ page }) => {
    test.skip(env.user === env.adminUser, 'EMBER_E2E_USER is the admin account.');
    await signIn(page, env.user, env.password, '/admin/');
    await expect(page.getByRole('heading', { name: 'Admins only' })).toBeVisible();
    // The API agrees, not only the page.
    const status = await page.evaluate(async () => (await fetch('/api/v1/plugins/immich/activation', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ active: false })
    })).status);
    expect([403, 404]).toContain(status);
  });

  test('signs out', async ({ page }) => {
    await signIn(page, env.user, env.password);
    await page.locator('summary', { hasText: 'Account menu for' }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    // Either Authentik's end-session page or straight back to Ember's sign-in page.
    await page.waitForURL(url => url.pathname !== '/', { timeout: 20_000 });
    const session = await (await page.request.get(new URL('/api/v1/auth/session', env.baseURL).href)).json();
    expect(session.authenticated).toBe(false);
  });

  test('stays signed in when the API restarts (Redis session store)', async ({ page }) => {
    test.skip(!env.restartApi, 'Set EMBER_E2E_RESTART_API to a command that restarts the API container.');
    await signIn(page, env.user, env.password);
    restartApi();
    await expect.poll(async () => (await page.request.get('/api/healthz').catch(() => null))?.status(),
      { timeout: 60_000 }).toBe(200);
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page).not.toHaveURL(/\/login\//);
  });
});

test.describe('as an admin', () => {
  test.skip(() => !env.adminUser || !env.adminPassword, 'Set EMBER_E2E_ADMIN_USER (and a password).');

  test('sees the admin page with live instance data', async ({ page }) => {
    await signIn(page, env.adminUser, env.adminPassword);
    await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Admin' }).click();
    await expect(page.getByRole('heading', { name: 'Admin', level: 1 })).toBeVisible();
    await expect(page.locator('#instance')).toContainText('ok');
    await expect(page.locator('#instance')).toContainText('OpenID Connect');
    await expect(page.getByText(/Unable to load/)).toHaveCount(0);
  });

  test('can open plugin settings', async ({ page }) => {
    await signIn(page, env.adminUser, env.adminPassword, '/plugins/');
    const settings = page.locator('#content summary', { hasText: /^Settings$/ });
    test.skip((await settings.count()) === 0, 'No plugins installed on this instance.');
    await settings.first().click();
    await expect(page.getByRole('button', { name: 'Save settings' }).first()).toBeVisible();
  });
});
