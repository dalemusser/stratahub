// Large unit files downloaded in pieces (plan item B.7) and the Background
// Fetch creation dedupe (DL-1), in the service worker
// (internal/app/features/missionhydrosci/static/sw-background-fetch.js).
//
// Some school web filters hold the last megabytes of every large object they
// scan, whole or ranged, so a unit's .data file never finishes there, while
// objects of about 12 MB pass. Files of 32 MB or more are also stored as 8 MB
// pieces at "<path>.pieces/NNN"; the manifest lists pieces/pieceBytes and the
// worker fetches the pieces, keeps each as it arrives, and assembles the file.
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
const CDN = 'https://cdn.example/mhs';

class FakeCache {
  constructor() { this.store = new Map(); }
  static url(key) { return typeof key === 'string' ? new URL(key, ORIGIN).href : key.url; }
  async match(key) {
    const e = this.store.get(FakeCache.url(key));
    return e ? new Response(e.body.slice(), { status: 200, headers: e.headers }) : undefined;
  }
  async put(key, response) {
    const buf = new Uint8Array(await response.arrayBuffer());
    this.store.set(FakeCache.url(key), { body: buf, headers: Object.fromEntries(response.headers) });
  }
  async delete(key) { return this.store.delete(FakeCache.url(key)); }
  async keys() { return [...this.store.keys()].map((u) => ({ url: u })); }
}

// A file and a content server that answers its pieces; failures[i] is how
// many times piece i fails first ('always' = never answers).
function pieced(size, pieceBytes, failures = {}) {
  const file = new Uint8Array(size).map((_, i) => (i * 13) & 255);
  const requests = [];
  const left = { ...failures };
  const fetch = async (url) => {
    requests.push(url);
    const m = /\.pieces\/(\d+)$/.exec(url);
    if (!m) throw new Error('whole file requested: ' + url);
    const i = parseInt(m[1], 10);
    if (left[i] === 'always' || left[i] > 0) {
      if (left[i] !== 'always') left[i]--;
      throw new TypeError('Failed to fetch');
    }
    return new Response(file.slice(i * pieceBytes, Math.min(size, (i + 1) * pieceBytes)), { status: 200 });
  };
  return { file, requests, fetch };
}

function load(fetchImpl, extra = {}) {
  const broadcasts = [];
  const sandbox = {
    self: { location: { origin: ORIGIN }, registration: extra.registration || {}, addEventListener() {} },
    BroadcastChannel: class { postMessage(m) { broadcasts.push(m); } },
    caches: { open: async () => sandbox.__cache, keys: async () => [] },
    fetch: fetchImpl,
    Request, Response, Headers, ReadableStream, TransformStream, DOMException, AbortController, URL, Uint8Array, Set, Map, Promise, JSON, Math, Date, String,
    decodeURIComponent,
    console: { log() {}, warn() {}, error() {} },
    setTimeout: (fn) => setImmediate(fn),
    clearTimeout: () => {},
    setImmediate,
  };
  sandbox.__cache = new FakeCache();
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox);
  return { sandbox, broadcasts, cache: sandbox.__cache };
}

const PATH = 'unit2/v1.0.0/Build/unit2.data.unityweb';
const KEY = '/missionhydrosci/content/' + PATH;

async function bytesAt(cache, key) {
  const r = await cache.match(key);
  return r ? new Uint8Array(await r.arrayBuffer()) : null;
}
async function leftovers(cache) {
  return (await cache.keys()).map((k) => k.url).filter((u) => /\?(piece|part)=/.test(u));
}

test('a pieced file is fetched as its pieces, through drops, and assembled byte for byte', async () => {
  const server = pieced(3123, 1000, { 1: 2, 3: 1 });
  const { sandbox, cache } = load(server.fetch);
  await cache.put(KEY + '?part=0', new Response(new Uint8Array(500))); // a partial from the whole-file path
  const file = { path: PATH, size: 3123, pieces: 4, pieceBytes: 1000 };
  assert.equal(sandbox.isPieced(file), true);
  const drops = [];
  let last = 0;
  await sandbox.fetchPiecedFile(cache, KEY, CDN, file, (b) => { last = b; }, null, (d) => drops.push(d));
  assert.deepEqual(await bytesAt(cache, KEY), server.file);
  assert.equal((await cache.match(KEY)).headers.get('content-length'), '3123');
  assert.equal(drops.length, 3, 'each failed piece attempt is reported as a drop');
  assert.equal(last, 3123, 'progress reaches the whole size');
  assert.deepEqual(await leftovers(cache), [], 'pieces and old parts are dropped');
  assert.ok(server.requests.every((u) => /\.pieces\/\d{3}$/.test(u)), 'only pieces are requested');
});

test('a resumed pieced download fetches only the missing pieces', async () => {
  const server = pieced(3123, 1000);
  const { sandbox, cache } = load(server.fetch);
  for (const i of [0, 1]) {
    await cache.put(KEY + '?piece=' + i, new Response(server.file.slice(i * 1000, (i + 1) * 1000), { headers: { 'content-length': '1000' } }));
  }
  await sandbox.fetchPiecedFile(cache, KEY, CDN, { path: PATH, size: 3123, pieces: 4, pieceBytes: 1000 }, () => {}, null, () => {});
  assert.deepEqual(server.requests.map((u) => u.slice(-3)), ['002', '003']);
  assert.deepEqual(await bytesAt(cache, KEY), server.file);
});

