// sw-background-fetch.js — Background Fetch handlers for the MHS service worker.

const MHS_CHANNEL_NAME = 'mhs-delivery';

/**
 * Starts a background fetch for a unit's files.
 * @param {string} unitId - The unit ID (e.g., "unit1").
 * @param {string} version - The unit version (e.g., "1.0.0").
 * @param {Array} files - Array of {path, size} from the content manifest.
 * @param {string} cdnBaseUrl - CDN base URL (e.g., "https://cdn.adroit.games/mhs").
 * @param {string} title - Display title for the download notification.
 */
async function startBackgroundFetch(unitId, version, files, cdnBaseUrl, title) {
  const fetchId = 'mhs-' + unitId;

  // Build requests for each file
  const requests = files.map(function(file) {
    return new Request(cdnBaseUrl + '/' + file.path, {
      mode: 'cors'
    });
  });

  const totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);

  try {
    const bgFetch = await self.registration.backgroundFetch.fetch(fetchId, requests, {
      title: title || 'Downloading ' + unitId,
      icons: [{
        sizes: '192x192',
        src: '/assets/mhs/icon-192.png',
        type: 'image/png'
      }],
      downloadTotal: 0
    });

    broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize });

    bgFetch.addEventListener('progress', function() {
      if (totalSize > 0) {
        const percent = Math.round((bgFetch.downloaded / totalSize) * 100);
        broadcastStatus(unitId, 'downloading', {
          downloaded: bgFetch.downloaded,
          downloadTotal: totalSize,
          percent: percent
        });
      }
    });

    return true;
  } catch (err) {
    // Background Fetch not supported or failed — fall back to regular fetch
    console.warn('Background Fetch failed, falling back to regular fetch:', err);
    return await fallbackFetch(unitId, version, files, cdnBaseUrl, title);
  }
}

/**
 * Fallback: download files using regular fetch when Background Fetch is unavailable.
 */
async function fallbackFetch(unitId, version, files, cdnBaseUrl, title) {
  const cacheName = unitCacheName(unitId, version);
  const cache = await caches.open(cacheName);
  const totalSize = files.reduce(function(sum, f) { return sum + f.size; }, 0);
  let downloaded = 0;

  broadcastStatus(unitId, 'downloading', { downloadTotal: totalSize, downloaded: 0, percent: 0 });

  try {
    for (const file of files) {
      const url = cdnBaseUrl + '/' + file.path;
      const response = await fetch(url, { mode: 'cors' });

      if (!response.ok) {
        throw new Error('Failed to fetch ' + file.path + ': ' + response.status);
      }

      const cacheKey = cacheKeyFromPath(file.path);
      await cache.put(cacheKey, response);

      downloaded += file.size;
      const percent = Math.round((downloaded / totalSize) * 100);
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
 */
async function handleBackgroundFetchSuccess(event) {
  const bgFetch = event.registration;
  const fetchId = bgFetch.id; // e.g., "mhs-unit1"
  const unitId = fetchId.replace('mhs-', '');

  // We need to find the unit version from the manifest
  let manifest = null;
  try {
    const manifestResponse = await fetch('/mhs/api/manifest');
    manifest = await manifestResponse.json();
  } catch (err) {
    console.error('Failed to fetch manifest during background fetch success:', err);
    return;
  }

  const unit = manifest.units.find(function(u) { return u.id === unitId; });
  if (!unit) {
    console.error('Unit not found in manifest:', unitId);
    return;
  }

  const cacheName = unitCacheName(unitId, unit.version);
  const cache = await caches.open(cacheName);
  const records = await bgFetch.matchAll();

  for (const record of records) {
    const response = await record.responseReady;
    // Extract file path from the CDN URL
    const url = new URL(record.request.url);
    // The CDN URL path is like /mhs/unit1/Build/unit1.data.br
    // We need to strip the /mhs/ prefix that's part of cdnBaseUrl path
    const cdnPath = url.pathname;
    // Find the matching file path from the manifest
    const matchingFile = unit.files.find(function(f) {
      return cdnPath.endsWith(f.path);
    });

    if (matchingFile) {
      const cacheKey = cacheKeyFromPath(matchingFile.path);
      await cache.put(cacheKey, response.clone());
    }
  }

  broadcastStatus(unitId, 'cached', {});
}

/**
 * Handles the backgroundfetchfail event: cleans up partial cache.
 */
async function handleBackgroundFetchFailure(event) {
  const bgFetch = event.registration;
  const fetchId = bgFetch.id;
  const unitId = fetchId.replace('mhs-', '');

  broadcastStatus(unitId, 'error', {
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
  const channel = new BroadcastChannel(MHS_CHANNEL_NAME);
  channel.postMessage({
    type: 'status',
    unitId: unitId,
    status: status,
    detail: detail || {}
  });
  channel.close();
}
