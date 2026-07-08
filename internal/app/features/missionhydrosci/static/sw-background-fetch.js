// sw-background-fetch.js — Background Fetch handlers for the Mission HydroSci service worker.

const MHS_CHANNEL_NAME = 'missionhydrosci-delivery';

// Fetch ID format: "missionhydrosci-{unitId}-v{version}" e.g., "missionhydrosci-unit1-v1.0.0"
// This encodes everything we need to cache responses without fetching the manifest.

function parseFetchId(fetchId) {
  // "missionhydrosci-unit1-v1.0.0" -> { unitId: "unit1", version: "1.0.0" }
  var match = fetchId.match(/^missionhydrosci-(.+)-v(\d+\.\d+\.\d+)$/);
  if (match) {
    return { unitId: match[1], version: match[2] };
  }
  // Legacy format: "missionhydrosci-unit1"
  return { unitId: fetchId.replace('missionhydrosci-', ''), version: null };
}

/**
 * Starts a background fetch for a unit's files.
 * Called from the SW message handler as a fallback when page-initiated fetch isn't available.
 */
async function startBackgroundFetch(unitId, version, files, cdnBaseUrl, title) {
  var fetchId = 'missionhydrosci-' + unitId + '-v' + version;

  var requests = files.map(function(file) {
    return new Request(cdnBaseUrl + '/' + file.path, { mode: 'cors' });
  });

  var totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);

  try {
    // Check for an existing fetch with the same ID
    var existing = await self.registration.backgroundFetch.get(fetchId);
    if (existing && existing.result === '') {
      // Active fetch already in progress — let it continue
      return true;
    }

    var bgFetch = await self.registration.backgroundFetch.fetch(fetchId, requests, {
      title: title || 'Downloading ' + unitId,
      icons: [{ sizes: '192x192', src: '/assets/mhs/icon-192.png', type: 'image/png' }],
      // Real total makes the browser's native download indicator determinate.
      downloadTotal: totalSize
    });

    broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize });

    bgFetch.addEventListener('progress', function() {
      if (totalSize > 0) {
        var percent = Math.round((bgFetch.downloaded / totalSize) * 100);
        broadcastStatus(unitId, 'downloading', {
          downloaded: bgFetch.downloaded,
          downloadTotal: totalSize,
          percent: percent
        });
      }
    });

    return true;
  } catch (err) {
    console.warn('Background Fetch failed, falling back to regular fetch:', err);
    return await fallbackFetch(unitId, version, files, cdnBaseUrl, title);
  }
}

// In-flight fallback downloads, keyed by "unitId-vVersion" ->
// { promise, aborter }. Two pages (the units page and the play page's
// completion overlay each construct their own delivery manager) can request
// a fallback for the same unit; only one sequential loop should run — its
// broadcasts reach every page. Keeping the run's promise lets a request
// that arrives while a canceled loop is still winding down wait for it to
// settle and then start, instead of being silently swallowed.
var fallbackRuns = new Map();

/**
 * Cancels in-flight fallback downloads. With unitId+version, cancels that
 * unit's loop; with neither, cancels all of them. purge=false (a retry-style
 * cancel) keeps the partially-written cache so the replacement loop can
 * resume from the last completed file; the default purges it (correct
 * before a cache delete/reset).
 */
function cancelActiveFallbacks(unitId, version, purge) {
  var doPurge = purge !== false;
  function cancelOne(run) {
    run.aborter.mhsPurgeOnAbort = doPurge;
    run.aborter.abort();
  }
  if (unitId && version) {
    var run = fallbackRuns.get(unitId + '-v' + version);
    if (run) cancelOne(run);
  } else {
    fallbackRuns.forEach(cancelOne);
  }
}

/**
 * Returns the keys ("unitId-vVersion") of fallback loops that are running
 * and not canceled — so a freshly-loaded page can adopt them.
 */
function listActiveFallbacks() {
  var keys = [];
  fallbackRuns.forEach(function(run, key) {
    if (!run.aborter.signal.aborted) keys.push(key);
  });
  return keys;
}

/**
 * Fetches one file and writes it to the cache, piping the body through a
 * byte counter so intra-file progress can be reported (a unit is dominated
 * by one large file, so per-file-boundary progress sits at 0% then jumps to
 * ~100%). The body is never buffered in SW memory; cache.put only stores a
 * fully-received response, so a mid-stream failure leaves no partial entry.
 */
