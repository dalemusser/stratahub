# Why PWA: The Case for Progressive Web App Delivery

## Overview

Mission HydroSci is a Unity WebGL game deployed to school Chromebooks and iPads. A standard Unity WebGL build produces static files (HTML, JS, WASM, data) that can be hosted on any web server and opened in a browser. This document explains why wrapping that build in a Progressive Web App served through StrataHub produces a fundamentally better product than serving the raw Unity build output directly.

The reasons fall into three categories: problems that can only be solved in the PWA layer, problems that are dramatically easier to solve there, and capabilities that don't exist without it.

---

## Problems That Can Only Be Solved in the PWA Layer

### 1. Offline Play on School Chromebooks

School Chromebooks have unreliable Wi-Fi. 30 students connecting simultaneously to download 100–200 MB per unit saturates the network. A raw Unity WebGL build streams all assets from the server on every launch — if the network is slow or down, the game doesn't load.

The PWA uses the **Background Fetch API** and **Cache API** to download all unit files once, store them locally, and serve them from cache on every subsequent launch. The service worker intercepts all content requests and serves cached files instantly, with zero network dependency. Students download once, play forever.

A raw `index.html` build cannot register a service worker, cannot use Background Fetch, and cannot manage the Cache API. There is no path from a static Unity build to offline-capable delivery without the PWA infrastructure.

### 2. Offline Play on iPads

Safari does not support the Background Fetch API. The PWA's service worker detects this and falls back to a sequential fetch strategy that downloads files through the service worker context and writes them to the Cache API. This fallback is invisible to the student — they tap Download, see a progress bar, and the unit is cached.

This fallback requires a service worker with custom download logic, a BroadcastChannel for progress communication back to the page, and a manifest-driven file list from the server. None of this exists in a raw Unity build.

### 3. User Identity Without Cross-Origin Cookies

Unity's built-in authentication calls `/api/user` on the game's origin (adroit.games). When the game is served through StrataHub (a different origin), that request fails because cookies don't cross origins.

The PWA play page includes an **identity bridge** — JavaScript that intercepts both `XMLHttpRequest` and `fetch` calls to `/api/user` and returns the authenticated user's identity directly from server-rendered data in the page. The user's name, email, and login ID are injected by StrataHub's Go backend using the active session. The game receives a valid user object without any cross-origin request ever being made.

A raw Unity build served from adroit.games or a CDN has no access to StrataHub's session data and no mechanism to inject it. The game either has no user identity or requires a separate authentication flow that students would need to complete manually.

### 4. iOS Audio Lifecycle Management

Unity WebGL does not manage AudioContext lifecycle in response to page visibility events on iOS. This causes three concrete problems: audio continues playing when the app is backgrounded, audio goes permanently silent when the user returns from the app switcher, and "zombie audio" from closed games plays over other pages when iOS restores pages from the back-forward cache.

The PWA play page wraps the AudioContext constructor to track all instances Unity creates, suspends them when the page becomes hidden, resumes them when the page becomes visible, and permanently closes them when the user navigates away. This is detailed in [ios_audio.md](ios_audio.md).

A raw Unity build has no awareness of iOS PWA lifecycle events and no mechanism to add this behavior without modifying the Unity build pipeline or the generated JavaScript, which would need to be repeated on every build.

### 5. iOS Fullscreen Keyboard Input

iPad Safari in fullscreen mode suppresses all letter key DOM events (A–Z). WASD movement and the M key (map) stop working entirely. This is a Safari/WebKit limitation with no JavaScript workaround — the events never enter the DOM. This is detailed in [ios_key_input.md](ios_key_input.md).

The PWA running in standalone mode (Add to Home Screen) provides a near-fullscreen experience where all keyboard input works normally. The PWA play page detects iPad and standalone mode and hides the fullscreen button to prevent students from accidentally entering the broken Safari fullscreen mode.

A raw Unity build has no platform detection, no ability to hide UI elements conditionally, and no alternative to Safari's broken fullscreen implementation.

### 6. Unit-to-Unit Navigation Control

When a student finishes a unit, the Unity game attempts to navigate to the next unit's URL on adroit.games. In the StrataHub context, this would take the student out of the managed environment entirely — leaving the authenticated session, the offline cache, and the download management behind.

