// Unit tests for MHSStepLog.signInState in
// internal/app/resources/assets/js/mhs-steplog.js: what a StrataHub answer
// to one of the play page's own posts says about the page's sign-in (the
// play page shows its "signed out" bar from it).
//
// Run from the repo root: node --test tests/js/*.test.mjs

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
const state = (status, who, pageUserId) => sandbox.window.MHSStepLog.signInState(status, who, pageUserId);

const me = '69b4449ec6006ac370dad9df';
const other = '6ab29dd9bf592cbac358a163';

test('an accepted post means signed in', () => {
  assert.equal(state(200, null, me), 'ok');
  assert.equal(state(201, null, me), 'ok');
  assert.equal(state(204, null, me), 'ok');
});

test('401 means the sign-in has ended', () => {
  assert.equal(state(401, null, me), 'signed-out');
});

test('a 404 alone says nothing until /api/user has been asked', () => {
  assert.equal(state(404, null, me), '');
  assert.equal(state(404, undefined, me), '');
});

test('a 404 with nobody signed in is signed out', () => {
  assert.equal(state(404, { isAuthenticated: false, user_id: '', name: '' }, me), 'signed-out');
});

test('a 404 with somebody else signed in', () => {
  assert.equal(state(404, { isAuthenticated: true, user_id: other, name: 'x' }, me), 'other-user');
});

test('a 404 with the same user signed in changes nothing (the record is gone, not the sign-in)', () => {
  assert.equal(state(404, { isAuthenticated: true, user_id: me, name: 'x' }, me), '');
});

test('other answers change nothing: a refused token, a server error, a throttle', () => {
  for (const status of [0, 302, 400, 403, 429, 500, 502, 503]) {
    assert.equal(state(status, null, me), '', 'status ' + status);
  }
  // A 403 is the page's token being renewed, not a sign-in problem, even if /api/user was asked.
  assert.equal(state(403, { isAuthenticated: false }, me), '');
});

test('without the page user id a different user cannot be told apart', () => {
  assert.equal(state(404, { isAuthenticated: true, user_id: other }, ''), '');
});
