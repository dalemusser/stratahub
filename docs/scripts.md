# MHS Delivery — Console Diagnostic Scripts

Copy and paste these scripts into the browser DevTools console (F12 > Console) on the MHS Units page to diagnose Background Fetch and caching issues.

---

## 1. CORS Test

Verify that cross-origin fetch to the CDN works from the browser.

```javascript
fetch('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js', { mode: 'cors' }).then(r => console.log('CORS OK:', r.status)).catch(e => console.error('CORS FAIL:', e));
```

---

## 2. Single File Background Fetch (Small)

Tests Background Fetch with a single small file (47KB loader.js). This isolates whether Background Fetch works at all for cross-origin requests.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-single', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js', { mode: 'cors' })], { title: 'Test: 1 small file', downloadTotal: 47338 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 3. Three Build Files (No Data File)

Tests Background Fetch with the 3 smaller Build files (loader + framework + wasm, ~13MB total). Excludes the large 90MB data file to test whether the number of files or their sizes causes stalling.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-3build', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.framework.js.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.wasm.unityweb', { mode: 'cors' })], { title: 'Test: 3 Build files (~13MB)', downloadTotal: 13026439 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 4. Data File Only (Large ~90MB)

Tests Background Fetch with just the large data file to determine if file size is the issue.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-data', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.data.unityweb', { mode: 'cors' })], { title: 'Test: data file only (~90MB)', downloadTotal: 90702588 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 5. All 4 Build Files

Tests Background Fetch with all 4 Build files (~104MB total, no StreamingAssets). Helps determine if the issue is the Build files or the StreamingAssets files (which have parentheses in filenames).

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-4build', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.framework.js.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.data.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.wasm.unityweb', { mode: 'cors' })], { title: 'Test: 4 Build files (~104MB)', downloadTotal: 103729027 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 6. StreamingAssets Only (Parentheses in Filenames)

Tests Background Fetch with only the StreamingAssets files. Some filenames contain parentheses, e.g. `localization-string-tables-english(en)_assets_all.bundle`. This tests whether special characters in URLs cause Background Fetch to stall.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-streaming', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/UnityServicesProjectConfiguration.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.bin', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.hash', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/settings.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_monoscripts_314487d01e3859e453439f5fed4827fd.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_unitybuiltinassets_37a606a8c17b700cd18d4235565ff884.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-assets-shared_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-locales_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-english(en)_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-spanish(es)_assets_all_8c30b7ec75faa266b485f4df63d1af19.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/AddressablesLink/link.xml', { mode: 'cors' })], { title: 'Test: StreamingAssets only (~416KB)', downloadTotal: 416637 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 7. Check Cache Status

Lists all MHS caches and their contents.

```javascript
caches.keys().then(names => { const mhs = names.filter(n => n.startsWith('mhs-')); console.log('MHS caches:', mhs); return Promise.all(mhs.map(n => caches.open(n).then(c => c.keys().then(keys => ({ name: n, count: keys.length, keys: keys.map(k => k.url) }))))); }).then(results => results.forEach(r => { console.log(r.name + ': ' + r.count + ' entries'); r.keys.forEach(k => console.log('  ' + k)); }));
```

---

## 8. List Active Background Fetches

Shows any currently active Background Fetch registrations.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.getIds()).then(ids => { if (ids.length === 0) { console.log('No active Background Fetches'); } else { console.log('Active Background Fetches:', ids); } });
```

---

## 9. Cancel All Background Fetches

Cancels (aborts) all active Background Fetch registrations.

```javascript
navigator.serviceWorker.ready.then(async reg => { const ids = await reg.backgroundFetch.getIds(); if (ids.length === 0) { console.log('Nothing to cancel'); return; } for (const id of ids) { const bgFetch = await reg.backgroundFetch.get(id); if (bgFetch) { await bgFetch.abort(); console.log('Cancelled:', id); } } });
```

---

## 10. Delete All MHS Caches

Clears all MHS-related caches (unit caches and app shell cache).

```javascript
caches.keys().then(names => Promise.all(names.filter(n => n.startsWith('mhs-')).map(n => caches.delete(n).then(() => console.log('Deleted:', n))))).then(() => console.log('All MHS caches deleted'));
```

---

## 11. Check downloadTotal vs Actual

After a Background Fetch starts, monitors its progress and reports actual downloaded bytes vs expected total. Replace `FETCH_ID` with the actual ID (e.g., `test-3build`).

```javascript
navigator.serviceWorker.ready.then(async reg => { const bgFetch = await reg.backgroundFetch.get('FETCH_ID'); if (!bgFetch) { console.log('No Background Fetch with that ID'); return; } console.log('ID:', bgFetch.id, '| result:', bgFetch.result, '| failureReason:', bgFetch.failureReason, '| downloaded:', bgFetch.downloaded, '/', bgFetch.downloadTotal, '| recordsAvailable:', bgFetch.recordsAvailable); });
```

---

## 12. Check test-streaming Status

Check the status of the StreamingAssets Background Fetch test to see if it failed and why.

```javascript
navigator.serviceWorker.ready.then(async reg => { const bgFetch = await reg.backgroundFetch.get('test-streaming'); if (!bgFetch) { console.log('No Background Fetch with that ID (already completed or never started)'); return; } console.log('ID:', bgFetch.id, '| result:', bgFetch.result, '| failureReason:', bgFetch.failureReason, '| downloaded:', bgFetch.downloaded, '/', bgFetch.downloadTotal, '| recordsAvailable:', bgFetch.recordsAvailable); });
```

---

## 13. Direct Fetch — Parenthesized Filenames

Tests whether the files with parentheses in their names can be fetched at all via regular fetch. If these return errors, that's our culprit.

```javascript
Promise.all([fetch('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-english(en)_assets_all.bundle', { mode: 'cors' }).then(r => console.log('english(en):', r.status, r.statusText)).catch(e => console.error('english(en) FAIL:', e)), fetch('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-spanish(es)_assets_all_8c30b7ec75faa266b485f4df63d1af19.bundle', { mode: 'cors' }).then(r => console.log('spanish(es):', r.status, r.statusText)).catch(e => console.error('spanish(es) FAIL:', e))]);
```

---

## 14. StreamingAssets WITHOUT Parenthesized Files (9 files)

Same as Script 6 but excludes the two files with parentheses. If this completes, parentheses are the issue.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-stream-noparen', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/UnityServicesProjectConfiguration.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.bin', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.hash', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/settings.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_monoscripts_314487d01e3859e453439f5fed4827fd.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_unitybuiltinassets_37a606a8c17b700cd18d4235565ff884.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-assets-shared_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-locales_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/AddressablesLink/link.xml', { mode: 'cors' })], { title: 'Test: StreamingAssets no parens (9 files)', downloadTotal: 413923 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 15. ONLY Parenthesized Files (2 files)

Tests just the two files with parentheses in their filenames.

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-parens-only', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-english(en)_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-spanish(es)_assets_all_8c30b7ec75faa266b485f4df63d1af19.bundle', { mode: 'cors' })], { title: 'Test: parens only (2 files)', downloadTotal: 2714 })).then(f => console.log('BG Fetch started:', f.id)).catch(e => console.error('BG Fetch error:', e));
```