The PWA play page intercepts both `window.location` changes (via the Navigation API on Chromebooks) and `window.open` calls (used by Unity's `Application.OpenURL`). When an external game URL is detected, navigation is blocked and a "Unit Complete" overlay is shown, directing the student back to the Units page where they can launch the next unit through the managed system.

A raw Unity build controls its own navigation. Preventing it requires wrapping the page.

---

## Problems That Are Dramatically Easier to Solve in the PWA Layer

### 7. Storage Management and Pre-Flight Checks

All 5 units total approximately 700 MB. Many school Chromebooks have 16–32 GB of total storage. The PWA Units page shows a storage quota bar using `navigator.storage.estimate()`, letting students and teachers see how much space is available before downloading. The download manager can check available space against a unit's size and show a clear message if there isn't enough room.

### 8. Download Progress and Status

The PWA Units page shows real-time download progress for each unit, cache status (downloaded / not downloaded / partial), and provides download, play, and clear buttons per unit. The service worker communicates progress back to the page via BroadcastChannel. If a download was in progress when the page was closed, the delivery manager reconnects to the active Background Fetch and resumes showing progress.

### 9. CDN Content Delivery with Cache-First Serving

Content files are stored on S3 behind CloudFront. The service worker implements a cache-first strategy for game content — if the file is in the local cache, it's served instantly without any network request. If not cached, the request falls through to a Go handler that 302-redirects to the CDN. This means:

- Cached units load in milliseconds regardless of network conditions
- Uncached units transparently load from the nearest CloudFront edge
- The student never sees a CDN URL or knows about the infrastructure

### 10. Version Management

Each unit has a version string embedded in its cache name (e.g., `missionhydrosci-unit-unit1-v1.0.0`). When a unit is updated, the version changes, the old cache is automatically cleaned up, and the student downloads the new version. The manifest API drives this entirely from the server — no client-side code changes needed for content updates.

---

## Capabilities That Don't Exist Without the PWA

### 11. Installable App Experience

On Chromebooks and iPads, the PWA can be installed (Add to Home Screen / Install App). This creates a dedicated app icon, launches in its own window without browser chrome, and provides the experience of a native app. For students, it's "tap the icon and play" — no URL to remember, no browser tabs, no address bar.

### 12. Service Worker as a Programmable Network Layer

The service worker is a programmable proxy that sits between the page and the network. This single capability enables offline play, cache management, background downloads, content routing, and version control. It is the foundational technology that makes everything else possible. Without it, every content request goes to the network, every time, with no local intelligence.

### 13. Platform-Specific Adaptation

The PWA play page adapts its behavior based on the platform:

| Behavior | Chromebook | iPad Safari | iPad PWA |
|---|---|---|---|
| Fullscreen button | Shown | Hidden (breaks WASD) | Hidden (already fullscreen) |
| Back button | Not needed (browser back) | Not needed (browser back) | Shown (no browser chrome) |
| Keyboard Lock API | Used in fullscreen | Not supported | N/A |
| Download method | Background Fetch | SW fallback fetch | SW fallback fetch |
| Audio lifecycle | Managed by browser | Managed by PWA | Managed by PWA |
| Navigation interception | Navigation API | window.open override | Both |

A raw Unity build is one-size-fits-all. The PWA layer is where platform differences are resolved.

### 14. Server-Driven Configuration

The PWA's content manifest is served by the StrataHub API (`/missionhydrosci/api/manifest`). This means the server controls:

- Which units are available
- What version of each unit is current
- The CDN base URL for content
- File sizes (for storage checks and progress calculation)

Changing any of these requires zero client-side updates. Update the manifest JSON on the server and every student's next page load picks up the change.

---

## What the PWA Layer Actually Does

For clarity, here is every piece of JavaScript that the PWA play page runs on top of the raw Unity WebGL build:

| Component | Purpose |
|---|---|
| Service worker registration | Enables offline play, cache management, background downloads |
| AudioContext tracking wrapper | Tracks all audio instances for lifecycle management |
| Identity bridge (XHR + fetch intercept) | Provides user identity without cross-origin cookies |
| External navigation blocker | Prevents Unity from navigating out of the managed environment |
| Audio suspend/resume on visibility change | Fixes iOS audio going silent after app switch |
| Audio cleanup on page leave | Prevents zombie audio from bfcache preservation |
| Unity instance cleanup (`Quit()`) | Properly tears down WASM and rendering on page leave |
| Fullscreen with container targeting | Routes keyboard events correctly to the Unity canvas |
| iPad/standalone detection | Hides fullscreen button where it's broken or unnecessary |
| Back button for PWA navigation | Provides navigation when there's no browser chrome |
| Keyboard Lock API | Captures all key events in fullscreen on Chromebooks |
| Aspect-ratio-preserving resize | Scales canvas correctly across screen sizes |
| Canvas focus management | Ensures keyboard input reaches Unity after fullscreen transitions |

None of these components modify the Unity build. They wrap it. The Unity WebGL output (loader, framework, WASM, data, streaming assets) is used exactly as-is from the build. The PWA layer is entirely additive.

---

## Summary

A raw Unity WebGL build is a game that runs in a browser tab. The PWA turns it into a managed, offline-capable, platform-adaptive application that handles authentication, storage, audio, input, navigation, and content delivery across Chromebooks and iPads in school environments where network reliability, device storage, and IT policies are real constraints.

Every component in the PWA layer exists because a specific, observed problem required it. The identity bridge exists because cross-origin cookies don't work. The audio management exists because iOS doesn't clean up AudioContexts. The fullscreen detection exists because iPad Safari breaks letter keys. The service worker exists because school Wi-Fi can't handle 30 simultaneous 200 MB downloads. None of these are theoretical — they were all discovered and fixed during testing on actual school hardware.
