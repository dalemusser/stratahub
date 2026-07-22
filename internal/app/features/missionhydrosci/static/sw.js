// sw.js — Main service worker entry point for Mission HydroSci content delivery.
// This file is concatenated with sw-cache.js and sw-background-fetch.js
// by the Go handler before being served at /sw.js.

const SW_VERSION = '1.0.11';

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
    // Clean up old app shell caches from previous SW versions
    caches.keys().then(function(names) {
      return Promise.all(
        names.filter(function(name) {
          return name.startsWith('missionhydrosci-app-shell-') && name !== APP_SHELL_CACHE;
        }).map(function(name) {
          return caches.delete(name);
        })
      );
    }).then(function() {
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

  // /missionhydrosci/content/* — cache-first, fallback to CDN redirect
  if (path.startsWith('/missionhydrosci/content/')) {
    event.respondWith(serveMHSContent(event.request, path));
    return;
  }

  // /assets/mhs/* — cache-first for MHS static assets
  if (path.startsWith('/assets/mhs/')) {
    event.respondWith(cacheFirst(event.request, APP_SHELL_CACHE));
    return;
  }

  // /missionhydrosci/units, /missionhydrosci/play/* — cache-first for offline support
  if (path === '/missionhydrosci/units' || path.startsWith('/missionhydrosci/play/')) {
    event.respondWith(networkFirstWithOffline(event.request));
    return;
  }

  // Hash-versioned app shell assets — exact-match cache-first. The ?v=<hash>
  // in the request is part of the cache key, so a new deploy's hash misses
  // the cache and fetches fresh on the FIRST load after a deploy.
  if (path === '/assets/css/tailwind.css' || path === '/assets/js/mhs-delivery.js') {
    event.respondWith(versionedAssetFirst(event.request));
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
 * Exact-match cache-first for hash-versioned assets (?v=<hash>).
 * - Same hash as cached: served instantly from cache.
 * - New hash (fresh HTML after a deploy): cache miss, fetched from network,
 *   stale versions of the same asset pruned, new version cached.
 * - Offline with no exact match: any cached version beats nothing.
 */
async function versionedAssetFirst(request) {
  var cache = await caches.open(APP_SHELL_CACHE);

  var exact = await cache.match(request);
  if (exact) {
    return exact;
  }

  try {
    var response = await fetch(request);
    if (response.ok) {
      var stale = await cache.keys(request, { ignoreSearch: true });
      for (var i = 0; i < stale.length; i++) {
        await cache.delete(stale[i]);
      }
      await cache.put(request, response.clone());
    }
    return response;
  } catch (err) {
    var fallback = await cache.match(request, { ignoreSearch: true });
    if (fallback) {
      return fallback;
    }
    throw err;
  }
}

/**
 * Cache-first strategy for static assets.
 */
async function cacheFirst(request, cacheName) {
  var cache = await caches.open(cacheName);
  var match = await cache.match(request, { ignoreSearch: true });
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
    // Bypass HTTP cache to ensure we always get the latest from the server.
    // iOS PWA can aggressively cache HTML responses; cache:'no-store' avoids this.
    var response = await fetch(request, { cache: 'no-store' });
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
      '<a href="/missionhydrosci/units">Go to Units</a></body></html>',
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
  } else if (data.action === 'fallbackDownload') {
    // Direct fallback to regular sequential fetch (skips Background Fetch).
    // Used when a Background Fetch stalls and the page needs to recover.
    event.waitUntil(
      fallbackFetch(
        data.unitId,
        data.version,
        data.files,
        data.cdnBaseUrl,
        data.title
      )
    );
  } else if (data.action === 'cancelFallbacks') {
    // Stop in-flight sequential fallback downloads before a cache purge —
    // empty unitId/version cancels all of them. purge=false (a retry-style
    // cancel) keeps the partial cache for resume; absent (older pages)
    // defaults to purging, the original behavior.
    cancelActiveFallbacks(data.unitId, data.version, data.purge);
  } else if (data.action === 'attachProgress') {
    // Periodic page keepalive while a download is active: (re)attaches the
    // SW's progress broadcasters (idempotent) and, as a message, wakes or
    // extends the SW so those listeners keep firing.
    event.waitUntil(reattachAllProgress());
  } else if (data.action === 'getActiveFallbacks') {
    // Reply with in-flight fallback download keys so a freshly-loaded page
    // can adopt the loops it lost track of across a reload.
    if (event.ports && event.ports[0]) {
      event.ports[0].postMessage({ activeFallbacks: listActiveFallbacks() });
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
