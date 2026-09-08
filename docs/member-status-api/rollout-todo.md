# Member Status API — Rollout To-Do

_Dale's working checklist for taking the Member Status API, the Surveys tab,
and the data viewers (Survey Events, Audit Log) to production and handing the
integration to Abt. Companion to the [Admin Guide](admin-guide.md) and the
[Provider Guide](provider-guide.md)._

> This file lives in a **public repository**. Never paste the shared key, a
> real survey URL, a student's name, or the server's address into it.
> Placeholders: `<KEY>`, `<USER_ID>`, `<workspace-host>` (the MHS workspace
> URL), and `<server>` (the SSH target; user, host, and key file are in
> `stratahub_update/aws_update.sh`, outside this repo). Remote paths are
> relative to the service user's home (`~`).

**Status (2026-09-07 — paused until Abt reports back or a problem arises):**

- Deployed 2026-08-29 (build `20260829-205653`) to both workspace hosts;
  the API, the Surveys tab, resource Survey Tracking, and both viewers are
  live (§1).
- Shared key set and ping-verified on **Dev MHS** and on **MHS** (§2).
- End-to-end checks passed 15/15 on Dev MHS with
  `scripts/member-status-api-check.sh` (§5); the dashboard and Survey Events
  screenshots in the guides are from that run.
- Hand-off sent to Abt 2026-09-07: MHS host, key, an **analyst login on
  MHS** for their developer, and a test student on MHS for their sends —
  Abt implements directly against MHS, not Dev MHS (§4).
- Survey Tracking + ABT-deidentified on the four MHS survey resources is
  being done by someone else (§3, open). §6 starts when Abt's first real
  events arrive; tick Task 7 in the plan then.

The original starting point: everything was on `main` at `5a2827c` with only
the settings-save fix (Task 0) in production; one deploy delivered the rest,
and it changes nothing user-visible until a key is set (the API answers
`401 not_configured` to any keyed request until then).

Order matters: **0 → 1 → 2 → 3 → 5 (self-test) → 4 (hand to Abt) → 6**.
Do steps 2, 3, and 5 on the **Dev MHS** workspace first, then repeat them on
**MHS**. (One binary serves every workspace; the key and the resource links
are per workspace.)

---

## 0. Before deploying

- [ ] Back up the databases (dumps `stratahub`, `stratalog`, `stratasave`,
      `mhsgrader` to `stratahub_update/backups/`):
      ```bash
      cd /Users/dale/Documents/catchupstratahub/stratahub_update && ./aws_db_backup.sh
      ```
- [ ] Keep the running binary so rollback is a rename:
      ```bash
      ssh <server> \
        'cp ~/stratahub-linux-386 ~/stratahub-linux-386.prev'
      ```
- [ ] Confirm `stratahub_update/config.toml` is the one you want uploaded
      (the update script uploads it alongside the binary).
- [ ] Optional: `cd stratahub && make test-safe` for a last local run.

## 1. Deploy

- [x] Run the update (builds `stratahub-linux-386`, uploads config + binary,
      stops/swaps/starts the `stratahub` systemd service):
      ```bash
      cd /Users/dale/Documents/catchupstratahub/stratahub_update && ./aws_update.sh
      ```
- [ ] Watch startup. First start creates the new collections and indexes
      (`member_status`, `member_status_log` with `idx_mslog_*` and the
      400-day `ttl_mslog_received`, and `idx_audit_ws_timestamp` on
      `audit_events`); index builds are logged as `ensuring index`:
      ```bash
      ssh <server> \
        "journalctl -u stratahub -n 300 --no-pager | grep -i 'ensuring index\|member_status\|mslog\|audit_ws\|listening\|error'"
      ```
- [x] Smoke checks (no login needed):
      ```bash
      HOST=https://<workspace-host>
      curl -sS $HOST/health

      # Route is live and CSRF-exempt; no key set yet → 401 not_configured.
      # Send a placeholder key: the API rejects a *missing* key before it
      # looks at the workspace, so an empty body always gets "unauthorized".
      curl -sS -i -X POST $HOST/api/member-status/ping \
        -H 'Content-Type: application/json' -d '{"key":"smoke-test"}'

      # Old audit address redirects to the viewer
      curl -sS -o /dev/null -w '%{http_code} -> %{redirect_url}\n' $HOST/audit
      ```
      Expect `200`, then `HTTP/… 401` with
      `{"ok":false,"error":"not_configured",…}`, then `302 -> …/views/audit-log`.
      (If a key is already set on the workspace the ping returns
      `401 unauthorized` / "Invalid shared key" instead — also fine, and it
      counts as one failed authentication toward the per-IP throttle.)
