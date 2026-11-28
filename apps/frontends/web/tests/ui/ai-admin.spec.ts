import { test, expect, signedIn, grace } from './mock-api';

// Admin → Agents & models (ADR 015): gateway models offered under a display
// name, and the agents that use them. Providers and keys live in the gateway.

test('an admin offers a gateway model and adds a router agent', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  api.ai.gateway = ['claude-sonnet', 'qwen3-8b'];
  await page.goto('/admin/ai/');
  await expect(page.getByRole('heading', { name: 'Agents & models' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /provider/i })).toHaveCount(0);

  // The gateway's models, offered with a click and a display name.
  const gateway = page.locator('#gateway');
  await gateway.getByRole('button', { name: 'Offer qwen3-8b' }).click();
  const models = page.locator('#models');
  await expect(models.getByLabel('Gateway model')).toHaveValue('qwen3-8b');
  await expect(models.getByLabel('Display name')).toBeFocused();
  await models.getByLabel('Display name').fill('Qwen 3 8B');
  await models.getByRole('button', { name: 'Add model' }).click();
  await expect(models.getByRole('status')).toHaveText('Saved Qwen 3 8B.');
  await expect(models.getByRole('row', { name: /Qwen 3 8B/ })).toContainText('qwen3-8b');
  await expect(gateway.getByRole('button', { name: 'qwen3-8b, already offered' })).toBeDisabled();
  expect(api.ai.models).toEqual([{ id: 'qwen3-8b', name: 'Qwen 3 8B' }]);

  // A router agent: the Codex harness image, using that model.
  const agents = page.locator('#agents');
  await expect(agents).toContainText('Fixture agent');
  await agents.getByLabel('Name').fill('Codex');
  await agents.getByLabel('Harness').selectOption('codex');
  await expect(agents.getByLabel('Worker')).toHaveCount(0);
  await agents.getByLabel('Router agent').fill('codex-cli');
  await agents.getByLabel(/Qwen 3 8B/).check();
  await agents.getByRole('button', { name: 'Add agent' }).click();
  await expect(agents.getByRole('row', { name: /Codex codex/ })).toContainText('Qwen 3 8B');
  await expect(agents.getByRole('row', { name: /Codex codex/ })).toContainText('router agent codex-cli');
  expect(api.ai.agents[0]).toMatchObject({ id: 'codex', harness: 'codex', routerAgent: 'codex-cli', models: ['qwen3-8b'], enabled: true });
  expect(api.ai.agents[0]).not.toHaveProperty('worker');

  // The model is in use, so it cannot be deleted yet.
  page.once('dialog', d => d.accept());
  await models.getByRole('row', { name: /Qwen 3 8B/ }).getByRole('button', { name: 'Delete' }).click();
  await expect(models.getByRole('status')).toContainText('an agent uses this model');
});

test('an OpenCode agent runs on a worker instead of a router agent', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  api.ai.workers = [{ id: 'opencode', harness: 'opencode' }];
  await page.goto('/admin/ai/');
  const agents = page.locator('#agents');
  await agents.getByLabel('Name').fill('Coder');
  await agents.getByLabel('Harness').selectOption('opencode');
  await expect(agents.getByLabel('Router agent')).toHaveCount(0);
  await agents.getByLabel('Worker').selectOption('opencode');
  await agents.getByRole('button', { name: 'Add agent' }).click();
  await expect(agents.getByRole('row', { name: /Coder coder/ })).toContainText('worker opencode');
  expect(api.ai.agents[0]).toMatchObject({ id: 'coder', harness: 'opencode', worker: 'opencode' });
  expect(api.ai.agents[0]).not.toHaveProperty('routerAgent');

  // Editing keeps the worker selected.
  await agents.getByRole('row', { name: /Coder coder/ }).getByRole('button', { name: 'Edit' }).click();
  await expect(agents.getByLabel('Worker')).toHaveValue('opencode');
});

test('without a gateway the page says where providers are configured', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  await page.goto('/admin/ai/');
  const gateway = page.locator('#gateway');
  await expect(gateway).toContainText('configured in the LiteLLM model gateway');
  await expect(gateway).toContainText('docs/models.md');
  expect(api.calls).not.toContain('GET /v1/admin/ai/gateway/models');

  // A model can still be added by its gateway name.
  const models = page.locator('#models');
  await models.getByLabel('Gateway model').fill('claude-sonnet');
  await models.getByLabel('Display name').fill('Claude Sonnet');
  await models.getByRole('button', { name: 'Add model' }).click();
  await expect(models.getByRole('row', { name: /Claude Sonnet/ })).toContainText('claude-sonnet');
});

test('people without the admin role cannot open Agents & models', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/admin/ai/');
  await expect(page.getByRole('heading', { name: 'Admins only' })).toBeVisible();
  expect(api.calls).not.toContain('GET /v1/admin/ai');
  await expect(page.getByRole('link', { name: 'Agents & models' })).toHaveCount(0);
});
