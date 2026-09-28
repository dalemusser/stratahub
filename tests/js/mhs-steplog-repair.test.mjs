// Unit tests for MHSStepLog.repairUnityPrefsBytes in
// internal/app/resources/assets/js/mhs-steplog.js: removing the empty
// entries that block the game's saved log queue from Unity's PlayerPrefs
// file (docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md §0).
//
// Run from the repo root: node --test tests/js/*.test.mjs
//
// The captured stores in the sibling mhs-updates checkout are used when it
// is present; those tests are skipped otherwise.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const script = fs.readFileSync(path.join(here, '../../internal/app/resources/assets/js/mhs-steplog.js'), 'utf8');
const sandbox = { window: {}, TextDecoder, TextEncoder, Uint8Array, console, setTimeout, clearTimeout };
vm.createContext(sandbox);
vm.runInContext(script, sandbox);
const repair = (u8) => sandbox.window.MHSStepLog.repairUnityPrefsBytes(u8);

// ---- a PlayerPrefs file builder and an independent reader ----------------

function lenBytes(n) {
  return n < 0x80 ? [n] : [0x80, n & 255, (n >>> 8) & 255, (n >>> 16) & 255, (n >>> 24) & 255];
}

// records: [key, value] with value a string, { int }, or { float }.
function prefsFile(records) {
  const parts = [Buffer.from('UnityPrf'), Buffer.from([0, 0, 1, 0, 0, 0, 0x10, 0])];
  for (const [k, v] of records) {
    const kb = Buffer.from(k, 'utf8');
    parts.push(Buffer.from(lenBytes(kb.length)), kb);
    if (typeof v === 'object' && 'int' in v) {
      const b = Buffer.alloc(5); b[0] = 0xfe; b.writeInt32LE(v.int, 1); parts.push(b);
    } else if (typeof v === 'object' && 'float' in v) {
      const b = Buffer.alloc(5); b[0] = 0xfd; b.writeFloatLE(v.float, 1); parts.push(b);
    } else {
      const vb = Buffer.from(v, 'utf8');
      parts.push(Buffer.from(lenBytes(vb.length)), vb);
    }
  }
  return new Uint8Array(Buffer.concat(parts));
}

function readLen(b, p) {
  if (b[p] < 0x80) return [b[p], p + 1];
  assert.equal(b[p], 0x80, `length marker at ${p}`);
  return [b.readUInt32LE(p + 1), p + 5];
}

// [[key, value]] with strings decoded and numbers as { int } / { float } hex.
function readPrefs(u8) {
  const b = Buffer.from(u8);
  assert.equal(b.subarray(0, 8).toString('latin1'), 'UnityPrf');
  const out = [];
  let p = 16;
  while (p < b.length) {
    const [kl, ks] = readLen(b, p);
    const key = b.subarray(ks, ks + kl).toString('utf8');
    const t = b[ks + kl];
    if (t === 0xfd || t === 0xfe) {
      out.push([key, { raw: b.subarray(ks + kl, ks + kl + 5).toString('hex') }]);
      p = ks + kl + 5;
    } else {
      const [vl, vs] = readLen(b, ks + kl);
      out.push([key, b.subarray(vs, vs + vl).toString('utf8')]);
      p = vs + vl;
    }
  }
  assert.equal(p, b.length, 'records end exactly at the end of the file');
  return out;
}

const cacheOf = (u8) => readPrefs(u8).find(([k]) => k === 'game_logs_cache.json')[1];

// The legacy cache as the schools' build writes it (JsonUtility, entries as
// JSON strings).
const legacyCache = (entries) => JSON.stringify({ logs: entries.map((e) => (typeof e === 'string' ? e : JSON.stringify(e))) });

const event = (i, extra = {}) => ({ game: 'mhs', user_id: 'aaaaaaaaaaaaaaaaaaaaaaaa', version: '20260914-', sceneName: 'Unit 1', timestamp: `2026-09-28T06:53:${String(i).padStart(2, '0')}.4590000Z`, eventType: 'PlayerPositionEvent', data: { position: { x: 18.891, y: 0.0, z: -57.984 } }, ...extra });

const others = [
  ['523c2d9e9d0c28c46bc1bab1c800073e', { int: 1 }],
  ['8fa374a9fcd2bd24094d68c87f08d6be', { float: 60 }],
  ['Language', ''],
  ['SaveSlot0', ''],
];
const after = [
  ['unity.player_session_count', '2'],
  ['unity_connect.session_id', '40334b95-2b51-0393-18ff-1fafec01f90a'],
];

// ---- tests ------------------------------------------------------------------

test('legacy cache with the empty entry first: removed, everything else unchanged', () => {
  const entries = ['{}', event(1), event(2), event(3, { data: {} })];
  const file = prefsFile([...others, ['game_logs_cache.json', legacyCache(entries)], ...after]);
  const r = repair(file);
  assert.equal(r.removed, 1);
  assert.equal(r.queued, 3);
  const before = readPrefs(file), got = readPrefs(r.bytes);
  assert.deepEqual(got.filter(([k]) => k !== 'game_logs_cache.json'), before.filter(([k]) => k !== 'game_logs_cache.json'));
  assert.deepEqual(got.map(([k]) => k), before.map(([k]) => k), 'record order kept');
  const logs = JSON.parse(cacheOf(r.bytes)).logs;
  assert.deepEqual(logs, JSON.parse(legacyCache(entries)).logs.slice(1), 'kept entries identical, in order (an entry whose data is {} is kept)');
});

