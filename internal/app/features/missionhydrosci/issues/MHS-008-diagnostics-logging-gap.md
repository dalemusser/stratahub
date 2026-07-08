# MHS-008 — Diagnostics gap: can't see cache inventory / failure reasons server-side

**Priority:** P3
**Status:** Documented

## Summary

When a device fails to download, there is no server-side visibility into *why*:
which unit-version caches exist, how big they are, whether Background Fetch was
available, whether storage was persisted, or what the failure reason was. The
existing device-status report captures quota/usage and per-unit status but not
enough to diagnose the accumulation and failure patterns in MHS-001..006.

> Dale (Discord, 6/30/26): "I may also need to add some logging so I can capture
> what is happening."

## Current state

`POST /api/device-status` already stores a useful baseline:

- Handler: `device_status.go:27-78`
- Model: `internal/domain/models/mhs_device_status.go` (via
  `store/mhsdevicestatus`)
- Client report: `templates/missionhydrosci_units.gohtml:427-455` — sends
  `storage_quota`, `storage_usage`, `pwa_installed`, `sw_registered`,
  `unit_status`, and rich `device_details`.

Gaps:
- No inventory of **which caches actually exist** (unit + version + size), so
  orphaned old-version caches (MHS-001) are invisible server-side.
- No record of download **failure reasons** (quota exceeded, network, BG fetch
  unavailable) or **which download path** was used.
- No record of `navigator.storage.persisted()` (MHS-005) or Background Fetch
  availability (MHS-004).

## Proposed fix

Extend the device-status payload and model with:
- `cache_inventory`: list of `{ name, unit_id, version, file_count, bytes }` for
  every `missionhydrosci-*` cache (enumerate `caches.keys()` and sum sizes).
- `background_fetch_available`: boolean.
- `storage_persisted`: boolean (`navigator.storage.persisted()`).
- `last_download` events: `{ unit_id, version, path: 'bgfetch'|'fallback',
  outcome, reason }`, reported on terminal statuses.

Then a lightweight admin/report view (or an existing MHS admin surface) can show
per-device cache bloat and failure trends.

## Affected code

- `internal/app/features/missionhydrosci/device_status.go`
- `internal/domain/models/mhs_device_status.go`
- `internal/app/store/mhsdevicestatus/`
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`
- `internal/app/resources/assets/js/mhs-delivery.js`

## Risk / notes

- Keep the payload bounded (cap event list length) to avoid large reports.
- This is observability, not a fix — but it validates the MHS-001..006
  hypotheses with real device data and confirms the guest-mode Background Fetch
  behavior (MHS-004).
- The `cache_inventory` should also help confirm whether anything **other** than
  our unit caches is consuming the origin quota (e.g. Unity's own asset cache).
  Game save data / prefs are stored server-side in `stratasave`, so they do not
  count against local quota — but Unity's WebGL loader may maintain its own
  IndexedDB asset cache; worth confirming from the estimate-vs-inventory delta.
- De-identification: device-status is keyed by `user_id` (ObjectID); keep any new
  fields free of PII (no login IDs, no emails).
</content>
