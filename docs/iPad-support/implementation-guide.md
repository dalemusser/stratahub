# iPad Look Support — Implementation Guide

This guide tells a Mission HydroSci game developer exactly how to reproduce the
iPad camera-look fix in the Unity project. For the conceptual background (why
the problem exists, why two-finger drag works), read
[`iPad-look-implementation.md`](iPad-look-implementation.md) first.

## TL;DR

On iPad, Safari has no Pointer Lock API, so `<Pointer>/delta` pins at the screen
edge and camera look freezes. We add a WebGL `wheel`-event listener that
captures two-finger trackpad drags (which are edge-independent), expose the
accumulated delta to C#, and on iPad feed that delta into the existing camera
rotation code instead of the broken pointer delta. Desktop is untouched.

## Files in this folder

| File | Purpose |
|---|---|
| `iPad-look-implementation.md` | Problem statement + player-facing instructions |
| `iPad-support-notes.md` | Original design notes |
| `implementation-guide.md` | **This file — step-by-step for a dev** |
| `code/iPadInput.jslib` | Source to drop into `Assets/Plugins/WebGL/` |
| `code/IPadLookInput.cs` | Source to drop into `Assets/Scripts/Managers/` |

## Prerequisites

- Unity 6000.0.44f1 (what MHS 2.0 uses). Any Unity 2021+ with the new Input
  System package should work identically.
- The new Input System package (`com.unity.inputsystem`) — already in MHS.
- WebGL build target.

## Change summary

Two new files, three edited files.

| Change | Path |
|---|---|
| **New** | `Assets/Plugins/WebGL/iPadInput.jslib` |
| **New** | `Assets/Scripts/Managers/IPadLookInput.cs` |
| **Edit** | `Assets/Scripts/Player/PlayerController.cs` |
| **Edit** | `Assets/Scripts/Camera/CameraManager.cs` |
| **Edit** | `Assets/Scripts/Camera/RotateCameraTarget.cs` |

## Step 1 — Add the jslib

Copy `code/iPadInput.jslib` to:

```
Assets/Plugins/WebGL/iPadInput.jslib
```

What it does:

- Runs once the first time C# calls `iPadInput_Init()`.
- Detects iPad by checking the UA string and, for iPadOS 13+ "desktop mode"
  where `navigator.platform` is `"MacIntel"`, by confirming
  `navigator.maxTouchPoints > 1`.
- On iPad, attaches a `wheel` listener to the Unity canvas with
  `{ passive: false }` so it can call `preventDefault()` (stops the browser
  page from scrolling under the canvas).
- Accumulates `deltaX`/`deltaY` in a JS buffer.
- Exposes two drain functions (`iPadInput_DrainDeltaX`, `iPadInput_DrainDeltaY`)
  that return the accumulated delta and zero the buffer.
- Returns `0` from `iPadInput_Init()` on non-iPad — nothing is attached, no
  side effects.

## Step 2 — Add the C# bridge

Copy `code/IPadLookInput.cs` to:

```
Assets/Scripts/Managers/IPadLookInput.cs
```

What it does:

- Static class `MHS.InputSystem.IPadLookInput`.
- `[RuntimeInitializeOnLoadMethod(BeforeSceneLoad)]` calls `iPadInput_Init()`
  once at game start. `IsActive` is `true` only on iPad WebGL builds.
- `DrainLookDelta()` returns a `Vector2` scaled and sign-corrected to match
  the desktop `Look` action's output shape (`ScaleVector2(0.05)` +
  `InvertVector2` on Y). Returns `Vector2.zero` in the Editor and on non-iPad
  browsers.
- Tunables exposed as `public static` fields: `SensitivityX`, `SensitivityY`,
  `InvertX`, `InvertY`. Adjust in code after device testing (see Tuning below).

## Step 3 — Edit `PlayerController.cs`

File: `Assets/Scripts/Player/PlayerController.cs`

The namespace `MHS.InputSystem` is already imported — no new `using` needed.

### 3a. `LateUpdate` — drain wheel delta before camera rotation

Find:

```csharp
private void LateUpdate()
{
    if(DialogueManager.isConversationActive) return;
        CameraRotation();
}
```

Replace with:

```csharp
private void LateUpdate()
{
    if(DialogueManager.isConversationActive) return;
    if(IPadLookInput.IsActive)
        InputLook = IPadLookInput.DrainLookDelta();
    CameraRotation();
}
```

### 3b. `OnLook` — ignore broken pointer deltas on iPad

Find:

```csharp
public void OnLook(InputAction.CallbackContext ctx)
{
    InputLook = ctx.ReadValue<Vector2>();
}
```

Replace with:

```csharp
public void OnLook(InputAction.CallbackContext ctx)
{
    if(IPadLookInput.IsActive) return;
    InputLook = ctx.ReadValue<Vector2>();
}
```

## Step 4 — Edit `CameraManager.cs` (drone camera)

File: `Assets/Scripts/Camera/CameraManager.cs`

The namespace `MHS.InputSystem` is already imported.

