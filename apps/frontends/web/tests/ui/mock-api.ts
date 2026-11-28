import { test as base, expect, type Page, type Route } from '@playwright/test';

// An in-memory stand-in for the Ember API, answering the routes the web app
// calls (contracts/openapi.yaml). Each test gets its own state and can change
// it, e.g. sign someone in or make a route answer 401.

export interface MockUser { subject: string; name?: string; email?: string; groups?: string[] }
export interface MockState {
  /** false = sign-in not configured (local mode). */
  enabled: boolean;
  /** The signed-in person, or null. */
  user: MockUser | null;
  role: 'admin' | 'user';
  /** Who signs in when the login route is followed. */
  next: { user: MockUser; role: 'admin' | 'user' };
  provider: { name?: string; accountUrl?: string };
  /** Paths (after /api) that answer 401, as if the session ended. */
  expiredPaths: Set<string>;
  notes: Record<string, string>;
  calls: string[];
  /** The session store is down: GET /v1/auth/session answers 503. */
  sessionUnavailable: boolean;
  /** The admin AI catalog (ember.ai-catalog/v2): no providers or keys. */
  ai: MockAI;
  /** Project chats of the bracket project, by ID (ADR 015). */
  chats: Record<string, MockChat>;
}

interface MockAI {
  models: { id: string; name: string }[];
  agents: { id: string; name: string; harness: string; routerAgent?: string; worker?: string; models: string[]; defaultModel?: string; enabled: boolean }[];
  /** Workers from the operator's agents file. */
  workers: { id: string; harness: string }[];
  /** The model gateway: null when not configured, else the model names it offers. */
  gateway: string[] | null;
  /** Agents from the operator's agents file, as GET /v1/agents lists them. */
  operatorAgents: { id: string; name: string; mode: 'session' | 'turn' }[];
}

export interface MockPermission { id: string; action: string; resources: string[] }
export interface MockChat {
  id: string; title: string; agent: string; mode: 'session' | 'turn'; running: boolean;
  messages: { id: string; role: 'user' | 'agent'; content: string; created: string; status?: string; error?: string }[];
  permissions: MockPermission[];
  /** Events waiting for the next connection to the chat's event stream. */
  events: { type: string; delta?: string }[];
  /** Messages that arrived while a session run was active. */
  steered: string[];
  /** Decisions sent for permissions, as "id:decision". */
  replies: string[];
  /** POST …/interrupt calls, with what they answered. */
  interrupts: boolean[];
  /** The events route answers 500, as if the API had no stream yet. */
  streamDown?: boolean;
}

// A small stand-in for /v1/admin/ai (apps/api/internal/httpapi/aiadmin.go).
function aiRoute(route: Route, s: MockState, method: string, path: string, body: () => Record<string, unknown>) {
  if (s.enabled && s.role !== 'admin')
    return json(route, { code: 'insufficient_role', message: 'this action needs the Ember admin role' }, 403);
  const ai = s.ai;
  if (path === '/v1/admin/ai' && method === 'GET') {
    return json(route, {
      persisted: true, chatEnabled: true,
      harnesses: ['claude-code', 'codex', 'opencode', 'custom'],
      workers: ai.workers, gateway: { configured: ai.gateway !== null },
      operatorAgents: ai.operatorAgents.map(({ id, name }) => ({ id, name })),
      models: ai.models, agents: ai.agents
    });
  }
  if (path === '/v1/admin/ai/gateway/models' && method === 'GET') {
    if (ai.gateway === null)
      return json(route, { code: 'gateway_unavailable', message: 'no model gateway is configured' }, 503);
    return json(route, { models: ai.gateway.map(id => ({ id })) });
  }
  const m = /^\/v1\/admin\/ai\/(models|agents)\/([^/]+)$/.exec(path);
  if (!m) return json(route, { code: 'not_found', message: 'no such route' }, 404);
  const [, kind] = m;
  const id = decodeURIComponent(m[2]);
  const list = (kind === 'models' ? ai.models : ai.agents) as { id: string }[];
  const index = list.findIndex(x => x.id === id);
  if (method === 'DELETE') {
    if (kind === 'models' && ai.agents.some(x => x.models.includes(id)))
      return json(route, { code: 'in_use', message: 'an agent uses this model; remove it from the agent first' }, 409);
    if (index >= 0) list.splice(index, 1);
    return route.fulfill({ status: 204 });
  }
  const input = body();
  let item: Record<string, unknown>;
  if (kind === 'models') item = { id, name: input.name };
  else {
    if (input.harness === 'opencode' && !ai.workers.some(w => w.id === input.worker))
      return json(route, { code: 'invalid_agent', message: 'harness opencode needs one of the configured workers' }, 400);
    if (input.harness === 'opencode' && input.routerAgent)
      return json(route, { code: 'invalid_agent', message: 'a worker agent has no router agent' }, 400);
    item = { ...input, id };
    if (!item.defaultModel) item.defaultModel = (item.models as string[])[0];
  }
  if (index >= 0) list[index] = item as never; else list.push(item as never);
  return json(route, item);
}

