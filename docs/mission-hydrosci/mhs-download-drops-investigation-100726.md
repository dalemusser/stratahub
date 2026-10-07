# MHS — "8 of 13 could not download Unit 1": the school whose network cuts transfers

**Date:** 2026-10-07 (investigation, fixes and CDN change all the same night)
**Status:** FIXES LIVE, VERIFICATION PENDING. StrataHub commit `4e8c194` (SW 1.0.16)
in production 02:14 PDT; CDN CORS corrected in the AWS console 02:35 PDT; the reply
to the teacher and the district tech is drafted (outside the repo) and not yet sent.
Hosts, the school, the teacher and the students are not named here (public repo);
the memory entry for this work has them.

## 0. Status and how to resume

What is done:

- Cause established from production records (§2–§3): the school's network path cuts
  long HTTPS transfers from the content CDN under classroom load; our direct download
  resumed each cut but gave up after five attempts per file.
- Service worker 1.0.16 and page changes (§4) deployed; all tests pass
  (`tests/js/sw-fallback-drops.test.mjs`, `TestUnitsPageScriptsParse`, the
  missionhydrosci package).
- CDN: OPTIONS allowed on both behaviours of the content distribution, S3 CORS
  exposes `ETag`, `Last-Modified`, `Content-Range`, `Accept-Ranges`; `/*` invalidated.
  Verified: preflights 200, Unity-style conditional GET 304, `Range`+`If-Range` 206.
- Plan items: DL-9 done, DL-10 open (`mhs-remaining-work-plan.md`).

To resume, in this order:

1. **Was the reply sent?** If not, it is on the Desktop
   (the reply draft dated 2026-10-07); the "what we are fixing" paragraph can now
   say the CDN change is live.
2. **Read the school's next class** (§5): download outcomes on the Devices tab, the
   StrataHub journal's `mhs download error` lines (they now carry the drop count), and
   the step logs' "picked up at … (N drops so far)". Success looks like slow downloads
   with many drops and no `download-failed` records; failure looks like `network` /
   `fallback` errors whose `rawError` says "N dropped connections resumed, then: …".
3. **Count `bgfetch-refused` fleet-wide** (§6.1). This is the first telemetry that says
   whether — and why — Background Fetch is refused on managed student Chromebooks.
4. Check the second-worst site (§6.2) improved too.
5. If the school still fails: their tech's answers to the two checks in the reply
   (TLS issuer of the CDN; three large curls during a class) decide whether the fix is
   theirs (filter bypass) or there is more for us to do.

## 1. The report

A teacher wrote that in one class 8 of 13 students could not get Unit 1 to download;
clearing the cache and resetting the Wi-Fi had not helped. The district tech forwarded
a HAR analysis: a reload failed on cache revalidation — `If-Modified-Since` and
`Cache-Control` headers triggered a CORS preflight that CloudFront answered with 403 —
and proposed allowing OPTIONS and those headers on the CDN.

## 2. What the records showed

All times Central (the school's zone). Two classes on the Monday: 12:17–12:52 pm (13
students) and 2:06–2:44 pm (10). Managed Chromebooks (Celeron / UHD 600, 4 GB RAM, 2
cores, Chrome 152/154), ~2.1–2.5 GB of browser storage quota, storage not persisted,
one public IP.

Every failure was the same event. Journal (`mhs download error`): 25 lines for the
school, all `error_class: network`, `path: fallback`, "Cache.put() encountered a
network error". The step logs show the shape:

```
12:17:53  11% · 14 of 132 MB · 5.7 MB/s
12:18:04  51% · 68 of 132 MB
12:18:11  picked up at 80 MB after a dropped connection
12:18:18  picked up at 96 MB after a dropped connection
12:18:25  picked up at 112 MB after a dropped connection
12:18:35  FAIL  The download was interrupted…          {"path":"fallback"}
```

