# Strata Hub user documentation — kickoff brief

## The task

Produce **end-user documentation** for Strata Hub by driving the live `project.adroit.games` workspace, capturing theme-matched screenshots, and writing markdown guides. The audience is people *using* Strata Hub (administrators and group leaders), not developers.

The first deliverable is a **getting-started / onboarding guide**. You build it from an empty workspace — creating the organization, groups, leaders, members, and resource assignments from the supplied roster — and capture each step as you go, so the walkthrough doubles as the real setup the screenshots show. The workspace is a disposable, isolated sandbox with no real users or PII, so you may create, edit, and log into accounts freely.

## Read these first

- **`.claude/skills/stratahub-docs/SKILL.md`** — read and follow it exactly. It defines the capture mechanics (build a screen once, flip the theme live, shoot light + dark), file naming, the `<picture>` markdown output, and the theme handling. It is the authoritative "how."
- **`docs/stratahub-user-documentation/persona-roster.md`** — the demo data to create. Use only these identities; do not invent additional people.

## Current state (assume true)

- Run every command from the repo root: `/Users/dale/Documents/catchupstratahub/stratahub`.
- The **admin session is already primed and logged in** via the persistent profile — opening `https://project.adroit.games` lands on `/dashboard`. If it ever lands on a login page instead, **STOP and ask me to re-prime it; never type the admin password yourself.**
- The workspace is **empty** (dashboard counts all at 0). Branding (Intelligence Builders logo + footer) and the admin account (Dale Musser) are already set, and the admin theme is **System**.
- The capture config (`.playwright/cli.config.json`) forces headed Chrome at 1440×900, light default. Screenshots require a **full repo-relative `--filename` path** into `docs/stratahub-user-documentation/images` — a bare filename lands in the wrong place (see the skill).
- Credentials live in `.playwright/secrets.env` (gitignored). **Never print, echo, or commit it.**

## Build & document sequence (the getting-started spine)

Using the skill's build-once / flip-live dual capture on every screen, work through the onboarding flow in order, capturing the state before and after each meaningful action, and writing it up in `docs/stratahub-user-documentation/getting-started.md`:

1. **Empty dashboard** (counts at 0) — establishes the starting point.
2. **Create the organization:** Riverbend Middle School.
3. **Create the two groups:** Grade 7 Science — Section A, and Section B.
4. **Add the leaders:** Marcus Webb → Section A, Diane Okafor → Section B. Set each new account's **Theme → System** immediately after creating it. For Marcus Webb, set a password and record `LEADER_EMAIL` / `LEADER_PASSWORD` in `secrets.env`.
5. **Add the members** (the eight in the roster, across the two sections). Set **Theme → System** on any account that will be logged into; for Aisha Rahman, set a password and record `MEMBER_EMAIL` / `MEMBER_PASSWORD` in `secrets.env`. The other members are roster-only — any throwaway password is fine.
6. **Create the resources (4) and materials (2)**, then assign per the roster (all four resources to Section A, the first two to Section B).
7. **Return to the dashboard** to show the now-populated counts.

## Output conventions (skill is authoritative; the essentials)

- Each screen: a short, plain end-user description (what it is, what they do here), then the image as a `<picture>` block with **relative** paths and a `-light` / `-dark` pair, both themes captured via the live flip.
- Use Strata Hub's own on-screen labels — Organizations, Groups, Leaders, Members, Resources, Materials.
- **Write for the end user. Never narrate the automation, the capture process, or the workspace's internal name ("Project").**

## Guardrails

- No credentials in any committed file. Passwords only in `secrets.env`.
- Set **Theme → System** on every account you create, or its light/dark capture will silently fail to flip.
- Keep the primed browser window open between commands; if it closes, reopen the project URL (the profile keeps you logged in). Don't close it mid-pass.
- If a UI flow doesn't match what's described here — a missing field, an extra step, a different assignment mechanism — **don't guess your way through a destructive action.** Capture what you can, note the discrepancy, and ask me.
- Don't commit anything. Leave the new markdown and images as working-tree changes for me to review and commit.

## Checkpoint — stop after step 4

This is the first full pass against the live sandbox, so don't run the whole thing end to end. Complete **steps 1–4** (empty dashboard → organization → both groups → both leaders), capturing and writing as you go, then **stop and show me**:

- the `getting-started.md` draft so far,
- the image files produced (confirm they're in `docs/stratahub-user-documentation/images`), and
- which `secrets.env` keys you set (key names only, not values).

I'll review the pattern before you continue to members, resources, and assignments.

## After getting-started

Once the onboarding guide is approved, the same pattern extends to per-feature reference sections (Organizations, Groups, Members, Reports, Resources, and so on) and to the **leader and member role views** — logging in as Marcus Webb and Aisha Rahman through the named sessions described in the skill. We'll scope those after reviewing the first guide.