The existing `OnLook` does the rotation math inline inside the callback. We
extract it into a helper, have `OnLook` early-return on iPad, and add a
`LateUpdate` that drains the wheel delta on iPad.

Find:

```csharp
public void OnLook(InputAction.CallbackContext ctx)
{

    if(DialogueManager.instance.isConversationActive) return;
    InputLook = ctx.ReadValue<Vector2>();

    if(IsCameraLocked || !DoTargetRotation || CurrentCamera == null || CameraTarget == null) return;

    _cinemachineTargetYaw += InputLook.x;
    _cinemachineTargetPitch += InputLook.y;

    _cinemachineTargetYaw = ClampAngle(_cinemachineTargetYaw, float.MinValue, float.MaxValue);
    _cinemachineTargetPitch = ClampAngle(_cinemachineTargetPitch, BottomClamp, TopClamp);

    CameraTarget.transform.rotation = Quaternion.Euler(_cinemachineTargetPitch, _cinemachineTargetYaw, 0);		
}
```

Replace with:

```csharp
public void OnLook(InputAction.CallbackContext ctx)
{
    if(IPadLookInput.IsActive) return;
    if(DialogueManager.instance.isConversationActive) return;
    InputLook = ctx.ReadValue<Vector2>();
    ApplyLook(InputLook);
}

private void LateUpdate()
{
    if(!IPadLookInput.IsActive) return;
    if(DialogueManager.instance.isConversationActive) return;
    Vector2 delta = IPadLookInput.DrainLookDelta();
    if(delta.sqrMagnitude <= 0f) return;
    InputLook = delta;
    ApplyLook(delta);
}

private void ApplyLook(Vector2 look)
{
    if(IsCameraLocked || !DoTargetRotation || CurrentCamera == null || CameraTarget == null) return;

    _cinemachineTargetYaw += look.x;
    _cinemachineTargetPitch += look.y;

    _cinemachineTargetYaw = ClampAngle(_cinemachineTargetYaw, float.MinValue, float.MaxValue);
    _cinemachineTargetPitch = ClampAngle(_cinemachineTargetPitch, BottomClamp, TopClamp);

    CameraTarget.transform.rotation = Quaternion.Euler(_cinemachineTargetPitch, _cinemachineTargetYaw, 0);
}
```

## Step 5 — Edit `RotateCameraTarget.cs`

File: `Assets/Scripts/Camera/RotateCameraTarget.cs`

This one needs a new `using`. Same pattern as `CameraManager`: extract a helper,
early-return on iPad in `OnLook`, add an `Update` pump.

Replace the top of the file and the `OnLook` method.

Find:

```csharp
using Obvious.Soap;
using UnityEngine;
using UnityEngine.InputSystem;
```

Replace with:

```csharp
using MHS.InputSystem;
using Obvious.Soap;
using UnityEngine;
using UnityEngine.InputSystem;
```

Find:

```csharp
private void OnLook(InputAction.CallbackContext ctx)
{
    Vector2 inputLook = ctx.ReadValue<Vector2>();

    if (_cameraTarget == null) return;

    _cinemachineTargetYaw += inputLook.x * _mouseSensitivity;
    _cinemachineTargetPitch += inputLook.y * _mouseSensitivity;

    _cinemachineTargetYaw = ClampAngle(_cinemachineTargetYaw, float.MinValue, float.MaxValue);
    _cinemachineTargetPitch = ClampAngle(_cinemachineTargetPitch, BottomClamp, TopClamp);

    _cameraTarget.transform.rotation = Quaternion.Euler(_cinemachineTargetPitch, _cinemachineTargetYaw, 0);
}
```

Replace with:

```csharp
private void OnLook(InputAction.CallbackContext ctx)
{
    if (IPadLookInput.IsActive) return;
    ApplyLook(ctx.ReadValue<Vector2>());
}

private void Update()
{
    if (!IPadLookInput.IsActive) return;
    Vector2 delta = IPadLookInput.DrainLookDelta();
    if (delta.sqrMagnitude > 0f) ApplyLook(delta);
}

private void ApplyLook(Vector2 inputLook)
{
    if (_cameraTarget == null) return;

    _cinemachineTargetYaw += inputLook.x * _mouseSensitivity;
    _cinemachineTargetPitch += inputLook.y * _mouseSensitivity;

    _cinemachineTargetYaw = ClampAngle(_cinemachineTargetYaw, float.MinValue, float.MaxValue);
    _cinemachineTargetPitch = ClampAngle(_cinemachineTargetPitch, BottomClamp, TopClamp);

    _cameraTarget.transform.rotation = Quaternion.Euler(_cinemachineTargetPitch, _cinemachineTargetYaw, 0);
}
```

## Step 6 — Build and test

1. Build for WebGL.
2. Deploy and open in Safari on an iPad **with a Magic Keyboard** connected.
3. In Safari, open the Web Inspector (Mac-connected) and confirm the log line:
   `IPadLookInput: iPad detected — using two-finger trackpad wheel events for look`.
   If you don't see it, iPad detection failed — see Troubleshooting.
