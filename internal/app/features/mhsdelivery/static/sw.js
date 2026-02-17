// sw.js — Main service worker entry point for MHS content delivery.
// This file is concatenated with sw-cache.js and sw-background-fetch.js
// by the Go handler before being served at /sw.js.

const SW_VERSION = '1.0.0';

// ---- Install ----
self.addEventListener('install', function(event) {
  event.waitUntil(
    caches.open(APP_SHELL_CACHE).then(function(cache) {
      return cache.addAll(APP_SHELL_URLS);
    }).then(function() {
      return self.skipWaiting();
    })
  );
});

// ---- Activate ----
self.addEventListener('activate', function(event) {
  event.waitUntil(
    Promise.resolve().then(function() {
      return self.clients.claim();
    })
  );
});

// ---- Fetch ----
self.addEventListener('fetch', function(event) {
  var url = new URL(event.request.url);

  // Only intercept same-origin requests
  if (url.origin !== self.location.origin) {
    return;
  }

  var path = url.pathname;

  // /mhs/content/* — cache-first, fallback to CDN redirect
  if (path.startsWith('/mhs/content/')) {
    event.respondWith(serveMHSContent(event.request, path));
    return;
  }

  // /assets/mhs/* — cache-first for MHS static assets
  if (path.startsWith('/assets/mhs/')) {
    event.respondWith(cacheFirst(event.request, APP_SHELL_CACHE));
    return;
  }

  // /mhs/units, /mhs/play/* — cache-first for offline support
  if (path === '/mhs/units' || path.startsWith('/mhs/play/')) {
    event.respondWith(networkFirstWithOffline(event.request));
    return;
  }

  // App shell assets — cache-first
  if (path === '/assets/css/tailwind.css' || path === '/assets/js/mhs-delivery.js') {
    event.respondWith(cacheFirst(event.request, APP_SHELL_CACHE));
    return;
  }

  // Everything else — pass through to network (don't interfere with StrataHub)
});

/**
 * Serves MHS content from cache, falling back to network.
 * Searches all unit caches for the requested path.
 */
async function serveMHSContent(request, path) {
  // Search all unit caches for this file
  var allCacheNames = await caches.keys();
  for (var i = 0; i < allCacheNames.length; i++) {
    var name = allCacheNames[i];
    if (name.startsWith(UNIT_CACHE_PREFIX)) {
      var cache = await caches.open(name);
      var match = await cache.match(path);
      if (match) {
        return match;
      }
    }
  }

  // Not in cache — let it fall through to the Go handler (302 redirect to CDN)
  return fetch(request);
}

/**
 * Cache-first strategy for static assets.
 */
async function cacheFirst(request, cacheName) {
  var cache = await caches.open(cacheName);
  var match = await cache.match(request);
  if (match) {
    return match;
  }
  var response = await fetch(request);
  if (response.ok) {
    cache.put(request, response.clone());
  }
  return response;
}

/**
 * Network-first with offline fallback for app pages.
 */
async function networkFirstWithOffline(request) {
  try {
    var response = await fetch(request);
    if (response.ok) {
      var cache = await caches.open(APP_SHELL_CACHE);
      cache.put(request, response.clone());
    }
    return response;
  } catch (err) {
    var cached = await caches.match(request);
    if (cached) {
      return cached;
    }
    // Return a basic offline page
    return new Response(
      '<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:4em;">' +
      '<h1>Offline</h1><p>You are offline. Cached units are still available.</p>' +
      '<a href="/mhs/units">Go to Units</a></body></html>',
      { headers: { 'Content-Type': 'text/html' } }
    );
  }
}

// ---- Message Handler ----
self.addEventListener('message', function(event) {
  var data = event.data;
  if (!data || !data.action) return;

  if (data.action === 'download') {
    event.waitUntil(
      startBackgroundFetch(
        data.unitId,
        data.version,
        data.files,
        data.cdnBaseUrl,
        data.title
      )
    );
  } else if (data.action === 'deleteUnit') {
    event.waitUntil(
      caches.delete(unitCacheName(data.unitId, data.version)).then(function() {
        broadcastStatus(data.unitId, 'not_cached', {});
      })
    );
  } else if (data.action === 'checkStatus') {
    event.waitUntil(
      checkUnitCacheStatus(data.unitId, data.version, data.files).then(function(status) {
        broadcastStatus(data.unitId, status, {});
      })
    );
  } else if (data.action === 'getVersion') {
    if (event.ports && event.ports[0]) {
      event.ports[0].postMessage({ version: SW_VERSION });
    }
  }
});

// ---- Background Fetch Events ----
self.addEventListener('backgroundfetchsuccess', function(event) {
  event.waitUntil(handleBackgroundFetchSuccess(event));
});

self.addEventListener('backgroundfetchfail', function(event) {
  event.waitUntil(handleBackgroundFetchFailure(event));
});

self.addEventListener('backgroundfetchclick', function(event) {
  handleBackgroundFetchClick(event);
});