---

## 16. Full Unit 1 — All 15 Files (console test)

Tests Background Fetch with all 15 files for Unit 1, exactly as the UI would, but directly from console. Uses `downloadTotal: 0` to avoid any mismatch issues (Chrome will show indeterminate progress but won't stall waiting for a byte count).

```javascript
navigator.serviceWorker.ready.then(reg => reg.backgroundFetch.fetch('test-full-unit1', [new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.loader.js', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.framework.js.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.data.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/Build/unit1.wasm.unityweb', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/UnityServicesProjectConfiguration.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.bin', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/catalog.hash', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/settings.json', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_monoscripts_314487d01e3859e453439f5fed4827fd.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/d6f764f6c7517dce0bac8c1e4f3831d8_unitybuiltinassets_37a606a8c17b700cd18d4235565ff884.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-assets-shared_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-locales_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-english(en)_assets_all.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/WebGL/localization-string-tables-spanish(es)_assets_all_8c30b7ec75faa266b485f4df63d1af19.bundle', { mode: 'cors' }), new Request('https://cdn.adroit.games/mhs/unit1/v1.0.0/StreamingAssets/aa/AddressablesLink/link.xml', { mode: 'cors' })], { title: 'Test: Full Unit 1 (15 files)', downloadTotal: 0 })).then(f => { console.log('BG Fetch started:', f.id); f.addEventListener('progress', () => console.log('Progress:', f.downloaded, 'bytes |', f.result || 'in-progress')); }).catch(e => console.error('BG Fetch error:', e));
```

---

## 17. Monitor Any Background Fetch by ID

Polls a Background Fetch every 3 seconds and logs progress. Replace TEST_ID with the actual fetch ID.

```javascript
(async () => { const reg = await navigator.serviceWorker.ready; const id = 'test-full-unit1'; let prev = -1; const iv = setInterval(async () => { const f = await reg.backgroundFetch.get(id); if (!f) { console.log('Fetch gone (completed or cancelled)'); clearInterval(iv); return; } if (f.downloaded !== prev) { prev = f.downloaded; console.log('downloaded:', f.downloaded, '/', f.downloadTotal, '| result:', f.result || '(pending)', '| failureReason:', f.failureReason || '(none)'); } if (f.result) { console.log('DONE:', f.result, f.failureReason || ''); clearInterval(iv); } }, 3000); })();
```

---

## 18. Service Worker Version Check

Checks which version of the service worker is running.

```javascript
new Promise(resolve => { const mc = new MessageChannel(); mc.port1.onmessage = e => { console.log('SW version:', e.data.version); resolve(); }; navigator.serviceWorker.controller.postMessage({ action: 'getVersion' }, [mc.port2]); });
```

---

## 13. Trigger Unit 1 Download (via normal UI flow)

Uses the `mhsDownload()` global function exposed by the units page to trigger a Unit 1 download exactly as the Download button would. Only works on the `/mhs/units` page.

```javascript
if (window.mhsDownload) { window.mhsDownload('unit1'); console.log('Download triggered via mhsDownload'); } else { console.log('mhsDownload not available - are you on /mhs/units?'); }
```