The first connection carried 50–70 MB; each resumed connection died after 1–3 s and
8–16 MB. `fetchAndCacheFileWithRetry` allowed five attempts per file, so the unit
failed at ~85% with the red "security-software" message, the page's never-give-up
retry waited 5/10/20/30/60 s and started over from the saved parts, and students either
converged after a few rounds (18 download-complete) or closed the tab (29
download-failed). The same device that failed at 12:18 finished its remaining 52 MB at
12:40 in 6 s with no drops, then a 214 MB unit in 29 s.

The teacher's own device test the Friday before (single Chromebook, school Wi-Fi) had
shown it already: Background Fetch received 0 bytes for 25 s → switched to direct →
reached 201 of 214 MB ("stuck at 94%" in her problem report) → every re-request for
the tail refused for four minutes. A second device later downloaded the whole unit in
18 s, then stalled at "Loading assets 0%" and hit "Unity failed to start" four times.

Cross-school comparison, member records since 2026-09-15 (dropped-connection resumes
per record): this school **3.2**; the second-worst site 1.6; the other Chromebook
schools 0.11, 0.09, 0.01; Windows/Mac sites ~0. CDN and API reachability checks from the
failing devices passed in 150–200 ms every time.

A second finding fell out of the comparison: on Chrome 150+ at **every** Chromebook
school, no record shows Background Fetch activity (0 frozen-switch events in 2,000+
records, against 64 of 124 on Chrome 147), while "picked up" resumes appear with no
switch step — the worker's `backgroundFetch.fetch()` rejects and `startBackgroundFetch`
fell back silently, so every record said `downloadMode: background` while the download
ran in the tab and died with it. Background Fetch works in Chrome 154 on an unmanaged
Mac, so this is device/policy-specific; the reason is unknown until the new telemetry
(§4) reports it.

## 3. Cause

1. **Theirs:** something on the school's egress path (most likely the web filter /
   firewall inspecting or scanning HTTPS downloads, possibly plus a saturated access
   point) terminates long transfers from the CDN under load and sometimes withholds
   the tail of a file. Asked of the district: decryption/scanning bypass for the CDN,
   app and API hosts; an `openssl s_client` issuer check and three large `curl`s during
   a class.
2. **Ours:** a five-attempt budget that failed a converging download; resume parts
   flushed only every 8 MB, so a connection cut under 8 MB saved nothing; the
   silent Background Fetch fallback.
3. **The tech's HAR (launch-side, real, second-order):** the CDN answered every
   OPTIONS preflight 403 (behaviours allowed GET/HEAD only) and exposed only
   `Content-Length`/`Content-Type`. The deployed Unity loader's default
   `cacheControl` is `must-revalidate` for the `.data` file: it keeps a copy in
   IndexedDB and, for a same-origin URL, revalidates with `If-Modified-Since` /
   `If-None-Match` + `Cache-Control: no-cache` — headers outside the CORS safelist. When
   the service worker does not answer from its cache (hard reload, uncontrolled page,
   cleared or evicted cache) the request follows our 302 to the CDN, is preflighted,
   and fails: "Unity failed to start". It also stored each launched unit twice (~120 MB
   extra per unit on 2 GB-quota devices). The Monday failures were not this: those
   devices had never run the game and sent no conditional headers. Note the interplay
   that made the CDN change all-or-nothing: `Range` with a simple value is safelisted
   (resumes worked), `If-Range` is not and was never sent only because `ETag` was not
   exposed — exposing `ETag` without allowing `If-Range` would have broken resumes.

## 4. What changed

Service worker **1.0.16** (`static/sw-background-fetch.js`, `static/sw.js`):

- `fetchAndCacheFileWithRetry` gives up only after `MAX_FUTILE_ATTEMPTS` (6)
  consecutive attempts that saved nothing new (measured from the resume-parts meta);
  a transfer without resume support keeps a fixed budget; backoff 1 s after progress,
  2·n s (≤10 s) after a futile attempt; `onDrop(info)` per retried failure.