4. Walk test:
   - Hold **W** → player walks forward.
   - Two-finger drag on the trackpad → camera rotates.
   - Hold **W** *and* two-finger drag simultaneously → walk and look together.
5. Drone test (Units where drone pilot state is reachable):
   - Switch to drone; two-finger drag rotates drone camera.
6. Desktop regression:
   - Run the same build on macOS / Windows Safari / Chrome. Mouse-look must
     behave exactly as before. `IPadLookInput.IsActive` will be `false`, and
     `OnLook` runs the original pointer path.

## Tuning

All tunables are `public static` fields on `IPadLookInput` and can be set from
anywhere before or during runtime. For a quick change, edit the defaults in
`IPadLookInput.cs`.

| Field | Default | Purpose |
|---|---|---|
| `SensitivityX` | `0.05f` | Horizontal scaling. Matches the desktop `ScaleVector2(0.05)` processor. Raise for faster horizontal look. |
| `SensitivityY` | `0.05f` | Vertical scaling. |
| `InvertX` | `true` | Assumes iPadOS **natural scrolling on** (system default): a two-finger swipe right fires `deltaX < 0`, which we negate so "swipe right looks right". If a tester has natural scrolling off, set to `false`. |
| `InvertY` | `true` | Same rationale for vertical. "Swipe up looks up". |

Recommended first tuning pass: build, hand it to a tester, ask:
- *"Does swiping right make you look right?"* If no → flip `InvertX`.
- *"Does swiping up make you look up?"* If no → flip `InvertY`.
- *"Does it feel too slow / too fast?"* → adjust `SensitivityX`/`SensitivityY`.

If you want these surfaced in the Unity Inspector, wrap them in a
`MonoBehaviour` on a DontDestroyOnLoad GameObject (e.g., add fields to
`MHSBridge` or a new `IPadLookSettings` component, and copy Inspector values to
the static fields in `Start`).

## How it coexists with desktop

- `IPadLookInput.IsActive` is `false` in the Unity Editor (short-circuited by
  `#if UNITY_WEBGL && !UNITY_EDITOR`) and on any non-iPad WebGL build (the
  jslib init returns `0`).
- When `IsActive` is `false`, every edit falls through to the original code
  path: `OnLook` reads `<Pointer>/delta` as before; the added `LateUpdate` /
  `Update` methods early-return without touching state.
- No scene changes are required. No prefab changes. No input-action-asset
  changes. The bindings in `Assets/Settings/Input/Controls.inputactions` stay
  as they are.

## Troubleshooting

**No log line / look still broken on iPad.**
Open Safari Web Inspector (requires Mac + USB). Check:
- Is `navigator.maxTouchPoints > 1`? On iPad it should be `5`. On desktop
  Safari it's `0`. If it's `0` on iPad, the detection isn't firing — most
  likely cause is "Request Desktop Website" overriding the UA to a real
  desktop UA *and* the browser synthetically reporting no touch support. Try
  disabling "Request Desktop Website" for the site in Safari settings.
- Is the `wheel` event firing? In the Inspector console, add
  `document.addEventListener('wheel', e => console.log('wheel', e.deltaX, e.deltaY))`
  and two-finger-drag. You should see events stream.

**Camera rotates backwards.**
Natural scrolling is off on the iPad. Flip `InvertX` and/or `InvertY`.

**Look feels too slow or too fast.**
Adjust `SensitivityX` / `SensitivityY`. Wheel deltas from a trackpad are
roughly in the same pixel-scale range as mouse deltas, but specific values
vary by hardware.

**Page scrolls under the canvas when two-finger-dragging.**
Something above the canvas in the DOM is also handling the wheel event, or
the WebGL template wraps the canvas in a scrollable container. The jslib
calls `preventDefault()` on the canvas listener, so it should already stop.
If not, verify the listener target is actually the canvas — check
`Module.canvas` in the console.

**Unity compile error: cannot find `IPadLookInput`.**
`PlayerController.cs` and `CameraManager.cs` already have
`using MHS.InputSystem;`. Confirm that's present. `RotateCameraTarget.cs`
required adding the `using` per Step 5.

## What this does NOT do

- **No one-finger look, no click-to-look.** Only two-finger drag. This was a
  deliberate choice: the player needs to hold W while looking, so a gesture
  requiring a click-hold would tie up the same hand.
- **No touch-screen support.** The fix targets the Magic Keyboard trackpad,
  not direct finger contact with the iPad screen. Touch is the next project
  ("Priority 2" in the original planning discussion).
- **No virtual on-screen joystick or buttons.** All non-look input (movement,
  tools, interaction) still goes through the physical keyboard unchanged.

## Future: when Apple ships Pointer Lock

If a future iPadOS Safari implements the Pointer Lock API, the cleanest path is
to delete this entire fix:

1. Delete `Assets/Plugins/WebGL/iPadInput.jslib`.
2. Delete `Assets/Scripts/Managers/IPadLookInput.cs`.
3. Revert the three edits (git revert the wiring commits).

The game will then use `CursorLockMode.Locked` on iPad the same way it does on
desktop, and the two-finger-drag gesture becomes unnecessary.
