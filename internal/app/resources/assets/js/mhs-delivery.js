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
  var DEFAULT_APP_SHELL_CACHE_PREFIX = 'mhs-app-shell-';

  // Live play-session heartbeat (written by the play page, read by
  // pruneStaleCaches). Entries older than the TTL are dead sessions. Hidden
  // tabs suspend timers (iOS fully suspends setInterval), so the TTL must
  // comfortably exceed how long a student may stay switched away mid-game:
  // a premature expiry lets a prune delete a live game's cache (breaks
  // offline play), while a stale entry merely delays pruning one cache.
  var ACTIVE_PLAY_KEY = 'mhs-active-play';
  var PLAY_HEARTBEAT_TTL_MS = 30 * 60 * 1000;
  var PLAY_HEARTBEAT_INTERVAL_MS = 60 * 1000;

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
    this._appShellCachePrefix = opts.appShellCachePrefix || DEFAULT_APP_SHELL_CACHE_PREFIX;
    this._activeDownloads = {}; // unitId -> true while a download is in progress
    this._downloadMonitors = {}; // unitId -> intervalId for stall detection
    this._stallState = {}; // unitId -> { lastProgressAt, maxDownloaded, stalled, fallback?, successSince?, successReconciled? }
    this._retryInFlight = {}; // unitId -> true while retryDownload's awaits are pending
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

    // Prune orphaned old-version caches. Must run after reconnecting so an
    // in-flight download of a valid unit is not disturbed; init-time pruning
    // also covers collection switches, which reload the page.
    await this.pruneStaleCaches();

    // Check initial cache status for all units
    await this.checkAllCacheStatus();

    // When tab becomes visible: reset stall monitors, recheck BG fetch states, then cache status
    var self = this;
    document.addEventListener('visibilitychange', function() {
      if (document.visibilityState === 'visible') {
        // Reset only the stall TIMER — progress isn't tracked while hidden,
        // so a threshold check right after returning would false-detect a
        // stall. The stalled FLAG must survive tab switches: it is cleared
        // only by genuinely new bytes (_reportDownloadProgress), otherwise a
        // dead download's Retry UI would be dismissed on every tab switch.
        var ids = Object.keys(self._stallState);
        for (var i = 0; i < ids.length; i++) {
          self._stallState[ids[i]].lastProgressAt = Date.now();
        }
        self._recheckActiveDownloads(); // Clear completed BG fetches from tracking
        self.checkAllCacheStatus();      // Detect cached/not_cached for cleared units
      }
    });
  };

  /**
   * Returns a map of unit cache names that have an in-flight Background Fetch,
   * for any version — one user's cleanup must not delete a cache another
   * session is actively downloading into.
   */
  MHSDeliveryManager.prototype._activeBGFetchCacheNames = async function() {
    var active = {};
    if (!navigator.serviceWorker) return active;
    try {
      var reg = await this._waitForSW(5000);
      if (!reg || !reg.backgroundFetch) return active;
      var ids = await reg.backgroundFetch.getIds();
      for (var i = 0; i < ids.length; i++) {
        if (!ids[i].startsWith(this._fetchIdPrefix)) continue;
        var bgFetch = await reg.backgroundFetch.get(ids[i]);
        if (bgFetch && bgFetch.result === '') {
          // Fetch ID "<fetchPrefix>unit1-v1.0.0" caches into "<cachePrefix>unit1-v1.0.0"
          active[this._unitCachePrefix + ids[i].substring(this._fetchIdPrefix.length)] = true;
        }
      }
    } catch (err) {
      // Background Fetch API may not be available — that's fine.
    }
    return active;
  };

  /**
   * Aborts every MHS Background Fetch, for any version — not just fetches for
   * current-manifest units. Used before purging caches so a late
   * backgroundfetchsuccess event cannot repopulate a deleted cache.
   */
  MHSDeliveryManager.prototype._abortAllBGFetches = async function() {
    if (!navigator.serviceWorker) return;
    try {
      var reg = await this._waitForSW(5000);
      if (!reg || !reg.backgroundFetch) return;
      var ids = await reg.backgroundFetch.getIds();
      for (var i = 0; i < ids.length; i++) {
        if (!ids[i].startsWith(this._fetchIdPrefix)) continue;
        try {
          var bgFetch = await reg.backgroundFetch.get(ids[i]);
          if (bgFetch) await bgFetch.abort();
        } catch (err) {
          // Best effort per fetch
        }
      }
    } catch (err) {
      // Background Fetch API may not be available — that's fine.
    }
  };

  /**
   * Tells the service worker to cancel in-flight sequential fallback
   * downloads (the non-Background-Fetch path). Without this, a cache purge
   * leaves the SW loop running: it would keep downloading and eventually
   * broadcast a false 'cached' for a unit whose cache was just deleted.
   * Omit unitId/version to cancel all fallback downloads. Pass purge=false
   * for a retry-style cancel that keeps the partial cache for resume; the
   * default purges it (correct before a cache delete/reset).
   */
  MHSDeliveryManager.prototype._cancelFallbackDownloads = function(unitId, version, purge) {
    if (navigator.serviceWorker && navigator.serviceWorker.controller) {
      navigator.serviceWorker.controller.postMessage({
        action: 'cancelFallbacks',
        unitId: unitId || '',
        version: version || '',
        purge: purge !== false
      });
    }
  };

  /**
   * Asks the service worker which sequential fallback downloads are in
   * flight. Resolves to an array of "unitId-vVersion" keys; resolves to []
   * on timeout (e.g. an older worker that doesn't know the message).
   */
  MHSDeliveryManager.prototype._getActiveFallbacks = function() {
    return new Promise(function(resolve) {
      if (!navigator.serviceWorker || !navigator.serviceWorker.controller) {
        resolve([]);
        return;
      }
      var settled = false;
      function finish(keys) {
        if (settled) return;
        settled = true;
        resolve(keys);
      }
      var timer = setTimeout(function() { finish([]); }, 3000);
      try {
        var channel = new MessageChannel();
        channel.port1.onmessage = function(event) {
          clearTimeout(timer);
          finish((event.data && event.data.activeFallbacks) || []);
        };
        navigator.serviceWorker.controller.postMessage(
          { action: 'getActiveFallbacks' }, [channel.port2]);
      } catch (err) {
        clearTimeout(timer);
        finish([]);
      }
    });
  };

  /**
   * Deletes unit caches whose unit+version is not in the current manifest.
   * Old-version caches become orphans when a build ships a bumped version;
   * without this they accumulate until the origin quota is exhausted.
   * Caches with an in-flight Background Fetch are skipped.
   */
  MHSDeliveryManager.prototype.pruneStaleCaches = async function() {
    if (!('caches' in window)) return;
    if (!this.manifest || !this.manifest.units) return;

    var valid = {};
    for (var i = 0; i < this.manifest.units.length; i++) {
      var unit = this.manifest.units[i];
      valid[this._unitCachePrefix + unit.id + '-v' + unit.version] = true;
    }

    var active = await this._activeBGFetchCacheNames();

    // Skip caches belonging to a live play session (heartbeat written by the
    // play page — see startPlayHeartbeat): a content deploy can bump a
    // unit's version while a game is mid-session on the old one, and the
    // running game still reads that cache.
    var livePlay = {};
    try {
      var playMap = JSON.parse(localStorage.getItem(ACTIVE_PLAY_KEY) || '{}');
      var nowTs = Date.now();
      for (var key in playMap) {
        if (nowTs - playMap[key] < PLAY_HEARTBEAT_TTL_MS) {
          livePlay[this._unitCachePrefix + key] = true;
        }
      }
    } catch (err) {
      // No readable heartbeat — proceed with the normal prune
    }

    try {
      var names = await caches.keys();
      for (var j = 0; j < names.length; j++) {
        var name = names[j];
        if (!name.startsWith(this._unitCachePrefix)) continue;
        if (valid[name] || active[name] || livePlay[name]) continue;
        // Stop any in-flight sequential fallback loop writing into this
        // doomed cache — otherwise it keeps downloading and finally
        // broadcasts a false 'cached' for a cache that no longer exists.
        // The cache name parses back into unit+version.
        var rest = name.substring(this._unitCachePrefix.length);
        var vIdx = rest.lastIndexOf('-v');
        if (vIdx > 0) {
          this._cancelFallbackDownloads(rest.substring(0, vIdx), rest.substring(vIdx + 2));
        }
        await caches.delete(name);
        console.log('Pruned stale unit cache:', name);
      }
    } catch (err) {
      console.warn('Stale cache prune failed:', err);
    }
  };

  // localStorage key of the live play-session heartbeat map, shared with the
  // pages that purge local MHS data.
  MHSDeliveryManager.ACTIVE_PLAY_KEY = ACTIVE_PLAY_KEY;

  /**
   * Registers a play session so pruneStaleCaches won't delete the cache a
   * running game is reading. Beats on an interval AND whenever the tab
   * becomes visible again — hidden tabs suspend timers (iOS fully suspends
   * setInterval), so a game the student switched away from must re-protect
   * itself the moment they return. Dead entries (crashed tabs never remove
   * theirs) are pruned on every write. Returns a stop function for pagehide.
   */
  MHSDeliveryManager.startPlayHeartbeat = function(unitId, version) {
    var mapKey = unitId + '-v' + version;

    function writeMap(mutate) {
      try {
        var map = JSON.parse(localStorage.getItem(ACTIVE_PLAY_KEY) || '{}');
        var now = Date.now();
        for (var k in map) {
          if (!(now - map[k] < PLAY_HEARTBEAT_TTL_MS)) delete map[k];
        }
        mutate(map, now);
        localStorage.setItem(ACTIVE_PLAY_KEY, JSON.stringify(map));
      } catch (e) { /* best effort */ }
    }

    function beat() {
      writeMap(function(map, now) { map[mapKey] = now; });
    }

    function onVisibilityChange() {
      if (document.visibilityState === 'visible') beat();
    }

    beat();
    var timer = setInterval(beat, PLAY_HEARTBEAT_INTERVAL_MS);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('pageshow', beat);

    return function stop() {
      clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('pageshow', beat);
      writeMap(function(map) { delete map[mapKey]; });
    };
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

      if (found === unit.files.length) {
        // Spot-check the largest file's cached size so eviction or partial
        // artifacts can't produce a false "Ready to play".
        var ok = await this._verifyLargestFile(cache, unit.files);
        return ok ? 'cached' : 'partial';
      }
      if (found === 0) return 'not_cached';
      return 'partial';
    } catch (err) {
      return 'not_cached';
    }
  };

  /**
   * Verifies the largest file's cached size against its manifest size.
   * Returns true when the size matches or cannot be determined.
   */
  MHSDeliveryManager.prototype._verifyLargestFile = async function(cache, files) {
    var largest = null;
    for (var i = 0; i < files.length; i++) {
      if (!largest || (files[i].size || 0) > (largest.size || 0)) largest = files[i];
    }
    if (!largest || !largest.size) return true;

    var match = await cache.match(this._contentPrefix + largest.path);
    if (!match) return false;
    var len = match.headers.get('content-length');
    if (len === null) return true; // size unknown (e.g. chunked) — accept
    if (parseInt(len, 10) === largest.size) return true;

    // The header can disagree with the stored body (e.g. transfer
    // compression keeps a compressed content-length on a decoded body).
    // Count the actual stored bytes before declaring the unit incomplete —
    // this rare path must not condemn a good unit to endless re-downloads.
    try {
      var blob = await match.clone().blob();
      if (blob.size !== largest.size) return false;
      // The header/body mismatch is persistent for the life of the entry,
      // so without a repair every future check re-reads this entire file —
      // a multi-hundred-MB read per check on a low-end device. We already
      // hold the bytes: rewrite the entry with a corrected content-length
      // so future checks take the header fast-path.
      try {
        var headers = new Headers(match.headers);
        headers.set('content-length', String(blob.size));
        await cache.put(this._contentPrefix + largest.path, new Response(blob, {
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
   * Parses a Background Fetch ID into { unitId, version }.
   * e.g., "missionhydrosci-unit1-v1.0.0" -> { unitId: "unit1", version: "1.0.0" }
   * Legacy IDs without a version yield version: null.
   */
  MHSDeliveryManager.prototype._parseFetchId = function(fetchId) {
    var prefix = this._fetchIdPrefix;
    if (!fetchId.startsWith(prefix)) return null;
    var rest = fetchId.substring(prefix.length); // "unit1-v1.0.0"
    var versionMatch = rest.match(/^(.+)-v(\d+\.\d+\.\d+)$/);
    if (versionMatch) return { unitId: versionMatch[1], version: versionMatch[2] };
    return { unitId: rest, version: null }; // Legacy format without version
  };

  var PROGRESS_POLL_MS = 5000;      // how often to re-obtain a fresh registration
  var STALL_THRESHOLD_MS = 150000;  // no byte progress for ~2.5 min (tab visible) => stalled
  var SUCCESS_BROADCAST_GRACE_MS = 45000; // BG fetch success but no 'cached' broadcast => reconcile

  /**
   * Reports download progress for a unit. This is the single owner of
   * stall-state mutation for Background Fetch downloads: strictly new bytes
   * (downloaded > maxDownloaded) are the ONLY progress criterion. A repeated
   * or regressed byte counter (stale progress event, browser-internal
   * request retry) is not progress — it must not refresh the stall timer and
   * above all must not dismiss a 'stalled' status and its Retry UI.
   */
  MHSDeliveryManager.prototype._reportDownloadProgress = function(unitId, unit, downloaded) {
    var state = this._stallState[unitId];
    if (state) {
      if (downloaded > state.maxDownloaded) {
        state.maxDownloaded = downloaded;
        state.lastProgressAt = Date.now();
        state.stalled = false;
      } else if (state.stalled) {
        return; // no new bytes — never overwrite the Retry UI
      }
      downloaded = state.maxDownloaded; // keep the reported value monotonic
    }
    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    var percent = totalSize > 0 ? Math.round((downloaded / totalSize) * 100) : 0;
    this._fireStatus(unitId, 'downloading', {
      downloaded: downloaded,
      downloadTotal: totalSize,
      percent: percent
    });
  };

  /**
   * Starts the shared stall-monitor scaffold for a unit: clears any existing
   * monitor, initializes the stall state (on the manager, not a closure, so
   * the visibility handler can reset the timer), and runs tickFn on an
   * interval until the download is no longer tracked as active.
   */
  MHSDeliveryManager.prototype._startStallMonitor = function(unitId, extraState, tickFn) {
    if (this._downloadMonitors[unitId]) {
      clearInterval(this._downloadMonitors[unitId]);
      delete this._downloadMonitors[unitId];
    }

    var state = {
      lastProgressAt: Date.now(),
      maxDownloaded: 0,
      stalled: false
    };
    if (extraState) {
      for (var k in extraState) state[k] = extraState[k];
    }
    this._stallState[unitId] = state;

    var self = this;
    this._downloadMonitors[unitId] = setInterval(function() {
      // Download completed/failed — stop monitoring
      if (!self._activeDownloads[unitId]) {
        clearInterval(self._downloadMonitors[unitId]);
        delete self._downloadMonitors[unitId];
        delete self._stallState[unitId];
        return;
      }
      tickFn();
    }, PROGRESS_POLL_MS);
  };

  /**
   * Fires 'stalled' once when the stall timer crosses the threshold. A
   * genuine stall surfaces as a status for the user to act on; the download
   * is never aborted here — aborting a healthy-but-slow download and
   * restarting it on the sequential fallback path converts slow-network
   * successes into failures.
   */
  MHSDeliveryManager.prototype._maybeFireStalled = function(unitId, unit) {
    var state = this._stallState[unitId];
    if (!state || state.stalled) return;
    if (Date.now() - state.lastProgressAt < STALL_THRESHOLD_MS) return;
    state.stalled = true;
    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    this._fireStatus(unitId, 'stalled', {
      downloaded: state.maxDownloaded,
      downloadTotal: totalSize,
      percent: totalSize > 0 ? Math.round((state.maxDownloaded / totalSize) * 100) : 0
    });
  };

  /**
   * Monitors a Background Fetch by polling backgroundFetch.get() for a fresh
   * registration object. Page-held registration objects can stop updating
   * `downloaded` (and stop firing progress events) while the download is
   * healthy — a fresh get() reflects current state, so both the UI percent
   * and stall detection are driven from it.
   *
   * A genuine stall surfaces as a 'stalled' status for the user to act on;
   * the download is never aborted here — aborting a healthy download and
   * restarting it on the sequential fallback path converts slow-network
   * successes into failures.
   */
  MHSDeliveryManager.prototype._monitorDownload = function(unitId, unit) {
    var self = this;
    var fetchId = this._fetchIdPrefix + unit.id + '-v' + unit.version;
    this._startStallMonitor(unitId, null, function() {
      self._pollDownloadOnce(unitId, fetchId, unit);
    });
  };

  // Internal: one poll tick — fetch a fresh registration, report progress
  // when bytes actually moved, and check for a stall on the fresh values.
  MHSDeliveryManager.prototype._pollDownloadOnce = async function(unitId, fetchId, unit) {
    var state = this._stallState[unitId];
    if (!state) return;

    var fresh;
    try {
      var reg = await this._waitForSW(4000);
      if (!reg || !reg.backgroundFetch) return;
      fresh = await reg.backgroundFetch.get(fetchId);
    } catch (err) {
      return; // transient — try again next tick
    }

    if (!fresh) {
      // Fetch disappeared without a success/failure event — e.g. the user
      // canceled it from the browser's download UI (no backgroundfetch*
      // event fires for that). Clear tracking and report the true cache
      // state so the unit doesn't sit at "Downloading" forever.
      delete this._activeDownloads[unitId];
      var status = await this._checkUnitCache(unit);
      this._fireStatus(unitId, status, {});
      return;
    }

    // Succeeded: completion normally travels on the SW broadcast channel
    // ('cached'), which also clears this monitor. But if the SW died during
    // the success handler's cache-copy (or threw before broadcasting), that
    // broadcast never arrives and the unit would sit at "Downloading"
    // forever — reconcile from the cache after a grace period.
    if (fresh.result === 'success') {
      var now = Date.now();
      if (!state.successSince) {
        state.successSince = now;
        return;
      }
      if (now - state.successSince < SUCCESS_BROADCAST_GRACE_MS) return;
      if (state.successChecking) return; // a reconcile from a previous tick is still running
      state.successChecking = true;
      var cacheState = await this._checkUnitCache(unit);
      state.successChecking = false;
      if (cacheState === 'cached') {
        this._fireStatus(unitId, 'cached', {});
      } else if (state.successReconciled) {
        // Two grace periods without converging — surface Retry.
        this._fireStatus(unitId, 'error', {
          error: 'Download finished but could not be saved. Please retry.'
        });
      } else {
        // The SW may legitimately still be copying records (a large unit
        // takes a while on slow storage) — allow one more grace period.
        state.successReconciled = true;
        state.successSince = now;
      }
      return;
    }

    if (fresh.result === 'failure') {
      // Normally the SW backgroundfetchfail broadcast reports this; fire as a
      // backstop in case this page missed the broadcast.
      if (fresh.failureReason === 'download-total-exceeded') {
        // Manifest sizes were stale (e.g. a same-version re-upload grew a
        // file) — refresh so a retry downloads with current sizes.
        this.refreshManifest();
      }
      this._fireStatus(unitId, 'error', {
        error: 'Download failed. Please check your connection and try again.',
        failureReason: fresh.failureReason || ''
      });
      return;
    }

    var downloaded = fresh.downloaded || 0;

    // Progress isn't tracked while hidden — just keep the stall timer current.
    if (document.visibilityState === 'hidden') {
      state.lastProgressAt = Date.now();
      return;
    }

    // _reportDownloadProgress owns all stall-state mutation: only strictly
    // new bytes count as progress, and a 'stalled' status is never
    // overwritten without them. The poller keeps only the threshold check.
    this._reportDownloadProgress(unitId, unit, downloaded);
    this._maybeFireStalled(unitId, unit);
  };

  /**
   * Watchdog for a SW sequential fallback download (no Background Fetch
   * registration to poll). Its liveness signal is the SW's progress
   * broadcasts — at least one per second while bytes move (fed into the
   * stall state by _fireStatus). If they go quiet for STALL_THRESHOLD_MS
   * while the tab is visible (SW killed mid-download, network black hole),
   * surface 'stalled' so the user gets the same Retry affordance as the
   * Background Fetch path. A retry re-posts fallbackDownload and resumes
   * from the last completed file, since partial caches are kept on failure.
   */
  MHSDeliveryManager.prototype._monitorFallbackDownload = function(unitId, unit) {
    var self = this;
    this._startStallMonitor(unitId, { fallback: true }, function() {
      var state = self._stallState[unitId];
      if (!state) return;
      if (document.visibilityState === 'hidden') {
        state.lastProgressAt = Date.now();
        return;
      }
      self._maybeFireStalled(unitId, unit);
    });
  };

  /**
   * Attaches progress listener and starts the poll-based monitor for a BG
   * fetch. Progress events are kept as a fast path when they do fire; the
   * poller drives the UI when they don't. Used by both downloadUnit and
   * _reconnectActiveDownloads.
   */
  MHSDeliveryManager.prototype._attachBGFetchListeners = function(unitId, bgFetch, unit) {
    var self = this;

    this._monitorDownload(unitId, unit);

    bgFetch.addEventListener('progress', function() {
      self._reportDownloadProgress(unitId, unit, bgFetch.downloaded || 0);
    });
  };

  /**
   * Aborts any existing Background Fetch for a unit and starts the download
   * over. Used as the user-initiated escape hatch when a download is stalled.
   */
  MHSDeliveryManager.prototype.retryDownload = async function(unitId) {
    if (!this.manifest) return;
    var unit = this.manifest.units.find(function(u) { return u.id === unitId; });
    if (!unit) return;

    // Re-entrancy guard: cacheStatus stays 'stalled' until the awaits below
    // complete, so a double-click would route here twice and interleave —
    // either aborting the fresh download or racing two downloadUnit runs
    // (a duplicate Background Fetch alongside a fallback loop).
    if (this._retryInFlight[unitId]) return;
    this._retryInFlight[unitId] = true;

    try {
      try {
        var reg = await this._waitForSW(5000);
        if (reg && reg.backgroundFetch) {
          var fetchId = this._fetchIdPrefix + unit.id + '-v' + unit.version;
          var existing = await reg.backgroundFetch.get(fetchId);
          if (existing) await existing.abort();
        }
      } catch (err) {
        // Best effort
      }

      // If the stalled download was a SW fallback loop that is alive but
      // wedged (hung fetch), a re-posted fallbackDownload would be swallowed
      // by the SW's dedupe. Cancel it first — purge=false keeps the partial
      // cache so the new loop resumes from the last completed file.
      this._cancelFallbackDownloads(unit.id, unit.version, false);

      delete this._activeDownloads[unitId];
      if (this._downloadMonitors[unitId]) {
        clearInterval(this._downloadMonitors[unitId]);
        delete this._downloadMonitors[unitId];
      }
      delete this._stallState[unitId];

      return await this.downloadUnit(unitId);
    } finally {
      delete this._retryInFlight[unitId];
    }
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

        var parsed = this._parseFetchId(ids[i]);
        if (!parsed) continue;
        var unitId = parsed.unitId;

        if (bgFetch.result === '') {
          // In progress — find matching manifest unit. The VERSION must
          // match too: an in-progress fetch for an old version (left behind
          // by a collection switch on another device, or a paused fetch
          // Chrome persisted from an earlier session) must not be adopted —
          // the monitor would poll the current-version fetch ID, find
          // nothing, and clear tracking with a status the auto-download
          // pipeline never recovers from, leaving the unit stuck at
          // "Downloading" with nothing running.
          var unit = this.manifest && this.manifest.units
            ? this.manifest.units.find(function(u) { return u.id === unitId; })
            : null;
          if (unit && parsed.version !== unit.version) unit = null;

          if (unit) {
            // Reconnect: set active tracking, attach listeners (which seeds
            // the stall state), then report progress through the shared
            // monotonic/sticky guard like every other progress source.
            this._activeDownloads[unitId] = true;
            this._attachBGFetchListeners(unitId, bgFetch, unit);
            this._reportDownloadProgress(unitId, unit, bgFetch.downloaded || 0);
            console.log('Reconnected to Background Fetch:', ids[i]);
          } else {
            // No manifest unit at this unit+version — genuinely stale.
            // Abort it: it can only produce an unwanted old-version cache,
            // and while in flight it blocks pruning of that cache.
            console.log('Aborting stale Background Fetch (no manifest unit+version match):', ids[i]);
            await bgFetch.abort();
          }
        }
        // Completed/failed BG fetches are skipped — checkAllCacheStatus handles them
      }
    } catch (err) {
      // Background Fetch API may not be available — that's fine.
    }

    // Adopt in-flight SW sequential fallback loops — they survive page
    // reloads (the SW outlives the page) but have no Background Fetch
    // registration to enumerate. Without adoption, a reload mid-download
    // loses the watchdog (a dead SW loop then strands the UI at
    // "Downloading"), lets checkAllCacheStatus flash 'partial' over the live
    // progress, and can even start a duplicate Background Fetch for the
    // same unit via the auto-download pipeline.
    try {
      var fbKeys = await this._getActiveFallbacks();
      if (fbKeys.length > 0 && this.manifest && this.manifest.units) {
        var fbKeySet = {};
        for (var f = 0; f < fbKeys.length; f++) fbKeySet[fbKeys[f]] = true;
        for (var u = 0; u < this.manifest.units.length; u++) {
          var mUnit = this.manifest.units[u];
          if (!fbKeySet[mUnit.id + '-v' + mUnit.version]) continue;
          if (this._activeDownloads[mUnit.id]) continue;
          // Loops for versions no longer in the manifest are deliberately
          // not adopted — pruneStaleCaches cancels them.
          this._activeDownloads[mUnit.id] = true;
          this._monitorFallbackDownload(mUnit.id, mUnit);
          console.log('Adopted in-flight fallback download:', mUnit.id + '-v' + mUnit.version);
        }
      }
    } catch (err) {
      // Best effort — the SW may not support the query yet.
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
      var hasBGFetch = !!(reg && reg.backgroundFetch);

      var checks = unitIds.map(function(unitId) {
        var unit = self.manifest && self.manifest.units
          ? self.manifest.units.find(function(u) { return u.id === unitId; })
          : null;
        if (!unit) return Promise.resolve();

        var st = self._stallState[unitId];
        var isBGDownload = hasBGFetch && st && !st.fallback;
        if (!isBGDownload) {
          // Fallback download (or no inspectable state) — there is no
          // registration to poll. Reconcile from the cache: if it completed
          // while hidden (broadcast missed), fire 'cached'; otherwise leave
          // tracking in place — the SW loop may still be running, and the
          // fallback watchdog surfaces a dead one.
          return self._checkUnitCache(unit).then(function(status) {
            if (status === 'cached') {
              self._fireStatus(unitId, 'cached', {});
            }
          }).catch(function() {
            // Ignore errors for individual checks
          });
        }

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
            self._reportDownloadProgress(unitId, unit, bgFetch.downloaded || 0);
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
          // Active fetch already in progress — reconnect instead of
          // aborting. Attach first (seeds the stall state), then report
          // through the shared monotonic/sticky guard.
          this._attachBGFetchListeners(unitId, existing, unit);
          this._reportDownloadProgress(unitId, unit, existing.downloaded || 0);
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
            // Real total makes the browser's native download indicator
            // determinate instead of an endless spinner.
            downloadTotal: totalSize
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
        // Watchdog: there is no registration to poll on this path, so watch
        // the broadcast stream for silence instead
        this._monitorFallbackDownload(unitId, unit);
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

    // Also stop any in-flight SW sequential fallback download for this unit
    this._cancelFallbackDownloads(unit.id, unit.version);

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
   * Deletes cached data and aborts active downloads for ALL units, across
   * ALL versions — not just units in the current manifest at their current
   * version. Orphaned old-version caches are cleared too.
   */
  MHSDeliveryManager.prototype.deleteAllUnits = async function() {
    // Abort first so a late backgroundfetchsuccess can't repopulate a cache,
    // and stop SW fallback loops so they can't broadcast a false 'cached'
    await this._abortAllBGFetches();
    this._cancelFallbackDownloads();

    if ('caches' in window) {
      try {
        var names = await caches.keys();
        for (var i = 0; i < names.length; i++) {
          if (names[i].startsWith(this._unitCachePrefix)) {
            await caches.delete(names[i]);
          }
        }
      } catch (err) {
        console.warn('Failed to delete unit caches:', err);
      }
    }

    if (this.manifest && this.manifest.units) {
      for (var j = 0; j < this.manifest.units.length; j++) {
        this._fireStatus(this.manifest.units[j].id, 'not_cached', {});
      }
    }
  };

  /**
   * Fully clears this device's local MHS footprint: aborts all MHS Background
   * Fetches, deletes every MHS cache (all unit versions and the app shell),
   * and removes the given localStorage keys. Server-side state — progress,
   * collection override, and game saves/settings (stratasave) — is untouched.
   * @param {string[]} [localStorageKeys] - localStorage keys to remove.
   */
  MHSDeliveryManager.prototype.purgeAllMHSData = async function(localStorageKeys) {
    // Abort first so a late backgroundfetchsuccess can't repopulate a cache,
    // and stop SW fallback loops so they can't broadcast a false 'cached'
    await this._abortAllBGFetches();
    this._cancelFallbackDownloads();

    if ('caches' in window) {
      try {
        var names = await caches.keys();
        for (var i = 0; i < names.length; i++) {
          // Select by the explicit cache families we own — unit caches and
          // the app shell. (Selecting by the fetch-ID prefix happened to
          // work with the shipped config but conflated two namespaces.)
          if (names[i].startsWith(this._unitCachePrefix) ||
              names[i].startsWith(this._appShellCachePrefix)) {
            await caches.delete(names[i]);
          }
        }
      } catch (err) {
        console.warn('Failed to delete MHS caches:', err);
      }
    }

    var keys = localStorageKeys || [];
    for (var k = 0; k < keys.length; k++) {
      try { localStorage.removeItem(keys[k]); } catch (err) { /* best effort */ }
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
      // Version-tagged broadcasts for a DIFFERENT version of a manifest unit
      // (e.g. a canceled old-version loop winding down after a deploy) must
      // not touch this page's state — a unitId-only terminal status would
      // clear tracking for the current version's live download. Untagged
      // broadcasts (older workers) pass through, the previous behavior.
      if (data.detail && data.detail.version && this.manifest && this.manifest.units) {
        var bUnit = this.manifest.units.find(function(u) { return u.id === data.unitId; });
        if (bUnit && bUnit.version !== data.detail.version) return;
      }
      if (data.status === 'error' && data.detail &&
          data.detail.failureReason === 'download-total-exceeded') {
        // Manifest sizes were stale (e.g. a same-version re-upload grew a
        // file) — refresh so a retry downloads with current sizes.
        this.refreshManifest();
      }
      this._fireStatus(data.unitId, data.status, data.detail);
    }
  };

  // Internal: fire all status callbacks
  MHSDeliveryManager.prototype._fireStatus = function(unitId, status, detail) {
    // Fallback downloads have no registration to poll — their liveness
    // signal is the SW's progress broadcasts. Feed the stall watchdog: any
    // 'downloading' broadcast means the loop is alive.
    if (status === 'downloading') {
      var fbState = this._stallState[unitId];
      if (fbState && fbState.fallback) {
        fbState.lastProgressAt = Date.now();
        if (detail && typeof detail.downloaded === 'number' && detail.downloaded > fbState.maxDownloaded) {
          fbState.maxDownloaded = detail.downloaded;
        }
        fbState.stalled = false;
      }
    }

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