- `writeParts` flushes the bytes received before a stream error as a short part (parts
  may differ in size; they are read in index order), so every cut moves the resume
  point. `fetchAndCacheFile` waits for both tee branches with `Promise.allSettled`
  before the caller measures progress or starts the next attempt.
- `runFallbackFetch` carries `drops` in every progress broadcast and in the resume
  (`resumedFrom`) broadcast; a final failure's `rawError` reads "N dropped connections
  resumed, then: …" and the detail has `drops`.
- `startBackgroundFetch`'s catch broadcasts `{status:'method', detail:{path:'fallback',
  reason, version}}` before falling back.
- `serveMHSContent` cache miss: `fetch(new Request(url, {method: GET|HEAD, mode:'cors',
  credentials:'omit'}))` — the page's conditional headers never reach the redirect.

Pages (`mhs-delivery.js`, templates):

- `_handleStatusUpdate` routes `method` to `_onDownloadMethod` (never to pages'
  status callbacks): marks the stall state `fallback`, `_preferFallback(true)` (the
  mode notice flips to "keep this tab open" on the next broadcast; the next download
  goes direct at once), step `method/warn` with the reason, `_reportDownloadError`
  with `errorClass: 'bgfetch-refused'`, `path: 'background'`.
- "picked up at … after a dropped connection (N drops so far)" is logged for the first
  three drops and every fifth; `mhsProgressLabel` (units, device test) appends
  "connection dropped N times — resuming" (blue, not amber: it is being handled).
- Play page: `cacheControl: function () { return 'no-store'; }` in
  `createUnityInstance` (the loader's `cachedFetch` is enabled only for
  `must-revalidate`/`immutable`, so this is a plain fetch with no IndexedDB).
- `MHSDeliveryManager.prototype.dropEngineCache()` deletes IndexedDB databases named
  `UnityCache*`; called after init on the units, manage and device-test pages (not the
  play page).

CDN (console, not code): content distribution behaviours `Default (*)` and `*/*.wasm`
allow GET, HEAD, OPTIONS (origin request policy was already the managed CORS-S3Origin,
which forwards `Origin` and the two `Access-Control-Request-*` headers); bucket CORS
`ExposeHeaders` = Content-Length, Content-Type, ETag, Last-Modified, Content-Range,
Accept-Ranges (AllowedHeaders was already `*`); `/*` invalidated because CloudFront
caches the CORS headers with the object. The test CDN host got the same. A runbook with
inspect/apply/rollback/verify scripts lives outside the repo (`cdn_cors_update/` beside
the deploy folders); there is no AWS CLI or credential on the dev Mac and the EC2 role
has no CloudFront rights — the console is how CDN changes get made.

## 5. Verifying at the school's next class (DL-10)

- **Devices tab** of the dashboard for the school's groups: unit cells should turn
  downloaded; a device still on the error badge is the signal to dig.
- **Journal** on the app host (local time of that server):
  `sudo journalctl -u stratahub --since "<date> 00:00" --no-pager -o cat | grep "mhs download error"`
  — filter on the school's user-id prefixes (two groups, two prefixes; see memory).
  Read `error_class`, `path`, `message`/`rawError` (drop count), `storage_*`.
- **Step logs** of their launch records (`mhs_device_tests`, `kind: member`,
  `user_id` ObjectID, `started_at`, `steps[].msg`): look for "picked up at … (N drops
  so far)" lines followed by `download/ok` rather than `download/fail`. The query
  scripts used for §2 are in §7.
- **Expected if the network is unchanged:** downloads finish, slowly, with drop counts
  in the tens; no `download-failed` records. **Expected if the district applied the
  bypass:** no drops at all.

## 6. Open questions

### 6.1 Why is Background Fetch refused on managed student Chromebooks?

Count and read the reasons once a school day has passed:
`journalctl -u stratahub … | grep "mhs download error" | grep bgfetch-refused` — the
`message` field carries the browser's rejection text; group by user agent. Hypotheses:
a Chrome download policy on the student OU (e.g. "block all downloads", which the
download request limiter may apply to Background Fetch), or a Chrome 150+ change for
enterprise-managed devices. If it is policy, the teacher guide and the district asks
should say so; if it is universal on Chrome 150+, the default copy ("you can close this
page") is wrong for every Chromebook and the units page should lead with the direct
path there.

### 6.2 The second-worst site

The same signature at lower intensity (1.6 drops per record, 20 of 64 downloads failed
in the window). The fix applies unchanged; confirm its failures stop.

### 6.3 Storage on the school's Chromebooks

~2.1–2.5 GB quota, `persisted: false`. Five units (788 MB) now fit without Unity's
duplicates; eviction under disk pressure would still mean a re-download. Installing the
PWA (persisted storage) on those carts is the mitigation if it shows up.

## 7. Query recipes used (read-only; run per the prod read-only recipe in memory)

Per-organization comparison (member records since a date; drops = "picked up"
steps, failure path from the fail step's detail):

```js
const db2 = db.getSiblingDB('stratahub');
const since = new Date('2026-09-15T00:00:00Z');
const orgName = {}; db2.organizations.find({}, {name:1}).forEach(o => orgName[o._id.toHexString()] = o.name);
const stats = {};
db2.mhs_device_tests.find({kind:'member', started_at:{$gte: since}},
  {organization_id:1, user_agent:1, diagnostics:1, 'steps.msg':1, 'steps.step':1, 'steps.state':1, 'steps.detail':1}).forEach(r => {
  const org = r.organization_id ? (orgName[r.organization_id.toHexString()] || '?') : '(none)';
  const s = stats[org] = stats[org] || {recs:0, dlok:0, dlfail:0, drops:0, failFallback:0, switched:0};
  s.recs++;
  const o = r.diagnostics && r.diagnostics.outcome;
  if (o === 'download-complete') s.dlok++; if (o === 'download-failed') s.dlfail++;
  for (const st of (r.steps||[])) {
    if (/picked up at .* after a dropped connection/.test(st.msg||'')) s.drops++;
    if (/switched to the direct download/.test(st.msg||'')) s.switched++;
    if (st.step === 'download' && st.state === 'fail' && st.detail && st.detail.path === 'fallback') s.failFallback++;
  }
});
for (const [k, s] of Object.entries(stats).sort((a,b) => b[1].recs - a[1].recs)) print(k.padEnd(28), JSON.stringify(s));
```

One student's records with step logs (Central time; `user_id` is an ObjectID):

```js
const ids = ['<24-hex user id>'].map(h => ObjectId(h));
db2.mhs_device_tests.find({kind:'member', user_id:{$in: ids}, started_at:{$gte: new Date('<ISO>')}}).sort({started_at:1}).forEach(r => {
  const ct = d => new Date(d.getTime() - 5*3600*1000).toISOString().slice(11,19);
  print('=== ' + ct(r.started_at) + ' ' + r.unit_id + ' v' + r.unit_version + ' stage=' + r.stage + ' failed_step=' + (r.failed_step||''));
  if (r.diagnostics) print('  ' + JSON.stringify(r.diagnostics).slice(0, 400));
  for (const s of (r.steps||[])) print('  ' + ct(s.at) + ' ' + s.step + '/' + s.state + ' ' + String(s.msg||'').slice(0, 200) + (s.detail ? ' ' + JSON.stringify(s.detail) : ''));
});
```

Device-test runs from the school's address or name: `kind: 'devicetest'` with
`remote_ip` or `form.school` (regex).

CDN probes (no AWS access needed): `cdn_cors_update/verify.sh` outside the repo, or
by hand — `curl -X OPTIONS -H 'Origin: https://<app-host>' -H 'Access-Control-Request-Method: GET' -H 'Access-Control-Request-Headers: if-modified-since,cache-control' https://<cdn-host>/mhs/unit1/v<ver>/Build/unit1.loader.js`
must answer 200 with `access-control-allow-headers`.
