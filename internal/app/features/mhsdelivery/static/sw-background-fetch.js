// sw-background-fetch.js — Background Fetch handlers for the MHS service worker.

const MHS_CHANNEL_NAME = 'mhs-delivery';

// Fetch ID format: "mhs-{unitId}-v{version}" e.g., "mhs-unit1-v1.0.0"
// This encodes everything we need to cache responses without fetching the manifest.

function parseFetchId(fetchId) {
  // "mhs-unit1-v1.0.0" -> { unitId: "unit1", version: "1.0.0" }
  var match = fetchId.match(/^mhs-(.+)-v(\d+\.\d+\.\d+)$/);
  if (match) {
    return { unitId: match[1], version: match[2] };
  }
  // Legacy format: "mhs-unit1"
  return { unitId: fetchId.replace('mhs-', ''), version: null };
}

/**
 * Starts a background fetch for a unit's files.
 * Called from the SW message handler as a fallback when page-initiated fetch isn't available.
 */
async function startBackgroundFetch(unitId, version, files, cdnBaseUrl, title) {
  var fetchId = 'mhs-' + unitId + '-v' + version;

  var requests = files.map(function(file) {
    return new Request(cdnBaseUrl + '/' + file.path, { mode: 'cors' });
  });

  var totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);

  try {
    // Abort any existing fetch with the same ID
    var existing = await self.registration.backgroundFetch.get(fetchId);
    if (existing) {
      await existing.abort();
    }

    var bgFetch = await self.registration.backgroundFetch.fetch(fetchId, requests, {
      title: title || 'Downloading ' + unitId,
      icons: [{ sizes: '192x192', src: '/assets/mhs/icon-192.png', type: 'image/png' }],
      downloadTotal: 0
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

/**
 * Fallback: download files using regular fetch when Background Fetch is unavailable.
 */
async function fallbackFetch(unitId, version, files, cdnBaseUrl, title) {
  var cacheName = unitCacheName(unitId, version);
  var cache = await caches.open(cacheName);
  var totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);
  var downloaded = 0;

  broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize, downloaded: 0, percent: 0 });

  try {
    for (var i = 0; i < files.length; i++) {
      var file = files[i];
      var url = cdnBaseUrl + '/' + file.path;
      var response = await fetch(url, { mode: 'cors' });

      if (!response.ok) {
        throw new Error('Failed to fetch ' + file.path + ': ' + response.status);
      }

      var cacheKey = cacheKeyFromPath(file.path);
      await cache.put(cacheKey, response);

      downloaded += file.size;
      var percent = Math.round((downloaded / totalSize) * 100);
      broadcastStatus(unitId, 'downloading', {
        downloaded: downloaded,
        downloadTotal: totalSize,
        percent: percent
      });
    }

    broadcastStatus(unitId, 'cached', {});
    return true;
  } catch (err) {
    console.error('Fallback fetch failed for ' + unitId + ':', err);
    broadcastStatus(unitId, 'error', { error: err.message });
    return false;
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
      var manifestResponse = await fetch('/mhs/api/manifest');
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
      // We need: /mhs/content/unit1/v1.0.0/Build/unit1.loader.js
      // Strategy: find "unitId/" in the pathname and build the key from there.
      var url = new URL(record.request.url);
      var pathname = decodeURIComponent(url.pathname);
      var marker = '/' + unitId + '/';
      var idx = pathname.indexOf(marker);
      if (idx !== -1) {
        var relativePath = pathname.substring(idx + 1); // "unit1/v1.0.0/Build/unit1.loader.js"
        var cacheKey = '/mhs/content/' + relativePath;
        await cache.put(cacheKey, response.clone());
        cached++;
      }
    } catch (err) {
      console.error('Failed to cache record:', record.request.url, err);
    }
  }

  console.log('Background Fetch success:', bgFetch.id, '- cached', cached, 'of', records.length, 'files');
  broadcastStatus(unitId, 'cached', {});
}

/**
 * Handles the backgroundfetchfail event: cleans up partial cache.
 */
async function handleBackgroundFetchFailure(event) {
  var bgFetch = event.registration;
  var parsed = parseFetchId(bgFetch.id);

  console.error('Background Fetch failed:', bgFetch.id, 'reason:', bgFetch.failureReason);
  broadcastStatus(parsed.unitId, 'error', {
    error: 'Download failed. Please check your connection and try again.'
  });
}

/**
 * Handles backgroundfetchclick: navigate user to units page.
 */
function handleBackgroundFetchClick(event) {
  event.waitUntil(
    clients.openWindow('/mhs/units')
  );
}

/**
 * Broadcasts a status update to all clients via BroadcastChannel.
 */
function broadcastStatus(unitId, status, detail) {
  var channel = new BroadcastChannel(MHS_CHANNEL_NAME);
  channel.postMessage({
    type: 'status',
    unitId: unitId,
    status: status,
    detail: detail || {}
  });
  channel.close();
}
