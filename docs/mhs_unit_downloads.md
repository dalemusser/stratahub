# MHS Manual Unit Download Bug

## Date
2026-03-11

## Summary
Manually downloading units from the "Manage downloads" section on the Mission HydroSci units page results in the download completing successfully but then immediately reverting to "Not downloaded." The root cause is the `autoCleanup` call in `runPipeline()`, which only preserves `currentUnit` and `nextUnit` and deletes everything else — including units the user just manually downloaded.

## Affected Files
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` — inline JS `runPipeline()` function (lines ~459–493)
- `internal/app/resources/assets/js/mhs-delivery.js` — `autoCleanup()` method (lines ~575–589)

## Reproduction
1. Play through Unit 1 and Unit 2 so that Unit 3 auto-downloads
2. Return to the units page — `currentUnit` = "unit3", `nextUnit` = "unit4"
3. Expand "Manage downloads" and click Download on Unit 5
4. The download completes (progress bar fills, status briefly shows "Downloaded")
5. Immediately reverts to "Not downloaded" with a Download button

## Root Cause

### The pipeline's autoCleanup strategy

`runPipeline()` in `missionhydrosci_units.gohtml` is the auto-download pipeline. After confirming the current unit is cached, it runs cleanup:

```javascript
// lines ~482–488
if (currentStatus === 'cached') {
  if (nextUnit) {
    // ...
    var keep = [currentUnit, nextUnit];
    manager.autoCleanup(keep);   // Only keeps current + next
  } else {
    manager.autoCleanup([currentUnit]);
  }
}
```

`autoCleanup` in `mhs-delivery.js` deletes the cache for any unit NOT in the `keepUnitIds` array:

```javascript
MHSDeliveryManager.prototype.autoCleanup = async function(keepUnitIds) {
  // ...
  for (var j = 0; j < this.manifest.units.length; j++) {
    var unit = this.manifest.units[j];
    if (keepSet[unit.id]) continue;
    var status = await this._checkUnitCache(unit);
    if (status === 'cached' || status === 'partial') {
      await this.deleteUnit(unit.id);   // Deletes cache, fires 'not_cached'
    }
  }
};
```

### The chain of events

When a manual download completes, the status callback fires with `status === 'cached'`:

```javascript
// line ~451
if (status === 'cached' && !isReloading) {
  runPipeline();
}
```

This triggers `runPipeline()`, which calls `autoCleanup(["unit3", "unit4"])`. Since unit5 is not in the keep list, `autoCleanup` finds it cached and deletes it via `deleteUnit()`. `deleteUnit` fires `_fireStatus('unit5', 'not_cached', {})`, which updates the UI back to "Not downloaded."

### Why unit4 (nextUnit) should survive

Unit4 IS in the keep list `["unit3", "unit4"]`, so `autoCleanup` skips it. Manual download of the `nextUnit` should work correctly. The bug specifically affects units beyond `nextUnit` (e.g., unit5 when current=unit3, next=unit4).

### Design conflict

The `autoCleanup` strategy was designed for the automatic download pipeline — the idea being that only the current and next units need to be cached at any time to conserve device storage. However, the "Manage downloads" section exposes manual download buttons for all non-completed units, creating a conflict: the user can download any unit, but the pipeline immediately deletes anything outside its two-unit window.

## Fix Considerations

The fix needs to reconcile the auto-cleanup storage strategy with the manual download capability. Options include:

1. **Track manually downloaded units** — maintain a set of user-requested downloads and include them in the `autoCleanup` keep list
2. **Don't run autoCleanup after manual downloads** — only run cleanup from the automatic pipeline flow, not when a manual download triggers `runPipeline`
3. **Remove autoCleanup from runPipeline entirely** — let users manage storage manually via the Clear buttons in "Manage downloads", and only clean up stale version caches (handled by `cleanStaleCaches` in the service worker)
4. **Disable manual download for units beyond nextUnit** — hide the Download button for units that autoCleanup would immediately delete (avoids user confusion but reduces functionality)
