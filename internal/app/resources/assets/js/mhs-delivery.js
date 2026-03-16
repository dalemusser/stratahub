// mhs-delivery.js — Client-side download manager for MHS content delivery PWA.

(function() {
  'use strict';

  var DEFAULT_SW_URL = '/sw.js';
  var DEFAULT_SW_SCOPE = '/';
  var DEFAULT_MANIFEST_URL = '/mhs/api/manifest';
  var DEFAULT_CONTENT_PREFIX = '/mhs/content/';
  var DEFAULT_CHANNEL_NAME = 'mhs-delivery';
  var DEFAULT_UNIT_CACHE_PREFIX = 'mhs-unit-';
  var DEFAULT_FETCH_ID_PREFIX = 'mhs-';

  /**
   * MHSDeliveryManager manages content downloads and cache status.
   * @param {Object} [opts] - Optional configuration overrides.
   * @param {string} [opts.swUrl] - Service worker URL (default: '/sw.js')
   * @param {string} [opts.manifestUrl] - Content manifest API URL (default: '/mhs/api/manifest')
   * @param {string} [opts.contentPrefix] - Content path prefix (default: '/mhs/content/')
   * @param {string} [opts.channelName] - BroadcastChannel name (default: 'mhs-delivery')
   * @param {string} [opts.unitCachePrefix] - Cache name prefix (default: 'mhs-unit-')
   */
  function MHSDeliveryManager(opts) {
    opts = opts || {};
    this._swUrl = opts.swUrl || DEFAULT_SW_URL;
    this._swScope = opts.swScope || DEFAULT_SW_SCOPE;
    this._manifestUrl = opts.manifestUrl || DEFAULT_MANIFEST_URL;
    this._contentPrefix = opts.contentPrefix || DEFAULT_CONTENT_PREFIX;
    this._channelName = opts.channelName || DEFAULT_CHANNEL_NAME;
    this._unitCachePrefix = opts.unitCachePrefix || DEFAULT_UNIT_CACHE_PREFIX;
    this._fetchIdPrefix = opts.fetchIdPrefix || DEFAULT_FETCH_ID_PREFIX;
    this.manifest = null;
    this.swRegistration = null;
    this.channel = null;
    this.statusCallbacks = [];
    this._activeDownloads = {}; // unitId -> true while a download is in progress
    this._downloadMonitors = {}; // unitId -> intervalId for stall detection
    this._stallState = {}; // unitId -> { lastDownloaded, stallCount } — exposed for reset on visibility change
  }

  /**
   * Initializes the delivery manager: registers SW, fetches manifest,
   * sets up BroadcastChannel listener, and checks initial cache status.
   */
  MHSDeliveryManager.prototype.init = async function() {
    // Register service worker (skip when swUrl is explicitly null/empty)
    if (this._swUrl && 'serviceWorker' in navigator) {
      try {
        this.swRegistration = await navigator.serviceWorker.register(this._swUrl, {
          scope: this._swScope
        });
        console.log('MHS Service Worker registered');
      } catch (err) {
        console.error('MHS SW registration failed:', err);
      }
    }

    // Set up BroadcastChannel for status updates from SW
    if (typeof BroadcastChannel !== 'undefined') {
      this.channel = new BroadcastChannel(this._channelName);
      var self = this;
      this.channel.addEventListener('message', function(event) {
        self._handleStatusUpdate(event.data);
      });
    }

    // Fetch content manifest
    await this.refreshManifest();

    // Reconnect to any active Background Fetches from previous sessions
    await this._reconnectActiveDownloads();

    // Check initial cache status for all units
    await this.checkAllCacheStatus();

    // When tab becomes visible: reset stall monitors, recheck BG fetch states, then cache status
    var self = this;
    document.addEventListener('visibilitychange', function() {
      if (document.visibilityState === 'visible') {
        // Reset stall monitors — bgFetch.downloaded is stale after being hidden,
        // so any check immediately after becoming visible would false-detect a stall.
        // Setting lastDownloaded to -1 guarantees the first check sees "progress."
        var ids = Object.keys(self._stallState);
        for (var i = 0; i < ids.length; i++) {
          self._stallState[ids[i]].stallCount = 0;
          self._stallState[ids[i]].lastDownloaded = -1;
        }
        self._recheckActiveDownloads(); // Clear completed BG fetches from tracking
        self.checkAllCacheStatus();      // Detect cached/not_cached for cleared units
      }
    });
  };

  /**
   * Fetches the content manifest from the server.
   */
  MHSDeliveryManager.prototype.refreshManifest = async function() {
    try {
      var response = await fetch(this._manifestUrl);
      this.manifest = await response.json();
    } catch (err) {
      console.error('Failed to fetch MHS content manifest:', err);
      this.manifest = { cdnBaseUrl: '', units: [] };
    }
  };

  /**
   * Registers a callback for status updates.
   * Callback receives: (unitId, status, detail)
   */
  MHSDeliveryManager.prototype.onStatus = function(callback) {
    this.statusCallbacks.push(callback);
  };

  /**
   * Checks cache status for all units and fires status callbacks.
   */
  MHSDeliveryManager.prototype.checkAllCacheStatus = async function() {
    if (!this.manifest || !this.manifest.units) return;

    for (var i = 0; i < this.manifest.units.length; i++) {
      var unit = this.manifest.units[i];
      // Don't override status for units with an active download
      if (this._activeDownloads[unit.id]) continue;
      var status = await this._checkUnitCache(unit);
      this._fireStatus(unit.id, status, {});
    }
  };

  /**
   * Checks if all files for a unit are cached.
   * @returns {Promise<string>} 'cached', 'not_cached', or 'partial'
   */
  MHSDeliveryManager.prototype._checkUnitCache = async function(unit) {
    if (!('caches' in window)) return 'not_cached';

    var cacheName = this._unitCachePrefix + unit.id + '-v' + unit.version;
    try {
      var cache = await caches.open(cacheName);
      var found = 0;

      for (var i = 0; i < unit.files.length; i++) {
        var key = this._contentPrefix + unit.files[i].path;
        var match = await cache.match(key);
        if (match) found++;
      }

      if (found === unit.files.length) return 'cached';
      if (found === 0) return 'not_cached';
      return 'partial';
    } catch (err) {
      return 'not_cached';
    }
  };

  /**
   * Waits for the service worker to become active, with a timeout.
   * Returns the ServiceWorkerRegistration or null if the SW is not ready.
   * This prevents navigator.serviceWorker.ready from hanging forever if the
   * SW fails to install/activate (e.g., due to cache.addAll failure).
   */
  MHSDeliveryManager.prototype._waitForSW = function(timeoutMs) {
    // Fast path: if our registered SW is already active, return it immediately
    if (this.swRegistration && this.swRegistration.active) {
      return Promise.resolve(this.swRegistration);
    }

    var ms = timeoutMs || 10000;
    return Promise.race([
      navigator.serviceWorker.ready,
      new Promise(function(resolve) {
        setTimeout(function() { resolve(null); }, ms);
      })
    ]);
  };

  /**
   * Extracts the unit ID from a Background Fetch ID.
   * e.g., "missionhydrosci-unit1-v1.0.0" -> "unit1"
   */
  MHSDeliveryManager.prototype._parseUnitIdFromFetchId = function(fetchId) {
    var prefix = this._fetchIdPrefix;
    if (!fetchId.startsWith(prefix)) return null;
    var rest = fetchId.substring(prefix.length); // "unit1-v1.0.0"
    var versionMatch = rest.match(/^(.+)-v\d+\.\d+\.\d+$/);
    if (versionMatch) return versionMatch[1];
    return rest; // Legacy format without version
  };

  /**
   * Monitors a Background Fetch for stalls. If no progress is made for 30s
   * (2 checks at 15s intervals), aborts the BG fetch and falls back to SW fetch.
   */
  MHSDeliveryManager.prototype._monitorDownload = function(unitId, bgFetch, unit) {
    // Clear any existing monitor for this unit
    if (this._downloadMonitors[unitId]) {
      clearInterval(this._downloadMonitors[unitId]);
      delete this._downloadMonitors[unitId];
    }

    // Stall state is on the manager (not closure) so the visibility handler can reset it
    this._stallState[unitId] = { lastDownloaded: bgFetch.downloaded || 0, stallCount: 0 };
    var self = this;

    this._downloadMonitors[unitId] = setInterval(function() {
      // BG fetch may have completed/failed — stop monitoring
      if (!self._activeDownloads[unitId]) {
        clearInterval(self._downloadMonitors[unitId]);
        delete self._downloadMonitors[unitId];
        delete self._stallState[unitId];
        return;
      }

      // Don't check when page is hidden — Chrome doesn't reliably update
      // bgFetch.downloaded on background pages, which causes false stall detection
      if (document.visibilityState === 'hidden') return;

      var state = self._stallState[unitId];
      if (!state) return;

      var current = bgFetch.downloaded || 0;
      if (current === state.lastDownloaded) {
        state.stallCount++;
        if (state.stallCount >= 2) {
          console.warn('Background Fetch stalled for', unitId, '- falling back to SW fetch');
          clearInterval(self._downloadMonitors[unitId]);
          delete self._downloadMonitors[unitId];
          delete self._stallState[unitId];

          // Abort the stalled BG fetch
          try { bgFetch.abort(); } catch (e) { /* best effort */ }

          // Fall back to SW sequential fetch
          if (navigator.serviceWorker && navigator.serviceWorker.controller) {
            navigator.serviceWorker.controller.postMessage({
              action: 'fallbackDownload',
              unitId: unit.id,
              version: unit.version,
              files: unit.files,
              cdnBaseUrl: self.manifest.cdnBaseUrl,
              title: 'Downloading ' + unit.title
            });
          }
        }
      } else {
        state.stallCount = 0;
        state.lastDownloaded = current;
      }
    }, 15000);
  };

  /**
   * Attaches progress listener and starts stall monitor for a BG fetch.
   * Used by both downloadUnit and _reconnectActiveDownloads.
   */
  MHSDeliveryManager.prototype._attachBGFetchListeners = function(unitId, bgFetch, unit) {
    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    var self = this;

    bgFetch.addEventListener('progress', function() {
      if (totalSize > 0) {
        var percent = Math.round((bgFetch.downloaded / totalSize) * 100);
        self._fireStatus(unitId, 'downloading', {
          downloaded: bgFetch.downloaded,
          downloadTotal: totalSize,
          percent: percent
        });
      }
    });

    this._monitorDownload(unitId, bgFetch, unit);
  };

  /**
   * Reconnects to active Background Fetches from previous sessions.
   * Instead of aborting all BG fetches, reconnects to in-progress ones
   * and skips completed/failed ones (checkAllCacheStatus handles those).
   */
  MHSDeliveryManager.prototype._reconnectActiveDownloads = async function() {
    if (!navigator.serviceWorker) return;

    try {
      var reg = await this._waitForSW(5000);
      if (!reg || !reg.backgroundFetch) return;

      var ids = await reg.backgroundFetch.getIds();
      for (var i = 0; i < ids.length; i++) {
        if (!ids[i].startsWith(this._fetchIdPrefix)) continue;

        var bgFetch = await reg.backgroundFetch.get(ids[i]);
        if (!bgFetch) continue;

        var unitId = this._parseUnitIdFromFetchId(ids[i]);
        if (!unitId) continue;

        if (bgFetch.result === '') {
          // In progress — find matching manifest unit
          var unit = this.manifest && this.manifest.units
            ? this.manifest.units.find(function(u) { return u.id === unitId; })
            : null;

          if (unit) {
            // Reconnect: set active tracking, fire current progress, attach listeners
            this._activeDownloads[unitId] = true;
            var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
            var percent = totalSize > 0 ? Math.round((bgFetch.downloaded / totalSize) * 100) : 0;
            this._fireStatus(unitId, 'downloading', {
              downloaded: bgFetch.downloaded,
              downloadTotal: totalSize,
              percent: percent
            });
            this._attachBGFetchListeners(unitId, bgFetch, unit);
            console.log('Reconnected to Background Fetch:', ids[i]);
          } else {
            // No matching manifest unit — genuinely stale, abort
            console.log('Aborting stale Background Fetch (no manifest unit):', ids[i]);
            await bgFetch.abort();
          }
        }
        // Completed/failed BG fetches are skipped — checkAllCacheStatus handles them
      }
    } catch (err) {
      // Background Fetch API may not be available — that's fine.
    }
  };

  /**
   * Rechecks active Background Fetches when tab becomes visible.
   * Handles BG fetches that completed/failed while tab was hidden.
   */
  MHSDeliveryManager.prototype._recheckActiveDownloads = function() {
    if (!navigator.serviceWorker) return;

    var self = this;
    var unitIds = Object.keys(this._activeDownloads);
    if (unitIds.length === 0) return;

    // Use _waitForSW as a promise — we need the registration
    this._waitForSW(5000).then(function(reg) {
      if (!reg || !reg.backgroundFetch) return;

      var checks = unitIds.map(function(unitId) {
        var unit = self.manifest && self.manifest.units
          ? self.manifest.units.find(function(u) { return u.id === unitId; })
          : null;
        if (!unit) return Promise.resolve();

        var fetchId = self._fetchIdPrefix + unit.id + '-v' + unit.version;
        return reg.backgroundFetch.get(fetchId).then(function(bgFetch) {
          if (!bgFetch) {
            // BG fetch disappeared — clear tracking so checkAllCacheStatus can detect state
            delete self._activeDownloads[unitId];
            return;
          }

          if (bgFetch.result === 'success') {
            // Completed while hidden — clear tracking so checkAllCacheStatus fires 'cached'
            delete self._activeDownloads[unitId];
            if (self._downloadMonitors[unitId]) {
              clearInterval(self._downloadMonitors[unitId]);
              delete self._downloadMonitors[unitId];
            }
          } else if (bgFetch.result === 'failure') {
            // Failed while hidden — clear tracking and fire error
            delete self._activeDownloads[unitId];
            if (self._downloadMonitors[unitId]) {
              clearInterval(self._downloadMonitors[unitId]);
              delete self._downloadMonitors[unitId];
            }
            self._fireStatus(unitId, 'error', {
              error: 'Download failed. Please check your connection and try again.'
            });
          } else {
            // Still in progress — fire current progress for UI update
            var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
            var percent = totalSize > 0 ? Math.round((bgFetch.downloaded / totalSize) * 100) : 0;
            self._fireStatus(unitId, 'downloading', {
              downloaded: bgFetch.downloaded,
              downloadTotal: totalSize,
              percent: percent
            });
          }
        }).catch(function() {
          // Ignore errors for individual checks
        });
      });

      return Promise.all(checks);
    }).catch(function() {
      // Ignore — best effort
    });
  };

  /**
   * Starts downloading a unit's files via Background Fetch.
   * If a BG fetch is already in progress for this unit, reconnects to it
   * instead of aborting. Falls back to SW sequential fetch if BG Fetch
   * API is unavailable or fails to start.
   */
  MHSDeliveryManager.prototype.downloadUnit = async function(unitId) {
    if (!this.manifest) {
      console.error('Manifest not loaded');
      return;
    }

    var unit = this.manifest.units.find(function(u) { return u.id === unitId; });
    if (!unit) {
      console.error('Unit not found:', unitId);
      return;
    }

    if (!navigator.serviceWorker) {
      this._fireStatus(unitId, 'error', { error: 'Service worker not supported.' });
      return;
    }

    // Prevent duplicate downloads — if already active, skip
    if (this._activeDownloads[unitId]) {
      return;
    }

    this._activeDownloads[unitId] = true;
    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    this._fireStatus(unitId, 'downloading', { percent: 0, downloaded: 0, downloadTotal: totalSize });

    try {
      var reg = await this._waitForSW(10000);
      if (!reg) {
        throw new Error('Service worker not ready. Please refresh the page and try again.');
      }

      var fetchId = this._fetchIdPrefix + unit.id + '-v' + unit.version;

      // Check for an existing BG fetch with the same ID
      if (reg.backgroundFetch) {
        var existing = await reg.backgroundFetch.get(fetchId);
        if (existing && existing.result === '') {
          // Active fetch already in progress — reconnect instead of aborting
          var percent = totalSize > 0 ? Math.round((existing.downloaded / totalSize) * 100) : 0;
          this._fireStatus(unitId, 'downloading', {
            downloaded: existing.downloaded,
            downloadTotal: totalSize,
            percent: percent
          });
          this._attachBGFetchListeners(unitId, existing, unit);
          console.log('Reconnected to existing Background Fetch:', fetchId);
          return;
        }
      }

      // No existing fetch — start a new Background Fetch
      if (reg.backgroundFetch) {
        var requests = unit.files.map(function(file) {
          return new Request(this.manifest.cdnBaseUrl + '/' + file.path, { mode: 'cors' });
        }.bind(this));

        try {
          var bgFetch = await reg.backgroundFetch.fetch(fetchId, requests, {
            title: 'Downloading ' + unit.title,
            icons: [{ sizes: '192x192', src: '/assets/mhs/icon-192.png', type: 'image/png' }],
            downloadTotal: 0
          });

          this._attachBGFetchListeners(unitId, bgFetch, unit);
          console.log('Started Background Fetch:', fetchId);
          return;
        } catch (bgErr) {
          console.warn('Background Fetch failed to start, falling back to SW fetch:', bgErr);
          // Fall through to SW sequential fetch below
        }
      }

      // Fallback: use SW sequential fetch
      if (navigator.serviceWorker.controller) {
        navigator.serviceWorker.controller.postMessage({
          action: 'fallbackDownload',
          unitId: unit.id,
          version: unit.version,
          files: unit.files,
          cdnBaseUrl: this.manifest.cdnBaseUrl,
          title: 'Downloading ' + unit.title
        });
      } else {
        throw new Error('Service worker not controlling the page. Please refresh and try again.');
      }
    } catch (err) {
      console.error('Download failed:', err);
      this._activeDownloads[unitId] = false;
      this._fireStatus(unitId, 'error', { error: 'Download failed: ' + err.message });
    }
  };

  /**
   * Deletes a unit's cache and aborts any active download for it.
   */
  MHSDeliveryManager.prototype.deleteUnit = async function(unitId) {
    if (!this.manifest) return;

    var unit = this.manifest.units.find(function(u) { return u.id === unitId; });
    if (!unit) return;

    // Abort any active Background Fetch for this unit
    try {
      var reg = await this._waitForSW(5000);
      if (reg && reg.backgroundFetch) {
        var fetchId = this._fetchIdPrefix + unit.id + '-v' + unit.version;
        var existing = await reg.backgroundFetch.get(fetchId);
        if (existing) {
          await existing.abort();
        }
      }
    } catch (err) {
      // Ignore — just best-effort cleanup
    }

    var cacheName = this._unitCachePrefix + unit.id + '-v' + unit.version;
    await caches.delete(cacheName);
    this._fireStatus(unitId, 'not_cached', {});
  };

  /**
   * Returns the unit info from the manifest.
   */
  MHSDeliveryManager.prototype.getUnit = function(unitId) {
    if (!this.manifest) return null;
    return this.manifest.units.find(function(u) { return u.id === unitId; }) || null;
  };

  /**
   * Checks if a unit is cached.
   * @param {string} unitId
   * @returns {Promise<string>} 'cached', 'not_cached', or 'partial'
   */
  MHSDeliveryManager.prototype.isCached = async function(unitId) {
    if (!this.manifest || !this.manifest.units) return 'not_cached';
    var unit = this.manifest.units.find(function(u) { return u.id === unitId; });
    if (!unit) return 'not_cached';
    return await this._checkUnitCache(unit);
  };

  /**
   * Returns the next unit's ID from the manifest array, or null if last.
   * @param {string} unitId
   * @returns {string|null}
   */
  MHSDeliveryManager.prototype.getNextUnit = function(unitId) {
    if (!this.manifest || !this.manifest.units) return null;
    for (var i = 0; i < this.manifest.units.length; i++) {
      if (this.manifest.units[i].id === unitId) {
        if (i + 1 < this.manifest.units.length) {
          return this.manifest.units[i + 1].id;
        }
        return null;
      }
    }
    return null;
  };

  /**
   * Deletes cached data and aborts active downloads for ALL units.
   * Unlike autoCleanup, this does not check cache status first,
   * ensuring active Background Fetches are also aborted.
   */
  MHSDeliveryManager.prototype.deleteAllUnits = async function() {
    if (!this.manifest || !this.manifest.units) return;
    for (var i = 0; i < this.manifest.units.length; i++) {
      await this.deleteUnit(this.manifest.units[i].id);
    }
  };

  /**
   * Deletes cached data for all units NOT in the keepUnitIds array.
   * @param {string[]} keepUnitIds - Unit IDs to keep cached
   */
  MHSDeliveryManager.prototype.autoCleanup = async function(keepUnitIds) {
    if (!this.manifest || !this.manifest.units) return;
    var keepSet = {};
    for (var i = 0; i < keepUnitIds.length; i++) {
      keepSet[keepUnitIds[i]] = true;
    }
    for (var j = 0; j < this.manifest.units.length; j++) {
      var unit = this.manifest.units[j];
      if (keepSet[unit.id]) continue;
      var status = await this._checkUnitCache(unit);
      if (status === 'cached' || status === 'partial') {
        await this.deleteUnit(unit.id);
      }
    }
  };

  /**
   * Gets an estimate of storage usage.
   * @returns {Promise<{usage: number, quota: number}|null>}
   */
  MHSDeliveryManager.prototype.getStorageEstimate = async function() {
    if (navigator.storage && navigator.storage.estimate) {
      return await navigator.storage.estimate();
    }
    return null;
  };

  // Internal: handle status updates from BroadcastChannel
  MHSDeliveryManager.prototype._handleStatusUpdate = function(data) {
    if (data && data.type === 'status') {
      this._fireStatus(data.unitId, data.status, data.detail);
    }
  };

  // Internal: fire all status callbacks
  MHSDeliveryManager.prototype._fireStatus = function(unitId, status, detail) {
    // Clear active download tracking and stall monitors on terminal statuses
    if (status === 'cached' || status === 'error' || status === 'not_cached') {
      delete this._activeDownloads[unitId];
      if (this._downloadMonitors[unitId]) {
        clearInterval(this._downloadMonitors[unitId]);
        delete this._downloadMonitors[unitId];
      }
      delete this._stallState[unitId];
    }
    for (var i = 0; i < this.statusCallbacks.length; i++) {
      try {
        this.statusCallbacks[i](unitId, status, detail || {});
      } catch (err) {
        console.error('Status callback error:', err);
      }
    }
  };

  // Export globally
  window.MHSDeliveryManager = MHSDeliveryManager;
})();
