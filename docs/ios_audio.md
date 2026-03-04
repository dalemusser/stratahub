# iOS PWA Audio Lifecycle — Unity WebGL

## Summary

Unity WebGL games running inside an iOS PWA (Add to Home Screen) have audio lifecycle problems that don't occur in desktop browsers or even mobile Safari. Audio can continue playing after the user leaves the game, go silent when the user returns from the app switcher, and persist as "zombie audio" across PWA sessions. This document explains the root causes and the fix applied in `missionhydrosci_play.gohtml`.

## What the User Experienced

The following sequence was observed on an iPad running Mission HydroSci as a PWA:

1. **Played several units normally** — audio worked fine through Units 1–3.

2. **During Unit 4, opened the iPad app switcher** (swiped up from the bottom). The game appeared as a tile alongside other apps, and **the game's music kept playing** even though the game was no longer in the foreground.

3. **Switched to another app (Settings), then returned to the game.** The game was visually running — rendering, animations, input all worked — but **there was no audio at all**. The game was completely silent.

4. **Closed the game** by tapping the back button to return to the Units page. **The Unit 4 music started playing over the Units page**, even though the game was no longer loaded.

5. **Launched a different unit.** The new unit's audio played, but **Unit 4's music continued playing simultaneously** underneath it — two soundtracks at once.

6. **Closed and relaunched the PWA.** The StrataHub Units page appeared, and **Unit 4's music immediately started playing again**. The only way to stop it was to delete the PWA from the Home Screen and clear all browser cache data.

## Root Cause

Three separate iOS behaviors combine to create this problem:

### 1. iOS Does Not Suspend AudioContexts on Visibility Change

When a PWA is backgrounded (app switcher, switching to another app, locking the screen), iOS fires the `visibilitychange` event with `document.visibilityState = 'hidden'`. However, **Unity WebGL does not listen for this event to suspend its AudioContext instances**. The Web Audio API AudioContexts keep running, which is why audio continued playing in the app switcher.

### 2. iOS Suspends AudioContexts at the OS Level Without Resuming Them

When the user fully switches to another app, iOS eventually suspends the AudioContext at the operating system level (below the JavaScript layer). When the user returns, the AudioContext enters a `suspended` state. **Unity does not check for or recover from this state** — it continues rendering the game and processing input, but all audio calls go to a suspended AudioContext and produce no sound. This is why the game appeared to work but was silent after returning from Settings.

### 3. iOS PWA Preserves Pages in the Back-Forward Cache (bfcache)

When the user navigates away from the play page (tapping the back button), a normal browser would destroy the page's JavaScript context, including all AudioContext instances. However, **iOS PWA preserves the entire page in the bfcache** — JavaScript context, WASM memory, AudioContext instances, and all. When the bfcache entry is later accessed or restored, iOS resumes the AudioContexts. This is the "zombie audio" — AudioContexts from a page the user already left, now playing without any game rendering to control them.

The combination: visibility change leaves AudioContexts in an inconsistent state, and bfcache preservation means they're never destroyed, allowing them to resurface later.

## The Fix

The fix is implemented in `missionhydrosci_play.gohtml` and consists of three parts:

### Part 1: AudioContext Tracking

Unity creates AudioContext instances internally through the Web Audio API. JavaScript has no built-in way to enumerate active AudioContexts, so we intercept the constructor before Unity loads to track every instance:

```javascript
// Runs as the first <script> in <body>, before Unity's loader
var OrigAC = window.AudioContext || window.webkitAudioContext;
var tracked = [];
window.__mhsAudioContexts = tracked;

function TrackedAudioContext(options) {
  var ctx = options ? new OrigAC(options) : new OrigAC();
  tracked.push(ctx);
  return ctx;
}
TrackedAudioContext.prototype = OrigAC.prototype;
window.AudioContext = TrackedAudioContext;
```

This must run before Unity's loader script is added to the page. Unity calls `new AudioContext()` which now goes through our wrapper, and the real AudioContext instance is saved in the tracked array.

### Part 2: Suspend/Resume on Visibility Changes

When the page becomes hidden (app switcher, switching apps, locking screen), we suspend all AudioContexts. When the page becomes visible again, we resume them:

```javascript
document.addEventListener('visibilitychange', function() {
  if (document.visibilityState === 'hidden') {
    // Suspend all AudioContexts — pauses audio processing
    contexts.forEach(function(ctx) {
      if (ctx.state === 'running') ctx.suspend();
    });
  } else {
    // Resume all AudioContexts — restores audio after suspend
    contexts.forEach(function(ctx) {
      if (ctx.state === 'suspended') ctx.resume();
    });
  }
});
```

`AudioContext.suspend()` pauses audio processing without destroying the context. `AudioContext.resume()` restarts it. This is the standard Web Audio API mechanism for this, but Unity doesn't use it.

### Part 3: Full Cleanup on Page Leave

When the user navigates away from the play page (back button, unit-complete overlay link, or the browser's own page lifecycle), we permanently shut down Unity and close all AudioContexts:

```javascript
function cleanupUnity() {
  // Quit Unity (stops rendering, WASM execution, and internal audio)
  if (window.__unityInstance) {
    window.__unityInstance.Quit();
    window.__unityInstance = null;
  }
  // Permanently close all AudioContexts
  contexts.forEach(function(ctx) { ctx.close(); });
  // Pause any <audio>/<video> elements
  document.querySelectorAll('audio, video').forEach(function(el) {
    el.pause();
    el.src = '';
  });
}

// Fires when the page is being navigated away or entering bfcache
window.addEventListener('pagehide', cleanupUnity);
```

The back button and unit-complete overlay links also call `cleanupUnity()` explicitly before navigating, as a belt-and-suspenders measure in case `pagehide` fires too late.

`AudioContext.close()` permanently releases all audio resources. Unlike `suspend()`, a closed context cannot be resumed — which is exactly what we want when leaving the page, to prevent zombie audio from bfcache restoration.

The Unity instance is stored globally (`window.__unityInstance`) when `createUnityInstance()` resolves, so the cleanup function can call `Quit()` on it. `Quit()` tears down the WASM runtime, stops the render loop, and releases Unity's internal resources.

## Behavior After the Fix

| Scenario | Before Fix | After Fix |
|---|---|---|
| Open app switcher while playing | Audio keeps playing | Audio suspends immediately |
| Return to game from app switcher | Audio is permanently dead | Audio resumes normally |
| Switch to another app and back | Audio dead, zombie audio later | Audio suspends and resumes cleanly |
| Navigate back to Units page | Zombie audio plays over Units page | Unity quits, all audio closed |
| Relaunch PWA after playing | Zombie audio from previous session | Clean start, no residual audio |

## Applicability

This fix is applied in **Mission HydroSci** (`missionhydrosci_play.gohtml`). When Mission HydroSci replaces MHS Units, the fix carries over automatically.

The issue is specific to:
- **iOS PWA** (Add to Home Screen) — regular Safari doesn't preserve pages in bfcache as aggressively
- **Unity WebGL** — Unity doesn't manage AudioContext lifecycle in response to page visibility events
- **iPad and iPhone** — desktop browsers handle AudioContext suspension/resumption automatically

## Files Changed

- `missionhydrosci/templates/missionhydrosci_play.gohtml` — AudioContext tracking wrapper, visibility change suspend/resume, full cleanup on page leave, Unity instance stored globally
