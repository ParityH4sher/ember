export interface Project { id: string; name: string }
export type Notes = Record<string, string | undefined>;
interface NoteEdit { path: string; content: string; previous: string | null }
export interface User { subject: string; name?: string; email?: string; groups?: string[] }
export interface Session { enabled: boolean; authenticated: boolean; user?: User; expiresAt?: string }

/** The API requires an OpenID Connect session (HTTP 401). */
export class SignInRequired extends Error {}

async function request(url: string, method = 'GET', data?: Project | NoteEdit): Promise<Response> {
  let response;
  try {
    response = await fetch(url, {
      method, cache: 'no-store',
      headers: data && { 'Content-Type': 'application/json' },
      body: data && JSON.stringify(data)
    });
  } catch {
    throw Error(method === 'PUT'
      ? 'Not saved. Keep this editor open, reconnect and try Save again.'
      : 'Cannot reach Ember. Reconnect and try again.');
  }
  if (response.status === 401) throw new SignInRequired('Sign in to continue.');
  if (!response.ok) {
    const error: unknown = await response.json().catch(() => null);
    throw Error(error && typeof error === 'object' && 'message' in error && typeof error.message === 'string'
      ? error.message : `Request failed (${response.status})`);
  }
  return response;
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
  await request(notesUrl(id), 'PUT', edit);
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
