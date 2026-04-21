# iPad Trackpad + the Look Problem

## Solution - Playing on iPad

To move, use W, A, S, D (or the arrow keys) on the keyboard — just like on a computer. To  look around, drag two fingers on the trackpad: swipe right to look right, swipe up to look up, and so on. You can hold W while two-finger-dragging to walk and look at the same time.

## How the Magic Keyboard trackpad works on iPad

The Magic Keyboard's trackpad on iPad is not a traditional mouse. iPadOS draws a small translucent circular cursor that snaps to UI elements, grows over text, and disappears when you're just typing. From the game's point of view, though, the browser still receives standard pointer events — pointerdown, pointermove, pointerup — with clientX / clientY and a  computed movementX / movementY. One-finger movement moves the cursor; a physical trackpad press (or tap-to-click) is a click; two-finger drag is a scroll gesture that fires wheel events instead of pointer events.

## Why camera look breaks on iPad

On desktop, Mission HydroSci calls Cursor.lockState = CursorLockMode.Locked. Pointer Lock hides the system cursor and tells the browser to deliver raw movement deltas — the cursor has no position, so movement has no edges. Move the mouse right forever and movementX keeps arriving.

iPadOS Safari does not implement the Pointer Lock API. There is no way for a web page to request it, no way to hide or warp the system cursor, and no way to get raw deltas. So the game falls back to reading <Pointer>/delta, which on iPad is just the frame-to-frame difference of the on-screen cursor position. The moment the cursor reaches the edge of the display, it stops moving — and since the cursor can't move, delta is zero. The camera freezes. The player keeps dragging the trackpad and nothing happens until they reset back toward the center.

The one-finger cursor has this edge problem by design. We can't fix it from JavaScript.

## How the implementation gets around it

The fix leverages the fact that two-finger trackpad drag on iPad doesn't move the cursor at all — it fires wheel events with deltaX and deltaY. Those deltas come from the finger motion, not the cursor, so they are not bounded by screen edges. You can two-finger-drag across the trackpad all day and deltas keep flowing.

So on iPad we replace the look input source:

1. iPadInput.jslib runs at startup, detects iPad via user-agent + navigator.maxTouchPoints (iPadOS 13+ masquerades as Mac, so the touchpoint check is needed), and attaches a wheel listener on the Unity canvas. Each event accumulates deltaX / deltaY into a JS buffer and calls preventDefault() so the page itself doesn't scroll. 

2. IPadLookInput.cs is a thin C# wrapper. At game start it calls the jslib init once; IsActive becomes true only on iPad WebGL (always false in Editor and on desktop). DrainLookDelta() pulls the accumulated buffer, zeros it, applies sensitivity and sign to match the desktop Look action's output shape, and returns a Vector2.

3. The three camera-look consumers (PlayerController, CameraManager for the drone, RotateCameraTarget) each got two small changes:   
- Their existing OnLook callback — the one bound to <Pointer>/delta — early-returns when IPadLookInput.IsActive, so the broken iPad pointer deltas are ignored.   
- A per-frame pump (LateUpdate / Update) drains the wheel buffer and feeds that delta into  the same rotation math the pointer path used.

The net effect: on iPad, two-finger trackpad drag produces continuous, edge-free look deltas  that flow into exactly the same camera-rotation code as before. The player can hold W with one hand and two-finger-drag with the other, mirroring the desktop experience. On every other platform IsActive is false and the code runs the original <Pointer>/delta path untouched.