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
    this.manifestLoaded = false; // true only after a fresh, successful manifest fetch
    this._csrfToken = opts.csrfToken || ''; // required to POST download-error telemetry
    this._downloadErrorUrl = opts.downloadErrorUrl || '/missionhydrosci/api/download-error';
    this._reportedErrors = {}; // dedupe telemetry: unitId|errorClass -> true (per manager lifetime)
    this.swRegistration = null;
    this.channel = null;
    this.statusCallbacks = [];
    this._appShellCachePrefix = opts.appShellCachePrefix || DEFAULT_APP_SHELL_CACHE_PREFIX;
    this._activeDownloads = {}; // unitId -> true while a download is in progress
    this._downloadMonitors = {}; // unitId -> intervalId for stall detection
    this._stallState = {}; // unitId -> { lastProgressAt, maxDownloaded, stalled, fallback?, successSince?, successReconciled? }
    this._retryInFlight = {}; // unitId -> true while retryDownload's awaits are pending
    this._progressKeepalive = null; // interval handle for the SW attachProgress keepalive
    this._wanted = {}; // unitId -> true while a requested download is outstanding (until 'cached' or removed)
    this._retryAttempts = {}; // unitId -> consecutive automatic retries without new bytes (drives the backoff)
    this._pendingRetries = {}; // unitId -> { kind, dueAt, attempt, base, timer, tick } while a retry is scheduled
    this._steplog = null; // MHSStepLog (see setStepLog); null = no per-load logging
    this._logSamples = {}; // unitId -> { at, bytes, bucket } throttling progress samples in the step log
    this.preflightResult = null; // last content-server/services reachability probe (see preflight)
    this.storagePersisted = null; // result of navigator.storage.persist() from init(); null if unavailable
  }

  // How often pages nudge the SW to keep progress broadcasts alive. This also
  // bounds how long a HEALTHY download can look silent after an SW restart, so
  // the frozen-switch window (FROZEN_SWITCH_MS) must stay comfortably above it.
  var PROGRESS_KEEPALIVE_MS = 10000;

  /**
   * While any download is active, periodically asks the SW to (re)attach its
   * progress broadcasters ('attachProgress'). The message doubles as an SW
   * keepalive: it wakes or extends the worker so its byte-progress listeners
   * keep firing, and after an SW restart it restores the listeners the
   * restart dropped. Stops itself when no downloads remain active.
   */
  MHSDeliveryManager.prototype._startProgressKeepalive = function(reg) {
    if (this._progressKeepalive) return;
    var self = this;
    var swAtStart = reg && reg.active;
    this._progressKeepalive = setInterval(function() {
      if (Object.keys(self._activeDownloads).length === 0) {
        clearInterval(self._progressKeepalive);
        self._progressKeepalive = null;
        return;
      }
      try {
        var target = (self.swRegistration && self.swRegistration.active) ||
          swAtStart ||
          (navigator.serviceWorker && navigator.serviceWorker.controller);
        if (target) target.postMessage({ action: 'attachProgress' });
      } catch (err) {
        // Best effort
      }
    }, this._tune('keepaliveMs', PROGRESS_KEEPALIVE_MS));
  };

  /**
   * Initializes the delivery manager: registers SW, fetches manifest,
   * sets up BroadcastChannel listener, and checks initial cache status.
   */
  MHSDeliveryManager.prototype.init = async function() {
    var self = this;
    // Register service worker (skip when swUrl is explicitly null/empty)
    if (this._swUrl && 'serviceWorker' in navigator) {
      this._step('sw', 'running', 'Registering the service worker…');
      try {
        this.swRegistration = await navigator.serviceWorker.register(this._swUrl, {
          scope: this._swScope
        });
        console.log('MHS Service Worker registered');
        this._step('sw', 'ok', 'Service worker registered' +
          (navigator.serviceWorker.controller ? ' and controlling this page' : ' (not yet controlling this page)'));
        // Its version, for the report (older workers don't answer: "unknown").
        this._getSWVersion().then(function(v) {
          if (!v) return;
          self._logContext('swVersion', v);
          self._step('sw', 'ok', 'Service worker active (v' + v + ')');
        });
      } catch (err) {
        console.error('MHS SW registration failed:', err);
        this._step('sw', 'fail', 'Service worker registration failed: ' + ((err && err.message) || err));
      }
    } else if (!('serviceWorker' in navigator)) {
      this._step('sw', 'fail', 'Service workers are not supported in this browser — downloads cannot be saved for offline play');
    }

    // Ask the browser to treat this origin's storage as durable (MHS-005).
    // Best effort: Chrome grants it silently for installed/engaged sites and
    // otherwise returns false; nothing prompts. The result is reported with
    // device status so evictable-storage devices are visible server-side.
    if (navigator.storage && navigator.storage.persist) {
      try {
        this.storagePersisted = await navigator.storage.persist();
        this._step('storage', this.storagePersisted ? 'ok' : 'info', this.storagePersisted
          ? 'Storage is persistent (protected from eviction)'
          : 'Storage is best-effort (the browser may evict downloads under pressure)');
      } catch (err) {
        this.storagePersisted = null;
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
    this._logStorageSummary().catch(function() {});

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
        self._kickPendingRetries();      // Run any retry whose countdown expired while hidden
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
    // Prune only from a manifest we actually loaded this session. A failed
    // fetch leaves an empty units array (or a stale one flagged not-loaded);
    // pruning from either would wipe valid downloads. An empty manifest with
    // no active collection is also not a mandate to delete everything.
    if (!this.manifestLoaded) return;
    if (!this.manifest || !this.manifest.units || this.manifest.units.length === 0) return;

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
        this._step('storage', 'info', 'Removed an old download: ' + name.substring(this._unitCachePrefix.length));
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
    this._step('manifest', 'running', 'Loading the game list…');
    try {
      var response = await fetch(this._manifestUrl);
      if (!response.ok) throw new Error('manifest HTTP ' + response.status);
      var parsed = await response.json();
      if (!parsed || !Array.isArray(parsed.units)) {
        throw new Error('manifest missing units array');
      }
      this.manifest = parsed;
      this.manifestLoaded = true;
      var names = [];
      for (var mi = 0; mi < parsed.units.length; mi++) {
        var mu = parsed.units[mi];
        names.push(mu.id + ' v' + mu.version + ' (' + fmtMB(mu.totalSize) + ' MB)');
      }
      this._step('manifest', parsed.units.length ? 'ok' : 'warn', parsed.units.length
        ? 'Game list loaded: ' + names.join(', ')
        : 'Game list loaded but it has no units (no active game version for this account)');
    } catch (err) {
      console.error('Failed to fetch MHS content manifest:', err);
      this._step('manifest', 'fail', 'Could not load the game list: ' + ((err && err.message) || err) +
        (this.manifest && this.manifest.units && this.manifest.units.length ? ' (keeping the previous list)' : ''));
      // Do NOT let a transient failure look like "the game has no units."
      // Reading an empty manifest as authoritative would prune every
      // downloaded unit cache (offline data loss — the feature's core
      // promise). Keep any manifest we already have; only fall back to an
      // empty one when we have nothing to serve. Either way, mark the
      // manifest NOT loaded so destructive operations (pruneStaleCaches,
      // the reconnect abort branch) stand down until a real manifest loads.
      if (!this.manifest) {
        this.manifest = { cdnBaseUrl: '', units: [] };
      }
      this.manifestLoaded = false;
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

    var summary = [];
    for (var i = 0; i < this.manifest.units.length; i++) {
      var unit = this.manifest.units[i];
      // Don't override status for units with an active download
      if (this._activeDownloads[unit.id]) { summary.push(unit.id + ' downloading'); continue; }
      var status = await this._checkUnitCache(unit);
      summary.push(unit.id + ' ' + (status === 'cached' ? 'downloaded' : status === 'partial' ? 'partial' : 'not downloaded'));
      this._fireStatus(unit.id, status, {});
    }
    if (summary.length) this._step('storage', 'info', 'Cache check: ' + summary.join(', '));
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
  // Quiet-download windows (tab visible, no new bytes). These are defaults;
  // the server can override any of them per deployment through the
  // manifest's `tuning` block (see _tune) without a JS deploy.
  var BG_STALL_MS = 150000;       // Background Fetch quiet this long => 'stalled' (the frozen-switch fires first)
  var FALLBACK_STALL_MS = 45000;  // SW fallback loop quiet this long => restarted automatically (every time)
  var SUCCESS_BROADCAST_GRACE_MS = 45000; // BG fetch success but no 'cached' broadcast => reconcile
  var FROZEN_SWITCH_MS = 25000;   // BG fetch frozen this long => switch to fallback (must exceed the keepalive)
  var PREFLIGHT_TIMEOUT_MS = 8000; // reachability probe budget per URL
  // Automatic retry backoff (seconds) after a failure; the last value repeats
  // forever. Downloads are never given up on: a network that is down now may
  // be up in a minute, and nobody should have to notice that and press a
  // button — the page shows a countdown and tries again by itself.
  var RETRY_BACKOFF_S = [5, 10, 20, 30, 60];
  var QUOTA_RETRY_S = 60; // out-of-space re-check interval (someone has to free space first)
  var SPACE_HEADROOM_BYTES = 20 * 1024 * 1024; // slack on top of a unit's size for the space preflight
  var PREFER_FALLBACK_KEY = 'mhs-prefer-fallback-until'; // localStorage: skip BG fetch until this time
  MHSDeliveryManager.PREFER_FALLBACK_KEY = PREFER_FALLBACK_KEY; // exported so Reset can purge it

  /**
   * Remembers (for 24h) that Background Fetch doesn't work on this device —
   * Chrome pauses background fetches indefinitely under device conditions
   * like a metered connection or battery saver (seen on some Chromebooks).
   * While set, downloads go straight to the SW sequential fallback, which
   * uses regular fetches and is not subject to download-service pausing.
   * With `set`, records the preference; without, reports whether it's active.
   */
  MHSDeliveryManager.prototype._preferFallback = function(set) {
    try {
      if (set) {
        localStorage.setItem(PREFER_FALLBACK_KEY, String(Date.now() + 24 * 60 * 60 * 1000));
        return true;
      }
      return Date.now() < parseInt(localStorage.getItem(PREFER_FALLBACK_KEY) || '0', 10);
    } catch (err) {
      return false;
    }
  };

  /**
   * Returns a timing value, preferring a positive number from the manifest's
   * `tuning` block (server-configurable per deployment) over the built-in
   * default. Keys: frozenSwitchMs, fallbackStallMs, bgStallMs, keepaliveMs.
   */
  MHSDeliveryManager.prototype._tune = function(name, dflt) {
    var t = this.manifest && this.manifest.tuning;
    var v = t && t[name];
    return (typeof v === 'number' && v > 0) ? v : dflt;
  };

  // One reachability probe: resolves {ok, ms, status, error}; never rejects.
  // 'cors' mode requires a readable OK response (the CDN serves CORS headers);
  // 'no-cors' mode treats any response, even an opaque one, as reachable,
  // which is enough to tell "blocked by a filter" from "up".
  function probeURL(url, mode) {
    var started = Date.now();
    var controller = (typeof AbortController !== 'undefined') ? new AbortController() : null;
    var timer = controller ? setTimeout(function() { controller.abort(); }, PREFLIGHT_TIMEOUT_MS) : null;
    var opts = { method: 'GET', mode: mode, cache: 'no-store', credentials: 'omit' };
    if (controller) opts.signal = controller.signal;
    return fetch(url, opts).then(function(resp) {
      if (timer) clearTimeout(timer);
      var ok = (mode === 'no-cors') ? true : !!resp.ok;
      try { if (resp.body && resp.body.cancel) resp.body.cancel(); } catch (e) { /* ignore */ }
      return { ok: ok, ms: Date.now() - started, status: resp.status || 0, error: ok ? '' : ('HTTP ' + resp.status) };
    }).catch(function(err) {
      if (timer) clearTimeout(timer);
      var aborted = err && err.name === 'AbortError';
      return { ok: false, ms: Date.now() - started, status: 0,
        error: aborted ? ('no response within ' + Math.round(PREFLIGHT_TIMEOUT_MS / 1000) + 's') : String((err && err.message) || err) };
    });
  }

  function hostOf(url) {
    try { return new URL(url, window.location.href).host; } catch (e) { return url; }
  }

  // The message shown when the content server cannot be reached.
  function cdnUnreachableMessage(host, error) {
    return 'Cannot reach the game content server (' + host + '). A school firewall or ' +
      'content filter may be blocking it — ask your IT staff to allow ' + host +
      ', then tap Retry.' + (error ? ' (' + error + ')' : '');
  }

  /**
   * Checks that the content server (and, informationally, the game services)
   * can be reached before a download starts, so a blocked CDN fails in
   * seconds with a specific message instead of sitting at 0% until the stall
   * machinery gives up. Probes the unit's smallest file on the CDN (CORS, must
   * be OK) and any `probes` the manifest lists (no-cors, reachability only).
   * A successful CDN result is cached for the manager's lifetime; a failed one
   * is not, so Retry re-checks. Never throws.
   */
  MHSDeliveryManager.prototype.preflight = async function(unit) {
    if (this.preflightResult && this.preflightResult.cdn && this.preflightResult.cdn.ok) {
      return this.preflightResult;
    }
    var result = { cdn: null, services: [], at: new Date().toISOString() };
    var base = (this.manifest && this.manifest.cdnBaseUrl) || '';
    if (base && unit && unit.files && unit.files.length) {
      var smallest = unit.files[0];
      for (var i = 1; i < unit.files.length; i++) {
        if ((unit.files[i].size || 0) < (smallest.size || 0)) smallest = unit.files[i];
      }
      this._step('cdn', 'running', 'Checking the content server (' + hostOf(base) + ')…');
      result.cdn = await probeURL(base + '/' + smallest.path, 'cors');
      result.cdn.host = hostOf(base);
      this._step('cdn', result.cdn.ok ? 'ok' : 'fail', result.cdn.ok
        ? 'Content server reachable (' + result.cdn.ms + ' ms)'
        : 'Cannot reach the content server ' + result.cdn.host + ': ' + result.cdn.error);
    }
    var probes = (this.manifest && this.manifest.probes) || [];
    var checks = [];
    for (var p = 0; p < probes.length; p++) {
      (function(probe) {
        checks.push(probeURL(probe.url, 'no-cors').then(function(r) {
          return { name: probe.name, host: hostOf(probe.url), ok: r.ok, ms: r.ms, error: r.error };
        }));
      })(probes[p]);
    }
    if (checks.length) {
      try { result.services = await Promise.all(checks); } catch (e) { /* individual probes never reject */ }
    }
    for (var s = 0; s < result.services.length; s++) {
      var sv = result.services[s];
      if (!sv.ok) {
        console.warn('MHS preflight: game service "' + sv.name + '" (' + sv.host + ') unreachable: ' + sv.error);
      }
      this._step('services', sv.ok ? 'ok' : 'warn', 'Game ' + sv.name + ' service (' + sv.host + ') ' +
        (sv.ok ? 'reachable (' + sv.ms + ' ms)' : 'unreachable: ' + sv.error + ' — the game may report a connection error'));
    }
    this.preflightResult = result;
    return result;
  };

  // Compact one-line summary of the last preflight for telemetry.
  MHSDeliveryManager.prototype._preflightSummary = function() {
    var r = this.preflightResult;
    if (!r) return '';
    var parts = [];
    if (r.cdn) parts.push('cdn:' + (r.cdn.ok ? 'ok ' + r.cdn.ms + 'ms' : 'FAIL ' + r.cdn.error));
    for (var i = 0; i < r.services.length; i++) {
      var sv = r.services[i];
      parts.push(sv.name + ':' + (sv.ok ? 'ok ' + sv.ms + 'ms' : 'FAIL ' + sv.error));
    }
    return parts.join('; ');
  };

  MHSDeliveryManager.prototype._findUnit = function(unitId) {
    if (!this.manifest || !this.manifest.units) return null;
    return this.manifest.units.find(function(u) { return u.id === unitId; }) || null;
  };

  // Sum of the manifest sizes of a unit's files that are not in its cache yet
  // (all of them when nothing is cached). Cache lookups only; no network.
  MHSDeliveryManager.prototype._missingBytes = async function(unit) {
    var total = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    if (!('caches' in window)) return total;
    try {
      var cache = await caches.open(this._unitCachePrefix + unit.id + '-v' + unit.version);
      var missing = 0;
      for (var i = 0; i < unit.files.length; i++) {
        var match = await cache.match(this._contentPrefix + unit.files[i].path);
        if (!match) missing += unit.files[i].size || 0;
      }
      return missing;
    } catch (err) {
      return total;
    }
  };

  /**
   * Per-unit space preflight (MHS-006): a download that cannot fit should
   * wait for space, not fail part-way through a long transfer (and, on
   * retries, not re-download a file every minute only to fail at cache.put).
   * Compares the bytes still missing from the unit's cache with the free
   * quota. Resolves null when no estimate exists.
   */
  MHSDeliveryManager.prototype._checkSpace = async function(unit) {
    if (!(navigator.storage && navigator.storage.estimate)) return null;
    var est;
    try { est = await navigator.storage.estimate(); } catch (err) { return null; }
    if (!est || !est.quota) return null;
    var missing = await this._missingBytes(unit);
    if (missing <= 0) return null;
    var need = Math.round(missing * 1.05) + SPACE_HEADROOM_BYTES;
    var free = est.quota - (est.usage || 0);
    if (free >= need) return { ok: true, freeBytes: free, needBytes: need };
    var mb = function(b) { return Math.round(b / 1048576); };
    return {
      ok: false, freeBytes: free, needBytes: need,
      summary: 'free ' + mb(free) + 'MB < need ' + mb(need) + 'MB',
      message: 'Not enough free space on this device: about ' + mb(free) + ' MB free, this unit still needs about ' +
        mb(need) + ' MB. Free up space or clear other downloads — the download continues automatically once there is room.'
    };
  };

  /**
   * Never give up. A requested download is a standing intent: it stays
   * "wanted" until the unit is cached or the user removes it, every failure
   * or stall schedules the next attempt here (backoff capped at a minute,
   * reset by real progress), and a 'retrying' status with a live countdown
   * keeps the page honest about what is happening. The Retry button becomes
   * "retry now": it only skips the wait (see retryNow).
   * kind: 'error' (re-run downloadUnit after the backoff) or 'stalled'
   * (restart at once through retryDownload, which resumes from the cache).
   */
  MHSDeliveryManager.prototype._scheduleRetry = function(unitId, kind, base) {
    this._clearPendingRetry(unitId);
    if (!this._wanted[unitId]) return;
    var attempt = (this._retryAttempts[unitId] || 0) + 1;
    this._retryAttempts[unitId] = attempt;

    var delayS = 0;
    if (kind !== 'stalled') {
      delayS = RETRY_BACKOFF_S[Math.min(attempt - 1, RETRY_BACKOFF_S.length - 1)];
      if (base.errorClass === 'quota') delayS = Math.max(delayS, QUOTA_RETRY_S);
    }
    // Repeated Background Fetch failures: prefer the direct path from now on.
    // It is a different mechanism (plain fetches inside the service worker)
    // and immune to the download-service problems that fail Background Fetch.
    if (base.path === 'bg' && attempt >= 3) this._preferFallback(true);

    var self = this;
    var pending = { kind: kind, dueAt: Date.now() + delayS * 1000, attempt: attempt, base: base, timer: null, tick: null };
    this._pendingRetries[unitId] = pending;
    this._step('download', 'warn', (base.error || 'Download interrupted.') + ' Retrying ' +
      (delayS > 0 ? 'in ' + delayS + ' s' : 'now') + ' (attempt ' + attempt + ')');

    function announce(remainingMs) {
      var d = {};
      for (var k in base) d[k] = base[k];
      d.local = true;
      d.attempt = attempt;
      d.retryInMs = Math.max(0, remainingMs);
      self._fireStatus(unitId, 'retrying', d);
    }

    function tick() {
      if (self._pendingRetries[unitId] !== pending) return; // superseded or canceled
      var remaining = pending.dueAt - Date.now();
      if (remaining > 0) {
        announce(remaining);
        pending.timer = setTimeout(tick, Math.min(1000, remaining));
        return;
      }
      delete self._pendingRetries[unitId];
      announce(0);
      var run = (pending.kind === 'stalled') ? self.retryDownload(unitId) : self.downloadUnit(unitId);
      Promise.resolve(run).catch(function(err) {
        console.warn('Automatic retry failed to start:', err);
      });
    }
    pending.tick = tick;
    tick();
  };

  // Cancels a scheduled retry. The intent to download stays unless
  // _forgetDownload is called as well.
  MHSDeliveryManager.prototype._clearPendingRetry = function(unitId) {
    var p = this._pendingRetries[unitId];
    if (!p) return;
    if (p.timer) clearTimeout(p.timer);
    delete this._pendingRetries[unitId];
  };

  // Drops the standing intent to download a unit: no further automatic
  // retries. Used when a unit is cached, deleted, or cleaned up.
  MHSDeliveryManager.prototype._forgetDownload = function(unitId) {
    this._clearPendingRetry(unitId);
    delete this._wanted[unitId];
    delete this._retryAttempts[unitId];
  };

  MHSDeliveryManager.prototype._forgetAllDownloads = function() {
    var ids = Object.keys(this._pendingRetries).concat(Object.keys(this._wanted));
    for (var i = 0; i < ids.length; i++) this._forgetDownload(ids[i]);
  };

  /**
   * Runs a scheduled retry now instead of waiting out the countdown (the
   * Retry button). With nothing scheduled it simply starts the download.
   */
  MHSDeliveryManager.prototype.retryNow = function(unitId) {
    var p = this._pendingRetries[unitId];
    this._clearPendingRetry(unitId);
    var run = (p && p.kind === 'stalled') ? this.retryDownload(unitId) : this.downloadUnit(unitId);
    return Promise.resolve(run).catch(function(err) { console.warn('Retry failed to start:', err); });
  };

  // Fires due retries at once when the tab becomes visible again — hidden
  // tabs throttle timers, so a countdown may have expired unnoticed.
  MHSDeliveryManager.prototype._kickPendingRetries = function() {
    var ids = Object.keys(this._pendingRetries);
    for (var i = 0; i < ids.length; i++) {
      var p = this._pendingRetries[ids[i]];
      if (p && p.tick) {
        if (p.timer) clearTimeout(p.timer);
        p.tick();
      }
    }
  };

  /**
   * Attaches an MHSStepLog (mhs-steplog.js): from then on the manager records
   * what it does — worker, game list, storage, reachability, method, download
   * progress and retries, verification — as timestamped steps the page shows
   * in its "Status details" panel and the user can copy into a report.
   */
  MHSDeliveryManager.prototype.setStepLog = function(log) {
    this._steplog = log || null;
  };

  // Records a step; never lets logging break delivery.
  MHSDeliveryManager.prototype._step = function(step, state, msg, detail) {
    if (!this._steplog) return;
    try { this._steplog.record(step, state, msg, detail); } catch (e) { /* ignore */ }
  };

  MHSDeliveryManager.prototype._logContext = function(key, value) {
    if (!this._steplog || !this._steplog.set) return;
    try { this._steplog.set(key, value); } catch (e) { /* ignore */ }
  };

  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  function fmtMB(bytes) { return Math.round((bytes || 0) / 1048576); }
  function fmtDuration(ms) {
    var s = Math.max(0, Math.round(ms / 1000));
    return Math.floor(s / 60) + ':' + pad2(s % 60);
  }

  /**
   * Asks the active worker for its version (a 'getVersion' message answered
   * over a MessageChannel). Resolves '' on timeout or an older worker.
   */
  MHSDeliveryManager.prototype._getSWVersion = function() {
    var self = this;
    return new Promise(function(resolve) {
      if (!navigator.serviceWorker) { resolve(''); return; }
      var settled = false;
      function finish(v) { if (settled) return; settled = true; resolve(v || ''); }
      var timer = setTimeout(function() { finish(''); }, 2000);
      self._waitForSW(1500).then(function(reg) {
        var target = (reg && reg.active) || navigator.serviceWorker.controller;
        if (!target) { clearTimeout(timer); finish(''); return; }
        try {
          var channel = new MessageChannel();
          channel.port1.onmessage = function(ev) { clearTimeout(timer); finish(ev.data && ev.data.version); };
          target.postMessage({ action: 'getVersion' }, [channel.port2]);
        } catch (e) { clearTimeout(timer); finish(''); }
      }).catch(function() { clearTimeout(timer); finish(''); });
    });
  };

  // One 'storage' line: usage, quota, persistence.
  MHSDeliveryManager.prototype._logStorageSummary = async function() {
    if (!this._steplog) return;
    try {
      var est = await this.getStorageEstimate();
      if (!est || !est.quota) return;
      var persisted = await this.getStoragePersisted();
      var text = fmtMB(est.usage) + ' MB used of ' + fmtMB(est.quota) + ' MB (' +
        Math.round((est.usage / est.quota) * 100) + '%)' +
        (persisted === null ? '' : ' · persisted: ' + (persisted ? 'yes' : 'no'));
      this._logContext('storage', text);
      this._step('storage', persisted === false ? 'warn' : 'ok', 'Storage: ' + text +
        (persisted === false ? ' — the browser may evict downloads under pressure' : ''));
    } catch (e) { /* ignore */ }
  };

  // After a completed download, confirm the files really are all there.
  MHSDeliveryManager.prototype._verifyForLog = async function(unitId) {
    if (!this._steplog) return;
    var unit = this._findUnit(unitId);
    if (!unit) return;
    try {
      var state = await this._checkUnitCache(unit);
      if (state === 'cached') {
        this._step('verify', 'ok', 'All ' + unit.files.length + ' files present; largest file size verified');
      } else {
        this._step('verify', 'warn', 'Files missing or a size mismatch after download (' + state + ')');
      }
    } catch (e) {
      this._step('verify', 'warn', 'Could not verify files: ' + ((e && e.message) || e));
    }
  };

  /**
   * Translates status events (from this page and from the worker's
   * broadcasts) into step-log lines. Progress is sampled at most every 5 s
   * or per 10% bucket; countdown ticks are skipped ('retrying' is logged once
   * per attempt by _scheduleRetry); detail.silent skips an event the caller
   * already logged under a better step.
   */
  MHSDeliveryManager.prototype._logStatus = function(unitId, status, detail, wasActive) {
    if (!this._steplog) return;
    detail = detail || {};
    if (detail.silent) return;
    var unit = this._findUnit(unitId);
    var title = unit ? unit.title : unitId;
    switch (status) {
      case 'downloading': {
        if (detail.note) { this._step('download', 'warn', detail.note); return; }
        if (detail.waitingMs >= 5000) {
          var wmsg = 'No data for ' + Math.round(detail.waitingMs / 1000) + ' s';
          if (typeof detail.switchInMs === 'number') wmsg += ' — switching to the direct download in ' + Math.round(detail.switchInMs / 1000) + ' s';
          else if (typeof detail.resumeInMs === 'number') wmsg += ' — restarting in ' + Math.round(detail.resumeInMs / 1000) + ' s';
          this._step('download', 'warn', wmsg);
          return;
        }
        if (typeof detail.downloaded !== 'number') return;
        var now = Date.now();
        var sample = this._logSamples[unitId];
        var pct = detail.percent || 0;
        var bucket = Math.floor(pct / 10);
        if (sample && now - sample.at < 5000 && bucket === sample.bucket) return;
        var rate = '';
        if (sample && detail.downloaded > sample.bytes && now > sample.at) {
          var bps = (detail.downloaded - sample.bytes) / ((now - sample.at) / 1000);
          rate = ' · ' + (bps / 1048576).toFixed(1) + ' MB/s';
          if (detail.downloadTotal && bps > 0) {
            rate += ' · ' + fmtDuration(((detail.downloadTotal - detail.downloaded) / bps) * 1000) + ' left';
          }
        }
        this._logSamples[unitId] = { at: now, bytes: detail.downloaded, bucket: bucket };
        this._step('download', 'running', title + ': ' + pct + '% · ' + fmtMB(detail.downloaded) + ' of ' +
          fmtMB(detail.downloadTotal) + ' MB' + rate);
        return;
      }
      case 'stalled':
        this._step('download', 'warn', title + ': no data received for ' + Math.round((detail.quietMs || 0) / 1000) +
          ' s — restarting the download');
        return;
      case 'retrying':
        return;
      case 'error':
        this._step('download', 'fail', title + ': ' + (detail.error || 'Download failed') +
          (detail.errorClass ? ' [' + detail.errorClass + (detail.failureReason ? ':' + detail.failureReason : '') + ']' : '') +
          (detail.rawError ? ' — ' + detail.rawError : ''), { path: detail.path || '' });
        return;
      case 'cached': {
        if (!wasActive) return; // an init-time cache check, not a download we ran
        delete this._logSamples[unitId];
        var st = this._stallState[unitId];
        var total = unit ? (unit.totalSize || 0) : 0;
        var took = st ? Date.now() - st.startedAt : 0;
        var msg = title + ': download complete';
        if (total) msg += ' — ' + fmtMB(total) + ' MB';
        if (took > 0) {
          msg += ' in ' + fmtDuration(took);
          if (total) msg += ' (' + ((total / 1048576) / (took / 1000)).toFixed(1) + ' MB/s)';
        }
        this._step('download', 'ok', msg);
        this._verifyForLog(unitId).catch(function() {});
        return;
      }
      default:
        return;
    }
  };

  // User-facing copy for the active download mode. 'background' is the default
  // (Background Fetch, OS-level — survives closing the tab); 'fallback' is the
  // escalation on devices where Chrome pauses Background Fetch, where the
  // download runs in the page's service worker and only progresses while a tab
  // is open. Templates render these beside the download UI so students/teachers
  // know whether they can walk away.
  MHSDeliveryManager.DOWNLOAD_MODE_MESSAGES = {
    background: 'Downloading in the background — you can switch tabs, use other apps, or close this page. Your units keep downloading and will be ready when you come back.',
    fallback: 'Keep this tab open and stay on this page until the download finishes. On this device, downloads pause if the tab is closed.'
  };

  // The mode the next/current download uses: 'fallback' once this device is
  // known to pause Background Fetch, else 'background'.
  MHSDeliveryManager.prototype.getDownloadMode = function() {
    return this._preferFallback() ? 'fallback' : 'background';
  };

  // True while any unit download is being tracked as active.
  MHSDeliveryManager.prototype.hasActiveDownload = function() {
    return Object.keys(this._activeDownloads).length > 0;
  };

  /**
   * Starts the SW sequential fallback download for a unit and its watchdog.
   * Returns false when no active service worker is available (nothing posted).
   */
  MHSDeliveryManager.prototype._startFallbackDownload = async function(unitId, unit) {
    if (!navigator.serviceWorker) return false;
    // The fallback loop runs INSIDE the service worker and reaches the page over
    // the BroadcastChannel, so it needs an ACTIVE worker to message — not one
    // that is CONTROLLING this page. Requiring control (the old behavior) made
    // the fallback silently fail right after a hard refresh or on a first visit
    // before the SW claimed the page, reverting the download to the pausable
    // Background Fetch. Prefer the controller when present (instant), else wait
    // briefly for an active registration and message that.
    var target = navigator.serviceWorker.controller;
    if (!target) {
      try {
        var reg = await this._waitForSW(6000);
        target = reg && reg.active;
      } catch (err) { /* fall through */ }
    }
    if (!target) return false;
    target.postMessage({
      action: 'fallbackDownload',
      unitId: unit.id,
      version: unit.version,
      files: unit.files,
      cdnBaseUrl: this.manifest.cdnBaseUrl,
      title: 'Downloading ' + unit.title
    });
    // Watchdog: there is no registration to poll on this path, so watch the
    // broadcast stream for silence instead.
    this._monitorFallbackDownload(unitId, unit);
    return true;
  };

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
    this._wanted[unitId] = true; // a tracked download (incl. one adopted after a reload) is a standing intent
    if (this._downloadMonitors[unitId]) {
      clearInterval(this._downloadMonitors[unitId]);
      delete this._downloadMonitors[unitId];
    }

    var state = {
      startedAt: Date.now(), // fixed start mark — lastProgressAt is reset by tab switches
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
    var threshold = state.fallback
      ? this._tune('fallbackStallMs', FALLBACK_STALL_MS)
      : this._tune('bgStallMs', BG_STALL_MS);
    var quietMs = Date.now() - state.lastProgressAt;
    if (quietMs < threshold) return;
    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    var percent = totalSize > 0 ? Math.round((state.maxDownloaded / totalSize) * 100) : 0;

    // Never give up: a quiet download (SW killed mid-file, connection
    // black-holed, a Background Fetch that could not be switched) is restarted
    // automatically, every time. retryDownload cancels the loop without
    // purging, so the replacement resumes from the last completed file.
    // 'stalled' still fires for pages that listen for it; the scheduled
    // restart follows at once (and the scheduler counts the attempts).
    state.stalled = true;
    var quietS = Math.round(quietMs / 1000);
    this._fireStatus(unitId, 'stalled', {
      downloaded: state.maxDownloaded,
      downloadTotal: totalSize,
      percent: percent,
      quietMs: quietMs,
      autoRetry: true
    });
    console.warn('Download quiet for ' + quietS + 's — restarting automatically:', unitId);
    this._scheduleRetry(unitId, 'stalled', {
      error: 'No data received for ' + quietS + ' s — restarting the download.',
      errorClass: 'stalled',
      downloaded: state.maxDownloaded,
      downloadTotal: totalSize,
      percent: percent
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
      // No registration. The SW may have fallen back to its sequential
      // download path (its Background Fetch failed to start) — adopt that
      // instead of declaring the download dead.
      try {
        var fbKeys = await this._getActiveFallbacks();
        if (fbKeys.indexOf(unit.id + '-v' + unit.version) !== -1) {
          state.fallback = true; // switch to broadcast-liveness monitoring
          this._maybeFireStalled(unitId, unit);
          return;
        }
      } catch (fbErr) {
        // Query unavailable — fall through to the disappeared handling
      }
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

    // Chrome streams live byte counts only to the browsing context that
    // STARTED the fetch. A page that reconnected after a reload/navigation
    // sees a frozen snapshot instead — but completed records are visible
    // from any context, so use their summed manifest sizes as a floor.
    // Reconnected pages then advance at file boundaries rather than
    // freezing at the reconnect-time value.
    try {
      var records = await fresh.matchAll();
      var completedBytes = 0;
      for (var r = 0; r < records.length; r++) {
        var settled = await Promise.race([
          records[r].responseReady.then(function() { return true; }, function() { return false; }),
          new Promise(function(resolve) { setTimeout(function() { resolve(false); }, 30); })
        ]);
        if (!settled) continue;
        var recUrl = records[r].request.url;
        for (var f = 0; f < unit.files.length; f++) {
          if (recUrl.indexOf(unit.files[f].path) !== -1) {
            completedBytes += unit.files[f].size || 0;
            break;
          }
        }
      }
      if (completedBytes > downloaded) downloaded = completedBytes;
    } catch (recErr) {
      // matchAll unavailable or transient — the byte counter alone is fine
    }

    // A Background Fetch whose progress has FROZEN — no new bytes for a
    // generous window while the fetch is still unfinished — is paused, not
    // downloading. On some devices Chrome pauses background fetches
    // indefinitely (metered connection, battery saver, a corrupt download
    // service — observed on Chromebooks, where the OS shows its own "Paused
    // Downloading…" notification). This catches BOTH shapes of that failure:
    // "zero bytes, never started" AND "a little downloaded, then stuck"
    // (e.g. paused at 12%) — the earlier zero-only check missed the latter,
    // which is the common field case. Switch to the SW sequential fallback,
    // which uses regular fetches and is immune to download-service pausing.
    //
    // Guards, in order:
    //  - noNewBytes THIS tick (not just an aged timestamp): a download that just
    //    resumed is never aborted, and a healthy slow download — which delivers
    //    bytes via the SW's live progress broadcasts or a completing file — is
    //    never touched.
    //  - VISIBLE tab only. This is essential, not cosmetic: a healthy
    //    Background Fetch pre-downloading with the tab HIDDEN (the pre-class
    //    case) can legitimately stop broadcasting for a while (SW idle-
    //    terminated between keepalives), which would look "frozen". We must not
    //    yank that onto the page-open fallback. The old zero-byte-only check
    //    couldn't misfire here (any progress made it immune); the frozen check
    //    can, so gate it on the page actually being in the foreground — which
    //    is also the only time the fallback (a page-open download) is useful.
    //  - No SW CONTROL requirement: the stalling download often started on a
    //    page loaded uncontrolled (hard refresh, or a first load right after
    //    clearing site data) where navigator.serviceWorker.controller stays null
    //    — the ACER's exact failure mode. _startFallbackDownload needs only an
    //    ACTIVE worker, so start it FIRST and abort the paused Background Fetch
    //    only once the fallback has taken over — never stranding the unit.
    // The partial bytes Chrome pulled before pausing aren't in our cache
    // (Background Fetch caches atomically on success), so aborting re-fetches
    // only cheap, re-downloadable bytes — far better than staying stuck.
    var noNewBytes = downloaded <= state.maxDownloaded;
    var frozenMs = this._tune('frozenSwitchMs', FROZEN_SWITCH_MS);
    var quietMs = Date.now() - state.lastProgressAt;
    var unitTotal = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);

    // While nothing arrives, tell the page how long it has been quiet and when
    // the switch is due, so the wait is visible instead of a frozen "0%".
    // `local` marks this as page-generated: it is not a liveness signal.
    if (fresh.result === '' && noNewBytes && !state.stalled &&
        document.visibilityState === 'visible' && quietMs >= PROGRESS_POLL_MS) {
      this._fireStatus(unitId, 'downloading', {
        downloaded: state.maxDownloaded,
        downloadTotal: unitTotal,
        percent: unitTotal > 0 ? Math.round((state.maxDownloaded / unitTotal) * 100) : 0,
        local: true,
        waitingMs: quietMs,
        switchInMs: Math.max(0, frozenMs - quietMs)
      });
    }

    if (fresh.result === '' && noNewBytes &&
        document.visibilityState === 'visible' &&
        quietMs > frozenMs) {
      console.warn('Background Fetch progress frozen for ' +
        Math.round(frozenMs / 1000) + 's (paused) — switching to fallback download:', fetchId);
      this._preferFallback(true);
      var frozenAt = state.maxDownloaded;
      var switched = await this._startFallbackDownload(unitId, unit); // resets the stall monitor
      if (switched) {
        try { await fresh.abort(); } catch (abortErr) { /* best effort */ }
        this._logContext('downloadMode', 'direct');
        this._step('method', 'warn', 'Background download stopped receiving data for ' + Math.round(quietMs / 1000) +
          ' s — switched to the direct download (keep this tab open)');
        this._fireStatus(unitId, 'downloading', {
          downloaded: frozenAt,
          downloadTotal: unitTotal,
          percent: unitTotal > 0 ? Math.round((frozenAt / unitTotal) * 100) : 0,
          local: true,
          silent: true,
          note: 'The background download stopped receiving data — switched to the direct download method. Keep this tab open until it finishes.'
        });
        return;
      }
      // No active worker to hand off to yet — leave the paused Background Fetch
      // in place and try again on the next poll tick rather than stranding it.
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
   * stall state by _fireStatus). If they go quiet for FALLBACK_STALL_MS
   * while the tab is visible (SW killed mid-download, network black hole),
   * resume once automatically, then surface 'stalled' so the user gets the
   * same Retry affordance as the Background Fetch path. A retry re-posts fallbackDownload and resumes
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
      // Surface the wait (and what happens next) while the loop is quiet.
      var quietMs = Date.now() - state.lastProgressAt;
      if (!state.stalled && quietMs >= PROGRESS_POLL_MS) {
        var threshold = self._tune('fallbackStallMs', FALLBACK_STALL_MS);
        var total = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
        var detail = {
          downloaded: state.maxDownloaded,
          downloadTotal: total,
          percent: total > 0 ? Math.round((state.maxDownloaded / total) * 100) : 0,
          local: true,
          waitingMs: quietMs
        };
        detail.resumeInMs = Math.max(0, threshold - quietMs);
        self._fireStatus(unitId, 'downloading', detail);
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
    // Fire a terminal status on the early returns — a silent return here leaves
    // the units/manage Retry button disabled (mhsDownload disables it before
    // calling) with the unit stuck showing "Stalled".
    this._wanted[unitId] = true;
    this._clearPendingRetry(unitId);
    if (!this.manifestLoaded) {
      this._fireStatus(unitId, 'error', { error: 'Could not load the game list. Check the connection.', errorClass: 'manifest' });
      return;
    }
    var unit = this._findUnit(unitId);
    if (!unit) {
      this._fireStatus(unitId, 'error', { error: 'This unit is not available in the current game version.', errorClass: 'unit-missing' });
      return;
    }

    // Re-entrancy guard: cacheStatus stays 'stalled' until the awaits below
    // complete, so a double-click would route here twice and interleave —
    // either aborting the fresh download or racing two downloadUnit runs
    // (a duplicate Background Fetch alongside a fallback loop).
    if (this._retryInFlight[unitId]) return;
    this._retryInFlight[unitId] = true;

    try {
      // A retry after a network fix must confirm the content server is
      // reachable before restarting anything (a failed probe is never cached).
      var pf = await this.preflight(unit);
      if (pf.cdn && !pf.cdn.ok) {
        this._fireStatus(unitId, 'error', {
          error: cdnUnreachableMessage(pf.cdn.host, pf.cdn.error),
          errorClass: 'cdn-unreachable',
          rawError: pf.cdn.error,
          path: ''
        });
        return;
      }

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

      // A retry follows a stall. On devices where Chrome pauses Background
      // Fetches (metered connection, battery saver — seen on Chromebooks),
      // starting another one just pauses again; the SW sequential fallback
      // uses regular fetches and is immune to that, so retries prefer it
      // (and remember the preference). _startFallbackDownload waits for an
      // active worker, so this works even right after a hard refresh; it falls
      // through to the normal path only if no worker becomes available.
      if (navigator.serviceWorker) {
        this._preferFallback(true);
        this._activeDownloads[unitId] = true;
        var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
        this._fireStatus(unitId, 'downloading', { percent: 0, downloaded: 0, downloadTotal: totalSize });
        if (await this._startFallbackDownload(unitId, unit)) {
          this._startProgressKeepalive(this.swRegistration);
          return;
        }
        delete this._activeDownloads[unitId]; // couldn't start — let downloadUnit try cleanly
      }

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
            // This page's view of the byte counter may be a frozen snapshot
            // (Chrome streams live bytes only to the creating context), so
            // also ask the SW to (re)attach its broadcaster and keep it fed.
            this._activeDownloads[unitId] = true;
            this._attachBGFetchListeners(unitId, bgFetch, unit);
            this._reportDownloadProgress(unitId, unit, bgFetch.downloaded || 0);
            if (reg.active) reg.active.postMessage({ action: 'attachProgress' });
            this._startProgressKeepalive(reg);
            console.log('Reconnected to Background Fetch:', ids[i]);
          } else if (this.manifestLoaded) {
            // No manifest unit at this unit+version — genuinely stale.
            // Abort it: it can only produce an unwanted old-version cache,
            // and while in flight it blocks pruning of that cache. Only do
            // this against a real manifest: if the manifest failed to load,
            // "no match" is meaningless and would abort a healthy download.
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
            // Completed while hidden, but the SW may STILL be copying records
            // into the cache (a large unit on slow storage takes many seconds).
            // Do NOT clear tracking yet: the sibling checkAllCacheStatus() runs
            // concurrently on this same visibility change and skips units in
            // _activeDownloads — clearing here lets it see the not-yet-complete
            // cache, fire 'partial', and trigger a DUPLICATE download via
            // self-heal. Only finish when the cache is actually complete;
            // otherwise leave tracking + the monitor in place and let
            // _pollDownloadOnce reconcile with its success grace.
            return self._checkUnitCache(unit).then(function(cacheStatus) {
              if (cacheStatus === 'cached') {
                delete self._activeDownloads[unitId];
                if (self._downloadMonitors[unitId]) {
                  clearInterval(self._downloadMonitors[unitId]);
                  delete self._downloadMonitors[unitId];
                }
                self._fireStatus(unitId, 'cached', {});
              } else if (self._downloadMonitors[unitId]) {
                // Still copying — start the poller's success-grace clock.
                var stNow = self._stallState[unitId];
                if (stNow && !stNow.successSince) stNow.successSince = Date.now();
              } else {
                // No monitor left to reconcile — re-arm one so the unit can't
                // sit at "Downloading" forever.
                self._monitorDownload(unitId, unit);
              }
            });
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
    // Prevent duplicate downloads — if already active, skip
    if (this._activeDownloads[unitId]) {
      return;
    }
    // A download request is a standing intent: the unit stays wanted until
    // it is cached or removed, and every failure below schedules a retry.
    this._wanted[unitId] = true;
    this._clearPendingRetry(unitId);

    // Every early return MUST fire a terminal status. Callers (the play-page
    // overlay, the units/manage pipelines) drive their UI off status events,
    // and the retry scheduler keys off 'error' — a silent return would leave
    // them hanging. A manifest that failed to load (or doesn't list the unit
    // yet) is re-fetched here, so a retry after an outage picks it up.
    var lateManifest = false;
    if (!this.manifestLoaded || !this._findUnit(unitId)) {
      await this.refreshManifest();
      lateManifest = this.manifestLoaded;
    }
    if (!this.manifestLoaded) {
      console.error('Manifest not loaded');
      this._fireStatus(unitId, 'error', { error: 'Could not load the game list. Check the connection.', errorClass: 'manifest' });
      return;
    }
    var unit = this._findUnit(unitId);
    if (!unit) {
      console.error('Unit not found:', unitId);
      this._fireStatus(unitId, 'error', { error: 'This unit is not available in the current game version.', errorClass: 'unit-missing' });
      return;
    }

    if (!navigator.serviceWorker) {
      this._fireStatus(unitId, 'error', { error: 'Service worker not supported.', errorClass: 'no-sw' });
      return;
    }

    this._activeDownloads[unitId] = true;
    // The manifest arrived late (init's own attempt had failed): refresh the
    // other units' statuses now that they are known. This unit is skipped by
    // checkAllCacheStatus because it is active.
    if (lateManifest) this.checkAllCacheStatus().catch(function() {});

    var totalSize = unit.totalSize || unit.files.reduce(function(sum, f) { return sum + f.size; }, 0);
    this._step('download', 'running', 'Requesting ' + unit.title + ' v' + unit.version + ' (' + fmtMB(totalSize) + ' MB)');
    this._fireStatus(unitId, 'downloading', { percent: 0, downloaded: 0, downloadTotal: totalSize });

    try {
      // Fail fast and specifically when the content server is blocked, instead
      // of starting a download that can only sit at 0% until the stall
      // machinery gives up (school filters and security software do this).
      // The scheduler keeps re-checking, so a filter fixed later just works.
      var pf = await this.preflight(unit);
      if (pf.cdn && !pf.cdn.ok) {
        this._fireStatus(unitId, 'error', {
          error: cdnUnreachableMessage(pf.cdn.host, pf.cdn.error),
          errorClass: 'cdn-unreachable',
          rawError: pf.cdn.error,
          path: ''
        });
        return;
      }

      // Space preflight (MHS-006): wait for room rather than fail mid-transfer.
      var space = await this._checkSpace(unit);
      if (space && !space.ok) {
        this._fireStatus(unitId, 'error', {
          error: space.message,
          errorClass: 'quota',
          rawError: space.summary,
          path: ''
        });
        return;
      }

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
          // through the shared monotonic/sticky guard. Ask the SW to
          // (re)attach its progress broadcaster too — this page's own view
          // of the byte counter may be a frozen snapshot (see below).
          this._attachBGFetchListeners(unitId, existing, unit);
          this._reportDownloadProgress(unitId, unit, existing.downloaded || 0);
          if (reg.active) reg.active.postMessage({ action: 'attachProgress' });
          this._startProgressKeepalive(reg);
          console.log('Reconnected to existing Background Fetch:', fetchId);
          this._logContext('downloadMode', 'background');
          this._step('method', 'info', 'Reconnected to a background download already in progress');
          return;
        }
      }

      // Devices where Chrome pauses Background Fetches (metered connection,
      // battery saver — seen on some Chromebooks) are remembered for a day:
      // go straight to the SW sequential fallback there.
      if (this._preferFallback() && await this._startFallbackDownload(unitId, unit)) {
        this._startProgressKeepalive(reg);
        console.log('Using SW fallback download (Background Fetch pauses on this device):', fetchId);
        this._logContext('downloadMode', 'direct');
        this._step('method', 'info', 'Direct download — this device paused background downloads earlier; keep this tab open');
        return;
      }

      // No existing fetch — have the SERVICE WORKER start the Background
      // Fetch. Chrome streams live byte progress only to the context that
      // created the fetch; a page context is lost on any reload/navigation,
      // freezing its displayed percentage. The SW is shared by every page
      // and broadcasts progress to all of them; the keepalive below keeps
      // it alive while the download runs.
      if (reg.backgroundFetch && reg.active) {
        reg.active.postMessage({
          action: 'download',
          unitId: unit.id,
          version: unit.version,
          files: unit.files,
          cdnBaseUrl: this.manifest.cdnBaseUrl,
          title: 'Downloading ' + unit.title
        });
        // Poll-based monitor for terminal states and the stall backstop;
        // live percentages arrive via the SW's progress broadcasts.
        this._monitorDownload(unitId, unit);
        this._startProgressKeepalive(reg);
        console.log('Requested SW Background Fetch:', fetchId);
        this._logContext('downloadMode', 'background');
        this._step('method', 'info', 'Background download (Chrome) — continues if you leave the page');
        return;
      }

      // Fallback: use SW sequential fetch
      if (!(await this._startFallbackDownload(unitId, unit))) {
        throw new Error('Service worker not available. Please refresh and try again.');
      }
      this._startProgressKeepalive(reg);
      this._logContext('downloadMode', 'direct');
      this._step('method', 'info', reg.backgroundFetch
        ? 'Direct download — the background download could not start; keep this tab open'
        : 'Direct download — this browser has no background download API (Safari/iPad, guest profiles); keep this tab open');
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
    this._forgetDownload(unitId); // the user removed it — stop retrying
    this._step('storage', 'info', 'Removed the downloaded files for ' + unitId);
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
    this._step('storage', 'info', 'Removing all downloaded units');
    // Abort first so a late backgroundfetchsuccess can't repopulate a cache,
    // and stop SW fallback loops so they can't broadcast a false 'cached'
    await this._abortAllBGFetches();
    this._cancelFallbackDownloads();
    this._forgetAllDownloads();

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
    this._step('storage', 'info', 'Resetting all local Mission HydroSci data');
    // Abort first so a late backgroundfetchsuccess can't repopulate a cache,
    // and stop SW fallback loops so they can't broadcast a false 'cached'
    await this._abortAllBGFetches();
    this._cancelFallbackDownloads();
    this._forgetAllDownloads();

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
   * Deletes cached data — and aborts stray in-flight downloads — for all
   * units NOT in the keepUnitIds array. The download check matters on its
   * own: Chrome persists paused Background Fetches across sessions, so a
   * unit downloaded long ago (even one whose cache is empty) can carry a
   * zombie "Downloading 0%" fetch that reconnect faithfully re-adopts on
   * every page load until something aborts it.
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
      if (status === 'cached' || status === 'partial' || this._activeDownloads[unit.id]) {
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

  /**
   * Whether the origin's storage is currently persisted (not evictable).
   * @returns {Promise<boolean|null>} null when the API is unavailable
   */
  MHSDeliveryManager.prototype.getStoragePersisted = async function() {
    if (navigator.storage && navigator.storage.persisted) {
      try { return await navigator.storage.persisted(); } catch (err) { return null; }
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
    var wasActive = !!this._activeDownloads[unitId];
    // Feed the stall watchdog from 'downloading' broadcasts. For fallback
    // loops any broadcast is a liveness signal (they fire at least once a
    // second while alive). For Background Fetch downloads the SW broadcasts
    // byte progress — strictly new bytes count as progress there, matching
    // the poller's monotonic/sticky rules.
    // Page-generated 'downloading' statuses (detail.local: waiting countdowns,
    // switch/resume notes) carry no new bytes and are NOT liveness signals.
    if (status === 'downloading' && !(detail && detail.local)) {
      var dlState = this._stallState[unitId];
      if (dlState) {
        var newBytes = detail && typeof detail.downloaded === 'number' &&
          detail.downloaded > dlState.maxDownloaded;
        if (newBytes) {
          dlState.maxDownloaded = detail.downloaded;
          dlState.lastProgressAt = Date.now();
          dlState.stalled = false;
          this._retryAttempts[unitId] = 0; // real progress resets the retry backoff
        } else if (dlState.fallback) {
          dlState.lastProgressAt = Date.now();
          dlState.stalled = false;
        }
      }
    }

    // Let pages show elapsed time beside the percentage.
    if (status === 'downloading' && detail && this._stallState[unitId] &&
        typeof detail.elapsedMs !== 'number') {
      detail.elapsedMs = Date.now() - this._stallState[unitId].startedAt;
    }

    // Step log (before the cleanup below, which drops the stall state's timings)
    this._logStatus(unitId, status, detail, wasActive);

    // Clear active download tracking and stall monitors on terminal statuses
    if (status === 'cached' || status === 'error' || status === 'not_cached') {
      delete this._activeDownloads[unitId];
      if (status === 'cached') this._forgetDownload(unitId); // done — no retries outstanding
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

    // Server-side telemetry for download failures (diagnostics gap MHS-008):
    // a raw cache/network error was previously only visible to the tester. All
    // error statuses funnel through here — SW broadcasts (via
    // _handleStatusUpdate) and page-side alike — so this is the one place to
    // report from, covering the launcher and the manage page.
    if (status === 'error') {
      this._reportDownloadError(unitId, detail || {});
    }

    // Never give up: a failed download that is still wanted gets its next
    // attempt scheduled (with a visible countdown) instead of a dead end.
    if (status === 'error' && this._wanted[unitId]) {
      var base = {};
      var src = detail || {};
      for (var key in src) base[key] = src[key];
      this._scheduleRetry(unitId, 'error', base);
    }
  };

  /**
   * Best-effort POST of a download failure to the server so failure prevalence
   * across devices is visible in the logs. Deduped to once per unit+errorClass
   * per manager lifetime so a retry loop can't flood. Skips silently when no
   * CSRF token was provided (the page didn't opt in) or on any network error.
   */
  MHSDeliveryManager.prototype._reportDownloadError = function(unitId, detail) {
    if (!this._csrfToken) return;
    var cls = detail.errorClass || 'generic';
    var key = unitId + '|' + cls;
    if (this._reportedErrors[key]) return;
    this._reportedErrors[key] = true;

    var deviceId = '';
    try { deviceId = localStorage.getItem('mhs-device-id') || ''; } catch (e) {}

    var self = this;
    function post(quota, usage) {
      var body = {
        device_id: deviceId,
        unit: unitId,
        version: detail.version || '',
        error_class: cls,
        message: String(detail.rawError || detail.error || '').slice(0, 500),
        path: detail.path || '',
        storage_quota: quota || 0,
        storage_usage: usage || 0,
        user_agent: (navigator && navigator.userAgent) || '',
        preflight: self._preflightSummary().slice(0, 300)
      };
      try {
        fetch(self._downloadErrorUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': self._csrfToken },
          body: JSON.stringify(body),
          keepalive: true
        }).catch(function() {});
      } catch (e) { /* best effort */ }
    }

    if (navigator.storage && navigator.storage.estimate) {
      navigator.storage.estimate()
        .then(function(est) { post(est.quota, est.usage); })
        .catch(function() { post(0, 0); });
    } else {
      post(0, 0);
    }
  };

  // Export globally
  window.MHSDeliveryManager = MHSDeliveryManager;
})();