const chatView = (c: MockChat) => ({
  schema: 'ember.chat/v1', id: c.id, title: c.title, agent: c.agent, created: '2026-11-28T09:00:00Z',
  updated: '2026-11-28T09:00:00Z', messages: c.messages, mode: c.mode, running: c.running
});
const stamp = () => new Date().toISOString();
const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

/** Queues events for the chat's stream; they arrive within about 100 ms. */
export function emit(s: MockState, chat: string, ...events: { type: string; delta?: string }[]) {
  s.chats[chat].events.push(...events);
}
/** Ends the active run with an answer, as the API's transcript copy would. */
export function finishRun(s: MockState, chat: string, answer: string) {
  const c = s.chats[chat];
  const pending = c.messages.find(m => m.status === 'pending');
  if (pending) Object.assign(pending, { status: 'completed', content: answer });
  c.running = false;
  emit(s, chat, { type: 'run.succeeded' });
}

// A stand-in for /v1/projects/bracket/chats… (ADR 015). The event stream is
// served as finite responses: each connection gets "connected" and the queued
// events, and `retry: 100` makes the browser reconnect at once, so a test's
// emit() shows up on the next connection.
async function chatRoute(route: Route, s: MockState, method: string, path: string, body: () => Record<string, unknown>) {
  const agentMode = (id: string) => s.ai.operatorAgents.find(a => a.id === id)?.mode
    ?? (s.ai.agents.find(a => a.id === id)?.worker ? 'session' : 'turn');
  if (path === '/v1/projects/bracket/chats') {
    if (method === 'POST') {
      const agent = String(body().agent);
      const id = Object.keys(s.chats).length.toString(16).padStart(16, '0');
      s.chats[id] = { id, title: '', agent, mode: agentMode(agent), running: false, messages: [], permissions: [],
        events: [], steered: [], replies: [], interrupts: [] };
      return json(route, chatView(s.chats[id]), 201);
    }
    return json(route, { chats: Object.values(s.chats).map(c => ({
      id: c.id, title: c.title, agent: c.agent, created: '2026-11-28T09:00:00Z', updated: '2026-11-28T09:00:00Z',
      messageCount: c.messages.length, pending: c.running })) });
  }
  const m = /^\/v1\/projects\/bracket\/chats\/([0-9a-f]{16})(?:\/(messages|events|interrupt|permissions)(?:\/([^/]+))?)?$/.exec(path);
  const c = m && s.chats[m[1]];
  if (!m || !c) return json(route, { code: 'chat_not_found', message: 'no such chat' }, 404);
  const [, , sub, permission] = m;
  if (!sub) return json(route, chatView(c));
  if (sub === 'messages' && method === 'POST') {
    const content = String(body().content);
    if (c.running && c.mode === 'turn')
      return json(route, { code: 'chat_busy', message: 'the agent is still answering' }, 409);
    c.title ||= content.slice(0, 60);
    c.messages.push({ id: `m${c.messages.length}`, role: 'user', content, created: stamp() });
    if (c.running) c.steered.push(content);
    else {
      c.messages.push({ id: `m${c.messages.length}`, role: 'agent', content: '', created: stamp(), status: 'pending' });
      c.running = true;
      c.events.push({ type: 'run.started' });
    }
    return json(route, chatView(c), 202);
  }
  if (sub === 'interrupt' && method === 'POST') {
    const interrupted = c.running;
    c.interrupts.push(interrupted);
    if (interrupted) {
      c.running = false;
      const pending = c.messages.find(x => x.status === 'pending');
      if (pending) Object.assign(pending, { status: 'failed', error: 'The run was stopped.' });
      c.events.push({ type: 'run.interrupted' });
    }
    return json(route, { interrupted });
  }
  if (sub === 'permissions' && !permission) return json(route, { permissions: c.mode === 'session' ? c.permissions : [] });
  if (sub === 'permissions' && method === 'POST') {
    const index = c.permissions.findIndex(p => p.id === permission);
    if (index < 0) return json(route, { code: 'permission_not_found', message: 'no such permission request' }, 404);
    c.permissions.splice(index, 1);
    c.replies.push(`${permission}:${String(body().decision)}`);
    c.events.push({ type: 'permission.replied' });
    return route.fulfill({ status: 204 });
  }
  if (sub === 'events') {
    if (c.streamDown) return json(route, { code: 'internal', message: 'stream unavailable' }, 500);
    // Hold the connection a little so that a test's events arrive together.
    for (let i = 0; i < 20 && !c.events.length; i++) await sleep(50);
    const events = [{ type: 'connected' }, ...c.events.splice(0)];
    return route.fulfill({
      status: 200, headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' },
      body: 'retry: 100\n\n: keepalive\n\n' + events.map(e => `data: ${JSON.stringify(e)}\n\n`).join('')
    }).catch(() => {}); // The page may have closed meanwhile.
  }
  return json(route, { code: 'not_found', message: `mock has no ${method} ${path}` }, 404);
}

