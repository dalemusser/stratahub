# MHS-020 — Minor fixes and cleanups from review round 2

**Priority:** P3 (none user-breaking; batch with the P0/P1 work above)
**Status:** **Fixed 2026-07-06** except the status→UI table refactor (last
carried item), deliberately deferred: reworking four working switch
statements right before a device smoke test is churn without a driving need.
20a: `refreshStorageUI()` always stamps the throttle. 20b:
`_recheckActiveDownloads` now branches on `hasBGFetch && st && !st.fallback`,
cache-reconcile the default. 20c: superseded by MHS-015's monitor
consolidation (`lastDownloaded` removed, comment fixed). Carried items:
`checkMemberAuth` core extracted (api_manifest.go), used by both
`requireMemberAuth` and `HandleSetToUnit`; dead `cleanStaleCaches` deleted;
`purgeAllMHSData` selects by explicit `unitCachePrefix`/`appShellCachePrefix`
(new option, passed by the units template).

Small items that survived verification but fell below the main findings, or
were confirmed-but-benign. Do them opportunistically while fixing
MHS-015..019 (several are fold-ins already noted in those files).

## 20a — storage-refresh throttle stamp not anchored (units template)

In the onStatus tail, `lastStorageRefresh` is stamped only in the throttled
('downloading') branch; eager refreshes don't stamp it. During the normal
pipeline (unit A fires 'cached' while unit B streams 'downloading'), each
eager refresh is followed by an immediate throttled refresh on a stale
stamp — two `navigator.storage.estimate()` pairs within a second per
transition. Fix: one `refreshStorageUI()` helper that always stamps, called
when `status !== 'downloading' || Date.now() - lastStorageRefresh > 10000`.

## 20b — `_recheckActiveDownloads` branch clarity (confirmed-benign)

`if (!hasBGFetch || (st && st.fallback))` routes a genuine BG download
through the cache-reconcile branch when `_waitForSW` times out transiently.
Verified benign (that branch only acts on 'cached', which is then correct),
but it is confusing to reason about. Prefer branching on `st && st.fallback`
alone, keeping cache-reconcile as the default when state is missing.

## 20c — fallback stall state carries dead fields; stale struct comment

`lastDownloaded` is written but never read on the fallback path, and the
constructor comment still describes the removed `{ lastDownloaded,
stallCount }` shape. Trim the fallback state to
`{ lastProgressAt, maxDownloaded, stalled, fallback: true }` and update the
comment. (Superseded if MHS-015's monitor consolidation lands first.)

## 20d — duplication fold-ins (tracked in their parent issues)

- Monitor scaffold + stalled-fire extraction → MHS-015.
- Convert the two inline 'downloading'-fire blocks (reconnect paths) to
  `_reportDownloadProgress` → MHS-015.
- Heartbeat protocol centralization into mhs-delivery.js → MHS-018.
- Heartbeat map prune-on-write → MHS-018.

## Carried from round 1 (still open, unchanged)

- `requireMemberAuth` (api_manifest.go) duplicates HandleSetToUnit's inline
  auth block (progress.go) — extract a transport-agnostic core used by both.
- `cleanStaleCaches` in sw-cache.js is dead code lacking the in-flight-
  download guard — delete it.
- `purgeAllMHSData` selects caches by `fetchIdPrefix` (a Background-Fetch-ID
  namespace repurposed as the cache-family namespace) — works with the
  shipped config, latent footgun with library defaults; select by explicit
  cache identities instead.
- 'stalled' (and statuses generally) are hand-mapped in four separate
  switch statements (three in units template, one in play overlay) — a
  status→UI table would make new statuses one-row changes.