- [ ] Log in as admin on MHS: menu shows **Audit Log** and **Survey Events**;
      `/views/audit-log` lists recent logins; `/views/survey-events` is empty;
      MHS Dashboard has a **Surveys** tab (all ○ Not started); a resource's
      Edit page shows **Survey Tracking**.
- [ ] **Rollback** (if anything is wrong): the new collections and indexes
      are harmless to leave in place.
      ```bash
      ssh <server> \
        'sudo systemctl stop stratahub && mv ~/stratahub-linux-386.prev ~/stratahub-linux-386 && sudo systemctl start stratahub && sudo systemctl status stratahub'
      ```

## 2. Set the shared key (per workspace)

- [x] Settings → **Member Status API — Shared Key** → **Generate** → **Save**
      → **Show** / **Copy**. Put the key in your password manager now; it is
      shown masked afterwards and is never logged.
- [ ] The change appears in **Audit Log** as `member_status_key_changed`.
- [x] Confirm the key works (also confirms the survey names the workspace
      expects):
      ```bash
      HOST=https://<workspace-host>
      KEY='<KEY>'
      curl -sS -X POST $HOST/api/member-status/ping \
        -H 'Content-Type: application/json' -d "{\"key\":\"$KEY\"}"
      ```
      Expect `{"ok":true,"workspace":"mhs","entities":["Pre","MHS Engagement","EWS Engagement","Post"]}`.
- [ ] **Browser calls (added 2026-09-08).** Abt calls from the survey page,
      not from a server, so list the page's origin: Settings → **Member
      Status API** → **Allowed browser origins** → the origin Abt's
      developer gave (scheme and host only, exactly as their browser reports
      it) → **Save**. Dev MHS first, then MHS. The change appears in
      **Audit Log** as `member_status_origins_changed`.
- [ ] Confirm the browser handshake (no key involved in this one):
      ```bash
      ORIGIN='https://<provider-origin>'
      curl -sS -i -X OPTIONS $HOST/api/member-status -H "Origin: $ORIGIN" \
        -H 'Access-Control-Request-Method: POST' \
        -H 'Access-Control-Request-Headers: content-type' | grep -i '^access-control'
      ```
      Expect `access-control-allow-origin: <the origin>`,
      `access-control-allow-methods: POST`, `access-control-max-age: 3600`,
      and **no** `access-control-allow-credentials`. (Or run the check
      script with `MEMBER_STATUS_ORIGIN`, §5.)

## 3. Link the four survey resources (Opened state)

- [ ] Resources → open each Abt survey resource → **Edit** → **Survey
      Tracking** → pick the survey → Save. Check the View page shows it.

      | Resource (Abt link) | Survey Tracking |
      |---------------------|-----------------|
      | Pre-survey          | Pre             |
      | MHS engagement      | MHS Engagement  |
      | EWS engagement      | EWS Engagement  |
      | Post-survey         | Post            |

- [ ] **Check each resource's URL identity mode.** The API identifies
      students by the 24-character hex `user_id` — the value StrataHub puts
      in `__userid` under **abt-deidentified** (or `hex`/`both`). Under
      **abt-identifiable** Abt receives the login email and has no hex id to
      report, so every callback would fail with `400 invalid_user_id`. If the
      resources are still on the identifiable mode, Abt needs the hex ids
      before the integration can work (see the open item in §7).
- [ ] Verify Opened: launch one linked survey as a test **member** (a test
      student account in a test group), then check MHS Dashboard → Surveys
      shows ◔ Opened for them, and Survey Events shows a `Launch` row.

## 4. Hand the integration to Abt

