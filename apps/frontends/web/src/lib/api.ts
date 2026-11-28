import type { Schema } from './schema';

export interface Project { id: string; name: string }
export type Notes = Record<string, string | undefined>;
interface NoteEdit { path: string; content: string; previous: string | null }
export interface User { subject: string; name?: string; email?: string; groups?: string[] }
/** The identity provider, as the operator named it in the API's sign-in configuration. */
export interface Provider { name?: string; accountUrl?: string }
export interface Session {
  enabled: boolean; authenticated: boolean; user?: User; role?: 'admin' | 'user'; expiresAt?: string;
  provider?: Provider
}
export interface Health { status: string; version: string }
export interface ServiceLink { name: string; url: string }
export interface Plugin {
  id: string; name: string; version: string; apiVersion: string; description?: string;
  active: boolean; configured: boolean; capabilities: string[];
  configurationSchema: Schema; configuration: Record<string, unknown>
}
export interface PluginOperation {
  operationId: string; name: string; description?: string; method: string; path: string;
  inputSchema: Schema; outputSchema: Schema
}
export interface PluginCatalog { pluginVersion: string; apiVersion: string; operations: PluginOperation[] }
/**
 * harness and model are set for agents an admin added on the Agents & models
 * page. mode (ADR 015): a session agent is one long-lived harness session per
 * chat that can be steered while it runs; a turn agent runs once per message.
 */
export type AgentMode = 'session' | 'turn';
export interface Agent { id: string; name: string; harness?: string; model?: string; mode?: AgentMode }
export interface AgentList { enabled: boolean; agents: Agent[] }

// Agents & models (admin only). Providers and their keys live in the model
// gateway (LiteLLM, docs/models.md); the catalog only offers gateway models
// under a display name and maps agents to them.
/** id is the gateway's model name. */
export interface CatalogModel { id: string; name: string }
/** A session worker from the operator's agents file. */
export interface AIWorker { id: string; harness: string }
/** Harness opencode runs on a worker; the other harnesses are router agents. */
export interface CatalogAgent {
  id: string; name: string; harness: string; routerAgent?: string; worker?: string;
  models: string[]; defaultModel?: string; enabled: boolean
}
export interface AICatalog {
  persisted: boolean; chatEnabled: boolean; harnesses: string[]; workers: AIWorker[];
  gateway: { configured: boolean }; operatorAgents: Agent[]; models: CatalogModel[]; agents: CatalogAgent[]
}
export interface ChatMessage {
  id: string; role: 'user' | 'agent'; content: string; created: string;
  status?: 'pending' | 'completed' | 'failed'; error?: string; run?: string;
}
/** mode and running come from APIs with ADR 015; older ones omit them. */
export interface Chat {
  schema: string; id: string; title: string; agent: string; created: string; updated: string;
  messages: ChatMessage[]; mode?: AgentMode; running?: boolean;
}
/** A tool permission a session agent waits for. */
export interface Permission { id: string; action: string; resources: string[] }
export type PermissionDecision = 'once' | 'reject';
/**
 * One event of a chat's live stream. connected and resync come from Ember;
 * the others are text.delta, tool.called|succeeded|failed,
 * run.started|succeeded|failed|interrupted and permission.asked|replied.
 */
export interface ChatEvent { type: string; delta?: string }
export interface ChatSummary {
  id: string; title: string; agent: string; created: string; updated: string;
  messageCount: number; pending: boolean;
}

/** The API requires an OpenID Connect session (HTTP 401). */
export class SignInRequired extends Error {}

// Told about every 401, so the app can react in one place (lib/session.svelte.ts).
let signInRequired = () => {};
export function onSignInRequired(handler: () => void): void { signInRequired = handler; }

async function request(url: string, method = 'GET', data?: unknown,
  unreachable = 'Cannot reach Ember. Reconnect and try again.'): Promise<Response> {
  let response;
  try {
    response = await fetch(url, {
      method, cache: 'no-store',
      headers: { Accept: 'application/json', ...(data !== undefined && { 'Content-Type': 'application/json' }) },
      body: data === undefined ? undefined : JSON.stringify(data)
    });
  } catch {
    throw Error(unreachable);
  }
  if (response.status === 401) {
    signInRequired();
    throw new SignInRequired('Your session has ended. Sign in again to continue.');
  }
  if (!response.ok) {
    const error: unknown = await response.json().catch(() => null);
    throw Error(error && typeof error === 'object' && 'message' in error && typeof error.message === 'string'
      ? error.message : `Request failed (${response.status})`);
  }
  return response;
}