export const ada: MockUser = { subject: '3f9c', name: 'Ada Lovelace', email: 'ada@example.com', groups: ['makers'] };
export const grace: MockUser = { subject: '7a21', name: 'Grace Hopper', email: 'grace@example.com', groups: ['ember-admins'] };

const plugins = [{
  id: 'immich', name: 'Immich', version: '0.1.0', apiVersion: '1', description: 'Read-only photo library.',
  active: true, configured: true, capabilities: ['photo-library'],
  configurationSchema: { type: 'object', properties: { url: { type: 'string', format: 'uri' } }, required: ['url'] },
  configuration: { url: 'https://photos.example.com' }
}, {
  id: 'printer', name: 'Printer', version: '0.2.0', apiVersion: '1',
  active: false, configured: false, capabilities: ['fabrication'],
  configurationSchema: { type: 'object', properties: {} }, configuration: {}
}];

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
}

/** Answers /api calls on `page` from `s`; the `api` fixture does this for the default page. */
export async function install(page: Page, s: MockState) {
  await page.route(/\/api\//, async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api/, '');
    const method = request.method();
    s.calls.push(`${method} ${path}`);
    const signedIn = !s.enabled || s.user !== null;

    if (path === '/healthz') return json(route, { status: 'ok', version: 'test' });
    if (path === '/v1/auth/session' && s.sessionUnavailable)
      return json(route, { code: 'session_store_unavailable', message: 'the session store is unavailable' }, 503);
    if (path === '/v1/auth/session') {
      if (!s.enabled) return json(route, { enabled: false, authenticated: false });
      const provider = Object.keys(s.provider).length ? { provider: s.provider } : {};
      return json(route, s.user
        ? { enabled: true, authenticated: true, user: s.user, role: s.role, expiresAt: '2030-01-01T08:00:00Z', ...provider }
        : { enabled: true, authenticated: false, ...provider });
    }
    if (path === '/v1/auth/login') {
      // Stand-in for the round trip through the identity provider.
      s.user = s.next.user;
      s.role = s.next.role;
      return route.fulfill({ status: 303, headers: { location: url.searchParams.get('returnTo') ?? '/' } });
    }
    if (path === '/v1/auth/logout' && method === 'POST') {
      s.user = null;
      return json(route, {});
    }
    if (!signedIn || s.expiredPaths.has(path))
      return json(route, { error: 'unauthenticated', message: 'sign in required' }, 401);

    if (path === '/v1/services') return json(route, { services: [{ name: 'Authentik', url: 'https://auth.example.com/' }] });
    if (path === '/v1/agents') return json(route, { enabled: true, agents: [...s.ai.operatorAgents,
      ...s.ai.agents.filter(a => a.enabled).map(a => ({ id: a.id, name: a.name, harness: a.harness,
        model: s.ai.models.find(m => m.id === a.defaultModel)?.name, mode: a.worker ? 'session' : 'turn' }))] });
    if (path.startsWith('/v1/admin/ai')) return aiRoute(route, s, method, path, () => request.postDataJSON());
    if (path === '/v1/plugins') return json(route, { plugins });
    if (path === '/v1/projects') return json(route, { projects: [{ id: 'bracket', name: 'Shelf bracket' }] });
    if (path === '/v1/projects/bracket/notes') {
      if (method === 'PUT') {
        const edit = request.postDataJSON() as { path: string; content: string };
        s.notes[edit.path] = edit.content;
        return route.fulfill({ status: 204 });
      }
      return json(route, { notes: s.notes });
    }
    if (path.startsWith('/v1/projects/bracket/chats')) return chatRoute(route, s, method, path, () => request.postDataJSON());
    if (/^\/v1\/plugins\/[^/]+\/api$/.test(path)) return json(route, { pluginVersion: '0.1.0', apiVersion: '1', operations: [] });
    return json(route, { error: 'not_found', message: `mock has no ${method} ${path}` }, 404);
  });
}

export const test = base.extend<{ api: MockState }>({
  api: async ({ page }, use) => {
    const state: MockState = {
      enabled: true, user: null, role: 'user', next: { user: ada, role: 'user' },
      provider: { name: 'Authentik', accountUrl: 'https://auth.example.com/if/user/' },
      expiredPaths: new Set(), sessionUnavailable: false, notes: { 'index.md': '# Shelf bracket\n\nSee [[loads]].' }, calls: [],
      ai: { models: [], agents: [], workers: [], gateway: null,
        operatorAgents: [{ id: 'fixture', name: 'Fixture agent', mode: 'turn' }] },
      chats: {}
    };
    await install(page, state);
    await use(state);
  }
});

/** Starts signed in as `user` with `role`. */
export function signedIn(api: MockState, user: MockUser = ada, role: 'admin' | 'user' = 'user') {
  api.user = user;
  api.role = role;
}

/** The account menu's toggle in the top bar. */
export function menuButton(page: Page, name?: string) {
  return page.locator('summary', { hasText: `Account menu for ${name ?? ''}` });
}

export { expect };
