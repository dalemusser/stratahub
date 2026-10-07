// The service worker's direct download on a network that keeps cutting the
// connection (internal/app/features/missionhydrosci/static/sw-background-fetch.js).
//
// West County, October 2026: the school's network path terminated every HTTPS
// transfer of the 132 MB unit file after a few seconds — 50–70 MB on the first
// connection, then 8–16 MB per resumed one. Each drop was resumed from the
// saved parts, but a fixed budget of five attempts per file failed the unit at
// 85%. The budget now counts only attempts that saved nothing new, so a
// converging download is never failed, while a dead link still gives up.
//
// Run from the repo root: node --test tests/js/*.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const staticDir = path.join(here, '../../internal/app/features/missionhydrosci/static');
const source = ['sw-cache.js', 'sw-background-fetch.js']
  .map((f) => fs.readFileSync(path.join(staticDir, f), 'utf8')).join('\n');

const ORIGIN = 'https://mhs.example';
const MB = 1024 * 1024;

// The Cache API, enough of it: entries keyed by URL, bodies read to the end
// on put (a body stream that errors makes put fail the way Chrome's does).
class FakeCache {
  constructor() { this.store = new Map(); }
  static url(key) { return typeof key === 'string' ? new URL(key, ORIGIN).href : key.url; }
  async match(key) {
    const e = this.store.get(FakeCache.url(key));
    return e ? new Response(e.body.slice(), { status: 200, headers: e.headers }) : undefined;
  }
  async put(key, response) {
    let buf;
    try { buf = new Uint8Array(await response.arrayBuffer()); } catch (e) {
      throw new TypeError("Failed to execute 'put' on 'Cache': Cache.put() encountered a network error");
    }
    this.store.set(FakeCache.url(key), { body: buf, headers: Object.fromEntries(response.headers) });
  }
  async delete(key) { return this.store.delete(FakeCache.url(key)); }
  async keys() { return [...this.store.keys()].map((u) => ({ url: u })); }
}

// A content server whose connections are cut after `cutAfter(connection)`
// bytes (Infinity = healthy). Honours Range; records every request.
function makeServer(fileBytes, cutAfter) {
  const file = new Uint8Array(fileBytes);
  for (let i = 0; i < file.length; i += 4096) file[i] = i & 255;
  const requests = [];
  const etag = '"abc"';
  const fetch = async (url, init) => {
    const n = requests.length;
    const range = init && init.headers && init.headers.Range;
    let offset = 0;
    if (range) offset = parseInt(range.replace('bytes=', ''), 10) || 0;
    requests.push({ url, offset, headers: init && init.headers });
    const limit = cutAfter(n);
    if (limit === 'refuse') throw new TypeError('Failed to fetch');
    const chunk = 256 * 1024;
    let sent = 0;
    const body = new ReadableStream({
      pull(controller) {
        if (offset + sent >= file.length) { controller.close(); return; }
        if (sent >= limit) { controller.error(new TypeError('network error')); return; }
        const end = Math.min(file.length, offset + sent + chunk, offset + limit);
        controller.enqueue(file.slice(offset + sent, end));
        sent = end - offset;
      }
    });
    const headers = { 'content-type': 'application/octet-stream', 'content-length': String(file.length - offset), etag };
    return new Response(body, { status: offset > 0 ? 206 : 200, headers });
  };
  return { file, requests, fetch };
}

function load(server) {
  const broadcasts = [];
  const sandbox = {
    self: { location: { origin: ORIGIN }, registration: {}, addEventListener() {} },
    BroadcastChannel: class { postMessage(m) { broadcasts.push(m); } },
    caches: { open: async () => sandbox.__cache, keys: async () => [] },
    fetch: server.fetch,
    Request, Response, Headers, ReadableStream, TransformStream, DOMException, AbortController, URL, Uint8Array, Set, Map, Promise, JSON, Math, Date, String,
    console: { log() {}, warn() {}, error() {} },
    // No real waiting in tests: every backoff sleep ends on the next tick.
    setTimeout: (fn) => setImmediate(fn),
    clearTimeout: () => {},
    setImmediate,
  };
  sandbox.__cache = new FakeCache();
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox);
  return { sandbox, broadcasts, cache: sandbox.__cache };
}

const KEY = '/missionhydrosci/content/unit1/v1.0.0/Build/unit1.data.unityweb';
const URL_ = 'https://cdn.example/mhs/unit1/v1.0.0/Build/unit1.data.unityweb';

test('a transfer cut every 3 MB still completes a 40 MB file', async () => {
  const server = makeServer(40 * MB, () => 3 * MB);
  const { sandbox, cache } = load(server);
  const drops = [];
  await sandbox.fetchAndCacheFileWithRetry(cache, KEY, URL_, 40 * MB, () => {}, undefined, () => {}, (d) => drops.push(d));
  const saved = await cache.match(KEY);
  assert.ok(saved, 'the file is in the cache');
  assert.deepEqual(new Uint8Array(await saved.arrayBuffer()), server.file, 'byte-identical after many resumes');
  assert.ok(drops.length >= 10, `many drops were resumed (${drops.length})`);
  assert.ok(server.requests.length > 5, 'far more than the old five attempts were made');
  const parts = (await cache.keys()).filter((k) => k.url.includes('?part'));
  assert.equal(parts.length, 0, 'resume parts are dropped after success');
});

