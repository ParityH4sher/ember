import { defineConfig } from '@playwright/test';

// Unit suite: plain TypeScript modules (registries, search, ordering,
// keyboard maths), run in Node by Playwright's test runner. No browser, build
// or server, so it takes about a second. Run: npm run test:unit. Svelte
// components are covered by the UI suite (playwright.config.ts).
export default defineConfig({
  testDir: 'tests/unit',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: 'list'
});