export async function getHealth(): Promise<Health> {
  return (await request('/api/healthz')).json();
}
export async function listServices(): Promise<{services: ServiceLink[]}> {
  return (await request('/api/v1/services')).json();
}

const projects = '/api/v1/projects';
const notesUrl = (id: string) => `${projects}/${encodeURIComponent(id)}/notes`;
export async function listProjects(): Promise<{projects: Project[]}> {
  return (await request(projects)).json();
}
export async function createProject(project: Project): Promise<Project> {
  return (await request(projects, 'POST', project)).json();
}
export async function loadNotes(id: string): Promise<{notes: Notes}> {
  return (await request(notesUrl(id))).json();
}
export async function saveNote(id: string, edit: NoteEdit): Promise<void> {
  await request(notesUrl(id), 'PUT', edit, 'Not saved. Keep this editor open, reconnect and try Save again.');
}

// Plugins. The browser loads no plugin code: it only renders the schemas the
// API publishes and calls the API's own routes.
const pluginUrl = (id: string) => `/api/v1/plugins/${encodeURIComponent(id)}`;
export async function listPlugins(): Promise<{plugins: Plugin[]}> {
  return (await request('/api/v1/plugins')).json();
}
export async function getPlugin(id: string): Promise<Plugin> {
  return (await request(pluginUrl(id))).json();
}
/** Needs the admin role. Omitted write-only values keep their stored value. */
export async function configurePlugin(id: string, values: Record<string, unknown>): Promise<void> {
  await request(`${pluginUrl(id)}/configuration`, 'PUT', { values });
}
/** Needs the admin role. */
export async function setPluginActivation(id: string, active: boolean): Promise<Plugin> {
  return (await request(`${pluginUrl(id)}/activation`, 'PUT', { active })).json();
}
export async function listPluginOperations(id: string): Promise<PluginCatalog> {
  return (await request(`${pluginUrl(id)}/api`)).json();
}
/** The only route an operation may be invoked at; never a catalog-supplied destination. */
export function operationPath(id: string, apiVersion: string, operationId: string): string {
  return `${pluginUrl(id)}/api/v${encodeURIComponent(apiVersion)}/${encodeURIComponent(operationId)}`;
}
export async function invokeOperation(path: string, input: Record<string, unknown>): Promise<{data?: unknown}> {
  return (await request(path, 'POST', input)).json();
}

const ai = '/api/v1/admin/ai';
const aiItem = (kind: 'models' | 'agents', id: string) => `${ai}/${kind}/${encodeURIComponent(id)}`;
/** Needs the admin role. */
export async function getAICatalog(): Promise<AICatalog> {
  return (await request(ai)).json();
}
/** The model names the gateway offers; fails (gateway_unavailable) when it is not configured or unreachable. */
export async function listGatewayModels(): Promise<string[]> {
  const body: {models: {id: string}[]} = await (await request(`${ai}/gateway/models`)).json();
  return body.models.map(m => m.id);
}
/** id is the gateway's model name. */
export async function saveModel(id: string, model: Omit<CatalogModel, 'id'>): Promise<CatalogModel> {
  return (await request(aiItem('models', id), 'PUT', model)).json();
}
export async function deleteModel(id: string): Promise<void> {
  await request(aiItem('models', id), 'DELETE');
}
export async function saveAgent(id: string, agent: Omit<CatalogAgent, 'id'>): Promise<CatalogAgent> {
  return (await request(aiItem('agents', id), 'PUT', agent)).json();
}
export async function deleteAgent(id: string): Promise<void> {
  await request(aiItem('agents', id), 'DELETE');
}

