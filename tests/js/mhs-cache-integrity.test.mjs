// Unit tests for MHSDeliveryManager.prototype._checkUnitCache in
// internal/app/resources/assets/js/mhs-delivery.js: every cached file's size
// and the loader's content are checked, damaged entries are removed so the
// next download fetches them again, and a size that stays "wrong" after two
// fresh downloads is kept (a manifest error must not cause endless
// downloads). docs/mission-hydrosci/mhs-issues-102826.md Issue 4, plan B.2.
//
// Run from the repo root: node --test tests/js/*.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const script = fs.readFileSync(path.join(here, '../../internal/app/resources/assets/js/mhs-delivery.js'), 'utf8');

// A minimal Cache Storage: one cache per name, entries keyed by URL string.
function fakeCaches() {
  const stores = new Map();
  const open = async (name) => {
    if (!stores.has(name)) stores.set(name, new Map());
    const m = stores.get(name);
    return {
      match: async (k) => (m.has(k) ? m.get(k).clone() : undefined),
      put: async (k, r) => { m.set(k, r); },
      delete: async (k) => m.delete(k),
    };
  };
  return { stores, api: { open, delete: async (n) => stores.delete(n) } };
}

function load() {
  const { stores, api } = fakeCaches();
  const mem = {};
  const steps = [];
  const sandbox = {
    window: { caches: api, location: { href: 'https://hub.example.org/missionhydrosci/units' } },
    caches: api, Response, Headers, Blob, URL, console, setTimeout, clearTimeout, Promise, JSON,
    localStorage: { getItem: k => (k in mem ? mem[k] : null), setItem: (k, v) => { mem[k] = String(v); }, removeItem: k => { delete mem[k]; } },
  };
  sandbox.window.localStorage = sandbox.localStorage;
  vm.createContext(sandbox);
  vm.runInContext(script, sandbox);
  const M = sandbox.window.MHSDeliveryManager;
  const mgr = new M({ contentPrefix: '/missionhydrosci/content/', unitCachePrefix: 'missionhydrosci-unit-' });
  mgr.setStepLog({ record: (step, state, msg) => steps.push({ step, state, msg }), onChange() {}, context: {} });
  return { mgr, stores, steps, mem };
}

const LOADER = 'function createUnityInstance(c, o, p) { return Promise.resolve(); }';
const unit = {
  id: 'unit2', version: '3.0.6', title: 'Unit 2',
  files: [
    { path: 'unit2/v3.0.6/Build/unit2.loader.js', size: LOADER.length },
    { path: 'unit2/v3.0.6/Build/unit2.wasm', size: 1000 },
    { path: 'unit2/v3.0.6/Build/unit2.data', size: 5000 },
  ],
};
const key = (f) => '/missionhydrosci/content/' + f.path;
const body = (n) => new Uint8Array(n);
const resp = (b, len) => new Response(b, { headers: len === undefined ? {} : { 'content-length': String(len) } });

async function fill(stores, overrides = {}) {
  const m = new Map();
  stores.set('missionhydrosci-unit-unit2-v3.0.6', m);
  m.set(key(unit.files[0]), overrides.loader || resp(LOADER, LOADER.length));
  m.set(key(unit.files[1]), overrides.wasm || resp(body(1000), 1000));
  m.set(key(unit.files[2]), overrides.data || resp(body(5000), 5000));
  return m;
}

test('an intact unit is cached', async () => {
  const { mgr, stores } = load();
  await fill(stores);
  assert.equal(await mgr._checkUnitCache(unit), 'cached');
});

test('a loader without createUnityInstance is removed and the unit is partial', async () => {
  const { mgr, stores, steps } = load();
  const html = '<html>Blocked by your organization</html>';
  const m = await fill(stores, { loader: resp(html, html.length) });
  assert.equal(await mgr._checkUnitCache(unit), 'partial');
  assert.equal(m.has(key(unit.files[0])), false, 'damaged loader still cached');
  assert.equal(m.size, 2, 'intact files kept');
  assert.ok(steps.some(s => s.step === 'verify' && /Removed 1 damaged file.*unit2\.loader\.js.*createUnityInstance/.test(s.msg)), JSON.stringify(steps));
});

test('a small file of the wrong size is removed (not only the largest is checked)', async () => {
  const { mgr, stores } = load();
  const m = await fill(stores, { wasm: resp(body(200), 200) });
  assert.equal(await mgr._checkUnitCache(unit), 'partial');
  assert.equal(m.has(key(unit.files[1])), false);
});

test('a header that disagrees with an intact body is accepted and corrected', async () => {
  const { mgr, stores } = load();
  const m = await fill(stores, { data: resp(body(5000), 1234) });
  assert.equal(await mgr._checkUnitCache(unit), 'cached');
  assert.equal(m.get(key(unit.files[2])).headers.get('content-length'), '5000');
});

test('a missing content-length with the right body is accepted', async () => {
  const { mgr, stores } = load();
  await fill(stores, { wasm: resp(body(1000)) });
  assert.equal(await mgr._checkUnitCache(unit), 'cached');
});

test('the same wrong size after two fresh downloads is kept, not removed again', async () => {
  const { mgr, stores, steps } = load();
  mgr.manifest = { units: [unit] };
  for (let round = 1; round <= 2; round++) {
    await fill(stores, { data: resp(body(4990), 4990) });
    assert.equal(await mgr._checkUnitCache(unit), 'partial', 'round ' + round);
  }
  const m = await fill(stores, { data: resp(body(4990), 4990) });
  assert.equal(await mgr._checkUnitCache(unit), 'cached');
  assert.equal(m.has(key(unit.files[2])), true);
  assert.ok(steps.some(s => /kept \(the expected size may be wrong\)/.test(s.msg)));
});

test('a damaged loader is removed every time (no limit)', async () => {
  const { mgr, stores } = load();
  mgr.manifest = { units: [unit] };
  for (let round = 1; round <= 4; round++) {
    await fill(stores, { loader: resp('x', 1) });
    assert.equal(await mgr._checkUnitCache(unit), 'partial', 'round ' + round);
  }
});

test('nothing cached is not_cached; some missing is partial', async () => {
  const { mgr, stores } = load();
  assert.equal(await mgr._checkUnitCache(unit), 'not_cached');
  const m = await fill(stores);
  m.delete(key(unit.files[1]));
  assert.equal(await mgr._checkUnitCache(unit), 'partial');
});
