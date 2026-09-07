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
 * of one tab. Nothing here talks to the network.
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
    } catch (e) { /* start fresh */ }
  };

  MHSStepLog.prototype._save = function() {
    if (!this._persist) return;
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
        startedAt: this.startedAt, context: this.context, entries: this.entries
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
    var last = this.entries[this.entries.length - 1];
    if (last && last.step === entry.step && last.state === entry.state && last.msg === entry.msg) {
      last.t = entry.t;
      last.at = entry.at;
      last.repeats = (last.repeats || 1) + 1;
      if (entry.detail) last.detail = entry.detail;
      entry = last;
    } else {
      this.entries.push(entry);
      if (this.entries.length > MAX_ENTRIES) this.entries.splice(0, this.entries.length - MAX_ENTRIES);
    }
    this._save();
    this._notify(entry);
    return entry;
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

  MHSStepLog.prototype.clear = function() {
    this.entries = [];
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
        (e.repeats > 1 ? ' (x' + e.repeats + ')' : '') + detailText(e.detail));
    }
    return lines.join('\n');
  };

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
      if (time) time.textContent = entry ? elapsed(entry.t) : '';
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
    log.onChange(function() { refresh(); });
    setOpen(!!opts.open);
    refresh();
    return { open: function() { setOpen(true); }, close: function() { setOpen(false); }, refresh: refresh };
  };

  window.MHSStepLog = MHSStepLog;
})();
