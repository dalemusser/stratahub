# URL Identity Parameters — Guide for Admins & Coordinators

_Audience: anyone who creates or edits Resources in StrataHub (admins, and
coordinators who have the "manage resources" permission)._

When a member opens a URL-based Resource, StrataHub can append information about
**who** opened it and **where they sit** (their group, organization, workspace)
to the launch URL as query parameters. The receiving site (a survey, a game, a
research tool) reads those parameters.

Each Resource has a **URL Identity Parameters** setting that controls *what, if
anything,* gets appended. This guide explains the choices and how to set them.

---

## Quick version

- The setting lives on the **create/edit Resource** form, labeled **"URL Identity
  Parameters."**
- The default is **None** — nothing is appended. Most resources should stay
  None.
- For research/data collection, use **De-identified hex IDs** — it's the
  privacy-safe, analytically-stable choice.
- **Human-readable**, **Both**, and **Legacy** put student names and/or logins in
  the URL (PII) — only use them when there's a specific, approved reason.
- The setting only matters for **URL resources** (it has no effect on uploaded
  files).

---

## Where to set it

On the **Add Resource** / **Edit Resource** form there's a select menu titled
**URL Identity Parameters**, just below the Content (URL/File) section. Pick the
scheme and save. If you pick a scheme that includes personal information, an
amber warning appears reminding you that the URL will carry PII.

You can see what each Resource currently uses in two places:

- The **Resources list** has an **Identity** column showing a colored badge
  (gray **None**, green **Hex**, red **Human/Both/Legacy**), with a key above the
  table.
- The **View Resource** page shows the selected scheme with its full description.

---

## The choices

| Scheme | What gets added to the URL | Contains PII? | When to use |
|--------|----------------------------|---------------|-------------|
| **None** *(default)* | Nothing | No | The default. Any resource that doesn't need to know who opened it. |
| **De-identified hex IDs** | `ws_id`, `org_id`, `group_id`, `user_id` (opaque 24-char IDs) | No | **Recommended** for surveys, tests, and research tools that store data. Privacy-safe and stable. |
| **Human-readable** | `ws`, `org`, `group`, `user`, `login_id` (names + email) | **Yes** | Only when a consumer specifically needs readable names, or for debugging. |
| **Both hex + human** | Everything from both schemes above | **Yes** | Debugging/verification, when you want to see readable values next to the IDs. |
| **Legacy** | `id` (=login/email), `org`, `group` | **Yes** | Deprecated. Only for older consumers that haven't migrated to hex yet. |

### Why "De-identified hex IDs" is the recommended choice

- **Privacy.** The values are opaque IDs, not names or emails. They identify a
  student only to someone holding the StrataHub database — which the research
  consumer does not. This is the FERPA / COPPA / IRB-appropriate choice.
- **Stability.** Hex IDs never change. Names and group/organization labels *can*
  be edited by an admin at any time. If a consumer stores a readable name as
  their key and someone later renames the group, their data silently stops
  lining up. Hex IDs don't have that problem.

### When *not* to use the PII schemes

`Human-readable`, `Both`, and `Legacy` place a student's name and/or login (email)
directly in the URL. URLs get logged, cached, and shared more easily than you'd
think. Only choose these when:

- a consumer has a specific, documented need for readable values, **and**
- that use is covered by your data-handling agreements (IRB / FERPA / COPPA).

When in doubt, use **De-identified hex IDs**.

---

## Worked example

For a member **Adrian Cole** (`acole@students.example.org`) in group **Dale's Fun
Science**, organization **Hillsdale Middle School**, workspace **dev**, here is
what each scheme appends to a resource's launch URL:

**None** — the URL is unchanged:
```
https://cdn.adroit.games/games/web/learn_addition.html
```

**De-identified hex IDs:**
```
…learn_addition.html?group_id=68049e383adb985c4a637183&org_id=68041da03916f91f24b1ec78&user_id=68f138c495cdf54a392b20aa&ws_id=695f5a3fa323f290a63b3fce
```

**Human-readable:**
```
…learn_fractions.html?group=Dale%27s+Fun+Science&login_id=acole%40students.example.org&org=Hillsdale+Middle+School&user=Adrian+Cole&ws=dev
```

**Both hex + human:**
```
…learn_subtraction.html?group=Dale%27s+Fun+Science&group_id=68049e383adb985c4a637183&login_id=acole%40students.example.org&org=Hillsdale+Middle+School&org_id=68041da03916f91f24b1ec78&user=Adrian+Cole&user_id=68f138c495cdf54a392b20aa&ws=dev&ws_id=695f5a3fa323f290a63b3fce
```

**Legacy:**
```
…learn_topographic_maps.html?group=Dale%27s+Fun+Science&id=acole%40students.example.org&org=Hillsdale+Middle+School
```

(Spaces show up as `+` and special characters as `%..` because the values are
URL-encoded. The full raw example lives in
`URL Identity Parameters Example and Test Case.txt` in this folder.)

---

## Things to know

- **URL resources only.** This setting does nothing for resources that are an
  uploaded file rather than a launch URL.
- **Changing it is safe and immediate.** It takes effect the next time a member
  opens the resource. Nothing is stored on the student's side.
- **Coordinators are workspace-wide for resources.** A coordinator with the
  manage-resources permission can set this on any resource in their workspace —
  resources are not scoped to a single organization.
- **Legacy is being retired.** It exists only so existing data consumers keep
  working while they switch to hex. Don't choose Legacy for anything new; move
  existing Legacy resources to **De-identified hex IDs** when the consumer is
  ready.

## Handing off to the data recipient

If you're turning on identity parameters for a survey/research consumer, give
them **`data-consumer-guide.md`** (in this folder). It explains exactly which
parameters each scheme sends and how the values are encoded, so they can parse
them correctly.
