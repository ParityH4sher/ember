---
sidebar_label: Web app shell plan
---

# Web app shell: sign-in page, dashboard, settings, account and admin

**Date:** 2026-11-23  
**Status:** Implemented in `apps/frontends/web`; verification is recorded in the
[implementation log](../progress/implementation-log.md#web-app-shell-2026-11-23).  
**Builds on:** [ADR 013](../decisions/013-single-web-frontend.md) (one web
frontend) and [Sign-in with OpenID Connect](../authentication.md).

## Goal

Turn the Svelte frontend from two loose pages into one application:

- a dedicated **sign-in page** with a single button that starts the Authentik
  (OIDC) sign-in;
- an **app shell** (sidebar navigation, user menu) shared by every page;
- a **dashboard**, **plugins**, **settings**, **account** (profile) and
  **admin** page, next to the existing Project notes.

## Constraints

- **No API changes.** Every page uses only routes in
  [`contracts/openapi.yaml`](https://github.com/ParityH4sher/ember/blob/main/contracts/openapi.yaml).
  The API has no user directory, no per-user preferences and no audit log, so
  the frontend does not pretend to have them.
- **The browser never sees a token.** Sign-in stays a full-page navigation to
  `/api/v1/auth/login`; the session is the API's `HttpOnly` cookie.
- **The API enforces roles.** The frontend hides controls a `user` cannot use
  and shows an explanation on admin pages; it is not a security boundary.
- **Sign-in may be off.** Without OIDC every caller is an admin and the sign-in
  page only says that sign-in is not configured.
- Static build (`adapter-static`, prerendered, trailing slashes), served by the
  gateway; no Node at runtime.

## Routes

| Path | Page | Who | Data |
| --- | --- | --- | --- |
| `/login/` | Sign-in: one **Sign in with Authentik** button | everyone | `GET /v1/auth/session` |
| `/` | Dashboard: API status, counts, recent projects, shared services | signed in | health, projects, plugins, agents, services |
| `/apps/notes/` | Project notes and chats (unchanged behavior) | signed in | projects, notes, chats |
| `/plugins/` | Plugins: run operations; admins also configure and activate | signed in | plugins |
| `/settings/` | Preferences for this browser: theme, density | signed in | `localStorage` only |
| `/account/` | Profile and account: name, email, subject, groups, role, session expiry, sign out | signed in | session |
| `/admin/` | Instance overview, plugin status, agents, services, access rules | `admin` | health, session, plugins, agents, services |

## Design

### Session store (`lib/session.svelte.ts`)

One reactive module owns the session. The root layout loads it once; pages
read `auth.session`, `auth.signedOut` and `auth.admin` instead of each calling
`getSession()`. It also holds a **leave guard** that a page with unsaved work
(Project notes) registers, so that sign-out from the user menu asks first.

### Auth gate (`routes/+layout.svelte`)

- On `/login/` the layout renders the page alone, without the shell.
- Elsewhere it waits for the session. If sign-in is enabled and nobody is
  signed in, it replaces the URL with `/login/?returnTo=<path and query>`.
- The sign-in page only accepts a `returnTo` that is a same-origin path
  (starts with one `/`), mirroring the API's own `safeReturnTo` check.
- An expired session during use still surfaces as `401`; Project notes keeps
  its "You are signed out" prompt, and other pages show the error.

### Shell (`lib/shell/`)

`AppShell.svelte` renders the sidebar (`Nav.svelte`), the top bar with the
page title and `UserMenu.svelte`, and the page. The Admin entry appears only
for admins. On narrow screens the sidebar becomes a toggled drawer.

### Preferences (`lib/prefs.svelte.ts`)

Theme (`system`, `light`, `dark`) and density (`comfortable`, `compact`) are
stored per browser in `localStorage`, wrapped in `try`/`catch`, and applied as
`data-theme` and `data-density` on `<html>`. A small inline script in
`app.html` applies them before first paint. The settings page says plainly
that they are not synced, because the API stores no per-user data.

### Styling

`app.css` defines colour, spacing and radius tokens on `:root` with a dark and
a light set; components use the tokens instead of literal colours. Shared
primitives (`Card`, `Stat`, `PageHeader`, `Badge`) replace per-page card CSS.

### Session ending during use

`api.ts` reports every `401` to `session.svelte.ts`. A page that was signed in
does not unmount: the shell shows **Sign in again**, which asks the page's
leave guard first. Project notes keeps its draft on screen; it cannot be saved
without a session, so the banner says to copy it. A `401` on the first load is
the ordinary signed-out case and goes to `/login/`.

### Account management

Ember does not own accounts. Name, email and groups come from the ID token at
sign-in and change at the identity provider; the account page says so. Group
changes take effect at the next sign-in, as documented for roles.

## Phases

| Phase | Work | Status |
| --- | --- | --- |
| W0 | This plan | Done |
| W1 | Tokens, light theme, preferences store, pre-paint theme script | Done |
| W2 | Session store, auth gate, `/login/` page | Done |
| W3 | Shell: sidebar, top bar, user menu, mobile drawer | Done |
| W4 | Dashboard, plugins, settings, account, admin pages | Done |
| W5 | Refactor: move home-page plugin UI to `/plugins/`, put Project notes in the shell, remove `Account.svelte`, tokens in existing components | Done |
| W6 | Gateway slash redirects for the new pages, documentation, verification | Done |
| W7 | Session ending while a page is open: one `401` hook in `api.ts`, a **Sign in again** banner in the shell; the page stays mounted | Done |
| W8 | In-app error and 404 page (`+error.svelte`, `404.html` fallback served by the web container's `nginx.conf`); installed app opens on the dashboard | Done |
| W9 | Provider name and account link from `GET /v1/auth/session` (`provider.name`, `provider.accountUrl`) | Done |
| W10 | Playwright: mocked UI suite with axe checks (CI), real-infrastructure suite, Authentik test-account blueprint, devcontainer browsers | Done |
| W11 | Redis session store in the API ([ADR 014](../decisions/014-redis-session-store.md)) | Done |

## Not in scope

- A user list or user administration in Ember. People and groups are managed
  in Authentik (**Directory → Users / Groups**).
- Server-side preferences, avatars, API tokens, or audit logs. Each needs an
  API and storage decision first.
