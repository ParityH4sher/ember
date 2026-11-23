import type { Schema } from './schema';

export interface Project { id: string; name: string }
export type Notes = Record<string, string | undefined>;
interface NoteEdit { path: string; content: string; previous: string | null }
export interface User { subject: string; name?: string; email?: string; groups?: string[] }
export interface Session {
  enabled: boolean; authenticated: boolean; user?: User; role?: 'admin' | 'user'; expiresAt?: string
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
export interface Agent { id: string; name: string }
export interface AgentList { enabled: boolean; agents: Agent[] }
export interface ChatMessage {
  id: string; role: 'user' | 'agent'; content: string; created: string;
  status?: 'pending' | 'completed' | 'failed'; error?: string; run?: string;
}
export interface Chat {
  schema: string; id: string; title: string; agent: string; created: string; updated: string;
  messages: ChatMessage[];
}
export interface ChatSummary {
  id: string; title: string; agent: string; created: string; updated: string;
  messageCount: number; pending: boolean;
}

/** The API requires an OpenID Connect session (HTTP 401). */
export class SignInRequired extends Error {}

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
  if (response.status === 401) throw new SignInRequired('Sign in to continue.');
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
/** Saves the message and starts one agent run; poll getChat for the answer. */
export async function sendMessage(id: string, chat: string, content: string): Promise<Chat> {
  return (await request(`${chatUrl(id, chat)}/messages`, 'POST', { content })).json();
}

/** Session state, or null when it cannot be read (offline or an older API). */
export async function getSession(): Promise<Session | null> {
  try {
    const response = await fetch('/api/v1/auth/session', { cache: 'no-store' });
    return response.ok ? await response.json() : null;
  } catch {
    return null;
  }
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
