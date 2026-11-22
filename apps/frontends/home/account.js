// Shows the optional OpenID Connect sign-in state. The API owns the session;
// this page only sees whether it exists, never a token.

export const signInUrl = () =>
  '/api/v1/auth/login?' + new URLSearchParams({returnTo: location.pathname + location.search});

async function signOut() {
  const res = await fetch('/api/v1/auth/logout', {method: 'POST', headers: {Accept: 'application/json'}});
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.message || `Ember returned HTTP ${res.status}`);
  // End the identity provider session too when it supports RP-initiated logout.
  const target = typeof body.endSessionUrl === 'string' ? new URL(body.endSessionUrl) : null;
  if (target && ['https:', 'http:'].includes(target.protocol)) location.assign(target.href);
  else location.reload();
}

// Renders the account bar into host and resolves to true when the API can be
// used: sign-in is disabled, or the visitor is signed in.
export async function account(host) {
  let session;
  try {
    const res = await fetch('/api/v1/auth/session', {headers: {Accept: 'application/json'}, cache: 'no-store'});
    if (!res.ok) return true; // let the regular requests report the problem
    session = await res.json();
  } catch {
    return true;
  }
  if (!session.enabled) return true;
  host.hidden = false;
  if (!session.authenticated) {
    const link = document.createElement('a');
    link.href = signInUrl();
    link.textContent = 'Sign in';
    host.replaceChildren('You are signed out. ', link);
    return false;
  }
  const button = document.createElement('button');
  button.textContent = 'Sign out';
  const status = document.createElement('span');
  status.setAttribute('role', 'status');
  button.onclick = () => {
    button.disabled = true;
    signOut().catch(err => { status.textContent = ' ' + err.message; button.disabled = false; });
  };
  const user = session.user || {};
  host.replaceChildren(`Signed in as ${user.name || user.email || user.subject} `, button, status);
  return true;
}
