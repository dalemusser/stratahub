// sw-cache.js — Cache utility functions for the Mission HydroSci service worker.

const APP_SHELL_CACHE = 'missionhydrosci-app-shell-v6';
const UNIT_CACHE_PREFIX = 'missionhydrosci-unit-';

// Returns the versioned URL for a hash-versioned asset, matching the
// ?v=<hash> URLs the page HTML uses. MHS_ASSET_VERSIONS is injected by the
// Go handler that serves this worker (sw.go).
function versionedAssetURL(path, version) {
  if (typeof MHS_ASSET_VERSIONS !== 'undefined' && version) {
    return path + '?v=' + version;
  }
  return path;
}

// App shell resources to pre-cache on install. Versioned assets are cached
// under their exact ?v=<hash> URL so a deploy's new hash is a cache miss and
// is fetched fresh on the first page load (no stale copy, no second reload).
const APP_SHELL_URLS = [
  '/missionhydrosci/units',
  versionedAssetURL('/assets/css/tailwind.css', typeof MHS_ASSET_VERSIONS !== 'undefined' ? MHS_ASSET_VERSIONS.tailwind : ''),
  versionedAssetURL('/assets/js/mhs-delivery.js', typeof MHS_ASSET_VERSIONS !== 'undefined' ? MHS_ASSET_VERSIONS.mhsDelivery : '')
];

/**
 * Returns the cache name for a specific unit + version.
 * e.g., 'missionhydrosci-unit-unit1-v1.0.0'
 */
function unitCacheName(unitId, version) {
  return UNIT_CACHE_PREFIX + unitId + '-v' + version;
}

/**
 * Converts a CDN file path to a same-origin cache key.
 * e.g., "unit1/Build/unit1.data.br" -> "/missionhydrosci/content/unit1/Build/unit1.data.br"
 */
function cacheKeyFromPath(filePath) {
  return '/missionhydrosci/content/' + filePath;
}

/**
 * Checks if all files for a unit are cached.
 * @param {string} unitId
 * @param {string} version
 * @param {Array} files - Array of {path, size} objects.
 * @returns {Promise<'cached'|'not_cached'|'partial'>}
 */
async function checkUnitCacheStatus(unitId, version, files) {
  const cache = await caches.open(unitCacheName(unitId, version));
  let found = 0;

  for (const file of files) {
    const key = cacheKeyFromPath(file.path);
    const match = await cache.match(key);
    if (match) {
      found++;
    }
  }

  if (found === files.length) {
    // Spot-check the largest file's cached size so eviction or partial
    // artifacts can't produce a false 'cached'.
    const ok = await verifyLargestFileSize(cache, files);
    return ok ? 'cached' : 'partial';
  }
  if (found === 0) return 'not_cached';
  return 'partial';
}

/**
 * Verifies the largest file's cached size against its manifest size.
 * Returns true when the size matches or cannot be determined.
 */
async function verifyLargestFileSize(cache, files) {
  let largest = null;
  for (const file of files) {
    if (!largest || (file.size || 0) > (largest.size || 0)) largest = file;
  }
  if (!largest || !largest.size) return true;

  const match = await cache.match(cacheKeyFromPath(largest.path));
  if (!match) return false;
  const len = match.headers.get('content-length');
  if (len === null) return true; // size unknown (e.g. chunked) — accept
  if (parseInt(len, 10) === largest.size) return true;

  // The header can disagree with the stored body (e.g. transfer compression
  // keeps a compressed content-length on a decoded body). Count the actual
  // stored bytes before declaring the unit incomplete — this rare path must
  // not condemn a good unit to endless re-downloads.
  try {
    const blob = await match.clone().blob();
    if (blob.size !== largest.size) return false;
    // The header/body mismatch is persistent for the life of the entry, so
    // without a repair every future check re-reads this entire file — a
    // multi-hundred-MB read per check on a low-end device. We already hold
    // the bytes: rewrite the entry with a corrected content-length so
    // future checks take the header fast-path.
    try {
      const headers = new Headers(match.headers);
      headers.set('content-length', String(blob.size));
      await cache.put(cacheKeyFromPath(largest.path), new Response(blob, {
        status: match.status,
        statusText: match.statusText,
        headers: headers
      }));
    } catch (repairErr) {
      // Verification already passed — the repair is only an optimization
    }
    return true;
  } catch (err) {
    return true; // can't verify — don't block a possibly-good unit
  }
}
