// sw-cache.js — Cache utility functions for the MHS service worker.

const APP_SHELL_CACHE = 'mhs-app-shell-v1';
const UNIT_CACHE_PREFIX = 'mhs-unit-';

// App shell resources to pre-cache on install.
const APP_SHELL_URLS = [
  '/mhs/units',
  '/assets/css/tailwind.css',
  '/assets/js/mhs-delivery.js'
];

/**
 * Returns the cache name for a specific unit + version.
 * e.g., 'mhs-unit-unit1-v1.0.0'
 */
function unitCacheName(unitId, version) {
  return UNIT_CACHE_PREFIX + unitId + '-v' + version;
}

/**
 * Converts a CDN file path to a same-origin cache key.
 * e.g., "unit1/Build/unit1.data.br" -> "/mhs/content/unit1/Build/unit1.data.br"
 */
function cacheKeyFromPath(filePath) {
  return '/mhs/content/' + filePath;
}

/**
 * Cleans up old caches that don't match current unit versions.
 * @param {ContentManifest} manifest - The current content manifest.
 */
async function cleanStaleCaches(manifest) {
  const validCacheNames = new Set([APP_SHELL_CACHE]);
  for (const unit of manifest.units) {
    validCacheNames.add(unitCacheName(unit.id, unit.version));
  }

  const allCaches = await caches.keys();
  for (const cacheName of allCaches) {
    if (cacheName.startsWith(UNIT_CACHE_PREFIX) || cacheName.startsWith('mhs-app-shell-')) {
      if (!validCacheNames.has(cacheName)) {
        await caches.delete(cacheName);
      }
    }
  }
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

  if (found === files.length) return 'cached';
  if (found === 0) return 'not_cached';
  return 'partial';
}
