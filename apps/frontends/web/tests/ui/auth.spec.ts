import { ada, expect, grace, menuButton, signedIn, test } from './mock-api';

test.describe('sign-in page', () => {
  test('signed-out visitors land on /login/ with one Authentik button and return where they were', async ({ page, api }) => {
    await page.goto('/settings/');
    await expect(page).toHaveURL('/login/?returnTo=%2Fsettings%2F');
    await expect(page.getByRole('heading', { name: 'Sign in to Ember' })).toBeVisible();
    // Exactly one way in.
    await expect(page.getByRole('button')).toHaveCount(1);
    await page.getByRole('button', { name: 'Sign in with Authentik' }).click();
    await expect(page).toHaveURL('/settings/');
    await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
    expect(api.calls).toContain('GET /v1/auth/login');
  });

  test('uses the provider name the API reports', async ({ page, api }) => {
    api.provider = { name: 'Keycloak' };
    await page.goto('/login/');
    await expect(page.getByRole('button', { name: 'Sign in with Keycloak' })).toBeVisible();
  });

  test('falls back to a generic label without a provider name', async ({ page, api }) => {
    api.provider = {};
    await page.goto('/login/');
    await expect(page.getByRole('button', { name: 'Sign in with single sign-on' })).toBeVisible();
  });

  for (const unsafe of ['//evil.example/', 'https://evil.example/', '/\\evil.example']) {
    test(`ignores an off-site returnTo (${unsafe})`, async ({ page, api: _ }) => {
      await page.goto('/login/?' + new URLSearchParams({ returnTo: unsafe }));
      await page.getByRole('button', { name: /^Sign in with/ }).click();
      await expect(page).toHaveURL('/');
      await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    });
  }

  test('a signed-in visitor is sent on from /login/', async ({ page, api }) => {
    signedIn(api);
    await page.goto('/login/?returnTo=%2Faccount%2F');
    await expect(page).toHaveURL('/account/');
  });

  test('without sign-in configured, the page says so and the app opens in local mode', async ({ page, api }) => {
    api.enabled = false;
    await page.goto('/login/');
    await expect(page.getByText('Sign-in is not configured')).toBeVisible();
    await page.getByRole('link', { name: 'Continue to Ember' }).click();
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByText('Local mode')).toBeVisible();
    // Without identities every caller is an admin.
    await expect(page.getByRole('link', { name: 'Admin' })).toBeVisible();
  });
});

test.describe('user menu and sign-out', () => {
  test('shows who is signed in and signs out to /login/', async ({ page, api }) => {
    signedIn(api);
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByText('Welcome back, Ada.')).toBeVisible();
    await menuButton(page, 'Ada Lovelace').click();
    await expect(page.getByText('Role: user')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByText('Role: user')).toBeHidden();
    await menuButton(page, 'Ada Lovelace').click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page).toHaveURL('/login/');
    expect(api.calls).toContain('POST /v1/auth/logout');
  });

  test('menu links open profile and settings', async ({ page, api }) => {
    signedIn(api);
    await page.goto('/');
    await menuButton(page).click();
    await page.getByRole('link', { name: 'Profile & account' }).last().click();
    await expect(page).toHaveURL('/account/');
  });
});

test.describe('roles', () => {
  test('a user sees no Admin entry, and /admin/ explains why', async ({ page, api }) => {
    signedIn(api, ada, 'user');
    await page.goto('/');
    await expect(page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Admin' })).toHaveCount(0);
    await page.waitForLoadState('networkidle');
    api.calls.length = 0;
    await page.goto('/admin/');
    await expect(page.getByRole('heading', { name: 'Admins only' })).toBeVisible();
    // No admin data was requested.
    expect(api.calls).not.toContain('GET /v1/agents');
  });

  test('a user can run plugins but not configure them', async ({ page, api }) => {
    signedIn(api, ada, 'user');
    await page.goto('/plugins/');
    await expect(page.getByRole('heading', { name: 'Immich' })).toBeVisible();
    await expect(page.locator('#content summary', { hasText: /^Settings$/ })).toHaveCount(0);
    await expect(page.locator('#content summary', { hasText: /^Operations$/ })).toHaveCount(2);
    await expect(page.getByText('An Ember admin must configure and activate this plugin')).toBeVisible();
  });

  test('an admin sees the instance overview', async ({ page, api }) => {
    signedIn(api, grace, 'admin');
    await page.goto('/');
    await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Admin' }).click();
    await expect(page).toHaveURL('/admin/');
    const table = page.getByRole('table');
    await expect(table.getByRole('row', { name: /Immich/ })).toContainText('active');
    await expect(table.getByRole('row', { name: /Printer/ })).toContainText('needs configuration');
    await expect(page.getByText('Fixture agent')).toBeVisible();
  });
});
