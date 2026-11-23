import { expect, test } from '@playwright/test';
import { env, requireDeployment } from './env';

// The deployed stack without signing in: gateway routing, the API through the
// gateway, the static web container, and that sign-in is enforced.
test.beforeEach(() => requireDeployment());

test('the gateway serves the API health check', async ({ request }) => {
  const response = await request.get('/api/healthz');
  expect(response.status()).toBe(200);
  expect(await response.json()).toMatchObject({ status: 'ok' });
});

test('sign-in is enforced and the provider is named', async ({ request }) => {
  const session = await (await request.get('/api/v1/auth/session')).json();
  expect(session).toMatchObject({ enabled: true, authenticated: false });
  expect(session.provider?.name, 'set providerName in auth.json').toBeTruthy();
  expect((await request.get('/api/v1/projects')).status()).toBe(401);
});

test('pages without a trailing slash redirect, unknown pages are 404 in the app', async ({ request, page }) => {
  for (const path of ['/login', '/admin', '/apps/notes']) {
    const response = await request.get(path, { maxRedirects: 0 });
    expect(response.status(), path).toBe(308);
    expect(response.headers().location, path).toBe(`${path}/`);
  }
  const response = await page.goto('/no-such-page/');
  expect(response?.status()).toBe(404);
  await expect(page.getByRole('heading', { name: /Page not found|Sign in to Ember/ })).toBeVisible();
});

test('a signed-out visit lands on the sign-in page with one button', async ({ page }) => {
  await page.goto('/admin/');
  await expect(page).toHaveURL(/\/login\/\?returnTo=%2Fadmin%2F$/);
  await expect(page.getByRole('button')).toHaveCount(1);
  await expect(page.getByRole('button', { name: /^Sign in with / })).toBeVisible();
});

test('the API refuses cross-site state changes', async ({ request }) => {
  const response = await request.post('/api/v1/auth/logout', {
    headers: { Origin: 'https://evil.example', 'Sec-Fetch-Site': 'cross-site' }
  });
  expect(response.status()).toBe(403);
});

test.describe('over HTTPS', () => {
  test.skip(!env.baseURL.startsWith('https:'), 'Cookie prefix checks need HTTPS.');
  test('the login state cookie is Secure, HttpOnly and __Host- prefixed', async ({ request }) => {
    const response = await request.get('/api/v1/auth/login?returnTo=%2F', { maxRedirects: 0 });
    expect([302, 303]).toContain(response.status());
    const cookies = response.headersArray().filter(h => h.name.toLowerCase() === 'set-cookie').map(h => h.value);
    expect(cookies.length).toBeGreaterThan(0);
    for (const cookie of cookies) {
      expect(cookie).toMatch(/^__Host-/);
      expect(cookie).toMatch(/;\s*Secure/i);
      expect(cookie).toMatch(/;\s*HttpOnly/i);
    }
  });
});
