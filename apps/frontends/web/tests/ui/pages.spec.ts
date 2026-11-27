import { ada, expect, signedIn, test } from './mock-api';

test.beforeEach(({ api }) => signedIn(api));

test('dashboard shows status, counts, projects and services', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('link', { name: /Core API/ }).or(page.getByText('Core API'))).toBeVisible();
  await expect(page.getByText('ok', { exact: true })).toBeVisible();
  await expect(page.getByText('Version test')).toBeVisible();
  await expect(page.getByText('1 / 2')).toBeVisible();
  await expect(page.getByText('1 need configuration')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Shelf bracket' })).toHaveAttribute('href', '/apps/notes/?project=bracket');
  const service = page.getByRole('link', { name: /Authentik/ });
  await expect(service).toHaveAttribute('href', 'https://auth.example.com/');
  await expect(service).toHaveAttribute('rel', 'noopener noreferrer');
});

test('the installed app starts on the desktop surface; the dashboard stays in the launcher', async ({ page, request }) => {
  const response = await request.get('/manifest.webmanifest');
  const manifestUrl = new URL(response.url());
  const manifest = await response.json();
  expect(manifest.start_url).toBe('./desktop/');
  // start_url resolves against the manifest's URL, and must stay in scope.
  const start = new URL(manifest.start_url, manifestUrl);
  expect(start.pathname).toBe('/desktop/');
  expect(start.pathname.startsWith(new URL(manifest.scope, manifestUrl).pathname)).toBe(true);
  await page.goto(start.pathname);
  await expect(page.getByRole('heading', { name: 'Desktop', exact: true })).toBeVisible();
  await page.keyboard.press('Control+KeyK');
  const launcher = page.getByRole('dialog', { name: 'App launcher' });
  await launcher.getByRole('button', { name: 'Dashboard', exact: true }).click();
  await expect(page).toHaveURL('/');
});

test('account page shows the identity, session and provider link', async ({ page }) => {
  await page.goto('/account/');
  await expect(page.locator('#profile').getByText(ada.email!)).toBeVisible();
  await expect(page.locator('#profile').getByText('makers')).toBeVisible();
  await expect(page.getByText('Authentik (OpenID Connect)')).toBeVisible();
  await expect(page.locator('time')).toHaveAttribute('datetime', '2030-01-01T08:00:00.000Z');
  await expect(page.getByRole('link', { name: /Manage your account at Authentik/ }))
    .toHaveAttribute('href', 'https://auth.example.com/if/user/');
});

test('account page hides an unsafe provider link', async ({ page, api }) => {
  api.provider = { name: 'Authentik', accountUrl: 'javascript:alert(1)' };
  await page.goto('/account/');
  await expect(page.locator('#profile').getByText(ada.email!)).toBeVisible();
  await expect(page.getByRole('link', { name: /Manage your account/ })).toHaveCount(0);
});

test('theme and density apply at once and survive a reload', async ({ page }) => {
  await page.goto('/settings/');
  const html = page.locator('html');
  await page.getByRole('radio', { name: /^Light/ }).check();
  await expect(html).toHaveAttribute('data-theme', 'light');
  await page.getByRole('radio', { name: /^Compact/ }).check();
  await expect(html).toHaveAttribute('data-density', 'compact');
  await expect(page.getByText('Saved in this browser.')).toBeVisible();
  await page.reload();
  // Applied by the inline script before the app starts.
  await expect(html).toHaveAttribute('data-theme', 'light');
  await expect(page.getByRole('radio', { name: /^Light/ })).toBeChecked();
  await page.getByRole('button', { name: 'Reset to defaults' }).click();
  await expect(html).toHaveAttribute('data-theme', 'system');
  await expect(html).toHaveAttribute('data-density', 'comfortable');
});

test('unknown paths show the app’s 404 page inside the shell', async ({ page }) => {
  const response = await page.goto('/no-such-page/');
  expect(response?.status()).toBe(404);
  await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
  await page.getByRole('link', { name: 'Go to the dashboard' }).click();
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
});

test('the sidebar marks the current page', async ({ page }) => {
  await page.goto('/plugins/');
  const nav = page.getByRole('navigation', { name: 'Main' });
  await expect(nav.getByRole('link', { name: 'Plugins' })).toHaveAttribute('aria-current', 'page');
  await expect(nav.getByRole('link', { name: 'Dashboard' })).not.toHaveAttribute('aria-current', 'page');
});

test.describe('session ending while a page is open', () => {
  test('keeps an unsaved note on screen and offers to sign in again', async ({ page, api }) => {
    await page.goto('/apps/notes/');
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.getByLabel('Markdown source').fill('# Shelf bracket\n\nUnsaved thought.');
    api.expiredPaths.add('/v1/projects/bracket/notes');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'Your session has ended.' })).toBeVisible();
    // Still on the page with the draft intact.
    await expect(page).toHaveURL(/\/apps\/notes\//);
    await expect(page.getByLabel('Markdown source')).toHaveValue('# Shelf bracket\n\nUnsaved thought.');

    // Signing in again asks before discarding the draft.
    page.once('dialog', dialog => dialog.dismiss());
    await page.getByRole('button', { name: 'Sign in again' }).click();
    await expect(page.getByLabel('Markdown source')).toHaveValue('# Shelf bracket\n\nUnsaved thought.');
  });

  test('on other pages the banner appears and sign-in returns to the page', async ({ page, api }) => {
    await page.goto('/plugins/');
    await expect(page.getByRole('heading', { name: 'Immich' })).toBeVisible();
    api.expiredPaths.add('/v1/plugins/immich/api');
    await page.locator('summary', { hasText: /^Operations$/ }).first().click();
    await expect(page.getByRole('alert').filter({ hasText: 'Your session has ended.' })).toBeVisible();
    api.expiredPaths.clear();
    await page.getByRole('button', { name: 'Sign in again' }).click();
    await expect(page).toHaveURL('/plugins/');
    await expect(page.getByRole('alert').filter({ hasText: 'Your session has ended.' })).toHaveCount(0);
  });
});

test.describe('sign-in temporarily unavailable (API answers 503)', () => {
  test('shows a retry page instead of the app, then recovers', async ({ page, api }) => {
    api.sessionUnavailable = true;
    await page.goto('/plugins/');
    await expect(page.getByRole('heading', { name: 'Sign-in is temporarily unavailable' })).toBeVisible();
    await expect(page).toHaveURL('/plugins/');
    api.sessionUnavailable = false;
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByRole('heading', { name: 'Plugins', level: 1 })).toBeVisible();
  });
});