test('a piece that never comes fails the attempt and keeps the pieces already stored', async () => {
  const server = pieced(3123, 1000, { 2: 'always' });
  const { sandbox, cache } = load(server.fetch);
  await assert.rejects(sandbox.fetchPiecedFile(cache, KEY, CDN, { path: PATH, size: 3123, pieces: 4, pieceBytes: 1000 }, () => {}, null, () => {}));
  assert.equal(await cache.match(KEY), undefined, 'no file under its own key');
  assert.equal((await leftovers(cache)).length, 2, 'pieces 0 and 1 are kept for the retry');
});

test('a manifest entry whose pieces do not cover the file is not pieced', () => {
  const { sandbox } = load(async () => { throw new Error('no network'); });
  assert.equal(sandbox.isPieced({ size: 3123, pieces: 3, pieceBytes: 1000 }), false);
  assert.equal(sandbox.isPieced({ size: 3123, pieces: 5, pieceBytes: 1000 }), false);
  assert.equal(sandbox.isPieced({ size: 3123 }), false);
});

test('the fallback run downloads pieced and whole files and reports the unit cached', async () => {
  const server = pieced(3123, 1000);
  const loader = new TextEncoder().encode('function createUnityInstance(){}');
  const fetchImpl = async (url, init) => (/loader\.js$/.test(url) ? new Response(loader.slice(), { status: 200 }) : server.fetch(url, init));
  const { sandbox, cache, broadcasts } = load(fetchImpl);
  const files = [
    { path: 'unit2/v1.0.0/Build/unit2.loader.js', size: loader.length },
    { path: PATH, size: 3123, pieces: 4, pieceBytes: 1000 },
  ];
  const ok = await sandbox.runFallbackFetch('unit2-v1.0.0', 'unit2', '1.0.0', files, CDN, new AbortController());
  assert.equal(ok, true);
  assert.deepEqual(await bytesAt(cache, KEY), server.file);
  assert.ok(broadcasts.some((m) => m.status === 'cached'), 'cached is broadcast');
});

test('Background Fetch: pieces are kept as pieces and assembled on success', async () => {
  const server = pieced(2500, 1000);
  const { sandbox, cache, broadcasts } = load(server.fetch);
  const records = [0, 1, 2].map((i) => ({
    request: { url: CDN + '/' + PATH + '.pieces/00' + i },
    responseReady: Promise.resolve(new Response(server.file.slice(i * 1000, Math.min(2500, (i + 1) * 1000)))),
  }));
  records.push({ request: { url: CDN + '/unit2/v1.0.0/Build/unit2.loader.js' }, responseReady: Promise.resolve(new Response('createUnityInstance')) });
  await sandbox.handleBackgroundFetchSuccess({ registration: { id: 'missionhydrosci-unit2-v1.0.0', matchAll: async () => records } });
  assert.deepEqual(await bytesAt(cache, KEY), server.file);
  assert.equal((await cache.match(KEY)).headers.get('content-length'), '2500');
  assert.ok(await cache.match('/missionhydrosci/content/unit2/v1.0.0/Build/unit2.loader.js'));
  assert.deepEqual(await leftovers(cache), []);
  assert.ok(broadcasts.some((m) => m.status === 'cached'));
});

test('Background Fetch requests list each piece of a pieced file', async () => {
  const fetched = [];
  const registration = {
    backgroundFetch: {
      get: async () => undefined,
      fetch: async (id, requests) => { fetched.push(...requests.map((r) => r.url)); return { id, result: '', addEventListener() {} }; },
    },
  };
  const { sandbox } = load(async () => { throw new Error('no network'); }, { registration });
  await sandbox.startBackgroundFetch('unit2', '1.0.0', [
    { path: 'unit2/v1.0.0/Build/unit2.loader.js', size: 10 },
    { path: PATH, size: 3123, pieces: 4, pieceBytes: 1000 },
  ], CDN, 't');
  assert.deepEqual(fetched.map((u) => u.replace(CDN + '/', '')), [
    'unit2/v1.0.0/Build/unit2.loader.js',
    PATH + '.pieces/000', PATH + '.pieces/001', PATH + '.pieces/002', PATH + '.pieces/003',
  ]);
});

test('DL-1: simultaneous download requests create one Background Fetch and no fallback', async () => {
  const regs = {};
  let created = 0;
  const tick = () => new Promise((r) => setImmediate(r));
  const registration = {
    backgroundFetch: {
      get: async (id) => { await tick(); return regs[id]; },
      fetch: async (id) => {
        await tick();
        if (regs[id]) throw new TypeError('duplicate id');
        created++;
        regs[id] = { id, result: '', downloaded: 0, downloadTotal: 10, addEventListener() {} };
        return regs[id];
      },
    },
  };
  const { sandbox } = load(async () => { throw new Error('no network'); }, { registration });
  let fallbacks = 0;
  sandbox.__fb = async () => { fallbacks++; return true; };
  vm.runInContext('fallbackFetch = __fb;', sandbox);
  const files = [{ path: 'unit1/v1.0.0/Build/unit1.loader.js', size: 10 }];
  const results = await Promise.all([1, 2, 3].map(() => sandbox.startBackgroundFetch('unit1', '1.0.0', files, CDN, 't')));
  assert.deepEqual(results, [true, true, true]);
  assert.equal(created, 1);
  assert.equal(fallbacks, 0, 'no second download starts');
});
