# URL Identity Parameters — Guide for Data Recipients

_Audience: developers and researchers who receive StrataHub identity information
via query parameters on a resource's launch URL (surveys, games, research tools)._

When a member opens a StrataHub resource that points at your URL, StrataHub may
append query parameters describing who opened it and where they sit in the
hierarchy. This document explains every parameter, the schemes that group them,
how the values are encoded, and how to parse them safely.

The formal, permanent definition of each parameter is in **`vocabulary.md`** (in
this folder). This guide is the practical, example-driven companion.

---

## How the data arrives

The parameters are standard URL query parameters appended to your launch URL. For
example, your resource URL `https://you.example.com/survey` might be opened as:

```
https://you.example.com/survey?group_id=68049e383adb985c4a637183&org_id=68041da03916f91f24b1ec78&user_id=68f138c495cdf54a392b20aa&ws_id=695f5a3fa323f290a63b3fce
```

Which parameters you receive depends on the **scheme** configured for that
resource on the StrataHub side. There are five (one of which sends nothing).

---

## The parameters

| Parameter | Meaning | Example value | Stable identifier? | Personal data? |
|-----------|---------|---------------|--------------------|----------------|
| `ws` | Workspace subdomain | `dev` | No (admin can rename) | No |
| `ws_id` | Workspace ID (24-char hex) | `695f5a3fa323f290a63b3fce` | **Yes** | No |
| `org` | Organization name | `Hillsdale Middle School` | No (editable) | Low |
| `org_id` | Organization ID (hex) | `68041da03916f91f24b1ec78` | **Yes** | No |
| `group` | Group name | `Dale's Fun Science` | No (editable) | Low |
| `group_id` | Group ID (hex) | `68049e383adb985c4a637183` | **Yes** | No |
| `user` | User's full name | `Adrian Cole` | No (editable) | **Yes (PII)** |
| `user_id` | User ID (hex) | `68f138c495cdf54a392b20aa` | **Yes** | No (pseudonymous) |
| `login_id` | User's login (email) | `acole@students.example.org` | No | **Yes (PII)** |
| `id` | **Legacy only** — user's login (email) under the old parameter name | `acole@students.example.org` | No | **Yes (PII)** |

**Use the `*_id` (hex) values as your durable keys.** They are immutable and
globally unique. Names, subdomains, and logins can be changed by an
administrator; if you key stored data on them, an edit on the StrataHub side will
silently break your joins. `group_id` alone is globally unique and (for anyone
holding the StrataHub database) implies its organization and workspace — but if
you don't hold that mapping and need org/workspace as analysis dimensions, use
the `org_id` / `ws_id` you're sent.

---

## The five schemes

| Scheme | Parameters you will receive |
|--------|-----------------------------|
| **None** | *(none — your URL is opened unchanged)* |
| **De-identified hex IDs** | `ws_id`, `org_id`, `group_id`, `user_id` |
| **Human-readable** | `ws`, `org`, `group`, `user`, `login_id` |
| **Both hex + human** | `ws`, `ws_id`, `org`, `org_id`, `group`, `group_id`, `user`, `user_id`, `login_id` |
| **Legacy** *(deprecated)* | `id` (=login/email), `org`, `group` |

Notes:

- **`user_id` is always the hex ID.** The one exception is the deprecated
  **Legacy** scheme, which carries the login/email under a separate parameter
  named **`id`** (not `user_id`). New integrations should never read `id`; use
  `user_id` (hex) or `login_id` (email).
- A parameter is **omitted entirely** if its value is empty on the StrataHub side
  (rather than being sent as an empty string). Always check for presence.

---

## How the values are encoded

Values use standard URL query encoding (`application/x-www-form-urlencoded`):

- **Spaces become `+`** — `Adrian Cole` → `Adrian+Cole`.
- **Reserved/special characters are percent-encoded** — `@` → `%40`,
  `'` → `%27`. So `acole@students.example.org` → `acole%40students.example.org`
  and `Dale's Fun Science` → `Dale%27s+Fun+Science`.