test('a link that stops delivering gives up after six futile attempts', async () => {
  // First connection delivers 10 MB, every later one is refused outright.
  const server = makeServer(40 * MB, (n) => (n === 0 ? 10 * MB : 'refuse'));
  const { sandbox, cache } = load(server);
  const drops = [];
  await assert.rejects(
    sandbox.fetchAndCacheFileWithRetry(cache, KEY, URL_, 40 * MB, () => {}, undefined, () => {}, (d) => drops.push(d)),
    /Failed to fetch/
  );
  // 1 productive attempt + MAX_FUTILE_ATTEMPTS refused ones; onDrop hears of
  // every retried failure (the last one is not retried).
  assert.equal(server.requests.length, 1 + sandbox.MAX_FUTILE_ATTEMPTS);
  assert.equal(drops.length, sandbox.MAX_FUTILE_ATTEMPTS);
  assert.equal(drops[0].futile, 0, 'the first failure had saved its 10 MB as parts');
  assert.equal(drops[drops.length - 1].futile, sandbox.MAX_FUTILE_ATTEMPTS - 1);
  const parts = (await cache.keys()).filter((k) => k.url.includes('?part='));
  assert.equal(parts.length, 2, 'an 8 MB part and the 2 MB flushed when the connection broke survive for the next attempt');
  const meta = await (await cache.match(KEY + '?parts')).json();
  // Erroring a ReadableStream discards the chunk still queued in it, so the
  // short part holds the bytes read before the break, not the last 256 KB.
  assert.ok(meta.bytes > 8 * MB && meta.bytes <= 10 * MB, `the resume point moved past the 8 MB part boundary (${meta.bytes})`);
});

test('a dead link gives up after six attempts', async () => {
  const server = makeServer(40 * MB, () => 'refuse');
  const { sandbox, cache } = load(server);
  await assert.rejects(sandbox.fetchAndCacheFileWithRetry(cache, KEY, URL_, 40 * MB, () => {}, undefined, () => {}), /Failed to fetch/);
  assert.equal(server.requests.length, sandbox.MAX_FUTILE_ATTEMPTS);
});

test('cancellation is never retried', async () => {
  const server = makeServer(40 * MB, () => 3 * MB);
  const { sandbox, cache } = load(server);
  const aborter = new AbortController();
  let drops = 0;
  const run = sandbox.fetchAndCacheFileWithRetry(cache, KEY, URL_, 40 * MB, () => {}, aborter.signal, () => {}, () => {
    drops++;
    aborter.abort();
  });
  await assert.rejects(run, (err) => err.name === 'AbortError');
  assert.equal(drops, 1);
});

test('the fallback loop reports the drops and finishes with cached', async () => {
  const server = makeServer(24 * MB, () => 5 * MB);
  const { sandbox, broadcasts } = load(server);
  const files = [{ path: 'unit1/v1.0.0/Build/unit1.data.unityweb', size: 24 * MB }];
  const ok = await sandbox.fallbackFetch('unit1', '1.0.0', files, 'https://cdn.example/mhs', 'Unit 1');
  assert.equal(ok, true);
  const statuses = broadcasts.map((b) => b.status);
  assert.equal(statuses[statuses.length - 1], 'cached');
  assert.ok(!statuses.includes('error'), 'no error was broadcast');
  const resumed = broadcasts.filter((b) => b.detail && b.detail.resumedFrom);
  assert.ok(resumed.length >= 4, `resumes were announced (${resumed.length})`);
  const withDrops = broadcasts.filter((b) => b.detail && b.detail.drops);
  assert.ok(withDrops.length > 0, 'progress carries the drop count');
  const maxDrops = Math.max(...withDrops.map((b) => b.detail.drops));
  assert.ok(maxDrops >= 4, `drop count climbed (${maxDrops})`);
});

test('the fallback loop reports a final failure with the drop count', async () => {
  const server = makeServer(24 * MB, (n) => (n === 0 ? 9 * MB : 'refuse'));
  const { sandbox, broadcasts } = load(server);
  const files = [{ path: 'unit1/v1.0.0/Build/unit1.data.unityweb', size: 24 * MB }];
  const ok = await sandbox.fallbackFetch('unit1', '1.0.0', files, 'https://cdn.example/mhs', 'Unit 1');
  assert.equal(ok, false);
  const err = broadcasts.find((b) => b.status === 'error');
  assert.ok(err, 'an error was broadcast');
  assert.equal(err.detail.errorClass, 'network');
  assert.equal(err.detail.path, 'fallback');
  assert.equal(err.detail.drops, sandbox.MAX_FUTILE_ATTEMPTS);
  assert.match(err.detail.rawError, /dropped connections resumed, then: Failed to fetch/);
});

test('a refused Background Fetch is announced before the direct download takes over', async () => {
  const server = makeServer(2 * MB, () => Infinity);
  const { sandbox, broadcasts } = load(server);
  sandbox.self.registration.backgroundFetch = {
    get: async () => undefined,
    fetch: async () => { throw new TypeError("Failed to execute 'fetch' on 'BackgroundFetchManager': permission denied"); }
  };
  const files = [{ path: 'unit1/v1.0.0/Build/unit1.loader.js', size: 2 * MB }];
  const ok = await sandbox.startBackgroundFetch('unit1', '1.0.0', files, 'https://cdn.example/mhs', 'Unit 1');
  assert.equal(ok, true);
  assert.equal(broadcasts[0].status, 'method');
  assert.equal(broadcasts[0].detail.path, 'fallback');
  assert.match(broadcasts[0].detail.reason, /^TypeError: .*permission denied/);
  assert.equal(broadcasts[0].detail.version, '1.0.0');
  assert.equal(broadcasts[broadcasts.length - 1].status, 'cached');
});