test('legacy cache without empty entries: nothing to write', () => {
  const file = prefsFile([...others, ['game_logs_cache.json', legacyCache([event(1), event(2)])], ...after]);
  const r = repair(file);
  assert.deepEqual({ removed: r.removed, queued: r.queued, bytes: r.bytes }, { removed: 0, queued: 2, bytes: null });
});

test('several empty and unreadable entries anywhere are removed', () => {
  const entries = ['{}', event(1), '{}', 'not json', 'null', '[]', event(2), '{}'];
  const r = repair(prefsFile([...others, ['game_logs_cache.json', legacyCache(entries)]]));
  assert.equal(r.removed, 6);
  assert.equal(r.queued, 2);
  assert.deepEqual(JSON.parse(cacheOf(r.bytes)).logs.map((s) => JSON.parse(s).timestamp), [event(1).timestamp, event(2).timestamp]);
});

test('the newer logger format (a plain array) keeps kept entries byte for byte', () => {
  const raw = '[{},{"game":"mhs","user_id":"aaaaaaaaaaaaaaaaaaaaaaaa","data":{"x":0.0,"y":60.0}}, {} ,{"game":"mhs","user_id":"bbbbbbbbbbbbbbbbbbbbbbbb","data":{}}]';
  const r = repair(prefsFile([...others, ['game_logs_cache.json', raw], ...after]));
  assert.equal(r.removed, 2);
  assert.equal(r.queued, 2);
  assert.equal(cacheOf(r.bytes), '[{"game":"mhs","user_id":"aaaaaaaaaaaaaaaaaaaaaaaa","data":{"x":0.0,"y":60.0}},{"game":"mhs","user_id":"bbbbbbbbbbbbbbbbbbbbbbbb","data":{}}]');
});

test('a value that shrinks below 128 bytes switches to the short length form', () => {
  const file = prefsFile([['game_logs_cache.json', legacyCache(Array(30).fill('{}'))], ...after]);
  assert.ok(cacheOf(file).length >= 128);
  const r = repair(file);
  assert.equal(r.removed, 30);
  assert.equal(r.queued, 0);
  assert.equal(cacheOf(r.bytes), '{"logs":[]}');
  assert.deepEqual(readPrefs(r.bytes).slice(1), after);
});

test('non-ASCII text in an entry survives', () => {
  const e = event(1, { data: { title: 'Río Grande — ☔ niño' } });
  const r = repair(prefsFile([['game_logs_cache.json', legacyCache(['{}', e])]]));
  assert.equal(JSON.parse(JSON.parse(cacheOf(r.bytes)).logs[0]).data.title, 'Río Grande — ☔ niño');
});

test('no log cache in the store: nothing to do', () => {
  const r = repair(prefsFile([...others, ...after]));
  assert.deepEqual({ removed: r.removed, queued: r.queued, bytes: r.bytes }, { removed: 0, queued: 0, bytes: null });
});

test('anything unexpected is left alone (null)', () => {
  const good = prefsFile([...others, ['game_logs_cache.json', legacyCache(['{}', event(1)])], ...after]);
  assert.equal(repair(new Uint8Array(0)), null, 'empty');
  assert.equal(repair(new Uint8Array(Buffer.from('NotUnity' + '\0'.repeat(20)))), null, 'wrong magic');
  assert.equal(repair(good.subarray(0, good.length - 7)), null, 'truncated');
  const unknownType = prefsFile([['a', 'b']]); unknownType[16 + 2] = 0x90;
  assert.equal(repair(unknownType), null, 'unknown type byte');
  assert.equal(repair(prefsFile([['game_logs_cache.json', '{"logs":["{}"'], ...after])), null, 'cache value not valid JSON');
  assert.equal(repair(prefsFile([['game_logs_cache.json', '{"other":1,"logs":["{}"]}']])), null, 'unexpected cache shape');
  const intCache = prefsFile([['game_logs_cache.json', { int: 3 }]]);
  assert.equal(repair(intCache), null, 'cache key holding a number');
});

test('a repaired store needs no second repair', () => {
  const r = repair(prefsFile([...others, ['game_logs_cache.json', legacyCache(['{}', event(1), event(2)])], ...after]));
  const again = repair(r.bytes);
  assert.equal(again.bytes, null);
  assert.equal(again.queued, 2);
});

// ---- the stores captured from real builds -----------------------------------

const specimens = path.join(here, '../../../mhs-updates/gamelogger-cache-overflow-091626/specimen');
const cases = [
  ['PlayerPrefs-2026-09-18-blocked-host.bin', 80],
  ['PlayerPrefs-2026-09-28-wedged-by-v2.8.1.bin', 38],
  ['PlayerPrefs-2026-09-28-after-v2.8.3-launch.bin', 40],
];
for (const [name, total] of cases) {
  const file = path.join(specimens, name);
  test(`captured store ${name}`, { skip: !fs.existsSync(file) && 'mhs-updates checkout not found' }, () => {
    const u8 = new Uint8Array(fs.readFileSync(file));
    const before = readPrefs(u8);
    const r = repair(u8);
    assert.equal(r.removed, 1, 'exactly the one empty entry at the head');
    assert.equal(r.queued, total - 1);
    const got = readPrefs(r.bytes);
    assert.deepEqual(got.filter(([k]) => k !== 'game_logs_cache.json'), before.filter(([k]) => k !== 'game_logs_cache.json'));
    const cache = JSON.parse(cacheOf(r.bytes));
    const entries = Array.isArray(cache) ? cache : cache.logs.map((s) => JSON.parse(s));
    assert.equal(entries.length, total - 1);
    assert.ok(entries.every((e) => Object.keys(e).length > 0));
    assert.equal(repair(r.bytes).bytes, null, 'idempotent');
  });
}
