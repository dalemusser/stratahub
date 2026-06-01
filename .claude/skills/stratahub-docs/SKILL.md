---
name: stratahub-docs
description: Generate end-user documentation for Strata Hub by driving the project.adroit.games workspace with playwright-cli — navigating screens, capturing matched light + dark screenshots into docs/stratahub-user-documentation/images, and writing markdown guides in docs/stratahub-user-documentation. Use whenever asked to document, screenshot, or write a guide for any Strata Hub feature, screen, or workflow.
---

# Strata Hub documentation pass

## What this does
Builds end-user documentation from the dedicated, isolated **project.adroit.games** workspace by driving the live UI with `playwright-cli`, capturing theme-matched screenshots, and writing markdown that embeds them. The workspace is a throwaway tenant with no real users or PII — anything may be created, edited, or logged into freely. It is already branded (Intelligence Builders logo, "Copyright 2026 Intelligence Builders" footer) and the admin account's theme is set to **System**.

## Run from the repo root
Run every command from `/Users/dale/Documents/catchupstratahub/stratahub`. All paths below are relative to it.

## Prerequisites (assume in place)
- `.playwright/cli.config.json` — chromium, persistent profile, headed, 1440×900 at 2× scale, `colorScheme: "light"`. (No `outputDir`: auto-named scratch like snapshots/console logs goes to the gitignored `.playwright-cli/`; screenshots are written by explicit `--filename` paths, below.)
- The **admin session is already primed** (logged in via the persistent profile). Verify by opening the workspace — you should land on `/dashboard`. If you land on a login page instead, STOP and ask the user to prime it by hand (`playwright-cli open https://project.adroit.games`, log in). Never type the admin password yourself, and never put credentials in committed files.
- `.playwright/secrets.env` (gitignored) holds `PROJECT_URL` and credentials. Never print, echo, or commit it.

## Keep the session window open
`playwright-cli` acts on the live session, and `run-code` does not navigate. If you close the window, the next command opens a fresh blank browser. Keep the primed window open between commands; if it's gone, re-open `https://project.adroit.games` (the persistent profile keeps you logged in).

## Dual-theme capture — build once, flip live
Strata Hub's theme switches **live**: in System mode the page reacts to `prefers-color-scheme` with no reload. `playwright-cli`'s `emulateMedia` sets that signal, so you can capture both themes of the *same* built-up state without rebuilding or reloading it.

For each screen:

1. Set light, then build the state you want to show — navigate to the screen and open the modal / advance the wizard / trigger the HTMX swap as needed:
   ```
   playwright-cli run-code "async (page) => { await page.emulateMedia({ colorScheme: 'light' }); }"
   ```
   (Light is the config default; this is explicit insurance and resets state from a prior screen's dark capture.)

2. Screenshot light. Snapshot first if you need an element ref. **Pass the full repo-relative path** in `--filename` — the CLI resolves it from the current directory (the repo root), so a bare name lands in the repo root, not the images folder:
   ```
   playwright-cli snapshot
   playwright-cli screenshot --filename=docs/stratahub-user-documentation/images/<kebab-name>-light.png            # whole viewport
   playwright-cli screenshot <ref> --filename=docs/stratahub-user-documentation/images/<kebab-name>-light.png      # one component
   ```

3. Flip to dark **live** — the page repaints in place and the open dialog / built state is preserved (no reload):
   ```
   playwright-cli run-code "async (page) => { await page.emulateMedia({ colorScheme: 'dark' }); }"
   ```

4. Repeat the *same* screenshot command (viewport or the same `<ref>`) with a `-dark` filename:
   ```
   playwright-cli screenshot --filename=docs/stratahub-user-documentation/images/<kebab-name>-dark.png
   ```

5. Move to the next screen. Reset to light at the top of each screen (step 1) so ordering stays deterministic.

The `screenshot` command captures the **viewport** at CSS resolution (~1440 px wide), which is fine for GitHub. For a tall screen that scrolls past the fold, or if you want true 2× retina images, capture via run-code instead — it takes the full path directly and exposes `fullPage` and `scale`:
```
playwright-cli run-code "async (page) => { await page.screenshot({ path: 'docs/stratahub-user-documentation/images/<kebab-name>-dark.png', fullPage: true, scale: 'device' }); }"
```

Fallback: if some element ever fails to repaint on a live flip, set the theme *before* navigating to that screen and capture each theme on its own fresh load (navigate-first). With live switching this should be unnecessary.

## Naming
- Name files by content, not order: `org-members-table-light.png`, not `shot-3-dark.png`.
- Kebab-case, with a `-light` / `-dark` suffix on every pair.
- Timestamps and ids vary between rebuilds — use fixed names for everything you create, and don't rely on volatile fields.

## Markdown structure
- One file per section: `docs/stratahub-user-documentation/<section>.md`.
- For each screen: a short end-user description (what it is, what they do here), then the theme-matched image via a **relative** path inside a `<picture>` block, so GitHub serves each reader the version matching their theme:
  ```html
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="images/<kebab-name>-dark.png">
    <img alt="<plain description>" src="images/<kebab-name>-light.png">
  </picture>
  ```
  The trailing `<img>` (light) is the fallback for renderers without `<picture>` support.
- Write for the end user. Never narrate the automation, the workspace name, or the capture process.

## Ground-up build = the getting-started guide
The workspace starts empty (all dashboard counts at 0), so the sequence you run to populate it IS the onboarding walkthrough. As the admin, capture each step into `docs/stratahub-user-documentation/getting-started.md`:
1. Create an organization.
2. Add users to it.
3. Assign resources.

Capture the state before and after each meaningful action so the guide shows the flow. Use clearly-fictional names for any demo people — ask the user for a persona set, or use an obviously-fake, consistent roster.

## Demo accounts & theme
When the build creates teacher/student (or any) accounts:
- Use clearly-fictional throwaway passwords and record them in `.playwright/secrets.env` (gitignored). Never use real credentials, and never commit them.
- Immediately set each new account's **Theme → System** (Preferences → Theme). Otherwise the live `colorScheme` flip won't drive that account's view and its dark/light capture will silently fail.

## Other-role views (extension)
To show the teacher or student perspective, log in as that account in a **separate named session with its own profile** — Chromium locks a profile to one process, so a second live login can't share the admin profile:
```
playwright-cli -s=teacher open https://project.adroit.games --profile=./.playwright/profile-teacher
```
Have the user prime each role session by hand once (same as the admin prime, using the credentials recorded in `.playwright/secrets.env`), then capture its screens with the same build-once / flip-live dual-theme loop above. (`profile-*` directories are already gitignored.)
