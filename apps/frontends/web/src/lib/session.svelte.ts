import { getSession, onSignInRequired, signInUrl, SignInUnavailable, signOut, type Session } from './api';
import { safeExternalUrl } from './url';

// The one place that knows the OpenID Connect sign-in state. The API owns the
// session in an HttpOnly cookie; the browser only learns whether it exists,
// never a token. The root layout loads it once; pages read `auth`.

class Auth {
  /** null when the API could not be asked (offline or an older API). */
  session = $state.raw<Session | null>(null);
  loaded = $state(false);
  /** The API answered 503: sign-in is configured but cannot be checked now. */
  unavailable = $state(false);
  /** Sign-in is configured and nobody is signed in. */
  signedOut = $derived(Boolean(this.session?.enabled && !this.session.authenticated));
  /** The session ended while a page was open (the API answered 401). The
      page stays mounted, so unsaved work survives until the person signs in. */
  expired = $state(false);
  /** Without sign-in every caller is an admin; the API enforces roles. */
  admin = $derived(!this.session?.enabled || this.session.role === 'admin');
  user = $derived(this.session?.user);
  displayName = $derived(this.user?.name || this.user?.email || this.user?.subject || '');
  /** The provider's display name; the operator sets it in the API's sign-in configuration. */
  providerName = $derived(this.session?.provider?.name || 'your identity provider');
  /** Where people manage their own account at the provider, if the operator set it. */
  accountUrl = $derived(safeExternalUrl(this.session?.provider?.accountUrl));
}

export const auth = new Auth();

export async function loadSession(): Promise<void> {
  try {
    auth.session = await getSession();
    auth.unavailable = false;
  } catch (error) {
    if (!(error instanceof SignInUnavailable)) throw error;
    auth.session = null;
    auth.unavailable = true;
  }
  auth.loaded = true;
}

// Any 401 from the API after the session was loaded means it ended (expiry,
// sign-out elsewhere, or an API restart without a shared session store).
onSignInRequired(() => { if (auth.session?.authenticated) auth.expired = true; });

// A page with unsaved work registers a guard; sign-in and sign-out ask it
// before leaving, because both are full-page navigations.
let leaveGuard: () => boolean = () => true;
export function setLeaveGuard(guard: (() => boolean) | null): void {
  leaveGuard = guard ?? (() => true);
}
export const mayLeave = (): boolean => leaveGuard();

/** A same-origin path, mirroring the API's safeReturnTo; anything else is "/". */
export function safeReturnTo(target: string | null): string {
  return target && target.startsWith('/') && !target.startsWith('//') && !target.includes('\\')
    && !/[\x00-\x1f\x7f]/.test(target) ? target : '/';
}

export const loginPath = (returnTo: string) => '/login/?' + new URLSearchParams({ returnTo });

export function signIn(returnTo = location.pathname + location.search): void {
  if (mayLeave()) location.assign(signInUrl(safeReturnTo(returnTo)));
}

/** Ends the Ember session, then the identity provider's when it supports RP-initiated logout. */
export async function logout(): Promise<void> {
  if (!mayLeave()) return;
  const target = await signOut();
  setLeaveGuard(null);
  if (target) location.assign(target);
  else location.assign(auth.session?.enabled ? '/login/' : '/');
}
