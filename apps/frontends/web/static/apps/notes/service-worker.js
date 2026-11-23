// Retires the service worker that Project notes registered here, with scope
// /apps/notes/, while it was a separate app (until November 2026). Browsers keep
// running a registered worker, and serving it the old cached shell, until a new
// version of this file replaces it; a missing file does not. This version
// deletes the old caches, unregisters itself and reloads open pages, so the
// app's own worker at /service-worker.js takes over.
// Delete this file once browsers that visited before the change are unlikely
// to still hold the old registration.
self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    const prefix = `ember-notes:${self.registration.scope}:`;
    const keys = await caches.keys();
    await Promise.all(keys.filter(key => key.startsWith(prefix)).map(key => caches.delete(key)));
    await self.registration.unregister();
    const pages = await self.clients.matchAll({ type: 'window' });
    await Promise.all(pages.map(page => page.navigate(page.url)));
  })());
});
