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
  /** The admin AI catalog; keys are kept here and never sent back. */
  ai: MockAI;
}

interface MockAI {
  providers: { id: string; name: string; kind: string; baseURL: string; local: boolean; apiKey?: string }[];
  models: { id: string; name: string; provider: string; upstream: string }[];
  agents: { id: string; name: string; harness: string; routerAgent: string; models: string[]; defaultModel?: string; enabled: boolean }[];
}

const defaultURL: Record<string, string> = { anthropic: 'https://api.anthropic.com/v1', openai: 'https://api.openai.com/v1' };

// A small stand-in for /v1/admin/ai (apps/api/internal/httpapi/aiadmin.go).
function aiRoute(route: Route, s: MockState, method: string, path: string, body: () => Record<string, unknown>) {
  if (s.enabled && s.role !== 'admin')
    return json(route, { code: 'insufficient_role', message: 'this action needs the Ember admin role' }, 403);
  const ai = s.ai;
  if (path === '/v1/admin/ai' && method === 'GET') {
    return json(route, {
      persisted: true, chatEnabled: true,
      providerKinds: ['anthropic', 'openai', 'ollama', 'openai-compatible'],
      harnesses: ['claude-code', 'codex', 'opencode', 'custom'],
      operatorAgents: [{ id: 'fixture', name: 'Fixture agent' }],
      providers: ai.providers.map(({ apiKey, ...p }) => ({ ...p, hasAPIKey: !!apiKey })),
      models: ai.models, agents: ai.agents
    });
  }
  const m = /^\/v1\/admin\/ai\/(providers|models|agents)\/([^/]+)(\/discover)?$/.exec(path);
  if (!m) return json(route, { code: 'not_found', message: 'no such route' }, 404);
  const [, kind, id, discover] = m;
  const list = ai[kind as keyof MockAI] as { id: string }[];
  const index = list.findIndex(x => x.id === id);
  if (discover) return json(route, { models: ['llama3.2:3b', 'qwen3:8b'] });
  if (method === 'DELETE') {
    if (kind === 'providers' && ai.models.some(x => x.provider === id))
      return json(route, { code: 'in_use', message: 'a model uses this provider; delete or move it first' }, 409);
    if (kind === 'models' && ai.agents.some(x => x.models.includes(id)))
      return json(route, { code: 'in_use', message: 'an agent uses this model; remove it from the agent first' }, 409);
    if (index >= 0) list.splice(index, 1);
    return route.fulfill({ status: 204 });
  }
  const input = body();
  let item: Record<string, unknown> = { ...input, id };
  if (kind === 'providers') {
    const old = ai.providers[index];
    const baseURL = String(input.baseURL || defaultURL[String(input.kind)] || '');
    const { apiKey: _key, clearApiKey, ...rest } = input;
    item = { ...rest, id, baseURL, apiKey: clearApiKey ? undefined : (input.apiKey ?? old?.apiKey) };
  }
  if (kind === 'agents' && !item.defaultModel) item.defaultModel = (item.models as string[])[0];
  if (index >= 0) list[index] = item as never; else list.push(item as never);
  if (kind === 'providers') {
    const { apiKey, ...p } = item;
    return json(route, { ...p, hasAPIKey: !!apiKey });
  }
  return json(route, item);
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
    if (path === '/v1/agents') return json(route, { enabled: true, agents: [{ id: 'fixture', name: 'Fixture agent' },
      ...s.ai.agents.filter(a => a.enabled).map(a => ({ id: a.id, name: a.name, harness: a.harness,
        model: s.ai.models.find(m => m.id === a.defaultModel)?.name }))] });
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
    if (path === '/v1/projects/bracket/chats') return json(route, { chats: [] });
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
      ai: { providers: [], models: [], agents: [] }
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
