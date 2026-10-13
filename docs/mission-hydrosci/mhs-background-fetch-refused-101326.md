# MHS — "background fetch refused": what it is and what it means

**Date:** 2026-10-13
**Status:** EXPLANATION, nothing broken for students. A decision is pending (retire
Background Fetch, or move the call into the page); it is tracked in
`mhs-remaining-work-plan.md` (DL-10). This answers open question §6.1 of
`mhs-download-drops-investigation-100726.md`. Schools, hosts and people are not named
here (public repo); the two schools below are the ones in that investigation.

The "background fetch refused" marker is Chrome turning down our first-choice download
path, and the system quietly switching to the second. It is not a failure for the
student, but it changed the ground rules under the download system.

## 1. The two download paths

- **Background Fetch** was the design's first choice. The browser's download service
  takes the list of unit files and fetches them at the OS level, so the download
  survives closing the tab, shows in Chrome's download tray, and resumes after sleep.
  The units page copy says "you can close this page" for this mode.
- **Direct download** is the fallback. The service worker fetches the files one by one
  with ordinary requests and stores them in the cache. It only runs while a tab on the
  site stays open.

Our service worker makes the Background Fetch call, not the page, because Chrome
streams live progress only to the context that created the fetch. A page loses that on
any reload, so the worker creates it and broadcasts progress to every page
(`internal/app/features/missionhydrosci/static/sw-background-fetch.js`,
`startBackgroundFetch`).

## 2. What Chrome did

Chrome 149 began rejecting the call from service worker contexts. The rejection is
immediate and reads:

```
NotAllowedError: Failed to execute 'fetch' on 'BackgroundFetchManager':
backgroundFetch.fetch() is not allowed in service worker environments
```

Confirmed against the Chromium source on 2026-10-13: the policy
`RestrictBackgroundFetchFromServiceWorkerEnabled` defaults to Enabled, is supported from
Chrome 149 on desktop, ChromeOS and Android, and is marked temporary, "will be removed
after M152". So from Chrome 153 on, the restriction is permanent and no district setting
can lift it. Calling Background Fetch from a page is still allowed.

## 3. What our code does when refused

1. The worker catches the rejection, broadcasts a `method` message with the reason, and
   starts the direct download at once. No time is lost.
2. The page (`mhs-delivery.js`, `_onDownloadMethod`) logs the step line
   "The browser refused the background download (…) — using the direct download; keep
   this tab open", posts a `bgfetch-refused` download error to the server journal
   (`mhs download error`, `error_class: bgfetch-refused`), and flips its copy to "keep
   this tab open".
3. The page remembers the refusal for one hour (`PREFER_FALLBACK_MS`) and goes straight
   to the direct path for the rest of that period. A fresh browser profile the next day
   asks again, which is why every student-day at the school whose Chromebooks get a new
   browser profile each period produces a new line.

Until service worker 1.0.16 (2026-10-07) this was invisible. The worker fell back
silently and every record said "background" while the download was actually dying with
the tab.

Where it shows up now: the step log of a launch record (`method/warn`), the journal line
above, the per-record summary (`internal/app/system/mhssteplog`, `KindBgfetchRefused`),
and the MHS Activity page (`/mhsactivity`) as the gray pill "background fetch refused
(N)" on a school row, counting students whose browser refused it in the window.

## 4. What the data showed for 2026-10-07 to 2026-10-09

| Measure | Value |
|---|---|
| Step lines "refused", all nine active schools | 875 |
| Journal lines / students / devices | 338 / 210 / 287 |
| Chrome versions refusing | 149 through 155 on ChromeOS, Windows and macOS |
| Launch records direct vs background | 1253 vs 171 |

The 171 background records are the second school of the investigation and a few
Chrome 147 devices elsewhere. That school's Chromebooks run Chrome 138 LTS, which still
allows the call. There the Background Fetch starts, receives zero bytes for 25 seconds,
and our frozen-switch hands off to the direct path. That is the separate
`bgfetch-frozen` flag, and it costs every download at that school half a minute before
it begins.

## 5. What it means

The direct path has carried every successful download at every school for weeks, so
refusal is not hurting students. That is why the activity page shows it as a gray,
informational pill rather than amber. The real losses are the ones the design promised:
downloads no longer survive a closed tab or a sleeping lid, and the teacher guide's
"download before class, then close it" workflow is wrong for any Chrome newer than 148.

The pending decision (DL-10 in the remaining-work plan): move the call into the page,
which Chrome still permits but brings back the frozen-progress-on-reload and
paused-on-battery-saver problems that pushed it into the worker, or retire Background
Fetch and lead with the direct path everywhere. The data favors retiring it. It would
also remove the Chrome 138 school's 25-second freeze and let the page copy be honest
from the first second.

## Sources

- [RestrictBackgroundFetchFromServiceWorkerEnabled.yaml in the Chromium tree](https://chromium.googlesource.com/chromium/src/+/main/components/policy/resources/templates/policy_definitions/Miscellaneous/RestrictBackgroundFetchFromServiceWorkerEnabled.yaml)
- [Introducing Background Fetch (Chrome Developers)](https://developer.chrome.com/blog/background-fetch)
