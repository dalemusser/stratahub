// Page side of the held tail (internal/app/resources/assets/js/mhs-delivery.js,
// plan B.3 in docs/mission-hydrosci/mhs-issues-102826.md): a direct download
// in a large file's last 16 MB (the SW marks those broadcasts `tail`) waits
// 5 minutes of quiet before a restart instead of 45 s, says that the file is
// being checked, and the step log records the wait once and every
// whole-file restart.
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

function load() {
  const steps = [];
  const sandbox = { window: { location: { href: 'https://hub.example.org/missionhydrosci/units' } }, URL, console, setTimeout, clearTimeout, Promise, JSON };
  vm.createContext(sandbox);
  vm.runInContext(script, sandbox);
  const mgr = new sandbox.window.MHSDeliveryManager({});
  mgr.manifest = { units: [{ id: 'unit2', title: 'Unit 2', version: '3.0.6', files: [] }] };
  mgr.setStepLog({ record: (step, state, msg, detail) => steps.push({ step, state, msg, detail }), onChange() {}, context: {} });
  return { mgr, steps };
}

test('the quiet limit is 5 minutes in the tail and 45 s elsewhere; the manifest can tune both', () => {
  const { mgr } = load();
  assert.equal(mgr._fallbackThreshold({ tail: false }), 45000);
  assert.equal(mgr._fallbackThreshold({ tail: true }), 300000);
  mgr.manifest.tuning = { tailStallMs: 600000, fallbackStallMs: 30000 };
  assert.equal(mgr._fallbackThreshold({ tail: true }), 600000);
  assert.equal(mgr._fallbackThreshold({ tail: false }), 30000);
});

test('byte progress sets and clears the tail state', () => {
  const { mgr } = load();
  mgr._stallState.unit2 = { maxDownloaded: 0, lastProgressAt: 0, fallback: true };
  mgr._fireStatus('unit2', 'downloading', { downloaded: 120 * 1048576, tail: true });
  assert.equal(mgr._stallState.unit2.tail, true);
  mgr._fireStatus('unit2', 'downloading', { downloaded: 130 * 1048576 });
  assert.equal(mgr._stallState.unit2.tail, false);
});

test('the tail wait is logged once per hold; a whole-file restart is logged', () => {
  const { mgr, steps } = load();
  for (let i = 0; i < 4; i++) {
    mgr._logStatus('unit2', 'downloading', { local: true, tailWait: true, downloaded: 201 * 1048576, note: 'Checking the file' }, true);
  }
  const waits = steps.filter((s) => s.detail && s.detail.tail_wait);
  assert.equal(waits.length, 1);
  assert.match(waits[0].msg, /last part is being held at 201 MB/);
  mgr._logStatus('unit2', 'downloading', { downloaded: 0, restartedFromZero: { file: 'unit2.data.unityweb', reason: 'held', heldAt: 201 * 1048576, size: 213 * 1048576, restarts: 1 } }, true);
  const r = steps.find((s) => s.detail && s.detail.restart_zero);
  assert.ok(r);
  assert.match(r.msg, /fetching unit2\.data\.unityweb whole again — its last part was held twice at 201 of 213 MB/);
});
