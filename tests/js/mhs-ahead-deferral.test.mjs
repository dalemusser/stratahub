// Ahead downloads put off for want of space (plan item 3a,
// docs/mission-hydrosci/mhs-resolution-plan-111426.md), in
// internal/app/resources/assets/js/mhs-delivery.js.
//
// The units and manage pages download the next unit while the current one is
// played (downloadUnit(next, { ahead: true })). On a device that holds about
// one unit (a profile kept in memory, wiped daily) that download runs out of
// space; it is then put off ('deferred': no failure report, no retry, its
// partial data removed) until the unit is needed, instead of failing and
// retrying every minute all day.
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

const unit = { id: 'unit3', version: '3.0.3', title: 'Unit 3', files: [{ path: 'unit3/v3.0.3/Build/unit3.data', size: 5000 }] };

function load(mem = {}) {
  const deleted = [];
  const api = { open: async () => ({ match: async () => undefined, put: async () => {}, delete: async () => true }), delete: async (n) => { deleted.push(n); return true; } };
  const steps = [];
  const sandbox = {
    window: { caches: api, location: { href: 'https://hub.example.org/missionhydrosci/units' } },
    caches: api, Response, Headers, URL, Uint8Array, console, setTimeout, clearTimeout, Promise, JSON, Date,
    localStorage: { getItem: k => (k in mem ? mem[k] : null), setItem: (k, v) => { mem[k] = String(v); }, removeItem: k => { delete mem[k]; } },
  };
  sandbox.window.localStorage = sandbox.localStorage;
  vm.createContext(sandbox);
  vm.runInContext(script, sandbox);
  const M = sandbox.window.MHSDeliveryManager;
  const mgr = new M({ contentPrefix: '/missionhydrosci/content/', unitCachePrefix: 'missionhydrosci-unit-' });
  mgr.setStepLog({ record: (step, state, msg, detail) => steps.push({ step, state, msg, detail }), onChange() {}, context: {} });
  mgr.manifest = { units: [unit] };
  mgr.manifestLoaded = true;
  mgr._waitForSW = async () => null;
  mgr._cancelFallbackDownloads = () => {};
  const statuses = [];
  mgr.onStatus((id, st, d) => statuses.push({ id, st, d }));
  return { mgr, steps, statuses, deleted, mem };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

test('an ahead download that runs out of space is put off: no failure, no retry, its data removed', async () => {
  const { mgr, steps, statuses, deleted, mem } = load();
  mgr._ahead.unit3 = true; mgr._wanted.unit3 = true; mgr._activeDownloads.unit3 = true;
  mgr._fireStatus('unit3', 'error', { error: 'Not enough free space', errorClass: 'quota', rawError: 'Quota exceeded.' });
  await settle(); await settle();
  assert.ok(!statuses.some((s) => s.st === 'error'), 'no error status reaches the page');
  assert.equal(statuses[statuses.length - 1].st, 'deferred');
  assert.equal(mgr._pendingRetries.unit3, undefined, 'no retry scheduled');
  assert.ok(!mgr._wanted.unit3 && !mgr._activeDownloads.unit3);
  assert.deepEqual(deleted, ['missionhydrosci-unit-unit3-v3.0.3'], 'the partial download is removed');
  assert.ok(!steps.some((s) => s.state === 'fail'), 'no failure in the step log');
  assert.ok(steps.some((s) => s.step === 'storage' && /ahead of time/.test(s.msg)));
  assert.ok(JSON.parse(mem['mhs-ahead-deferred'])['unit3|3.0.3'], 'remembered for later page loads');
});

test('a put-off unit is not downloaded ahead again, on this page or the next; a plain request lifts it', async () => {
  const mem = {};
  const first = load(mem);
  first.mgr._ahead.unit3 = true;
  first.mgr._fireStatus('unit3', 'error', { errorClass: 'quota' });
  await settle(); await settle();

  const { mgr, statuses } = load(mem); // a later page load
  await mgr.downloadUnit('unit3', { ahead: true });
  assert.equal(statuses[statuses.length - 1].st, 'deferred');
  assert.ok(!mgr._wanted.unit3 && !mgr._activeDownloads.unit3, 'nothing started');

  mgr._activeDownloads.unit3 = true; // stop downloadUnit right after the mark is handled
  await mgr.downloadUnit('unit3');
  assert.equal(mem['mhs-ahead-deferred'], '{}', 'a needed download lifts the deferral');
  assert.equal(mgr._ahead.unit3, undefined);
});

test('a manager retry keeps the ahead mark; a needed download that runs out of space still fails and retries', async () => {
  const { mgr, statuses } = load();
  mgr._ahead.unit3 = true; mgr._activeDownloads.unit3 = true;
  await mgr.downloadUnit('unit3', { keep: true });
  assert.equal(mgr._ahead.unit3, true);

  delete mgr._ahead.unit3; mgr._wanted.unit3 = true;
  mgr._fireStatus('unit3', 'error', { errorClass: 'quota' });
  assert.ok(statuses.some((s) => s.st === 'error'));
  assert.ok(mgr._pendingRetries.unit3, 'the needed unit keeps retrying');
  mgr._clearPendingRetry('unit3');
});

test('deferrals expire after 12 hours', () => {
  const { mgr, mem } = load({ 'mhs-ahead-deferred': JSON.stringify({ 'unit3|3.0.3': Date.now() - 13 * 3600e3 }) });
  assert.equal(mgr._aheadDeferred('unit3'), false);
  assert.equal(mem['mhs-ahead-deferred'], '{}');
});
