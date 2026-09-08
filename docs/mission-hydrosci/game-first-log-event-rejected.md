# Mission HydroSci game: the first log event of a session is rejected

**Date observed:** 2026-09-08
**For:** the game (Unity / MHSBridge) developers
**Status:** open — a game-side fix is needed; nothing in StrataHub or the log service is at fault

## Summary

The first log event the game sends in a session goes out **without a
`user_id`**. The log service rejects it with `400 MISSING_FIELD` ("missing or
invalid 'user_id' field"). Every later event in the same session carries the
id and is accepted (`201 Created`). So each session loses its first event.

This is **pre-existing and not caused by the device test**: the log service
recorded **739** such rejections in the seven days before this was noticed,
from real student devices (Chromebooks) as well as from test runs.

## Evidence

Rejections logged by the log service (`logapi/handler.go`, code
`MISSING_FIELD`, game `mhs`), by day:

| Day (2026) | Rejections |
|---|---|
| Sep 1 | 64 |
| Sep 2 | 99 |
| Sep 3 | 70 |
| Sep 4 | 398 |
| Sep 5 | 19 |
| Sep 6 | 27 |
| Sep 7 | 59 |
| Sep 8 (partial) | 3 |

A sample rejection came from a Chromebook (`Mozilla/5.0 (X11; CrOS x86_64
…) … Chrome/151…`) during a normal student session.

Captured in a browser session on 2026-09-08 (a device-test run, id
`ffffffff9863af2400f41211`), in order:

1. Console: `MHSBridge: Config loaded from host page`
2. Console: `MHSBridge: PWA mode activated, user_id=ffffffff9863af2400f41211`
3. `POST …/api/log/submit` → **400**, body
   `{"error":"missing or invalid 'user_id' field","code":"MISSING_FIELD"}`
4. Console (Unity): `ArgumentException: JSON must represent an object type.`
   (the game failing to parse the error body it did not expect)
5. `POST …/api/settings/load` → 200, `POST …/api/state/load` → 200
6. `POST …/api/log/submit` → **201** (and every later one)

The submissions in this sequence were all sent after the bridge reported the
identity as active (step 2), so the id was known to the bridge when the first
event was built.

## What the log service requires

`user_id` must be present and match `^[0-9a-f]{24}$` (24 lowercase hex
characters). `playerId` and `login_id` are not accepted. The host page
provides the id in `window.__mhsBridgeConfig.identity.user_id` before the
game loads, and again through `MHSBridge.OnPWAReady`.

## Where to look in the game

- The first event is likely built (or queued) before `MHSBridge` has copied
  the identity into the logger, so its `user_id` is empty or missing, while
  later events read the populated value. Check the order of the logger's
  session-start event relative to `MHSBridge.LoadConfig` / `OnPWAReady`, and
  whether queued events capture the id at enqueue time or at send time.
- The rejected event is dropped, not retried: after the 400 the game moves on.

## Suggested fix

Hold outgoing log events until the identity has been applied (or stamp the
id at send time), and treat a `400 MISSING_FIELD` as "re-send with the id"
rather than discarding. Either change makes the first event of every session
arrive.

## How to verify a fix

- In the browser: DevTools → Network, filter `log/submit`; the first POST of
  a session should return 201.
- On the log service host: `journalctl -u stratalog | grep MISSING_FIELD`
  should stop growing after the new build is deployed.