async function fetchAndCacheFile(cache, cacheKey, url, onBytes, signal) {
  var response = await fetch(url, { mode: 'cors', signal: signal });
  if (!response.ok) {
    throw new Error('Failed to fetch ' + url + ': ' + response.status);
  }

  if (!response.body || !response.body.pipeThrough || typeof TransformStream === 'undefined') {
    await cache.put(cacheKey, response);
    return;
  }

  var received = 0;
  var counter = new TransformStream({
    transform: function(chunk, controller) {
      received += chunk.byteLength;
      onBytes(received);
      controller.enqueue(chunk);
    }
  });

  var headers = new Headers(response.headers);
  if (headers.get('content-encoding')) {
    // response.body is the decoded stream — encoding headers no longer apply.
    // Note: Content-Encoding is not a CORS-safelisted response header, so for
    // cross-origin CDN responses this branch cannot see it; the size verifiers
    // compensate by falling back to counting stored bytes on a header/manifest
    // size mismatch (see verifyLargestFileSize / _verifyLargestFile).
    headers.delete('content-encoding');
    headers.delete('content-length');
  }

  await cache.put(cacheKey, new Response(response.body.pipeThrough(counter), {
    status: response.status,
    statusText: response.statusText,
    headers: headers
  }));
}

// Retries a flaky file a couple of times with a short backoff before giving
// up on the whole unit. Cancellation (aborted signal) is never retried, and
// an abort that lands during the backoff sleep exits immediately — a
// canceled loop must not hold its dedupe slot for extra seconds.
async function fetchAndCacheFileWithRetry(cache, cacheKey, url, onBytes, signal) {
  var maxAttempts = 3;
  for (var attempt = 1; ; attempt++) {
    try {
      await fetchAndCacheFile(cache, cacheKey, url, onBytes, signal);
      return;
    } catch (err) {
      if ((signal && signal.aborted) || attempt >= maxAttempts) throw err;
      await new Promise(function(resolve) { setTimeout(resolve, attempt * 2000); });
      if (signal && signal.aborted) {
        throw new DOMException('Fallback download canceled', 'AbortError');
      }
    }
  }
}

/**
 * Fallback: download files using regular fetch when Background Fetch is unavailable.
 */
async function fallbackFetch(unitId, version, files, cdnBaseUrl, title) {
  var fallbackKey = unitId + '-v' + version;

  for (;;) {
    var existing = fallbackRuns.get(fallbackKey);
    if (!existing) break;
    if (!existing.aborter.signal.aborted) {
      // Another live sequential loop is already downloading this unit
      return true;
    }
    // A canceled loop is still winding down (its abort cleanup awaits a
    // cache delete). Wait for it to settle rather than being swallowed by
    // the dedupe — this also guarantees the old loop's terminal broadcast
    // lands before the new loop's first 'downloading'.
    try { await existing.promise; } catch (err) { /* settled is all we need */ }
  }

  var aborter = new AbortController();
  var promise = runFallbackFetch(fallbackKey, unitId, version, files, cdnBaseUrl, aborter);
  fallbackRuns.set(fallbackKey, { promise: promise, aborter: aborter });
  return promise;
}

// The actual sequential download loop, tracked in fallbackRuns by fallbackFetch.
async function runFallbackFetch(fallbackKey, unitId, version, files, cdnBaseUrl, aborter) {
  var cacheName = unitCacheName(unitId, version);
  var totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);
  var downloaded = 0;
  var lastBroadcast = 0;

  // All broadcasts from this loop carry the version so pages can ignore
  // statuses for a version they are not on — a canceled old-version loop
  // winding down after a deploy must not clear tracking (or flash statuses)
  // for a page that is downloading the unit's NEW version.
  function broadcastProgress(bytes) {
    var percent = totalSize > 0 ? Math.round((bytes / totalSize) * 100) : 0;
    broadcastStatus(unitId, 'downloading', {
      downloaded: bytes,
      downloadTotal: totalSize,
      percent: percent,
      version: version
    });
  }

  broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize, downloaded: 0, percent: 0, version: version });

  // Everything that can reject stays inside the try: a failure before the
  // finally would leak this run's fallbackRuns entry, permanently swallowing
  // future download requests for this unit+version until the SW restarts.
  try {
    var cache = await caches.open(cacheName);

    for (var i = 0; i < files.length; i++) {
      if (aborter.signal.aborted) {
        throw new DOMException('Fallback download canceled', 'AbortError');
      }

      var file = files[i];
      var cacheKey = cacheKeyFromPath(file.path);

      // Skip files already in cache (e.g., from a previous partial download)
      var existing = await cache.match(cacheKey);
      if (existing) {
        downloaded += file.size;
        broadcastProgress(downloaded);
        continue;
      }

      var url = cdnBaseUrl + '/' + file.path;
      await fetchAndCacheFileWithRetry(cache, cacheKey, url, function(fileReceived) {
        // Throttle intra-file progress broadcasts
        var now = Date.now();
        if (now - lastBroadcast < 1000) return;
        lastBroadcast = now;
        broadcastProgress(downloaded + fileReceived);
      }, aborter.signal);

      downloaded += file.size;
      broadcastProgress(downloaded);
    }

    broadcastStatus(unitId, 'cached', { version: version });
    return true;
  } catch (err) {
    if (aborter.signal.aborted) {
      if (aborter.mhsPurgeOnAbort !== false) {
        // Canceled by a Clear/Reset/delete — remove anything this loop wrote
        // after the purge, and confirm the cleared state to all pages.
        try { await caches.delete(cacheName); } catch (delErr) { /* best effort */ }
        broadcastStatus(unitId, 'not_cached', { version: version });
      }
      // A retry-style cancel (purge=false) keeps the partial cache for
      // resume and broadcasts nothing — a terminal broadcast would clear
      // the retrying page's fresh tracking; the replacement loop's
      // 'downloading' takes over instead.
      return false;
    }
    console.error('Fallback fetch failed for ' + unitId + ':', err);
    // Keep already-cached files: the skip-already-cached check above lets a
    // retry resume where this attempt stopped — essential on flaky networks,
    // where re-downloading a large unit from zero may never converge.
    // Reclamation of abandoned partials is handled by pruneStaleCaches
    // (version changes) and the explicit Clear/Reset controls.
    broadcastStatus(unitId, 'error', { error: err.message, version: version });
    return false;
  } finally {
    fallbackRuns.delete(fallbackKey);
  }
}

