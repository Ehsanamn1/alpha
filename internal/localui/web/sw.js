'use strict';
const CACHE = 'alpha-ui-v3';
// Cache static assets only. The root HTML embeds a short-lived session token and
// must never be cached or served offline after the process restarts.
const ASSETS = ['/assets/app.css', '/assets/app.js', '/assets/icon.svg', '/manifest.webmanifest'];
self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(ASSETS)).then(() => self.skipWaiting()));
});
self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key)))).then(() => self.clients.claim()));
});
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith('/api/') || url.pathname === '/' || event.request.method !== 'GET') return;
  const isStaticAsset = url.pathname.startsWith('/assets/') || url.pathname === '/manifest.webmanifest' || url.pathname === '/sw.js';
  if (!isStaticAsset) return;
  event.respondWith(fetch(event.request).then((response) => {
    if (response.ok) {
      const copy = response.clone();
      caches.open(CACHE).then((cache) => cache.put(event.request, copy));
    }
    return response;
  }).catch(() => caches.match(event.request).then((cached) => cached || Response.error())));
});