- [x] Send, through a private channel for the key (not the same message as
      the rest if you can help it):
  - the **Provider Guide** (`docs/member-status-api/provider-guide.md` — the
    complete contract: endpoint, payload, names, semantics, error codes,
    curl/Python examples);
  - the **host** (the MHS workspace URL; not written here, this repo is public);
  - the **key**;
  - the four `entity` strings exactly: `Pre`, `MHS Engagement`,
    `EWS Engagement`, `Post` (the ping response lists them too);
  - the reminder that `user_id` is the `__userid` value from the survey links.
- [x] Ask them to: (1) run the `ping` first; (2) send `started` and
      `completed` as they happen, one request per event; (3) quote the
      `event_id` from any response when asking about a specific send.
- [x] Optional: give Abt's developer an **analyst** login on the **Dev MHS**
      workspace (System Users → new analyst) so they can watch their test
      events land in Survey Events. Analysts see the whole workspace, so do
      this on the test workspace, not on MHS.
- [ ] **2026-09-08:** tell Abt's developer that their origin is listed on
      MHS and point them at the provider guide's *Calling from a web page*
      section (the `fetch` example; a CORS error in the console means the
      origin does not match what is listed; the key being visible in the
      page is accepted on our side).
- [x] Message skeleton:

      > StrataHub is ready to receive survey status. Endpoint and format are in
      > the attached guide; the host is <workspace-host>. The shared key
      > follows separately. Please start with the ping call to confirm the key
      > and the survey names, then send `started`/`completed` events as they
      > occur. Responses include an `event_id` — include it if you ask us about
      > a specific event. The student id to send is the `__userid` value on the
      > survey links.

## 5. Verify end to end (curl)

**Quick way:** `scripts/member-status-api-check.sh` runs every check in this
section and prints PASS/FAIL for each, with the event ids to look up
afterwards. The key goes in the environment, never on the command line:

```bash
MEMBER_STATUS_KEY='<KEY>' scripts/member-status-api-check.sh $HOST <USER_ID>
# optional third argument: a survey the student has not completed yet (default Pre)
# add MEMBER_STATUS_ORIGIN='https://<provider-origin>' for the three browser (CORS) checks
```

The commands below are the same checks by hand.

Set these once per shell. Get a test student's `user_id` from Survey Events
(filter Student by name; the id is in the row tooltip and the ▸ Details
panel), from the Members Report CSV, or from the launch row created in §3.

```bash
HOST=https://<workspace-host>
KEY='<KEY>'
UID_HEX='<USER_ID>'          # 24 lowercase hex characters
J='Content-Type: application/json'
```

- [x] Ping:
      ```bash
      curl -sS -X POST $HOST/api/member-status/ping -H "$J" -d "{\"key\":\"$KEY\"}"
      ```
      → `{"ok":true,"workspace":"mhs","entities":[…]}`
- [x] Started:
      ```bash
      curl -sS -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"$KEY\",\"user_id\":\"$UID_HEX\",\"entity\":\"Pre\",\"state\":\"started\",\"occurred_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
      ```
      → `200 {"ok":true,"event_id":"…","entity_key":"pre","known_entity":true,"state":"started","started_at":"…"}`
- [x] Completed:
      ```bash
      curl -sS -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"$KEY\",\"user_id\":\"$UID_HEX\",\"entity\":\"Pre\",\"state\":\"completed\"}"
      ```
      → `200 … "state":"completed","started_at":"…","completed_at":"…"`
- [x] Idempotent / monotonic — resend `started`; status stays `completed`
      and the timestamps do not move:
      ```bash
      curl -sS -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"$KEY\",\"user_id\":\"$UID_HEX\",\"entity\":\"pre\",\"state\":\"Started\"}"
      ```
      (also shows the case-insensitive matching on `entity` and `state`)
- [x] Unknown student → `404 unknown_user`, and the request still gets an
      `event_id` (it is in Survey Events under Result → *Rejected: unknown user*):
      ```bash
      curl -sS -i -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"$KEY\",\"user_id\":\"0123456789abcdef01234567\",\"entity\":\"Pre\",\"state\":\"started\"}"
      ```
- [x] Unrecognized survey name → still `200`, `"known_entity":false`, flagged
      amber in Survey Events:
      ```bash
      curl -sS -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"$KEY\",\"user_id\":\"$UID_HEX\",\"entity\":\"Mid Survey\",\"state\":\"started\"}"
      ```
