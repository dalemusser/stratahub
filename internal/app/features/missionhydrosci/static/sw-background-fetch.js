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

// Classifies a caught download/cache error into a machine-readable class + a
// student-friendly, actionable message. Used for both the UI status and the
// server-side telemetry (see mhs-delivery.js _reportDownloadError).
//   - quota:   out of device storage (QuotaExceededError).
//   - network: the transfer was interrupted mid-stream — the classic
//              "Cache.put() encountered a network error" / "Failed to fetch".
//              Usually flaky network or security software (antivirus/VPN)
//              inspecting downloads, NOT the user's account.
//   - generic: anything else.
function classifyDownloadError(err) {
  var name = (err && err.name) || '';
  var msg = (err && err.message) ? String(err.message) : String(err || '');
  if (name === 'QuotaExceededError' || /quota|exceeded the quota/i.test(msg)) {
    return {
      cls: 'quota',
      message: 'Not enough free space on this device to save the download. Free up space (or clear other downloads) and try again.'
    };
  }
  if (name === 'TypeError' || /cache\.put|network error|failed to fetch|load failed|networkerror/i.test(msg)) {
    return {
      cls: 'network',
      message: 'The download was interrupted. This is usually a network or security-software (antivirus/VPN) issue, not your account — check your connection and tap Retry. If it keeps failing, try a different network.'
    };
  }
  return { cls: 'generic', message: 'The download could not be completed. Please tap Retry.' };
}

// Fetch IDs whose progress broadcaster is attached in THIS SW lifetime. The
// SW can be terminated and restarted mid-download — the set resets with it,
// and the pages' periodic 'attachProgress' keepalive re-attaches.
var attachedProgressIds = new Set();

/**
 * Attaches a progress listener that broadcasts byte-level progress to every
 * client. Chrome streams live byte counts only to the context that created a
 * Background Fetch — a page context is lost on any reload or navigation,
 * freezing its display — so the SW (the one context shared by all pages)
 * owns creation and progress broadcasting. Idempotent per SW lifetime.
 */
function attachProgressBroadcast(bgFetch, unitId, version) {
  if (attachedProgressIds.has(bgFetch.id)) return;
  attachedProgressIds.add(bgFetch.id);
  bgFetch.addEventListener('progress', function() {
    var total = bgFetch.downloadTotal || 0;
    if (total > 0) {
      broadcastStatus(unitId, 'downloading', {
        downloaded: bgFetch.downloaded,
        downloadTotal: total,
        percent: Math.round((bgFetch.downloaded / total) * 100),
        version: version
      });
    }
  });
}

/**
 * (Re)attaches progress broadcasters to every in-flight MHS Background
 * Fetch. Sent periodically by pages while a download is active: the message
 * doubles as an SW keepalive so the listeners keep firing, and after an SW
 * restart it restores the listeners the restart dropped.
 */
async function reattachAllProgress() {
  if (!self.registration.backgroundFetch) return;
  try {
    var ids = await self.registration.backgroundFetch.getIds();
    for (var i = 0; i < ids.length; i++) {
      if (!ids[i].startsWith('missionhydrosci-')) continue;
      var bgFetch = await self.registration.backgroundFetch.get(ids[i]);
      if (!bgFetch || bgFetch.result !== '') continue;
      var parsed = parseFetchId(ids[i]);
      attachProgressBroadcast(bgFetch, parsed.unitId, parsed.version);
    }
  } catch (err) {
    // Best effort
  }
}

/**
 * Starts a background fetch for a unit's files. Called from the SW message
 * handler — pages request downloads here (rather than creating the fetch
 * themselves) so the SW is the creating context and can broadcast live
 * progress to every page; see attachProgressBroadcast.
 */
