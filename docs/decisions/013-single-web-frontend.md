# ADR 013: One web frontend

Status: Accepted  
Date: 2026-11-23  
Scope: Ember's browser frontends and how the gateway serves them. Amends the
image list of [ADR 011](011-prebuilt-registry-images.md).

**Problem.** Ember had two frontends, each in its own container behind the
gateway. A static home page (`apps/frontends/home`, plain HTML and JavaScript)
served `/` with API status, sign-in state, shared service links and plugin
forms. The SvelteKit Notes PWA (`apps/frontends/web`) served `/apps/notes/`.
Both implemented sign-in, sign-out and API error handling separately. The
operator decided that, for now, Ember supports one frontend: the Svelte one.

**Decision.**

- `apps/frontends/web` is the only frontend. It owns the site root: the home
  page is the `/` route and Project notes the `/apps/notes/` route, so links,
  sign-in `returnTo` values and installed apps keep working.
- The home page's logic moved into it: plugin forms are built from the API's
  JSON Schemas by `SchemaFields.svelte`, and both pages share `lib/api.ts` and
  `Account.svelte`. The browser still loads no plugin code or remote HTML.
- `apps/frontends/` stays as the folder for frontends; `home/` is removed.
- One Compose service, `web`, replaces `home` and `notes`; its image is
  `ember-web`. The gateway sends `/api/` to the API and everything else to
  `web`, and redirects relatively so redirects keep the public origin.
- Deployments remove orphans: `install.sh` runs `up --remove-orphans`, and the
  `ember` role sets `remove_orphans`, so upgraded hosts stop the old `home` and
  `notes` containers.

**Consequences.**

- The service worker now covers the whole site. Browsers that used Notes before
  still hold its worker, scoped to `/apps/notes/`, which serves a cached copy of
  the old app and is not removed when its script disappears. A small
  replacement at `static/apps/notes/service-worker.js` unregisters it and
  deletes its caches. Delete that file once old registrations are unlikely.
- The web manifest keeps its `id` and its start page (`/apps/notes/`), so
  existing installs update in place; its name is now "Ember".
- CI publishes three images instead of four. `ember-web` is a new GHCR package,
  which can start private: make it public after its first publish
  ([DEC-008](../planning/human-decisions.md#dec-008---visibility-of-ember-container-images)).
  Until it exists and is public, a registry-mode deploy fails when it pulls
  `ember-web`. The `ember-home` and
  `ember-notes` packages are no longer updated; delete them when nothing pins
  an old tag.
- A second frontend would again need its own build context, Compose service and
  gateway location.
