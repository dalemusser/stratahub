# WEBAudio Issues — Unity WebGL

## The Bug

Unity WebGL's audio module (`WEBAudio`) has an internal bug in its `onstatechange` handler for AudioContext. When the AudioContext state changes (suspend, resume, close), Unity's handler iterates over `WEBAudio.audioInstances` using `.forEach()`:

```javascript
// Inside Unity's compiled WASM/JS bridge (we don't control this code)
WEBAudio.audioContext.onstatechange = function() {
  WEBAudio.audioInstances.forEach(function(instance) {
    // resume buffered sounds, etc.
  });
};
```

In some Unity versions (2021.x+), `WEBAudio.audioInstances` was changed from an array to a plain object (keyed by instance ID) for O(1) deletion performance. But `.forEach()` only exists on arrays, not plain objects. The handler crashes with:

```
TypeError: WEBAudio.audioInstances.forEach is not a function
  at WEBAudio.audioContext.onstatechange
```

We cannot fix this in Unity's build output — it's compiled into the WASM/JS bridge blob.

## When It Triggers

AudioContext state changes happen in several scenarios:

| Trigger | State Change | How It Happens |
|---|---|---|
| Tab switch away | running → suspended | Our `suspendAllAudio()` calls `ctx.suspend()` |
| Tab switch back | suspended → running | Our `resumeAllAudio()` calls `ctx.resume()` |
| Unit teardown | running → closed | Our `cleanupUnity()` calls `ctx.close()` |
| Browser autoplay policy | suspended → running | Browser resumes context after user interaction |
| iOS app switcher | running → suspended | OS suspends audio at system level |

Any of these triggers Unity's buggy `onstatechange` handler.

## Issue 1: Crash During Unit Teardown

### Scenario

When a student completes a unit, the play page tears down Unity (`Quit()`) and closes all AudioContexts before navigating to the next unit. Calling `ctx.close()` after `Quit()` triggers an `onstatechange` event. By that point, `Quit()` has already torn down WEBAudio internals, and `WEBAudio.audioInstances` is in an invalid state.

### Error

```
TypeError: WEBAudio.audioInstances.forEach is not a function
  at WEBAudio.audioContext.onstatechange (blob:...)
```

### Original Fix (Teardown Order)

In `cleanupUnity()`, detach Unity's `onstatechange` handlers **before** calling `Quit()`:

```javascript
function cleanupUnity() {
  var contexts = window.__mhsAudioContexts || [];

  // 1. Detach Unity's onstatechange handlers FIRST
  for (var i = 0; i < contexts.length; i++) {
    try { contexts[i].onstatechange = null; } catch (e) {}
  }

  // 2. Now safe to quit Unity (tears down WEBAudio internals)
  if (window.__unityInstance) {
    try { window.__unityInstance.Quit(); } catch (e) {}
    window.__unityInstance = null;
  }

  // 3. Close AudioContexts (state change fires, but handler is null)
  for (var i = 0; i < contexts.length; i++) {
    try { contexts[i].close(); } catch (e) {}
  }
}
```

This fixed the teardown crash but did not protect against state changes during normal gameplay (tab switching).

## Issue 2: Crash During Tab Switching

### Scenario

While playing a unit, the user switches to another tab (e.g., to check download progress on the StrataHub units page). The `visibilitychange` handler suspends all AudioContexts. When the user switches back, `resumeAllAudio()` resumes the contexts, triggering `onstatechange`. Unity's buggy handler crashes.

### Error

Same as Issue 1:

```
TypeError: WEBAudio.audioInstances.forEach is not a function
  at WEBAudio.audioContext.onstatechange (blob:...)
```

### Fix (Safe onstatechange Wrapper)

The teardown fix only covered the `cleanupUnity()` path. To handle ALL state changes, the fix intercepts the `onstatechange` property at AudioContext creation time.

The play page already wraps `AudioContext` to track instances (for iOS audio lifecycle management). The wrapper now also shadows the `onstatechange` property with a safe version that wraps any handler in try-catch:

```javascript
function TrackedAudioContext(options) {
  var ctx = options ? new OrigAC(options) : new OrigAC();
  tracked.push(ctx);

  // Shadow onstatechange with a safe wrapper.
  // When Unity sets ctx.onstatechange = fn, we store fn but don't
  // bind it directly. Instead, an addEventListener('statechange')
  // callback invokes fn wrapped in try-catch.
  var _handler = null;
  Object.defineProperty(ctx, 'onstatechange', {
    configurable: true,
    enumerable: true,
    get: function() { return _handler; },
    set: function(fn) { _handler = fn; }
  });
  ctx.addEventListener('statechange', function(e) {
    if (_handler) {
      try { _handler.call(ctx, e); } catch (err) {
        console.warn('Suppressed audio statechange error:', err.message);
      }
    }
  });

  return ctx;
}
```

**How it works:**

1. `Object.defineProperty` shadows the native `onstatechange` accessor on the instance. Unity's `ctx.onstatechange = fn` stores `fn` in `_handler` but does NOT register it with the native event system.
2. A `statechange` event listener (registered via `addEventListener`) invokes `_handler` wrapped in try-catch. This is equivalent to the native `onstatechange` behavior but safe.
3. If Unity reads `ctx.onstatechange` back, the getter returns the original function — transparent to Unity.
4. If Unity sets `ctx.onstatechange = null`, the getter returns null and the event listener skips invocation.

This protects against the crash in all scenarios (teardown, tab switch, autoplay policy, iOS app switcher) without affecting audio functionality. The `cleanupUnity()` teardown-order fix remains as a belt-and-suspenders measure.

## Audio Lifecycle Context

The `onstatechange` wrapper is part of a broader audio lifecycle system for Unity WebGL on iOS PWA. See `ios_audio.md` for the full system covering:

- AudioContext tracking via constructor interception
- Suspend/resume on visibility changes (app switcher, tab switching)
- Full teardown on page leave (back button, bfcache, unit transitions)

## Files

- `missionhydrosci/templates/missionhydrosci_play.gohtml` — AudioContext tracking wrapper with `onstatechange` safe wrapper, suspend/resume on visibility change, full cleanup on page leave