- [x] Wrong key → `401 unauthorized` (do this once or twice, not in a loop:
      20 failures from one address in 5 minutes trips `429 rate_limited`;
      it clears on the next success or after 5 minutes):
      ```bash
      curl -sS -i -X POST $HOST/api/member-status -H "$J" \
        -d "{\"key\":\"wrong\",\"user_id\":\"$UID_HEX\",\"entity\":\"Pre\",\"state\":\"started\"}"
      ```
- [x] Then look at it from the inside:
  - **Survey Events** (tick *Live* while sending): one row per request
    above, `Provider` source, the states as sent, the results; ▸ Details
    shows the request exactly as sent and the `event_id` you got back.
    `images/survey-events-test.png` shows what the finished run looks like.
  - **MHS Dashboard → Surveys**: the test student shows ◑ Completed for Pre
    with the first `started`/`completed` receipt times; the modal links to
    their Survey Events.
  - **Server log** for the warnings on the unknown-user and
    unrecognized-name sends:
    ```bash
    ssh <server> \
      "journalctl -u stratahub -n 200 --no-pager | grep -i 'member status'"
    ```
- [ ] Clean up the test student's status if you used a real class group
      (the store keeps history; simplest is to use a test group that is not
      part of the study).

## 6. After go-live

- [ ] First week: open Survey Events daily with Result → *Rejected (any)* on
      MHS. `unknown_user` rejections mean Abt is sending an id that is not an
      active member here (wrong workspace, removed student, or an email
      instead of the hex id).
- [ ] If a name from Abt changes: add it under `api_names` in
      `internal/app/resources/mhs_member_status.json`, redeploy; events already
      sent under the new name were stored and appear once it is configured
      (Admin Guide §4).
- [ ] Key rotation: Generate + Save; the old key stops at once, so agree the
      switch time with Abt first.
- [ ] Record completion: tick Task 7 in `docs/member-status-api/plan.md` and
      note the go-live date.

## 7. Open items to keep in view

- **Abt's identity-mode decision** (identifiable vs de-identified links). The
  callback only works when Abt holds the hex `user_id` — i.e. the resources
  are on `abt-deidentified` (or Abt has been given the hex ids another way).
  Don't hand Abt the Members Report CSV for this; it is the re-identification
  key.
- The Survey Events view shows student names to analysts (as the Members
  Report does). Fine for staff; give an outside developer an analyst login
  only on a test workspace.
- **The key is in Abt's survey page** (browser calls, accepted 2026-09-08):
  anyone who views the page source can read it. It can only report status.
  If it turns up outside the survey site, rotate it (§6) and tell Abt.
- Nothing here touches `stratalog`, `stratasave`, or `mhsgrader`.

## Reference

| Thing | Where |
|-------|-------|
| Deploy / stop / backup scripts | `/Users/dale/Documents/catchupstratahub/stratahub_update/{aws_update.sh, aws_stop.sh, aws_db_backup.sh}` |
| Server | SSH target and key file: see `stratahub_update/aws_update.sh` (not in this repo); service `stratahub`; binary `~/stratahub-linux-386` and config `~/config.toml` in the service user's home |
| Logs | `journalctl -u stratahub -f` |
| Key setting | Settings → Member Status API — Shared Key (`/settings`) |
| Resource link | Resources → Edit → Survey Tracking |
| Viewers | `/views/survey-events`, `/views/audit-log` (`/audit` redirects) |
| Survey list | `internal/app/resources/mhs_member_status.json` (embedded; edit → redeploy) |
| Contract for Abt | `docs/member-status-api/provider-guide.md` |
| Admin how-to + troubleshooting | `docs/member-status-api/admin-guide.md` (§6 has the symptom → fix table) |
| Check script | `scripts/member-status-api-check.sh <host> [<user_id>] [<survey>]`, key in `MEMBER_STATUS_KEY`, provider origin (optional, adds the CORS checks) in `MEMBER_STATUS_ORIGIN` — [how-to](check-script.md) |
| Allowed browser origins | Settings → Member Status API → Allowed browser origins (`/settings`) |
