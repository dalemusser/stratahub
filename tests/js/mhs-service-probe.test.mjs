// Unit tests for MHSDeliveryManager.probeServices in
// internal/app/resources/assets/js/mhs-delivery.js: the play page's probe of
// the game services, sent the way the game sends its requests. Only a
// readable {"ok":true} counts as getting through; a request the page cannot
// read an answer to is "blocked"; any other readable answer is "odd" (never
// reported as a block). See docs/mission-hydrosci/mhs-issues-102826.md Issue 3.
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

// A sandbox whose fetch answers per URL: a function (url, opts) => Promise.
function load(answer) {
  const calls = [];
  const sandbox = {
    window: { location: { href: 'https://hub.example.org/missionhydrosci/play/unit2' } },
    URL, AbortController, console, setTimeout, clearTimeout, Promise,
    fetch: (url, opts) => { calls.push({ url, opts }); return answer(url, opts); },
  };
  sandbox.window.fetch = sandbox.fetch;
  vm.createContext(sandbox);
  vm.runInContext(script, sandbox);
  return { M: sandbox.window.MHSDeliveryManager, calls };
}

const probes = [
  { name: 'log', url: 'https://log.example.org/health', post: 'https://log.example.org/api/log/probe' },
  { name: 'save', url: 'https://save.example.org/health', post: 'https://save.example.org/api/state/probe' },
];
const okBody = () => Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{"ok":true,"service":"x"}') });
const opaque = () => Promise.resolve({ ok: false, status: 0, type: 'opaque' });

test('both services answer: ok, sent in the game\'s form with the game\'s key', async () => {
  const { M, calls } = load((url, opts) => (opts.method === 'POST' ? okBody() : opaque()));
  const res = await M.probeServices(probes, { log: 'Bearer log-key', save: 'Bearer save-key' });
  assert.deepEqual(res.map(r => [r.name, r.state]), [['log', 'ok'], ['save', 'ok']]);
  const posts = calls.filter(c => c.opts.method === 'POST');
  assert.equal(posts.length, 2);
  for (const c of posts) {
    assert.equal(c.opts.mode, 'cors');
    assert.equal(c.opts.headers['Content-Type'], 'application/json');
    assert.match(c.opts.body, /probe/);
  }
  assert.equal(posts.find(c => c.url.includes('log.')).opts.headers.Authorization, 'Bearer log-key');
  assert.equal(posts.find(c => c.url.includes('save.')).opts.headers.Authorization, 'Bearer save-key');
});

test('the POST is stopped while the plain GET passes: blocked, and the host answers', async () => {
  const { M } = load((url, opts) => {
    if (opts.method === 'POST' && url.includes('log.')) return Promise.reject(new TypeError('Failed to fetch'));
    return opts.method === 'POST' ? okBody() : opaque();
  });
  const res = await M.probeServices(probes, {});
  const log = res.find(r => r.name === 'log');
  assert.equal(log.state, 'blocked');
  assert.equal(log.hostAnswers, true);
  assert.equal(log.host, 'log.example.org');
  assert.match(log.error, /Failed to fetch/);
  assert.equal(res.find(r => r.name === 'save').state, 'ok');
});

test('a readable answer that is not the service (an older service, a 404) is odd, not blocked', async () => {
  const { M } = load((url, opts) => (opts.method === 'POST'
    ? Promise.resolve({ ok: false, status: 404, text: () => Promise.resolve('404 page not found') })
    : opaque()));
  const res = await M.probeServices(probes, {});
  assert.deepEqual(res.map(r => r.state), ['odd', 'odd']);
});

test('a 200 that is not the service\'s JSON is odd too', async () => {
  const { M } = load((url, opts) => (opts.method === 'POST'
    ? Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('<html>blocked by policy</html>') })
    : opaque()));
  const res = await M.probeServices(probes, {});
  assert.deepEqual(res.map(r => r.state), ['odd', 'odd']);
});

test('probes without a post URL are left out; no key sends a placeholder', async () => {
  const { M, calls } = load((url, opts) => (opts.method === 'POST' ? okBody() : opaque()));
  const res = await M.probeServices([{ name: 'log', url: 'https://log.example.org/health' }, probes[1]]);
  assert.deepEqual(res.map(r => r.name), ['save']);
  assert.equal(calls.find(c => c.opts.method === 'POST').opts.headers.Authorization, 'Bearer probe');
});
