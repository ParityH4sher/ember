import { existsSync, readFileSync } from 'node:fs';
import { parseEnv } from 'node:util';
import { defineConfig, devices } from '@playwright/test';

// Real-infrastructure suite: a deployed Ember (gateway, API, web, Redis)
// signing in through a real Authentik. Nothing is mocked. Configure with
// EMBER_E2E_* variables, in the environment or in the repository's root .env
// (gitignored; see .env.example). Only EMBER_E2E_* lines are taken from that
// file, which also holds Ansible's operator secrets, and variables already set
// in the environment win. EMBER_E2E_ENV_FILE names another file instead
// (scripts/test-real.sh uses it). See tests/real/README.md.
const envFile = process.env.EMBER_E2E_ENV_FILE || new URL('../../../.env', import.meta.url).pathname;
if (existsSync(envFile)) {
  for (const [key, value] of Object.entries(parseEnv(readFileSync(envFile, 'utf8'))))
    if (key.startsWith('EMBER_E2E_') && process.env[key] === undefined) process.env[key] = value;
}
// An origin: scheme and host (and port), e.g. https://ember.example.com.
// A trailing slash is fine; a path is not, because Ember owns its host's root.
function origin(raw: string | undefined): string | undefined {
  if (!raw) return undefined;
  let url: URL;
  try { url = new URL(raw); } catch {
    throw new Error(`EMBER_E2E_BASE_URL must include the protocol, e.g. https://ember.example.com (got "${raw}")`);
  }
  if (!['https:', 'http:'].includes(url.protocol) || url.pathname !== '/' || url.search || url.hash || url.username)
    throw new Error(`EMBER_E2E_BASE_URL must be an origin like https://ember.example.com, without a path (got "${raw}")`);
  return url.origin;
}
const baseURL = origin(process.env.EMBER_E2E_BASE_URL);
process.env.EMBER_E2E_BASE_URL = baseURL ?? '';

export default defineConfig({
  testDir: 'tests/real',
  // Tests share accounts and server-side sessions; keep them in order.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report-real' }]],
  outputDir: 'test-results-real',
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // Self-signed lab certificates: opt in explicitly.
    ignoreHTTPSErrors: process.env.EMBER_E2E_IGNORE_HTTPS_ERRORS === '1'
  }
});