- **Parameters are sorted alphabetically by key.** Do **not** rely on a specific
  order, and do not assume order to distinguish schemes — read parameters by
  name.

**Parse with a real URL/query-string parser** (e.g. `URLSearchParams` in
JavaScript, `urllib.parse.parse_qs` in Python, `url.Values` in Go). These decode
`+` and `%..` for you. If you split the string by hand on `&` and `=` without
decoding, you will get raw `+` and `%27` in your values.

---

## Worked example

A member — **Adrian Cole** (`acole@students.example.org`), group **Dale's Fun
Science**, organization **Hillsdale Middle School**, workspace **dev** — opens
four different resources, each configured with a different scheme.

The underlying values:

| | Name / human-readable | Hex ID |
|--|----------------------|--------|
| Workspace | `dev` | `695f5a3fa323f290a63b3fce` |
| Organization | `Hillsdale Middle School` | `68041da03916f91f24b1ec78` |
| Group | `Dale's Fun Science` | `68049e383adb985c4a637183` |
| User | `Adrian Cole` | `68f138c495cdf54a392b20aa` |
| Login | `acole@students.example.org` | — |

**De-identified hex IDs:**
```
https://cdn.adroit.games/games/web/learn_addition.html?group_id=68049e383adb985c4a637183&org_id=68041da03916f91f24b1ec78&user_id=68f138c495cdf54a392b20aa&ws_id=695f5a3fa323f290a63b3fce
```

**Human-readable:**
```
https://cdn.adroit.games/games/web/learn_fractions.html?group=Dale%27s+Fun+Science&login_id=acole%40students.example.org&org=Hillsdale+Middle+School&user=Adrian+Cole&ws=dev
```
Decoded: `group="Dale's Fun Science"`, `login_id="acole@students.example.org"`,
`org="Hillsdale Middle School"`, `user="Adrian Cole"`, `ws="dev"`.

**Both hex + human:**
```
https://cdn.adroit.games/games/web/learn_subtraction.html?group=Dale%27s+Fun+Science&group_id=68049e383adb985c4a637183&login_id=acole%40students.example.org&org=Hillsdale+Middle+School&org_id=68041da03916f91f24b1ec78&user=Adrian+Cole&user_id=68f138c495cdf54a392b20aa&ws=dev&ws_id=695f5a3fa323f290a63b3fce
```

**Legacy** (deprecated — note `id` carries the email):
```
https://cdn.adroit.games/games/web/learn_topographic_maps.html?group=Dale%27s+Fun+Science&id=acole%40students.example.org&org=Hillsdale+Middle+School
```

(The raw source for this example is `URL Identity Parameters Example and Test
Case.txt` in this folder.)

---

## Parsing checklist

1. **Read by name, not by position.** Parameter order is alphabetical and not
   meaningful.
2. **URL-decode every value** (or use a query-string parser that does). Expect
   `+` for spaces and `%..` for special characters.
3. **Treat missing as missing.** A parameter is absent if its value was empty; do
   not assume every scheme parameter is always present.
4. **Key your stored data on the hex IDs** (`user_id`, `group_id`, `org_id`,
   `ws_id`). Treat names, `login_id`, and the legacy `id` as display/contact
   data, not as join keys.
5. **`user_id` = hex, always.** Only the deprecated Legacy scheme uses `id`
   (=email). If you're being onboarded today, you should be receiving hex
   (`user_id`), not `id`.

---

## Privacy classification

- **No personal data:** `ws`, `ws_id`, `org_id`, `group_id`, `user_id`.
  (`user_id` is pseudonymous — it maps to a person only via the StrataHub
  database, which you do not hold.)
- **Low (institutional):** `org`, `group` — names of an organization/group.
- **Direct personal data (PII):** `user` (a person's name), `login_id` and the
  legacy `id` (a person's email).

If you receive the De-identified hex IDs scheme, you are receiving a
de-identified dataset. Handle the Human-readable, Both, and Legacy schemes
according to your data-handling agreement (FERPA / COPPA / IRB), since they carry
direct PII.

---

## See also

- **`vocabulary.md`** — the formal, permanent definition of each parameter (the
  contract you can rely on long-term).
