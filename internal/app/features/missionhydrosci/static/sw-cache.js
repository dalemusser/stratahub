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

// NOTE: unit cache-status checking and largest-file size verification live on
// the page side (mhs-delivery.js: _checkUnitCache / _verifyLargestFile). The SW
// copies were only reachable via a 'checkStatus' message action that no page
// sends anymore, so they were removed to avoid two hand-maintained copies
// drifting apart.
