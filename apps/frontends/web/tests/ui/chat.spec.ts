import AxeBuilder from '@axe-core/playwright';
import { test, expect, signedIn, emit, finishRun, type MockState } from './mock-api';
import type { Page } from '@playwright/test';

// Project chat in the Notes app (ADR 015): live text from the event stream,
// steering a running session agent, stopping it and answering its permission
// requests. Turn agents run once per message and cannot be steered.

const chatId = '00000000000000c1';

async function openChat(page: Page, api: MockState, agent: 'coder' | 'fixture') {
  signedIn(api);
  api.ai.operatorAgents.push({ id: 'coder', name: 'Coder', mode: 'session' });
  api.chats[chatId] = {
    id: chatId, title: 'Bracket loads', agent, mode: agent === 'coder' ? 'session' : 'turn', running: false,
    messages: [], permissions: [], events: [], steered: [], replies: [], interrupts: []
  };
  await page.goto(`/apps/notes/?project=bracket&chat=${chatId}`);
  await expect(page.locator('#chat-title')).toHaveText('Bracket loads');
}

test('a session agent streams its answer, takes more input and asks for permission', async ({ page, api }) => {
  await openChat(page, api, 'coder');
  const chat = api.chats[chatId];
  const input = page.getByRole('textbox', { name: 'Message' });

  await input.fill('How thick must the bracket be?');
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Agent is working.' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Stop agent' })).toBeVisible();

  // Live text arrives as deltas while the run is active.
  emit(api, chatId, { type: 'text.delta', delta: 'Checking ' }, { type: 'text.delta', delta: 'the loads…' },
    { type: 'tool.called' });
  await expect(page.locator('#live-text')).toHaveText('Checking the loads…');
  await expect(page.getByRole('region', { name: /Tool activity/ })).toContainText('Using a tool');

  // Sending while it runs steers the run instead of waiting.
  await input.fill('Assume 20 kg.');
  const steer = page.getByRole('button', { name: 'Send to running agent' });
  await expect(steer).toBeEnabled();
  await steer.click();
  await expect(page.getByRole('status').filter({ hasText: 'Message sent to the running agent.' })).toBeVisible();
  expect(chat.steered).toEqual(['Assume 20 kg.']);
  await expect(input).toHaveValue('');

  // A permission request, allowed once.
  chat.permissions.push({ id: 'per_1', action: 'edit', resources: ['loads.md'] });
  emit(api, chatId, { type: 'permission.asked' });
  const request = page.getByRole('region', { name: 'The agent asks for permission' });
  await expect(request).toContainText('edit: loads.md');
  await request.getByRole('button', { name: 'Allow once' }).click();
  await expect(request).toHaveCount(0);
  expect(chat.replies).toEqual(['per_1:once']);

  // The run ends: the transcript copy replaces the live text.
  finishRun(api, chatId, 'Use 4 mm steel.');
  await expect(page.locator('#transcript')).toContainText('Use 4 mm steel.');
  await expect(page.locator('#live-text')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Stop agent' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeVisible();
});

test('Stop interrupts the run and reports what happened', async ({ page, api }) => {
  await openChat(page, api, 'coder');
  const chat = api.chats[chatId];
  await page.getByRole('textbox', { name: 'Message' }).fill('Write a long report.');
  await page.keyboard.press('Control+Enter');
  const stop = page.getByRole('button', { name: 'Stop agent' });
  await stop.click();
  await expect(page.getByRole('status').filter({ hasText: 'Stopped the agent.' })).toBeVisible();
  expect(chat.interrupts).toEqual([true]);
  await expect(page.locator('#transcript')).toContainText('The run was stopped.');
  await expect(stop).toHaveCount(0);
});

test('Stop says so when the run had already ended', async ({ page, api }) => {
  await openChat(page, api, 'coder');
  const chat = api.chats[chatId];
  await page.getByRole('textbox', { name: 'Message' }).fill('Quick question.');
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop agent' })).toBeVisible();
  // The run ends on the server just before the interrupt reaches it.
  await page.route(/\/chats\/[0-9a-f]+\/interrupt$/, route => {
    chat.running = false;
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ interrupted: false }) });
  });
  await page.getByRole('button', { name: 'Stop agent' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'The agent was not running, so nothing was stopped.' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Stop agent' })).toHaveCount(0);
});

test('a turn agent cannot be messaged again until it answers', async ({ page, api }) => {
  await openChat(page, api, 'fixture');
  const input = page.getByRole('textbox', { name: 'Message' });
  await input.fill('First question.');
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  await input.fill('Second question.');
  await expect(page.getByRole('button', { name: 'Waiting for answer…' })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Send to running agent' })).toHaveCount(0);
  finishRun(api, chatId, 'Fixture answer.');
  await expect(page.locator('#transcript')).toContainText('Fixture answer.');
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeEnabled();
});

test('without the event stream the chat falls back to slow polling', async ({ page, api }) => {
  await openChat(page, api, 'coder');
  const chat = api.chats[chatId];
  chat.streamDown = true;
  await page.getByRole('textbox', { name: 'Message' }).fill('Are you there?');
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Live updates disconnected. Reconnecting…' }))
    .toBeVisible({ timeout: 10_000 });
  const pending = chat.messages.find(m => m.status === 'pending')!;
  Object.assign(pending, { status: 'completed', content: 'Polled answer.' });
  chat.running = false;
  chat.events = [];
  await expect(page.locator('#transcript')).toContainText('Polled answer.', { timeout: 10_000 });
});

for (const theme of ['dark', 'light'] as const) {
  test(`a running chat with a permission request has no axe violations (${theme})`, async ({ page, api }) => {
    await page.addInitScript(t => localStorage.setItem('ember:prefs', JSON.stringify({ theme: t })), theme);
    await openChat(page, api, 'coder');
    const chat = api.chats[chatId];
    await page.getByRole('textbox', { name: 'Message' }).fill('Edit the loads note.');
    await page.getByRole('button', { name: 'Send', exact: true }).click();
    chat.permissions.push({ id: 'per_1', action: 'edit', resources: ['loads.md'] });
    emit(api, chatId, { type: 'text.delta', delta: 'Working on it' }, { type: 'tool.failed' }, { type: 'permission.asked' });
    await expect(page.getByRole('button', { name: 'Allow once' })).toBeVisible();
    await expect(page.locator('#live-text')).toBeVisible();
    await expect(page.getByRole('region', { name: /Tool activity/ })).toContainText('Tool failed');
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    expect(results.violations.map(v => `${v.id}: ${v.nodes.map(n => n.target.join(' ')).join(', ')}`)).toEqual([]);
  });
}