/** Agents the operator registered; chat runs are off when enabled is false. */
export async function listAgents(): Promise<AgentList> {
  return (await request('/api/v1/agents')).json();
}
const chatsUrl = (id: string) => `${projects}/${encodeURIComponent(id)}/chats`;
const chatUrl = (id: string, chat: string) => `${chatsUrl(id)}/${encodeURIComponent(chat)}`;
export async function listChats(id: string): Promise<{chats: ChatSummary[]}> {
  return (await request(chatsUrl(id))).json();
}
export async function createChat(id: string, agent: string): Promise<Chat> {
  return (await request(chatsUrl(id), 'POST', { agent })).json();
}
export async function getChat(id: string, chat: string): Promise<Chat> {
  return (await request(chatUrl(id, chat))).json();
}
/**
 * Saves the message and starts an agent run. While a session agent runs, the
 * message steers that run instead. The answer arrives through the event
 * stream (openChatEvents) and getChat.
 */
export async function sendMessage(id: string, chat: string, content: string): Promise<Chat> {
  return (await request(`${chatUrl(id, chat)}/messages`, 'POST', { content })).json();
}
/** Stops the chat's active run; false when none was running. */
export async function interruptChat(id: string, chat: string): Promise<boolean> {
  const body: {interrupted?: unknown} = await (await request(`${chatUrl(id, chat)}/interrupt`, 'POST', {})).json();
  return body.interrupted === true;
}
export async function listPermissions(id: string, chat: string): Promise<Permission[]> {
  return (await (await request(`${chatUrl(id, chat)}/permissions`)).json()).permissions;
}
export async function replyPermission(id: string, chat: string, permission: string, decision: PermissionDecision): Promise<void> {
  await request(`${chatUrl(id, chat)}/permissions/${encodeURIComponent(permission)}`, 'POST', { decision });
}
/**
 * Opens the chat's event stream (same origin, so the session cookie goes
 * along) and returns a function that closes it. The browser retries a stream
 * that drops, but not one whose first request failed (an API restarting, a
 * 401): that one is reopened here with a growing delay. onError reports each
 * failure; the next connected event means the stream is back.
 */
export function openChatEvents(id: string, chat: string,
  on: { event: (event: ChatEvent) => void; error: () => void }): () => void {
  let source: EventSource | undefined, timer: ReturnType<typeof setTimeout> | undefined;
  let closed = false, delay = 3000;
  const open = () => {
    const current = source = new EventSource(`${chatUrl(id, chat)}/events`);
    current.onmessage = ({ data }) => {
      let event: unknown;
      try { event = JSON.parse(data); } catch { return; }
      if (!event || typeof event !== 'object' || !('type' in event) || typeof event.type !== 'string') return;
      delay = 3000;
      const delta = 'delta' in event && typeof event.delta === 'string' ? event.delta : undefined;
      on.event({ type: event.type, ...(delta !== undefined && { delta }) });
    };
    current.onerror = () => {
      if (closed) return;
      on.error();
      if (current.readyState === EventSource.CLOSED) {
        timer = setTimeout(open, delay);
        delay = Math.min(delay * 2, 30_000);
      }
    };
  };
  open();
  return () => { closed = true; clearTimeout(timer); source?.close(); };
}

/** Session state, or null when it cannot be read (offline or an older API). */
export async function getSession(): Promise<Session | null> {
  let response;
  try {
    response = await fetch('/api/v1/auth/session', { cache: 'no-store' });
  } catch {
    return null;
  }
  // Sign-in is configured but its session store or identity provider is down.
  if (response.status === 503) throw new SignInUnavailable();
  return response.ok ? await response.json() : null;
}
/** Sign-in is configured but temporarily unavailable (HTTP 503). */
export class SignInUnavailable extends Error {
  constructor() { super('Sign-in is temporarily unavailable. Try again in a moment.'); }
}
/** Full-page navigation to the API, which redirects to the identity provider. */
export function signInUrl(returnTo: string): string {
  return '/api/v1/auth/login?' + new URLSearchParams({ returnTo });
}
/** Ends the Ember session; returns the identity provider's logout URL, if any. */
export async function signOut(): Promise<string | undefined> {
  const body: unknown = await (await request('/api/v1/auth/logout', 'POST')).json().catch(() => null);
  const target = body && typeof body === 'object' && 'endSessionUrl' in body && typeof body.endSessionUrl === 'string'
    ? new URL(body.endSessionUrl) : null;
  return target && ['https:', 'http:'].includes(target.protocol) ? target.href : undefined;
}
