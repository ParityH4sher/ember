import { defineConfig, devices } from '@playwright/test';

// UI suite: the built app on `vite preview`, with every /api call answered by
// tests/ui/mock-api.ts. Needs no API, identity provider or containers.
// Run: npm run test:ui. For the suite against a real deployment see
// playwright.real.config.ts.
export default defineConfig({
  testDir: 'tests/ui',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: { baseURL: 'http://127.0.0.1:4173', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] }, testIgnore: /mobile\.spec\.ts/ },
    { name: 'mobile', use: { ...devices['Pixel 7'] }, testMatch: /mobile\.spec\.ts/ }
  ],
  webServer: {
    command: 'npm run build && npx vite preview --host 127.0.0.1 --port 4173 --strictPort',
    url: 'http://127.0.0.1:4173/login/',
    reuseExistingServer: !process.env.CI,
    timeout: 180_000
  }
});
