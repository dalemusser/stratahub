/**
 * mhs-steplog.js — the per-load step log for Mission HydroSci pages.
 *
 * One MHSStepLog per tab records what the units, manage and play pages do
 * while loading a unit — service worker, game list, storage, content server
 * and game-service checks, download method, download progress and retries,
 * verification, launch, and the game itself — as timestamped entries. The
 * delivery manager writes into it (setStepLog), pages render it through
 * MHSStepLog.mount (the "Status details" panel), and "Copy report" turns it
 * into plain text a user can paste into a message. The log is mirrored to
 * sessionStorage so it survives reloads and the units → play → units moves
 * of one tab. The only network use is opt-in: enableServerFlush streams a
 * device-test run's entries to its record, and relayTo/listenTo pass one
 * tab's entries to another over a BroadcastChannel (the device test's game
 * tab reporting back to its run page).
 */
(function() {
  'use strict';

  var MAX_ENTRIES = 300;
  var STORAGE_KEY = 'mhs-steplog';
  var RESTORE_MAX_AGE_MS = 6 * 60 * 60 * 1000; // a tab's log is kept across page moves for 6 h
  var MAX_DETAIL_KEYS = 20;
  var MAX_DETAIL_STRING = 300;

  // The steps shown as rows, in order. Other step ids may be recorded (they
  // appear in the full text log) but do not get a row.
  var STEPS = [
    { id: 'sw',       label: 'Service worker' },
    { id: 'manifest', label: 'Game list' },
    { id: 'storage',  label: 'Storage' },
    { id: 'cdn',      label: 'Content server' },
    { id: 'services', label: 'Game services' },
    { id: 'method',   label: 'Download method' },
    { id: 'download', label: 'Download' },
    { id: 'verify',   label: 'Verify files' },
    { id: 'launch',   label: 'Launch' },
    { id: 'game',     label: 'Game' }
  ];
  var STEP_LABELS = {};
  for (var si = 0; si < STEPS.length; si++) STEP_LABELS[STEPS[si].id] = STEPS[si].label;

  // Entry states: running (in progress), ok, warn, fail, info.
  var STATE_RANK = { info: 0, running: 1, ok: 2, warn: 3, fail: 4 };

  function MHSStepLog(opts) {
    opts = opts || {};
    this.context = {};
    this.entries = [];
    this.startedAt = Date.now();
    this._listeners = [];
    this._persist = opts.persist !== false;
    this._flushed = 0; // entries already sent to the server (see enableServerFlush)
    this._restore();
  }
  MHSStepLog.STEPS = STEPS;
  MHSStepLog.STEP_LABELS = STEP_LABELS;

  MHSStepLog.prototype._restore = function() {
    if (!this._persist) return;
    try {
      var raw = sessionStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      var saved = JSON.parse(raw);
      if (!saved || !Array.isArray(saved.entries)) return;
      if (Date.now() - (saved.startedAt || 0) > RESTORE_MAX_AGE_MS) return;
      this.startedAt = saved.startedAt;
      this.entries = saved.entries.slice(-MAX_ENTRIES);
      this.context = saved.context || {};
      this._flushed = Math.min(saved.flushed || 0, this.entries.length);
    } catch (e) { /* start fresh */ }
  };

  MHSStepLog.prototype._save = function() {
    if (!this._persist) return;
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
        startedAt: this.startedAt, context: this.context, entries: this.entries, flushed: this._flushed
      }));
    } catch (e) { /* private mode or quota — the in-memory log still works */ }
  };

  /**
   * Marks a page boundary and merges page context (page, host, workspace,
   * unit, version, unitTitle, collection, deviceId, deviceType, userAgent,
   * deliveryVersion, steplogVersion, swVersion, downloadMode, storage).
   * Empty values are ignored so a page never blanks what an earlier one set.
   */
  MHSStepLog.prototype.start = function(context) {
    var ctx = context || {};
    for (var k in ctx) {
      if (ctx[k] !== undefined && ctx[k] !== null && ctx[k] !== '') this.context[k] = ctx[k];
    }
    var what = 'Opened the ' + (ctx.page || 'page') + ' page';
    if (ctx.unit) what += ' for ' + ctx.unit + (ctx.version ? ' v' + ctx.version : '');
    this.record('page', 'info', what);
    return this;
  };

  MHSStepLog.prototype.set = function(key, value) {
    if (value === undefined || value === null || value === '') return;
    this.context[key] = value;
    this._save();
  };

  function compactDetail(detail) {
    var out = {};
    var n = 0;
    for (var k in detail) {
      if (n >= MAX_DETAIL_KEYS) break;
      var v = detail[k];
      if (typeof v === 'function' || v === undefined) continue;
      if (typeof v === 'string' && v.length > MAX_DETAIL_STRING) v = v.slice(0, MAX_DETAIL_STRING) + '…';
      if (typeof v === 'object' && v !== null) {
        try { v = JSON.stringify(v).slice(0, MAX_DETAIL_STRING); } catch (e) { v = String(v); }
      }
      // The server keeps detail as strings only; one number would make it
      // refuse the whole report.
      if (typeof v !== 'string') v = String(v);
      out[k] = v;
      n++;
    }
    return out;
  }

  /**
   * Records one entry. Consecutive identical entries (same step, state and
   * message) collapse into one with a repeat count, so a tight loop can't
   * flood the log.
   */
  MHSStepLog.prototype.record = function(stepId, state, msg, detail) {
    var entry = {
      t: Date.now() - this.startedAt,
      at: new Date().toISOString(),
      step: String(stepId || 'info'),
      state: STATE_RANK.hasOwnProperty(state) ? state : 'info',
      msg: String(msg || '')
    };
    if (detail && typeof detail === 'object') entry.detail = compactDetail(detail);
    return this._push(entry);
  };

  // Appends an entry (collapsing a repeat of the last one), trims the log,
  // saves and notifies. Shared by record() and the cross-tab relay.
  MHSStepLog.prototype._push = function(entry) {
    var last = this.entries[this.entries.length - 1];
    if (last && last.step === entry.step && last.state === entry.state && last.msg === entry.msg &&
        (last.from || '') === (entry.from || '')) {
      last.t = entry.t;
      last.at = entry.at;
      last.repeats = (last.repeats || 1) + 1;
      if (entry.detail) last.detail = entry.detail;
      entry = last;
    } else {
      this.entries.push(entry);
      if (this.entries.length > MAX_ENTRIES) {
        var removed = this.entries.length - MAX_ENTRIES;
        this.entries.splice(0, removed);
        this._flushed = Math.max(0, this._flushed - removed);
        if (this._acked) for (var k in this._acked) delete this._acked[k]; // indexes shifted; a resend is deduped server-side
      }
    }
    this._save();
    this._notify(entry);
    return entry;
  };

  // ---- Cross-tab relay ----------------------------------------------------
  // The device test opens the game in a second tab. That tab relays each
  // entry it records to the run page, whose panel then follows the launch
  // and the game live and whose Copy report includes them. Relayed entries
  // carry `from` (the sending tab's name) and are never flushed to the server
  // by the receiving page: the tab that made them already stores them.

  /** Channel name for a device-test run's relay. */
  MHSStepLog.relayChannel = function(testId) {
    return 'mhs-steplog-relay-' + String(testId || '');
  };

  /** Sends every entry this log records to `channelName`. Returns true when supported. */
  MHSStepLog.prototype.relayTo = function(channelName, from) {
    if (typeof BroadcastChannel === 'undefined' || !channelName) return false;
    var ch;
    try { ch = new BroadcastChannel(channelName); } catch (e) { return false; }
    var tag = String(from || 'other tab');
    this.onChange(function(entry) {
      if (!entry || entry.from) return; // never echo what was itself received
      try { ch.postMessage({ entry: entry, from: tag }); } catch (e) { /* closed channel */ }
    });
    return true;
  };

  /** Records entries arriving on `channelName` as if they were this tab's, tagged with their origin. */
  MHSStepLog.prototype.listenTo = function(channelName) {
    if (typeof BroadcastChannel === 'undefined' || !channelName) return false;
    var self = this, ch;
    try { ch = new BroadcastChannel(channelName); } catch (e) { return false; }
    ch.onmessage = function(ev) {
      var d = ev && ev.data;
      if (!d || !d.entry || typeof d.entry !== 'object') return;
      self._receive(d.entry, d.from);
    };
    return true;
  };

  // Stores a relayed entry on this log's timeline (t from its wall clock,
  // so the run page's panel shows when it happened relative to this page).
  MHSStepLog.prototype._receive = function(e, from) {
    var atMs = Date.parse(e.at);
    var entry = {
      t: isNaN(atMs) ? Date.now() - this.startedAt : Math.max(0, atMs - this.startedAt),
      at: typeof e.at === 'string' ? e.at : new Date().toISOString(),
      step: String(e.step || 'info'),
      state: STATE_RANK.hasOwnProperty(e.state) ? e.state : 'info',
      msg: String(e.msg || ''),
      from: String(from || 'other tab')
    };
    if (e.detail && typeof e.detail === 'object') entry.detail = compactDetail(e.detail);
    return this._push(entry);
  };

  MHSStepLog.prototype.latest = function(stepId) {
    for (var i = this.entries.length - 1; i >= 0; i--) {
      if (this.entries[i].step === stepId) return this.entries[i];
    }
    return null;
  };

  // The most recent entry overall (what is happening now).
  MHSStepLog.prototype.current = function() {
    return this.entries.length ? this.entries[this.entries.length - 1] : null;
  };

  MHSStepLog.prototype.onChange = function(cb) {
    if (typeof cb === 'function') this._listeners.push(cb);
  };

  MHSStepLog.prototype._notify = function(entry) {
    for (var i = 0; i < this._listeners.length; i++) {
      try { this._listeners[i](entry, this); } catch (e) { /* a broken listener must not break the log */ }
    }
  };

  // Forgets everything, context included — used when a tab moves between a
  // student's pages and a device-test run, which must not share a log.
  MHSStepLog.prototype.reset = function() {
    this.context = {};
    this.clear();
  };

  MHSStepLog.prototype.clear = function() {
    this.entries = [];
    this._flushed = 0;
    this.startedAt = Date.now();
    this._save();
    this._notify(null);
  };

  MHSStepLog.prototype.toJSON = function() {
    return { startedAt: new Date(this.startedAt).toISOString(), context: this.context, entries: this.entries };
  };

  function pad2(n) { return (n < 10 ? '0' : '') + n; }

  // "+m:ss.t" from milliseconds since the log started.
  function elapsed(ms) {
    var s = Math.max(0, ms) / 1000;
    var m = Math.floor(s / 60);
    var rest = s - m * 60;
    return '+' + m + ':' + (rest < 10 ? '0' : '') + rest.toFixed(1);
  }
  MHSStepLog.elapsed = elapsed;

  function detailText(detail) {
    if (!detail) return '';
    var parts = [];
    for (var k in detail) {
      var v = detail[k];
      if (v === '' || v === null || v === undefined) continue;
      parts.push(k + '=' + v);
    }
    return parts.length ? ' {' + parts.join(', ') + '}' : '';
  }

  /** Plain-text report: a context header, then one line per entry. */
  MHSStepLog.prototype.toText = function() {
    var c = this.context;
    var now = new Date();
    var lines = [];
    lines.push('Mission HydroSci status report');
    lines.push('Generated: ' + now.toISOString() + ' (local ' + now.toString() + ')');
    lines.push('Page: ' + (c.page || '?') + ' · Host: ' + (c.host || location.host) +
      (c.workspace ? ' · Workspace: ' + c.workspace : ''));
    if (c.unit || c.collection) {
      lines.push('Unit: ' + (c.unitTitle ? c.unitTitle + ' ' : '') + (c.unit || '?') + (c.version ? ' v' + c.version : '') +
        (c.collection ? ' · Collection: ' + c.collection : ''));
    }
    lines.push('Device: ' + (c.deviceId || 'unknown') + (c.deviceType ? ' · ' + c.deviceType : '') +
      (c.userAgent ? ' · ' + c.userAgent : ''));
    lines.push('App: delivery ' + (c.deliveryVersion || '?') + ' · steplog ' + (c.steplogVersion || '?') +
      ' · service worker ' + (c.swVersion || 'unknown') + (c.downloadMode ? ' · download mode: ' + c.downloadMode : ''));
    if (c.storage) lines.push('Storage: ' + c.storage);
    lines.push('Log started: ' + new Date(this.startedAt).toISOString());
    lines.push('Steps:');
    for (var i = 0; i < this.entries.length; i++) {
      var e = this.entries[i];
      lines.push('[' + elapsed(e.t) + '] ' + (STEP_LABELS[e.step] || e.step) + ' · ' + e.state + ' · ' + e.msg +
        (e.repeats > 1 ? ' (x' + e.repeats + ')' : '') + (e.from ? ' [' + e.from + ']' : '') + detailText(e.detail));
    }
    return lines.join('\n');
  };

  var FLUSH_MAX_ENTRIES = 40; // per request, so a final keepalive body stays under the 64 KB limit

  /**
   * Streams new entries to the server: POST {context, entries:[...]} to url
   * every intervalMs (default 5 s), whenever the tab is hidden, and on
   * pagehide (keepalive). The count already sent travels with the log in
   * sessionStorage, so a page move never resends history; a failed post is
   * simply retried on the next tick. Relayed entries (from another tab) are
   * skipped: the tab that made them stores them. Used by the device test.
   *
   * Hiding and then closing a tab fires two final flushes while the first
   * may still be in flight; a final flush therefore sends only what no
   * in-flight request already carries, and the server's confirmations are
   * joined in order, so no entry goes out twice.
   */
  MHSStepLog.prototype.enableServerFlush = function(url, csrfToken, intervalMs, opts) {
    var self = this;
    opts = opts || {};
    var token = typeof csrfToken === 'function' ? csrfToken : function() { return csrfToken || ''; };
    var inflight = 0;        // requests in flight
    var inflightThrough = 0; // index just past the last entry any in-flight request carries
    var acked = {};          // start index -> end index of ranges the server has confirmed
    self._acked = acked;     // _push empties it when old entries are dropped
    function advance() {
      var f = self._flushed;
      while (acked.hasOwnProperty(f)) { var next = acked[f]; delete acked[f]; f = next; }
      if (f > self._flushed) { self._flushed = f; self._save(); }
    }
    function flush(final) {
      if (inflight && !final) return Promise.resolve(false);
      var start = inflight ? Math.max(self._flushed, inflightThrough) : self._flushed;
      var through = Math.min(self.entries.length, start + FLUSH_MAX_ENTRIES);
      for (var k in acked) { var ks = +k; if (ks > start && ks < through) through = ks; } // stop short of a confirmed range
      if (through <= start) return Promise.resolve(true);
      var pending = [];
      for (var i = start; i < through; i++) if (!self.entries[i].from) pending.push(self.entries[i]);
      if (!pending.length) { acked[start] = through; advance(); return Promise.resolve(true); }
      inflight++;
      inflightThrough = Math.max(inflightThrough, through);
      var init = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': token() },
        body: JSON.stringify({ context: self.context, entries: pending }),
        keepalive: !!final
      };
      var p;
      try { p = fetch(url, init); } catch (e) { p = Promise.reject(e); }
      return p.then(function(resp) {
        if (resp && resp.ok) { acked[start] = through; advance(); return true; }
        // A stale CSRF token (the cookie was re-issued under the page) is
        // refused with 403; let the page fetch a fresh one. The entries
        // stay unacknowledged and go out on the next tick with it.
        if (resp && resp.status === 403 && typeof opts.onForbidden === 'function') {
          try { opts.onForbidden(); } catch (e) { /* ignore */ }
        }
        return false;
      }).catch(function() { return false; }).then(function(ok) {
        inflight--;
        if (!inflight) inflightThrough = self._flushed;
        return ok;
      });
    }
    this.flush = function(final) { return flush(!!final); };
    if (this._flushTimer) clearInterval(this._flushTimer);
    this._flushTimer = setInterval(function() { flush(false); }, intervalMs || 5000);
    window.addEventListener('pagehide', function() { flush(true); });
    document.addEventListener('visibilitychange', function() {
      if (document.visibilityState === 'hidden') flush(true);
    });
    flush(false);
  };

  // ---- CSRF token keeper ---------------------------------------------------
  // A page's CSRF token is minted at load time against a cookie the server
  // may re-issue later (it expires after some hours, or a sign-out clears
  // it). A device-test page can be open long past that, so its posts need a
  // token that can be renewed: MHSStepLog.csrf() keeps the current token,
  // renews it by re-reading the page's own HTML, and retries a refused post
  // once with the new one.
  // The token in the page is HTML-escaped ('+' becomes '&#43;'), so it is
  // read through an HTML parser, never straight out of the text.
  function tokenFromHTML(html) {
    if (!html) return '';
    try {
      if (typeof DOMParser !== 'undefined') {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        var meta = doc.querySelector('meta[name="csrf-token"]');
        if (meta && meta.content) return meta.content;
        var input = doc.querySelector('input[name="csrf_token"]');
        if (input && input.value) return input.value;
      }
    } catch (e) { /* fall through */ }
    var m = /name="csrf-token"\s+content="([^"]+)"/.exec(html) || /name="csrf_token"\s+value="([^"]+)"/.exec(html);
    if (!m) return '';
    return m[1].replace(/&#(\d+);/g, function(_, n) { return String.fromCharCode(parseInt(n, 10)); })
      .replace(/&#x([0-9a-f]+);/gi, function(_, h) { return String.fromCharCode(parseInt(h, 16)); })
      .replace(/&amp;/g, '&').replace(/&quot;/g, '"').replace(/&lt;/g, '<').replace(/&gt;/g, '>');
  }

  MHSStepLog.csrf = function(initialToken, pageUrl) {
    var current = initialToken || '';
    var refreshing = null;
    var lastRefresh = 0;
    function apply(t) {
      current = t;
      try {
        var meta = document.querySelector('meta[name="csrf-token"]');
        if (meta) meta.content = t;
        var inputs = document.querySelectorAll('input[name="csrf_token"]');
        for (var i = 0; i < inputs.length; i++) inputs[i].value = t;
      } catch (e) { /* ignore */ }
    }
    function refresh() {
      if (refreshing) return refreshing;
      refreshing = fetch(pageUrl || window.location.href, { credentials: 'same-origin', cache: 'no-store', headers: { 'Accept': 'text/html' } })
        .then(function(r) { return r.ok ? r.text() : ''; })
        .then(function(html) {
          var t = tokenFromHTML(html);
          if (t) { apply(t); lastRefresh = Date.now(); return true; }
          return false;
        })
        .catch(function() { return false; })
        .then(function(ok) { refreshing = null; return ok; });
      return refreshing;
    }
    // fetch() with the current token; on 403, renew it and try once more.
    function send(url, init) {
      init = init || {};
      function go() {
        var headers = {};
        for (var k in (init.headers || {})) headers[k] = init.headers[k];
        headers['X-CSRF-Token'] = current;
        var i2 = {};
        for (var k2 in init) i2[k2] = init[k2];
        i2.headers = headers;
        if (!i2.credentials) i2.credentials = 'same-origin';
        return fetch(url, i2);
      }
      return go().then(function(resp) {
        if (resp.status !== 403) return resp;
        return refresh().then(function(ok) { return ok ? go() : resp; });
      });
    }
    return {
      get: function() { return current; },
      refresh: refresh,
      // Renew when the token is older than maxAgeMs (used on tab return).
      refreshIfStale: function(maxAgeMs) {
        if (Date.now() - (lastRefresh || pageLoadedAt) < (maxAgeMs || 0)) return Promise.resolve(true);
        return refresh();
      },
      fetch: send
    };
  };
  var pageLoadedAt = Date.now();

  /** Copies text to the clipboard; resolves true on success. */
  MHSStepLog.copyText = function(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(function() { return true; }, function() { return legacyCopy(text); });
    }
    return Promise.resolve(legacyCopy(text));
  };
  function legacyCopy(text) {
    try {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.position = 'fixed';
      ta.style.top = '-1000px';
      document.body.appendChild(ta);
      ta.select();
      var ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return !!ok;
    } catch (e) {
      return false;
    }
  }

  // ---- Panel rendering ---------------------------------------------------

  var STATE_ICON = { running: '●', ok: '✓', warn: '!', fail: '✕', info: '·', pending: '–' };
  var STATE_CLASS = {
    running: 'text-blue-600 dark:text-blue-400',
    ok: 'text-green-600 dark:text-green-400',
    warn: 'text-amber-600 dark:text-amber-400',
    fail: 'text-red-600 dark:text-red-400',
    info: 'text-gray-500 dark:text-gray-400',
    pending: 'text-gray-400 dark:text-gray-500'
  };

  /**
   * Binds a log to the shared "Status details" panel (template
   * mhs_steplog_panel). Rows are rebuilt on every change; the panel opens
   * itself the first time something goes wrong, or immediately with
   * opts.open. Returns { open(), close(), refresh() }.
   */
  MHSStepLog.mount = function(log, root, opts) {
    opts = opts || {};
    if (!root) return null;
    var q = function(suffix) { return root.querySelector('[data-steplog="' + suffix + '"]'); };
    var toggle = q('toggle'), body = q('body'), rows = q('rows'), live = q('live'), summary = q('summary');
    var chevron = q('chevron'), copyBtn = q('copy'), copied = q('copied'), full = q('full'), fullText = q('text');
    var reportWrap = q('report-wrap'), noteEl = q('note'), sendBtn = q('send'), sentEl = q('sent');
    var rowTpl = root.querySelector('template[data-steplog="row"]');
    var isOpen = false;
    var autoOpened = false;

    function setOpen(open) {
      isOpen = !!open;
      if (body) body.classList.toggle('hidden', !isOpen);
      if (toggle) toggle.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
      if (chevron) chevron.textContent = isOpen ? '▾' : '▸';
    }

    function rowFor(step, entry) {
      var state = entry ? entry.state : 'pending';
      var node;
      if (rowTpl && rowTpl.content) {
        node = rowTpl.content.firstElementChild.cloneNode(true);
      } else {
        node = document.createElement('li');
        node.innerHTML = '<span data-steplog="icon"></span> <span data-steplog="label"></span> <span data-steplog="msg"></span> <span data-steplog="time"></span>';
      }
      var icon = node.querySelector('[data-steplog="icon"]');
      var label = node.querySelector('[data-steplog="label"]');
      var msg = node.querySelector('[data-steplog="msg"]');
      var time = node.querySelector('[data-steplog="time"]');
      if (icon) { icon.textContent = STATE_ICON[state] || STATE_ICON.info; icon.className = 'inline-block w-4 text-center font-bold ' + (STATE_CLASS[state] || STATE_CLASS.info); }
      if (label) label.textContent = step.label;
      if (msg) { msg.textContent = entry ? entry.msg : '—'; msg.className = 'ml-1 ' + (entry ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400 dark:text-gray-500'); }
      if (time) time.textContent = entry ? elapsed(entry.t) + (entry.from ? ' · ' + entry.from : '') : '';
      return node;
    }

    function refresh() {
      if (rows) {
        rows.innerHTML = '';
        for (var i = 0; i < STEPS.length; i++) {
          rows.appendChild(rowFor(STEPS[i], log.latest(STEPS[i].id)));
        }
      }
      var cur = log.current();
      if (live) {
        live.textContent = cur ? ('Now: ' + (STEP_LABELS[cur.step] || cur.step) + ' — ' + cur.msg + ' (' + elapsed(cur.t) + ')') : '';
      }
      if (summary) {
        var warnings = 0, failures = 0;
        for (var s = 0; s < STEPS.length; s++) {
          var e = log.latest(STEPS[s].id);
          if (e && e.state === 'warn') warnings++;
          if (e && e.state === 'fail') failures++;
        }
        var text = cur ? cur.msg : '';
        if (text.length > 70) text = text.slice(0, 67) + '…';
        var flags = [];
        if (failures) flags.push(failures + ' problem' + (failures > 1 ? 's' : ''));
        if (warnings) flags.push(warnings + ' warning' + (warnings > 1 ? 's' : ''));
        summary.textContent = (flags.length ? flags.join(', ') + ' · ' : '') + text;
      }
      if (fullText && full && full.open) fullText.textContent = log.toText();
      if (!autoOpened && !isOpen && cur && (cur.state === 'fail' || cur.state === 'warn')) {
        autoOpened = true;
        setOpen(true);
      }
    }

    if (toggle) toggle.addEventListener('click', function() { setOpen(!isOpen); });
    if (full) full.addEventListener('toggle', function() { if (full.open && fullText) fullText.textContent = log.toText(); });
    if (copyBtn) {
      copyBtn.addEventListener('click', function() {
        MHSStepLog.copyText(log.toText()).then(function(ok) {
          if (!copied) return;
          copied.textContent = ok ? 'Copied' : 'Could not copy — open "Full log" and select the text';
          copied.classList.remove('hidden');
          setTimeout(function() { copied.classList.add('hidden'); }, 4000);
        });
      });
    }
    // Optional "Send report": the page supplies a function(note) returning a
    // promise; the log itself travels with it (see the page's callback).
    if (typeof opts.report === 'function' && reportWrap && sendBtn) {
      reportWrap.classList.remove('hidden');
      sendBtn.addEventListener('click', function() {
        var note = ((noteEl && noteEl.value) || '').trim();
        if (!note) { if (sentEl) sentEl.textContent = 'Type a few words first.'; return; }
        sendBtn.disabled = true;
        log.record('page', 'info', 'Report sent: ' + note.slice(0, 200));
        Promise.resolve(opts.report(note)).then(function(ok) {
          if (sentEl) sentEl.textContent = ok === false ? 'Could not send right now. Use Copy report instead.' : 'Sent — thank you.';
          if (ok !== false && noteEl) noteEl.value = '';
        }).catch(function() {
          if (sentEl) sentEl.textContent = 'Could not send right now. Use Copy report instead.';
        }).then(function() { sendBtn.disabled = false; });
      });
    }
    log.onChange(function() { refresh(); });
    setOpen(!!opts.open);
    refresh();
    return { open: function() { setOpen(true); }, close: function() { setOpen(false); }, refresh: refresh };
  };

  // --- The game's PlayerPrefs store -------------------------------------
  // Unity keeps the game's PlayerPrefs (where the game caches gameplay logs
  // it has not managed to send) as one file inside the IndexedDB database
  // it mounts at /idbfs, under a path ending in /PlayerPrefs, and caps the
  // store at 1 MB on WebGL. A store near the cap has been collecting unsent
  // logs for a long time: the game's sender has not been keeping up. See
  // docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md §4.
  MHSStepLog.UNITY_PREFS_CAP = 1048576;

  // Resolves the store's size in bytes: 0 when the game has never run here,
  // null when it cannot be read (no IndexedDB, an older browser without
  // indexedDB.databases(), a read that fails or takes too long). Read-only,
  // and never creates the database: the game must be the one to do that.
  MHSStepLog.readUnityPrefsBytes = function() {
    return new Promise(function(resolve) {
      var done = false;
      function finish(v) { if (!done) { done = true; resolve(v); } }
      try {
        if (!window.indexedDB || typeof indexedDB.databases !== 'function') return finish(null);
        setTimeout(function() { finish(null); }, 4000);
        indexedDB.databases().then(function(list) {
          var present = false;
          for (var i = 0; i < (list || []).length; i++) if (list[i] && list[i].name === '/idbfs') present = true;
          if (!present) return finish(0);
          var req = indexedDB.open('/idbfs');
          req.onerror = function() { finish(null); };
          req.onblocked = function() { finish(null); };
          req.onupgradeneeded = function() { try { req.transaction.abort(); } catch (e) {} finish(null); };
          req.onsuccess = function() {
            var db = req.result;
            function close() { try { db.close(); } catch (e) {} }
            try {
              if (!db.objectStoreNames.contains('FILE_DATA')) { close(); return finish(0); }
              var store = db.transaction('FILE_DATA', 'readonly').objectStore('FILE_DATA');
              // Keys first (paths), then only the PlayerPrefs entries' values, so
              // the other files the game may keep here are never loaded.
              var keys = [];
              var kc = store.openKeyCursor();
              kc.onerror = function() { close(); finish(null); };
              kc.onsuccess = function(ev) {
                var c = ev.target.result;
                if (c) {
                  var key = String(c.key || '');
                  if (key.length >= 12 && key.slice(-12) === '/PlayerPrefs') keys.push(c.key);
                  c.continue();
                  return;
                }
                if (!keys.length) { close(); return finish(0); }
                var total = 0, pending = keys.length;
                keys.forEach(function(k) {
                  var g = store.get(k);
                  g.onerror = function() { if (--pending === 0) { close(); finish(total); } };
                  g.onsuccess = function() {
                    var v = g.result || {}, contents = v.contents;
                    if (contents && typeof contents.byteLength === 'number') total += contents.byteLength;
                    else if (contents && typeof contents.length === 'number') total += contents.length;
                    if (--pending === 0) { close(); finish(total); }
                  };
                });
              };
            } catch (e) { close(); finish(null); }
          };
        }).catch(function() { finish(null); });
      } catch (e) { finish(null); }
    });
  };

  // "812 KB of 1,024 KB (79%)" for a step-log line or a report.
  MHSStepLog.unityPrefsText = function(bytes) {
    var cap = MHSStepLog.UNITY_PREFS_CAP;
    var pct = Math.round(bytes * 100 / cap);
    return Math.round(bytes / 1024) + ' KB of ' + (cap / 1024).toLocaleString() + ' KB (' + pct + '%)';
  };

  // --- Unblocking the game's saved log queue ------------------------------
  // The schools' build (v2.8.1) empties an unsent log entry in place when a
  // send has failed twice, then reads an empty entry at the head of its queue
  // as "nothing to send". It sends nothing more on that device, and since it
  // saves the queue in its PlayerPrefs store with the empty entry still
  // first, every later launch is stuck the same way. Removing the empty
  // entries before the game starts lets it send the whole saved queue on
  // that launch (verified on v2.8.1, 2026-09-28). See
  // docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md §0.
  MHSStepLog.UNITY_LOG_CACHE_KEY = 'game_logs_cache.json';

  // Unity's WebGL PlayerPrefs file: "UnityPrf" and 8 more header bytes, then
  // one record per key: the key as a length-prefixed string, a type byte,
  // the value. 0xFD is a 4-byte float and 0xFE a 4-byte int; any other type
  // byte starts a string: below 0x80 it is the byte length itself, 0x80 is
  // followed by a 4-byte little-endian length. Anything else is unknown.
  function prefsLen(u8, p) {
    if (p >= u8.length) return null;
    var b = u8[p];
    if (b < 0x80) return [b, p + 1];
    if (b === 0x80 && p + 5 <= u8.length) {
      return [(u8[p + 1] | (u8[p + 2] << 8) | (u8[p + 3] << 16) | (u8[p + 4] << 24)) >>> 0, p + 5];
    }
    return null;
  }

  // The raw text of each top-level element of the JSON array whose '[' is at
  // text[open], and the index of its ']'; null if the array does not close.
  function jsonArrayElements(text, open) {
    var els = [], depth = 0, inStr = false, esc = false, start = -1;
    for (var i = open; i < text.length; i++) {
      var ch = text.charAt(i);
      if (inStr) {
        if (esc) esc = false;
        else if (ch === '\\') esc = true;
        else if (ch === '"') inStr = false;
        continue;
      }
      if (depth === 1 && start < 0 && !/\s/.test(ch) && ch !== ',' && ch !== ']') start = i;
      if (ch === '"') { inStr = true; continue; }
      if (ch === '[' || ch === '{') { depth++; continue; }
      if (ch === ']' || ch === '}') {
        depth--;
        if (depth === 0) {
          if (start >= 0) els.push(text.slice(start, i).trim());
          return { els: els, close: i };
        }
        continue;
      }
      if (depth === 1 && ch === ',') {
        if (start >= 0) els.push(text.slice(start, i).trim());
        start = -1;
      }
    }
    return null;
  }

  function isLogEntry(v) {
    return !!v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length > 0;
  }

  // Drops the empty (or unreadable) entries from the cache value. Both cache
  // formats: the schools' build writes {"logs":["<entry json>", …]}, the
  // newer logger a plain array of entries. Kept entries stay byte for byte.
  // Returns { text, removed, queued }, or null when the value is not one of
  // the two formats.
  function repairLogCacheText(text) {
    var doc = JSON.parse(text);
    var legacy = !!doc && !Array.isArray(doc) && typeof doc === 'object' && Array.isArray(doc.logs);
    if (!legacy && !Array.isArray(doc)) return null;
    var m = legacy ? /^\s*\{\s*"logs"\s*:\s*\[/.exec(text) : /^\s*\[/.exec(text);
    if (!m) return null;
    var open = m.index + m[0].length - 1;
    var scan = jsonArrayElements(text, open);
    var total = legacy ? doc.logs.length : doc.length;
    if (!scan || scan.els.length !== total) return null;
    var kept = [];
    for (var i = 0; i < scan.els.length; i++) {
      var v = JSON.parse(scan.els[i]);
      if (legacy) {
        try { v = JSON.parse(v); } catch (e) { v = null; }
      }
      if (isLogEntry(v)) kept.push(scan.els[i]);
    }
    var out = text.slice(0, open + 1) + kept.join(',') + text.slice(scan.close);
    var check = JSON.parse(out);
    if ((legacy ? check.logs.length : check.length) !== kept.length) return null;
    return { text: out, removed: total - kept.length, queued: kept.length };
  }

  // Pure: takes a PlayerPrefs file's bytes. Returns null when the file is not
  // laid out as expected (it must be left alone), else
  // { removed, queued, bytes } where bytes is the repaired file, or null when
  // nothing needed removing. Every record other than the log cache is copied
  // unchanged.
  MHSStepLog.repairUnityPrefsBytes = function(u8) {
    try {
      if (!u8 || u8.length < 16 || String.fromCharCode.apply(null, u8.subarray(0, 8)) !== 'UnityPrf') return null;
      var dec = new TextDecoder('utf-8', { fatal: true });
      var p = 16, cache = null;
      // Walk the whole file first: an unexpected layout anywhere means the
      // lengths cannot be trusted, and nothing is written.
      while (p < u8.length) {
        var kl = prefsLen(u8, p);
        if (!kl) return null;
        var kEnd = kl[1] + kl[0];
        if (kEnd >= u8.length) return null;
        var t = u8[kEnd], vStart, vEnd, str = !(t === 0xFD || t === 0xFE);
        if (str) {
          var vl = prefsLen(u8, kEnd);
          if (!vl) return null;
          vStart = vl[1]; vEnd = vStart + vl[0];
        } else {
          vStart = kEnd + 1; vEnd = vStart + 4;
        }
        if (vEnd > u8.length) return null;
        if (dec.decode(u8.subarray(kl[1], kEnd)) === MHSStepLog.UNITY_LOG_CACHE_KEY) {
          if (!str) return null;
          cache = { typeAt: kEnd, start: vStart, end: vEnd };
        }
        p = vEnd;
      }
      if (!cache) return { removed: 0, queued: 0, bytes: null };
      var fixed = repairLogCacheText(dec.decode(u8.subarray(cache.start, cache.end)));
      if (!fixed) return null;
      if (!fixed.removed) return { removed: 0, queued: fixed.queued, bytes: null };
      var vb = new TextEncoder().encode(fixed.text), n = vb.length;
      var lenBytes = n < 0x80 ? [n] : [0x80, n & 255, (n >>> 8) & 255, (n >>> 16) & 255, (n >>> 24) & 255];
      var out = new Uint8Array(cache.typeAt + lenBytes.length + n + (u8.length - cache.end));
      out.set(u8.subarray(0, cache.typeAt), 0);
      out.set(lenBytes, cache.typeAt);
      out.set(vb, cache.typeAt + lenBytes.length);
      out.set(u8.subarray(cache.end), cache.typeAt + lenBytes.length + n);
      return { removed: fixed.removed, queued: fixed.queued, bytes: out };
    } catch (e) {
      return null;
    }
  };

  // Repairs every PlayerPrefs store of this site in place. Call before the
  // Unity loader starts, so the game loads the repaired store. Never creates
  // the database. Resolves (never rejects) with { state, stores, removed,
  // queued, unreadable }: state 'repaired', 'clean', 'absent' (the game has
  // never saved anything here), or 'unsupported' | 'timeout' | 'error' (not
  // checked; the game starts as usual).
  MHSStepLog.repairUnityLogCache = function(timeoutMs) {
    return new Promise(function(resolve) {
      var done = false;
      function finish(v) { if (!done) { done = true; resolve(v); } }
      try {
        if (!window.indexedDB || typeof indexedDB.databases !== 'function' ||
            typeof TextDecoder !== 'function' || typeof TextEncoder !== 'function') return finish({ state: 'unsupported' });
        setTimeout(function() { finish({ state: 'timeout' }); }, timeoutMs || 3000);
        indexedDB.databases().then(function(list) {
          var present = false;
          for (var i = 0; i < (list || []).length; i++) if (list[i] && list[i].name === '/idbfs') present = true;
          if (!present) return finish({ state: 'absent' });
          var req = indexedDB.open('/idbfs');
          req.onerror = function() { finish({ state: 'error' }); };
          req.onblocked = function() { finish({ state: 'error' }); };
          req.onupgradeneeded = function() { try { req.transaction.abort(); } catch (e) {} finish({ state: 'error' }); };
          req.onsuccess = function() {
            var db = req.result;
            function close() { try { db.close(); } catch (e) {} }
            try {
              if (!db.objectStoreNames.contains('FILE_DATA')) { close(); return finish({ state: 'absent' }); }
              // Keys first, so the other files the game may keep here are
              // never loaded.
              var keys = [];
              var kc = db.transaction('FILE_DATA', 'readonly').objectStore('FILE_DATA').openKeyCursor();
              kc.onerror = function() { close(); finish({ state: 'error' }); };
              kc.onsuccess = function(ev) {
                var c = ev.target.result;
                if (c) {
                  var key = String(c.key || '');
                  if (key.length >= 12 && key.slice(-12) === '/PlayerPrefs') keys.push(c.key);
                  c.continue();
                  return;
                }
                if (!keys.length) { close(); return finish({ state: 'absent' }); }
                var res = { state: 'clean', stores: keys.length, removed: 0, queued: 0, unreadable: 0 };
                // Read and write in one read-write transaction: IndexedDB runs
                // it apart from the game's own writes, so a store read here is
                // never written back over newer contents.
                var tx = db.transaction('FILE_DATA', 'readwrite');
                var store = tx.objectStore('FILE_DATA');
                keys.forEach(function(k) {
                  var g = store.get(k);
                  g.onsuccess = function() {
                    var v = g.result || {}, contents = v.contents;
                    var u8 = contents instanceof Uint8Array ? contents : (contents ? new Uint8Array(contents) : null);
                    var r = u8 ? MHSStepLog.repairUnityPrefsBytes(u8) : null;
                    if (!r) { res.unreadable++; return; }
                    res.queued += r.queued;
                    if (!r.bytes) return;
                    // Same timestamp and mode: the game loads every stored file
                    // at startup, and a game already running elsewhere keeps
                    // winning with its own later writes.
                    var nv = {};
                    for (var f in v) if (Object.prototype.hasOwnProperty.call(v, f)) nv[f] = v[f];
                    nv.contents = r.bytes;
                    store.put(nv, k);
                    res.removed += r.removed;
                  };
                });
                tx.oncomplete = function() {
                  close();
                  if (res.removed > 0) res.state = 'repaired';
                  finish(res);
                };
                tx.onerror = tx.onabort = function() { close(); finish({ state: 'error' }); };
              };
            } catch (e) { close(); finish({ state: 'error' }); }
          };
        }).catch(function() { finish({ state: 'error' }); });
      } catch (e) { finish({ state: 'error' }); }
    });
  };

  // What one of StrataHub's answers says about the page's sign-in. status is
  // the HTTP status of one of the play page's own posts (a heartbeat, the
  // launch record). A 404 for the launch's own record does not say why, so
  // the page then asks /api/user and passes its answer as who; pageUserId is
  // the user the page was rendered for.
  //   'ok'          the server accepted the post
  //   'signed-out'  the sign-in has ended (401, or /api/user says so)
  //   'other-user'  somebody else is signed in on this device now
  //   ''            nothing can be concluded (leave the state as it is)
  MHSStepLog.signInState = function(status, who, pageUserId) {
    if (status === 401) return 'signed-out';
    if (status >= 200 && status < 300) return 'ok';
    if (status === 404 && who) {
      if (!who.isAuthenticated) return 'signed-out';
      if (who.user_id && pageUserId && who.user_id !== pageUserId) return 'other-user';
    }
    return '';
  };

  window.MHSStepLog = MHSStepLog;
})();
