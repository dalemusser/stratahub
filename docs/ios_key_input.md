# iOS Safari Fullscreen Keyboard Input — Unity WebGL

## Summary

iPad Safari in fullscreen mode completely suppresses all letter key DOM events (A–Z). Arrow keys, spacebar, tab, and punctuation work normally, but no letter key event (`keydown`, `keyup`, `keypress`, `input`) ever reaches the page. This is a Safari/WebKit limitation at the browser level — no JavaScript workaround exists. The only way to get both a near-fullscreen experience and full keyboard input on iPad is to run the app as an installed PWA.

## The Problem

Mission HydroSci uses WASD keys for player movement and the M key to open the map. These are essential controls. When a student enters fullscreen mode on iPad Safari, all letter keys stop working. The game appears frozen or unresponsive to movement input, even though arrow keys and spacebar still function.

## What We Tested

### Test 1: Capture-Phase Key Listener

Added a `keydown` listener on `document` in the capture phase (the earliest possible point in DOM event propagation) to log every key event in fullscreen:

- **Spacebar** — events fired, target: `unity-canvas`
- **Arrow keys** — events fired, target: `unity-canvas`
- **Tab** — events fired, target: `unity-canvas`
- **Any letter key (A–Z)** — no events at all

The events are not being consumed by a handler higher in the DOM tree — they never enter the DOM in the first place. Safari intercepts them before they reach JavaScript.

### Test 2: Hidden Input Element

Created a hidden `<input>` element and focused it in fullscreen to test whether letter keys generate `input` events on form elements:

- **Punctuation (`;`, `"`, `{`, `}`, `\`)** — `input` events fired
- **Letter keys** — no `input` events

This confirmed the suppression is not specific to canvas elements or Unity — Safari blocks letter key events page-wide in fullscreen.

### Test 3: Keyboard Lock API

Called `navigator.keyboard.lock()` after entering fullscreen, which is designed to capture all key events for games. No effect — the Keyboard Lock API is not supported in Safari, and even if it were, the events are blocked at a level below the API.

### Test 4: Focusing an Input in Fullscreen

Attempted to programmatically focus a hidden input element while in fullscreen. Tapping the focus button immediately exited fullscreen. iPad Safari does not allow input elements to receive focus while in fullscreen mode — it exits fullscreen first.

## Why This Happens

iPad Safari's fullscreen implementation (the Fullscreen API / `requestFullscreen()`) routes letter key events to a system-level text input layer that doesn't forward them to the web page. This may be related to Safari's virtual keyboard management — the browser appears to intercept letter keys to determine whether a text input context is active, and in fullscreen it never resolves them back to the page.

Non-letter keys (arrows, spacebar, tab, escape) bypass this interception because they are classified as navigation/control keys rather than text input keys.

This behavior has been observed on iPadOS with an external keyboard. It does not occur on macOS Safari or macOS Chrome, even in fullscreen.

## How Each Environment Behaves

| Environment | Fullscreen Available | Letter Keys Work | WASD Playable |
|---|---|---|---|
| iPad Safari (normal mode) | N/A | Yes | Yes |
| iPad Safari (fullscreen) | Yes | **No** | **No** |
| iPad PWA (standalone mode) | N/A — already near-fullscreen | Yes | Yes |
| macOS Safari (fullscreen) | Yes | Yes | Yes |
| macOS Chrome (fullscreen) | Yes | Yes | Yes |
| Chromebook Chrome (fullscreen) | Yes | Yes | Yes |

## The Solution: PWA as the Fullscreen Alternative

An installed PWA (Add to Home Screen) on iPad runs in standalone display mode. This provides a near-fullscreen experience — the app fills the entire screen except for a small status bar (time, battery, Wi-Fi) at the top. Critically, **all keyboard input works normally in standalone mode**, including letter keys.

This makes the PWA the only viable way to get both a fullscreen-like experience and full keyboard input for Unity WebGL games on iPad.

### What We Changed

1. **Fullscreen button is hidden on iPad and in PWA standalone mode.** The button is unnecessary in a PWA (already near-fullscreen) and harmful in Safari (breaks WASD). Detection uses two checks:

   - **Standalone mode:** `window.matchMedia('(display-mode: standalone)').matches` or `navigator.standalone === true`
   - **iPad detection:** `navigator.maxTouchPoints > 0` combined with user agent containing `iPad` or `Macintosh` (iPadOS reports as Macintosh in its user agent string)

2. **PWA install banner** is shown on the Mission HydroSci Units page when the browser supports installation, encouraging students to install the app for the best experience.

3. **Back button added to the play page** for PWA navigation. In standalone mode there is no browser back button, so the play page includes a back arrow in the top-left corner to return to the Units page.

### What Students Should Do

Students on iPad should install Mission HydroSci as a PWA (Add to Home Screen) from the Units page. This gives them:

- Near-fullscreen display without Safari's toolbar
- Full keyboard input including WASD and M
- Offline access to downloaded units
- A dedicated app icon on their Home Screen

## Files Changed

- `missionhydrosci/templates/missionhydrosci_play.gohtml` — Fullscreen button hidden on iPad/standalone, back button added, iPad and standalone mode detection
- `missionhydrosci/templates/missionhydrosci_units.gohtml` — PWA install banner, manifest override for correct PWA scope
- `missionhydrosci/manifest.go` — PWA manifest with `scope: "/"` so all navigation stays within the app
