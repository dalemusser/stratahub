# ABT Survey URL Identity Options

_Audience: the Abt survey/data team. This document describes the two
configurations StrataHub offers for identifying respondents on Abt survey
links. Last updated: 2026-08-17._

When a student opens one of the Abt survey resources in StrataHub, StrataHub
appends three query parameters to the survey link:

- **`__userid`** (two leading underscores) — the respondent ID
- **`group`** — the student's group
- **`org`** — the student's organization

Two configurations are available. **Both send exactly the same three parameter
names** — they differ only in the values carried. The configuration is set
per-resource on the StrataHub side.

---

## The two options

| Parameter | **ABT-identifiable** | **ABT-deidentified** |
|-----------|----------------------|----------------------|
| `__userid` | Student's login (email address) | Student's StrataHub user ID (24-char hex) |
| `group` | Group name (free text) | Group ID (24-char hex) |
| `org` | Organization name (free text) | Organization ID (24-char hex) |
| Personal data | **Yes — the login is an email address (direct PII)** | No — all values are opaque IDs (de-identified) |

### Example — ABT-identifiable

For a student `acole@students.example.org` in group "Dale's Fun Science" at
"Hillsdale Middle School", a survey link is opened as (the URL shown is a
generic stand-in — the same applies to each real survey link):

```
https://surveys.example.com/wix/p123456789012.aspx?__userid=acole%40students.example.org&group=Dale%27s+Fun+Science&org=Hillsdale+Middle+School
```

Decoded: `__userid="acole@students.example.org"`,
`group="Dale's Fun Science"`, `org="Hillsdale Middle School"`.

### Example — ABT-deidentified

The same student on the same survey:

```
https://surveys.example.com/wix/p123456789012.aspx?__userid=68f138c495cdf54a392b20aa&group=68049e383adb985c4a637183&org=68041da03916f91f24b1ec78
```

Decoded: `__userid="68f138c495cdf54a392b20aa"`,
`group="68049e383adb985c4a637183"`, `org="68041da03916f91f24b1ec78"`.

---

## Value details

- **Hex IDs are 24-character lowercase hexadecimal strings.** They are
  immutable — a student, group, or organization keeps the same ID for the life
  of the study — and globally unique, so they are safe to use as durable keys
  for stored data.
- **Names and logins are editable** by school/site administrators and can
  change mid-study. If you key stored data on them, an edit on the StrataHub
  side will silently break your joins; hex IDs do not have that problem.
- **Values are URL-encoded** (`application/x-www-form-urlencoded`): spaces
  arrive as `+`, and reserved characters are percent-encoded (`@` → `%40`,
  `'` → `%27`). Parse with a standard query-string parser, which decodes these
  for you.
- **Read parameters by name, not position.** Parameter order in the URL is not
  meaningful.
- **A parameter is omitted entirely** if its value is empty on the StrataHub
  side (rather than being sent as an empty string). Always check for presence.

---

## Switching between the options

Because both options use identical parameter names, a survey can be switched
from one option to the other **with no change to the survey configuration**.
The only operational difference is the respondent-ID list preloaded on the Abt
side:

- **ABT-identifiable** — the preloaded IDs are student logins (emails).
- **ABT-deidentified** — the preloaded IDs are 24-character hex user IDs.
  StrataHub provides these for a study roster via its Members Report export.

The `group` and `org` values are captured as open-ended strings either way, so
no change is needed there in either direction.

---

## Privacy note

**ABT-identifiable** places each student's login — an email address, which is
direct PII — in the survey URL. Handle data collected under this option
according to the applicable data-handling agreements (FERPA / COPPA / IRB).

**ABT-deidentified** sends only opaque identifiers. The dataset it produces is
de-identified: the IDs map back to individuals only through the StrataHub
database, which Abt does not hold. Group- and organization-level analysis
remains possible by keying on the `group` and `org` hex IDs.
