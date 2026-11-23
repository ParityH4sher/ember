import { expect, test, type Page } from '@playwright/test';
import { execSync } from 'node:child_process';

// Settings for the real-infrastructure suite; see README.md next to this file.
export const env = {
  baseURL: process.env.EMBER_E2E_BASE_URL || '',
  user: process.env.EMBER_E2E_USER || '',
  password: process.env.EMBER_E2E_PASSWORD || '',
  adminUser: process.env.EMBER_E2E_ADMIN_USER || '',
  adminPassword: process.env.EMBER_E2E_ADMIN_PASSWORD || process.env.EMBER_E2E_PASSWORD || '',
  /** Shell command that restarts the API container, for the session-store test. */
  restartApi: process.env.EMBER_E2E_RESTART_API || ''
};

/** Skips the whole file unless the suite is pointed at a deployment. */
export function requireDeployment() {
  test.skip(!env.baseURL, 'Set EMBER_E2E_BASE_URL to run the real-infrastructure suite.');
}

/**
 * Signs in through the real identity provider: Ember's /login/ page, then
 * Authentik's identification and password stages, then back to Ember.
 * Authentik's flow UI uses open shadow roots, which Playwright's locators
 * pierce. Selectors follow Authentik 2026.8's default authentication flow.
 */
export async function signIn(page: Page, username: string, password: string, path = '/') {
  await page.goto(path);
  await expect(page).toHaveURL(/\/login\/\?returnTo=/);
  await page.getByRole('button', { name: /^Sign in with/ }).click();

  const ember = new URL(env.baseURL).host;
  await page.waitForURL(url => url.host !== ember, { timeout: 20_000 });
  const uid = page.locator('input[name="uidField"]');
  await uid.fill(username);
  // Some flows ask for the password on the same page; others on the next.
  const passwordField = page.locator('input[name="password"]');
  if (!(await passwordField.isVisible())) await uid.press('Enter');
  await passwordField.fill(password);
  await passwordField.press('Enter');

  // An explicit-consent flow shows one more page the first time.
  const consent = page.getByRole('button', { name: /^Continue$/ });
  await Promise.race([
    page.waitForURL(url => url.host === ember, { timeout: 20_000 }),
    consent.waitFor({ timeout: 20_000 }).then(() => consent.click())
  ]);
  await page.waitForURL(url => url.host === ember && !url.pathname.startsWith('/api/'), { timeout: 20_000 });
  await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
}

export function restartApi() {
  execSync(env.restartApi, { stdio: 'inherit', timeout: 120_000, shell: '/bin/sh' });
}