/**
 * Handles the backgroundfetchsuccess event: re-keys responses to same-origin URLs.
 * Does NOT fetch the manifest — derives cache keys directly from CDN URLs.
 */
async function handleBackgroundFetchSuccess(event) {
  var bgFetch = event.registration;
  var parsed = parseFetchId(bgFetch.id);
  var unitId = parsed.unitId;
  var version = parsed.version;

  if (!version) {
    // Legacy fetch ID without version — try to get version from manifest as last resort
    try {
      var manifestResponse = await fetch('/missionhydrosci/api/manifest');
      var manifest = await manifestResponse.json();
      var unit = manifest.units.find(function(u) { return u.id === unitId; });
      if (unit) version = unit.version;
    } catch (err) {
      console.error('Cannot determine version for', bgFetch.id);
    }
    if (!version) {
      console.error('No version for Background Fetch:', bgFetch.id);
      broadcastStatus(unitId, 'error', { error: 'Download completed but caching failed.' });
      return;
    }
  }

  var cacheName = unitCacheName(unitId, version);
  var cache = await caches.open(cacheName);
  var records = await bgFetch.matchAll();
  var cached = 0;

  for (var i = 0; i < records.length; i++) {
    var record = records[i];
    try {
      var response = await record.responseReady;
      // Derive cache key from CDN URL:
      // CDN URL: https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js
      // We need: /missionhydrosci/content/unit1/v1.0.0/Build/unit1.loader.js
      // Strategy: find "unitId/" in the pathname and build the key from there.
      var url = new URL(record.request.url);
      var pathname = decodeURIComponent(url.pathname);
      var marker = '/' + unitId + '/';
      var idx = pathname.indexOf(marker);
      if (idx !== -1) {
        var relativePath = pathname.substring(idx + 1); // "unit1/v1.0.0/Build/unit1.loader.js"
        var cacheKey = '/missionhydrosci/content/' + relativePath;
        await cache.put(cacheKey, response.clone());
        cached++;
      }
    } catch (err) {
      console.error('Failed to cache record:', record.request.url, err);
    }
  }

  console.log('Background Fetch success:', bgFetch.id, '- cached', cached, 'of', records.length, 'files');
  broadcastStatus(unitId, 'cached', { version: version });
}

/**
 * Handles the backgroundfetchfail event: cleans up partial cache.
 */
async function handleBackgroundFetchFailure(event) {
  var bgFetch = event.registration;
  var parsed = parseFetchId(bgFetch.id);

  console.error('Background Fetch failed:', bgFetch.id, 'reason:', bgFetch.failureReason);
  var failDetail = {
    error: 'Download failed. Please check your connection and try again.',
    // 'download-total-exceeded' tells the client its manifest sizes are stale
    // (e.g. a same-version re-upload) so it can refresh before a retry.
    failureReason: bgFetch.failureReason || ''
  };
  if (parsed.version) failDetail.version = parsed.version;
  broadcastStatus(parsed.unitId, 'error', failDetail);
}

/**
 * Handles backgroundfetchclick: navigate user to units page.
 */
function handleBackgroundFetchClick(event) {
  event.waitUntil(
    clients.openWindow('/missionhydrosci/units')
  );
}

/**
 * Broadcasts a status update to all clients via BroadcastChannel.
 * One shared channel for the SW's lifetime — progress broadcasts fire every
 * second during fallback downloads, so a per-message channel would be churn.
 */
var mhsStatusChannel = null;

function broadcastStatus(unitId, status, detail) {
  if (!mhsStatusChannel) {
    mhsStatusChannel = new BroadcastChannel(MHS_CHANNEL_NAME);
  }
  mhsStatusChannel.postMessage({
    type: 'status',
    unitId: unitId,
    status: status,
    detail: detail || {}
  });
}
