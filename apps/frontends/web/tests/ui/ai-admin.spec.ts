import { test, expect, signedIn, grace } from './mock-api';

// Admin → Agents & models: providers, models and agents for project chat.

test('an admin connects Ollama and Claude and adds an agent', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  await page.goto('/admin/ai/');
  await expect(page.getByRole('heading', { name: 'Agents & models' })).toBeVisible();

  // A hosted provider: the key goes to the API and is never shown again.
  const providers = page.locator('#providers');
  await providers.getByLabel('Name').fill('Claude');
  await providers.getByLabel('Kind').selectOption('anthropic');
  await providers.getByLabel('API key').fill('sk-ant-test-secret');
  await providers.getByRole('button', { name: 'Add provider' }).click();
  await expect(providers.getByRole('status')).toHaveText('Saved Claude.');
  const claude = providers.getByRole('row', { name: /Claude claude/ });
  await expect(claude).toContainText('https://api.anthropic.com/v1');
  await expect(claude).toContainText('key set');
  await expect(page.locator('body')).not.toContainText('sk-ant-test-secret');
  expect(api.ai.providers[0]).toMatchObject({ id: 'claude', kind: 'anthropic', apiKey: 'sk-ant-test-secret' });

  // A LAN Ollama: fetch its models and add one with a click.
  await providers.getByLabel('Name').fill('Home Ollama');
  await providers.getByLabel('Kind').selectOption('ollama');
  await providers.getByLabel('Base URL').fill('http://ollama.lan:11434');
  await providers.getByRole('button', { name: 'Add provider' }).click();
  const ollama = providers.getByRole('row', { name: /Home Ollama/ });
  await expect(ollama).toContainText('local');
  await ollama.getByRole('button', { name: 'Fetch models' }).click();
  await expect(providers.getByText('Home Ollama offers 2 models.')).toBeVisible();
  await providers.getByRole('button', { name: 'qwen3:8b +' }).click();

  const models = page.locator('#models');
  await expect(models.getByLabel('Provider', { exact: true })).toHaveValue('home-ollama');
  await expect(models.getByLabel("Provider's model ID")).toHaveValue('qwen3:8b');
  await models.getByLabel('Name').fill('Qwen 3 8B');
  await models.getByRole('button', { name: 'Add model' }).click();
  await expect(models.getByRole('row', { name: /Qwen 3 8B/ })).toContainText('Home Ollama');
  await expect(providers.getByRole('button', { name: 'qwen3:8b ✓' })).toBeDisabled();

  // An agent: the Codex harness image, using that model.
  const agents = page.locator('#agents');
  await expect(agents).toContainText('Fixture agent');
  await agents.getByLabel('Name').fill('Codex');
  await agents.getByLabel('Harness').selectOption('codex');
  await agents.getByLabel('Router agent').fill('codex-cli');
  await agents.getByLabel(/Qwen 3 8B/).check();
  await agents.getByRole('button', { name: 'Add agent' }).click();
  await expect(agents.getByRole('row', { name: /Codex codex/ })).toContainText('Qwen 3 8B');
  expect(api.ai.agents[0]).toMatchObject({ id: 'codex', harness: 'codex', routerAgent: 'codex-cli', models: ['qwen3-8b'], enabled: true });

  // The model is in use, so it cannot be deleted yet.
  page.once('dialog', d => d.accept());
  await models.getByRole('row', { name: /Qwen 3 8B/ }).getByRole('button', { name: 'Delete' }).click();
  await expect(models.getByRole('status')).toContainText('an agent uses this model');
});

test('editing a provider keeps its key unless replaced or removed', async ({ page, api }) => {
  signedIn(api, grace, 'admin');
  api.ai.providers.push({ id: 'openai', name: 'OpenAI', kind: 'openai', baseURL: 'https://api.openai.com/v1', local: false, apiKey: 'sk-old' });
  await page.goto('/admin/ai/');
  const providers = page.locator('#providers');
  await providers.getByRole('row', { name: /OpenAI/ }).getByRole('button', { name: 'Edit' }).click();
  await expect(providers.getByLabel('API key')).toHaveValue('');
  await expect(providers.getByLabel('API key')).toHaveAttribute('placeholder', 'Stored; type to replace');
  await providers.getByLabel('Name').fill('OpenAI (Codex)');
  await providers.getByRole('button', { name: 'Save provider' }).click();
  await expect(providers.getByRole('row', { name: /OpenAI \(Codex\)/ })).toContainText('key set');
  expect(api.ai.providers[0].apiKey).toBe('sk-old');

  await providers.getByRole('row', { name: /OpenAI/ }).getByRole('button', { name: 'Edit' }).click();
  await providers.getByLabel('Remove the stored key').check();
  await providers.getByRole('button', { name: 'Save provider' }).click();
  await expect(providers.getByRole('row', { name: /OpenAI/ })).toContainText('needs key');
});

test('people without the admin role cannot open Agents & models', async ({ page, api }) => {
  signedIn(api);
  await page.goto('/admin/ai/');
  await expect(page.getByRole('heading', { name: 'Admins only' })).toBeVisible();
  expect(api.calls).not.toContain('GET /v1/admin/ai');
  await expect(page.getByRole('link', { name: 'Agents & models' })).toHaveCount(0);
});
