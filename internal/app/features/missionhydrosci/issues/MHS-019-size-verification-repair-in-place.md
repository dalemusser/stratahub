# MHS-019 — Size-verification blob fallback re-reads huge files every check; repair the entry instead

**Priority:** P2 (severe only under a persistent header mismatch, but the
fix is cheap and the failure mode is a device-killing perf cliff)
**Status:** **Fixed 2026-07-06.** After a successful blob verification, both
verifiers (`_verifyLargestFile` in mhs-delivery.js, `verifyLargestFileSize`
in sw-cache.js) rewrite the entry with a corrected `content-length` so every
future check takes the header fast-path. Repair failures are swallowed —
verification already passed, the rewrite is only an optimization.
**Area:** `internal/app/resources/assets/js/mhs-delivery.js`
(`_verifyLargestFile`), `internal/app/features/missionhydrosci/static/sw-cache.js`
(`verifyLargestFileSize`)

CONFIRMED. The round-1 hardening falls back to `match.clone().blob()` —
reading the entire largest file — whenever the cached `content-length`
disagrees with the manifest size. That mismatch is **persistent for the life
of the cache entry** in the documented case (cross-origin responses where
`Content-Encoding` is invisible to CORS, so a compressed content-length is
stored against the decoded body). Then every `_checkUnitCache` — init for
all units, every visibilitychange, every recheck — pays a multi-hundred-MB
read per affected unit on a low-end Chromebook/iPad: seconds of jank, GC
pressure, possible tab kill.

## Fix

After a blob verification **succeeds**, repair the entry in place: you
already hold the bytes, so
`cache.put(key, new Response(blob, { status, statusText, headers: <copied
headers with corrected content-length> }))` — a one-time write that makes
every future check take the header fast-path. Apply identically in both
copies (client and SW). An in-memory verified-set is a cheaper per-session
alternative but doesn't survive reloads; repair-in-place does.

## Also note (same code, no separate issue)

The mismatch case is currently theoretical under the expected CloudFront
config (pre-compressed Unity files served identity, sizes recorded from the
stored bytes). This issue matters as insurance: if CDN config ever drifts,
the failure without this fix is not just wrong status — it's the perf
cliff above on every page interaction.
