# StrataHub Accessibility Tracker

**Purpose:** One place to record accessibility issues and their status across all
StrataHub features, so we can make coordinated passes (fix the same class of
issue everywhere at once) rather than rediscovering them feature by feature.

**How to use this doc**
- When a review or audit finds an accessibility issue, add a row to the
  relevant feature section (or open a new one). Don't fix silently without
  recording it here.
- When you fix one, update its **Status** and date rather than deleting the row —
  the history is useful when the same pattern shows up elsewhere.
- The **Standard** section below is the bar we hold to. When you build new UI,
  check it against that list before it ships so this tracker stays short.

**Status values:** `open` · `in progress` · `done (YYYY-MM-DD)` · `won't fix (reason)`
**Severity:** A (blocks a user from completing a task) · B (significant barrier,
workaround exists) · C (polish / below-standard but usable)

---

## The standard we hold to

These apply to every feature. Most are already met by the shared layout; the
gaps below are where individual features fall short.

1. **Dialogs/modals:** `role="dialog"` + `aria-modal="true"` + a label
   (`aria-labelledby` or `aria-label`); move focus into the dialog on open and
   restore it to the trigger on close; trap Tab within; Escape and
   backdrop-click close. (Or use native `<dialog>`.)
2. **Dynamic status:** any text that updates without a navigation (progress %,
   "Ready", saved/failed toasts) sits in an `aria-live="polite"` region.
3. **Form controls:** every input has a programmatic label (`<label for>` or
   `aria-label`) — placeholders are not labels.
4. **Touch targets:** interactive controls are at least ~44×44 px, achieved with
   padding rather than a bigger glyph. Matters most on the tablet/Chromebook
   fleet.
5. **Contrast:** text that carries meaning meets WCAG AA (4.5:1 normal, 3:1 for
   large/bold). Muted gray is fine for decoration, not for state.
6. **Keyboard:** everything reachable and operable by keyboard; visible focus
   ring; no key-only affordance advertised on a touch-only device (and vice
   versa).
7. **Dark mode:** contrast holds in both themes (this is already a hard project
   rule for other reasons).

---

## Mission HydroSci

Seeded from the 2026-07-16 code/design review
(`docs/mission-hydrosci/mhs-code-design-review-071626.md`). The MHS game itself
is keyboard/trackpad-driven and out of scope here; these are the StrataHub
wrapper pages (launcher, play-page chrome, manage).

| # | Location | Issue | Std | Sev | Status |
|---|----------|-------|-----|-----|--------|
| MHS-A1 | `_units.gohtml` help modal; `_manage.gohtml` collection-picker modal; `_play.gohtml` help overlay | No `role="dialog"`/`aria-modal`/label; focus not moved into modal on open nor restored on close; no focus trap. | 1 | B | done (2026-07-17) — dialog roles + labels on all three; the two page modals (help, picker) move focus in on open, restore on close, and trap Tab. See note below on the play overlay. |
| MHS-A2 | `_units.gohtml` `#unit-status`, `#status-<id>`, `#sw-status`, `#next-unit-status`, `#progress-text`; `_manage.gohtml` `#gate-error`/`#gate-loading`; `_play.gohtml` loading text | Download/launch/auth state changes are not announced — no `aria-live`. | 2 | B | done (2026-07-17) — `aria-live="polite"` (or `role="status"`/`role="alert"`) added to the status lines and the gate error/loading. |
| MHS-A3 | `_units.gohtml` `?` help button and install-dismiss `×`; `_play.gohtml` fullscreen / back | Touch targets below ~44px on the tablet audience. | 4 | C | done (2026-07-17) — play back/fullscreen now 44px; units help button 32px + larger dismiss hit area via padding. (Units `?` kept at 32px to fit beside the H1 — acceptable; revisit if field feedback wants larger.) |
| MHS-A4 | `_units.gohtml` progress-dot labels + `text-gray-400` status lines | State-bearing text below AA contrast (~2.8:1). | 5 | C | done (2026-07-17) — state-bearing text bumped to gray-500 / dark:gray-400. |
| MHS-A5 | `_manage.gohtml` gate inputs (keyword / login-id / password / code) | Inputs have placeholders but no `<label>`/`aria-label`. | 3 | B | done (2026-07-17) — `aria-label` added to all four gate inputs. |
| MHS-A6 | `_play.gohtml` help overlay (`?`-key only) + "Press ? for help" hint | On touch-only iPads the overlay is unreachable and the hint is misleading. | 6 | C | partial (2026-07-17) — the misleading hint is now suppressed on touch-only devices (no fine pointer). A tap affordance to open the controls overlay on touch was NOT added (the game itself is keyboard/trackpad-driven, so a bare-touch iPad can't really play it); revisit only if touch iPads become a supported play surface. |

**Note on the play controls overlay (MHS-A1):** it got `role="dialog"`/`aria-modal`/label and closes on tap/`?`, but does not yet move/trap focus, because it overlays a running Unity `<canvas>` (`tabindex="-1"`) with no natural focusable content and is a keyboard-only affordance already. Full focus management there is lower value; left as a follow-up if needed.

---

## Other features

_No audit recorded yet._ When auditing another feature (Organizations, Groups,
Members, Resources, Materials, Settings, Reports, Dashboard, …), add a section
here with the same table shape. Good candidates for a first cross-feature pass:
the shared modal/confirm-dialog component, table row-action buttons (touch
targets), and any toast/flash message (aria-live).
