// mhs-delivery.js — Client-side download manager for MHS content delivery PWA.

(function() {
  'use strict';

  var MHS_CHANNEL_NAME = 'mhs-delivery';
  var UNIT_CACHE_PREFIX = 'mhs-unit-';

  /**
   * MHSDeliveryManager manages content downloads and cache status.
   */
  function MHSDeliveryManager() {
    this.manifest = null;
    this.swRegistration = null;
    this.channel = null;
    this.statusCallbacks = [];
  }

  /**
   * Initializes the delivery manager: registers SW, fetches manifest,
   * sets up BroadcastChannel listener, checks initial cache status.
   */
  MHSDeliveryManager.prototype.init = async function() {
    // Register service worker
    if ('serviceWorker' in navigator) {
      try {
        this.swRegistration = await navigator.serviceWorker.register('/sw.js', {
          scope: '/'
        });
        console.log('MHS Service Worker registered');
      } catch (err) {
        console.error('MHS SW registration failed:', err);
      }
    }

    // Set up BroadcastChannel for status updates from SW
    if (typeof BroadcastChannel !== 'undefined') {
      this.channel = new BroadcastChannel(MHS_CHANNEL_NAME);
      var self = this;
      this.channel.addEventListener('message', function(event) {
        self._handleStatusUpdate(event.data);
      });
    }

    // Fetch content manifest
    await this.refreshManifest();

    // Check initial cache status for all units
    await this.checkAllCacheStatus();
  };

  /**
   * Fetches the content manifest from the server.
   */
  MHSDeliveryManager.prototype.refreshManifest = async function() {
    try {
      var response = await fetch('/mhs/api/manifest');
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

    var cacheName = UNIT_CACHE_PREFIX + unit.id + '-v' + unit.version;
    try {
      var cache = await caches.open(cacheName);
      var found = 0;

      for (var i = 0; i < unit.files.length; i++) {
        var key = '/mhs/content/' + unit.files[i].path;
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
   * Starts downloading a unit's files via the service worker.
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

    this._fireStatus(unitId, 'downloading', { percent: 0, downloaded: 0, downloadTotal: unit.totalSize });

    // Initiate Background Fetch directly from the page context.
    // The SW handles backgroundfetchsuccess to re-key responses into cache.
    try {
      var reg = await navigator.serviceWorker.ready;
      var cdnBaseUrl = this.manifest.cdnBaseUrl;
      var requests = unit.files.map(function(file) {
        return new Request(cdnBaseUrl + '/' + file.path, { mode: 'cors' });
      });

      var fetchId = 'mhs-' + unit.id;
      var self = this;

      // Abort any existing Background Fetch with the same ID (e.g., from a previous download)
      var existing = await reg.backgroundFetch.get(fetchId);
      if (existing) {
        await existing.abort();
      }

      var bgFetch = await reg.backgroundFetch.fetch(fetchId, requests, {
        title: 'Downloading ' + unit.title,
        icons: [{ sizes: '192x192', src: '/assets/mhs/icon-192.png', type: 'image/png' }],
        downloadTotal: 0
      });

      console.log('Background Fetch started:', fetchId);

      bgFetch.addEventListener('progress', function() {
        if (unit.totalSize > 0) {
          var percent = Math.round((bgFetch.downloaded / unit.totalSize) * 100);
          self._fireStatus(unitId, 'downloading', {
            downloaded: bgFetch.downloaded,
            downloadTotal: unit.totalSize,
            percent: percent
          });
        }
      });
    } catch (err) {
      console.error('Background Fetch failed:', err);
      // Fall back to SW-based regular fetch
      if (navigator.serviceWorker.controller) {
        console.log('Falling back to regular fetch via SW');
        navigator.serviceWorker.controller.postMessage({
          action: 'download',
          unitId: unit.id,
          version: unit.version,
          files: unit.files,
          cdnBaseUrl: this.manifest.cdnBaseUrl,
          title: 'Downloading ' + unit.title
        });
      } else {
        this._fireStatus(unitId, 'error', { error: 'Download failed: ' + err.message });
      }
    }
  };

  /**
   * Deletes a unit's cache.
   */
  MHSDeliveryManager.prototype.deleteUnit = async function(unitId) {
    if (!this.manifest) return;

    var unit = this.manifest.units.find(function(u) { return u.id === unitId; });
    if (!unit) return;

    var cacheName = UNIT_CACHE_PREFIX + unit.id + '-v' + unit.version;
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
