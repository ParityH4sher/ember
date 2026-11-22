import { assets, immutable, prerendered } from '$app/manifest';
import { version } from '$app/env';

declare const self: ServiceWorkerGlobalScope;

const prefix = `ember-notes:${self.registration.scope}:`;
const cacheName = prefix + version;
const shell = new Set([...immutable, ...assets, ...prerendered].map(({path}) =>
  new URL(path.replace(/^\//, ''), self.registration.scope).pathname));

self.addEventListener('install', event => {
  event.waitUntil(caches.open(cacheName).then(cache =>
    cache.addAll([...shell].map(path => new Request(path, {cache: 'reload'})))));
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys.filter(k => k.startsWith(prefix) && k !== cacheName).map(k => caches.delete(k)));
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);
  if (request.method !== 'GET' || url.origin !== self.location.origin || !shell.has(url.pathname)) return;
  event.respondWith(caches.open(cacheName).then(async cache =>
    (await cache.match(url.pathname)) || fetch(request)));
});
