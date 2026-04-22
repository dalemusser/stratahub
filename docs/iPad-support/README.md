# iPad Support — Superseded

**The content in this folder is historical and no longer the authoritative design.**

See **[`../iPadScrollWheelSupport/README.md`](../iPadScrollWheelSupport/README.md)** for the current iPad camera-look design (a single input-asset edit — no jslib, no pump, no controller patches).

The files here document earlier approaches that were explored and rejected before the final design landed:

- `iPad-support-notes.md`, `iPad-look-implementation.md` — early conceptual notes about why iPad needs special handling. The "why" content is still accurate; the "how" content is not.
- `implementation-guide.md`, `code/iPadInput.jslib`, `code/IPadLookInput.cs` — the jslib + per-frame pump + four-controller-patch approach. Not to be implemented.
- `index.html` — reference standalone launcher with the experimental host-page synthetic-event shim. Still used for some ongoing test builds; will be removed once the scroll-wheel binding ships in official builds.

Kept for reference only.