async function startBackgroundFetch(unitId, version, files, cdnBaseUrl, title) {
  var fetchId = 'missionhydrosci-' + unitId + '-v' + version;

  // SW-side dedupe across the two download paths. The SW is the only context
  // shared by every page and it survives page reloads, so this is the one
  // place the paths can be deduped reliably: if a sequential fallback loop is
  // already downloading this unit+version, adopt it (its broadcasts already
  // reach every page) instead of starting a concurrent Background Fetch of the
  // same files. Without this, a page that sends 'download' while the SW is
  // mid-fallback — or a shift-reload where the page can't see the loop via
  // getActiveFallbacks — starts a second full download over the same link.
  // (The reverse direction is intentionally NOT deduped: fallbackFetch must be
  // free to run even when a Background Fetch exists, because the prefer-fallback
  // path deliberately escapes a paused/zero-byte Background Fetch.)
  var fallbackKey = unitId + '-v' + version;
  var activeFallback = fallbackRuns.get(fallbackKey);
  if (activeFallback && !activeFallback.aborter.signal.aborted) {
    return true;
  }

  var requests = files.map(function(file) {
    return new Request(cdnBaseUrl + '/' + file.path, { mode: 'cors' });
  });

  var totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);

  try {
    // Check for an existing fetch with the same ID
    var existing = await self.registration.backgroundFetch.get(fetchId);
    if (existing && existing.result === '') {
      // Active fetch already in progress — adopt it: (re)attach the progress
      // broadcaster and rebroadcast its current state.
      attachProgressBroadcast(existing, unitId, version);
      if (existing.downloadTotal > 0) {
        broadcastStatus(unitId, 'downloading', {
          downloaded: existing.downloaded,
          downloadTotal: existing.downloadTotal,
          percent: Math.round((existing.downloaded / existing.downloadTotal) * 100),
          version: version
        });
      }
      return true;
    }

    var bgFetch = await self.registration.backgroundFetch.fetch(fetchId, requests, {
      title: title || 'Downloading ' + unitId,
      icons: [{ sizes: '192x192', src: '/assets/mhs/icon-192.png', type: 'image/png' }],
      // Real total makes the browser's native download indicator determinate.
      downloadTotal: totalSize
    });

    broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize, downloaded: 0, percent: 0, version: version });
    attachProgressBroadcast(bgFetch, unitId, version);

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
    // cross-origin CDN responses this branch cannot see it; the page-side size
    // verifier compensates by falling back to counting stored bytes on a
    // header/manifest size mismatch (see mhs-delivery.js _verifyLargestFile).
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
    var info = classifyDownloadError(err);
    broadcastStatus(unitId, 'error', {
      error: info.message,                                  // friendly, shown in UI
      errorClass: info.cls,                                 // machine-readable, for telemetry
      rawError: (err && err.message ? String(err.message) : String(err)).slice(0, 500),
      path: 'fallback',
      version: version
    });
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
  var firstErr = null;

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
        // Put the response directly — it isn't read again, so cloning here only
        // tees the body and buffers the whole (multi-hundred-MB) file in SW
        // memory with no backpressure, on exactly the low-end devices the OS
        // kills first.
        await cache.put(cacheKey, response);
        cached++;
      } else {
        // Couldn't map this record to a cache key — count it as not cached so
        // we don't later declare the unit complete with a file missing.
        console.error('Background Fetch record has no unit marker, not cached:', record.request.url);
      }
    } catch (err) {
      console.error('Failed to cache record:', record.request.url, err);
      if (!firstErr) firstErr = err; // representative error for messaging + telemetry
    }
  }

  console.log('Background Fetch success:', bgFetch.id, '- cached', cached, 'of', records.length, 'files');

  // Only "Ready to play" if EVERY file actually made it into the cache. A
  // per-record cache.put can fail mid-copy (QuotaExceededError is the #1 field
  // failure), and an unmappable record is skipped above — broadcasting 'cached'
  // then would show "Ready to play" over an incomplete unit, and the gap would
  // surface only later, worst case offline where the CDN-redirect fallback
  // can't help. Report 'error' so the unit stays not-ready and can be retried.
  if (records.length === 0 || cached !== records.length) {
    console.error('Background Fetch incomplete cache:', cached, 'of', records.length, '— reporting error');
    // Attribute to the actual failure when we caught one (network interruption
    // vs. genuinely out of space) rather than always blaming storage.
    var info = firstErr
      ? classifyDownloadError(firstErr)
      : { cls: 'incomplete', message: 'The download finished but some files could not be saved. This is usually a network interruption or low device storage — please tap Retry.' };
    broadcastStatus(unitId, 'error', {
      version: version,
      error: info.message,
      errorClass: info.cls,
      rawError: firstErr && firstErr.message ? String(firstErr.message).slice(0, 500) : ('cached ' + cached + ' of ' + records.length),
      path: 'bg'
    });
    return;
  }

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
    failureReason: bgFetch.failureReason || '',
    errorClass: bgFetch.failureReason ? ('bgfetch-' + bgFetch.failureReason) : 'bgfetch-failed',
    path: 'bg'
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
