#!/usr/bin/env bash
# member-status-api-check.sh — exercise a workspace's Member Status API end to end.
#
# Runs the checks from docs/member-status-api/rollout-todo.md §5 and prints
# PASS/FAIL for each one. The first group needs no student and changes
# nothing. The second group, enabled by passing a student id, records survey
# status for that one student — use a test student, ideally on a test
# workspace.
#
# Usage:
#   MEMBER_STATUS_KEY='<KEY>' scripts/member-status-api-check.sh https://<workspace-host> [<user_id>] [<survey>]
#
#   <workspace-host>  the workspace URL, e.g. the Dev MHS workspace
#   <user_id>         optional 24-character hex id of a test student
#   <survey>          optional survey name for the student checks (default: Pre);
#                     pick one the student has not completed for a clean
#                     started → completed ladder
#
# The key comes from the environment so it never lands in this file or in
# shell history. One wrong-key request is sent on purpose; it counts as a
# single failed authentication toward the per-IP throttle (20 per 5 minutes)
# and the next successful request clears it.
#
# Exit status: 0 when every check passed, 1 otherwise, 2 on bad usage.

set -u

HOST="${1:-}"
UID_HEX="${2:-}"
ENTITY="${3:-Pre}"
KEY="${MEMBER_STATUS_KEY:-}"

if [ -z "$HOST" ] || [ -z "$KEY" ]; then
  sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi
HOST="${HOST%/}"
if [ -n "$UID_HEX" ] && ! printf '%s' "$UID_HEX" | grep -Eq '^[0-9a-f]{24}$'; then
  echo "user_id must be 24 lowercase hex characters" >&2
  exit 2
fi

J='Content-Type: application/json'
PASS=0
FAIL=0
BODY=""
CODE=""
EVENT_IDS=""

# request METHOD PATH DATA [extra curl args...]  → sets CODE and BODY
request() {
  local method=$1 path=$2 data=$3
  shift 3
  local tmp
  tmp=$(mktemp)
  if [ -n "$data" ]; then
    CODE=$(curl -sS -m 20 -o "$tmp" -w '%{http_code}' -X "$method" "$HOST$path" -H "$J" -d "$data" "$@") || CODE="000"
  else
    CODE=$(curl -sS -m 20 -o "$tmp" -w '%{http_code}' -X "$method" "$HOST$path" "$@") || CODE="000"
  fi
  BODY=$(cat "$tmp")
  rm -f "$tmp"
}

# jstr FIELD → string value of a top-level JSON string field ("" if absent)
jstr() { printf '%s' "$BODY" | grep -o "\"$1\":\"[^\"]*\"" | head -1 | sed -e 's/^[^:]*://' -e 's/^"//' -e 's/"$//'; }
# jraw FIELD → raw value of a non-string field (true/false/number)
jraw() { printf '%s' "$BODY" | grep -o "\"$1\":[^,}]*" | head -1 | sed 's/^[^:]*://'; }

report() {
  local name=$1 ok=$2 detail=$3
  if [ "$ok" = 1 ]; then PASS=$((PASS + 1)); printf 'PASS  %-48s %s\n' "$name" "$detail"
  else FAIL=$((FAIL + 1)); printf 'FAIL  %-48s %s\n' "$name" "$detail"; fi
}

# expect NAME CODE [ok | <error code>] [extra condition: 1 or 0]
expect() {
  local name=$1 code=$2 want=${3-} extra=${4-1}
  local ok=1 detail="HTTP $CODE"
  [ "$CODE" = "$code" ] || ok=0
  if [ "$want" = ok ]; then
    [ "$(jraw ok)" = true ] || ok=0
  elif [ -n "$want" ]; then
    detail="$detail error=$(jstr error)"
    [ "$(jstr error)" = "$want" ] || ok=0
  fi
  [ "$extra" = 1 ] || ok=0
  local eid
  eid=$(jstr event_id)
  if [ -n "$eid" ]; then detail="$detail event_id=$eid"; EVENT_IDS="$EVENT_IDS $eid"; fi
  report "$name" "$ok" "$detail"
}

status_body() { # status_body USER ENTITY STATE [OCCURRED_AT]
  local occurred=${4-}
  if [ -n "$occurred" ]; then
    printf '{"key":"%s","user_id":"%s","entity":"%s","state":"%s","occurred_at":"%s"}' "$KEY" "$1" "$2" "$3" "$occurred"
  else
    printf '{"key":"%s","user_id":"%s","entity":"%s","state":"%s"}' "$KEY" "$1" "$2" "$3"
  fi
}

echo "Member Status API check against $HOST"
echo

request GET /health ""
expect "health" 200

request POST /api/member-status/ping "{\"key\":\"$KEY\"}"
expect "ping with the key" 200 ok
if [ "$CODE" = 200 ]; then
  echo "      workspace: $(jstr workspace)"
  echo "      entities:  $(printf '%s' "$BODY" | grep -o '"entities":\[[^]]*\]' | sed 's/^"entities"://')"
fi

request POST /api/member-status/ping "{}"
expect "ping, empty body (missing key)" 401 unauthorized

request POST /api/member-status/ping '{"key":"not-the-key"}'
expect "ping, wrong key (one failed auth)" 401 unauthorized

request POST /api/member-status/ping "{}" -H "Authorization: Bearer $KEY"
expect "ping, key in a Bearer header" 200 ok

request GET /api/member-status ""
expect "GET on the endpoint" 405

request POST /api/member-status '[1,2]'
expect "non-object JSON body" 400 bad_json

request POST /api/member-status "$(status_body 0123456789abcdef01234567 "$ENTITY" started)"
expect "unknown student (carries an event_id)" 404 unknown_user "$([ -n "$(jstr event_id)" ] && echo 1 || echo 0)"

request POST /api/member-status "$(status_body student@example.org "$ENTITY" started)"
expect "email instead of hex id" 400 invalid_user_id

request POST /api/member-status "$(status_body 0123456789abcdef01234567 "$ENTITY" opened)"
expect "state 'opened' from the provider" 400 invalid_state

request POST /api/member-status "$(status_body 0123456789abcdef01234567 "$ENTITY" started yesterday)"
expect "bad occurred_at" 400 invalid_occurred_at

if [ -n "$UID_HEX" ]; then
  echo
  echo "Student checks for $UID_HEX on \"$ENTITY\""
  echo

  request POST /api/member-status "$(status_body "$UID_HEX" "$ENTITY" started "$(date -u +%Y-%m-%dT%H:%M:%SZ)")"
  state=$(jstr state)
  cond=0; [ "$(jraw known_entity)" = true ] && [ -n "$(jstr started_at)" ] && { [ "$state" = started ] || [ "$state" = completed ]; } && cond=1
  expect "started" 200 ok "$cond"
  echo "      state=$state started_at=$(jstr started_at)"

  request POST /api/member-status "$(status_body "$UID_HEX" "$ENTITY" completed)"
  s1=$(jstr started_at); c1=$(jstr completed_at)
  cond=0; [ "$(jstr state)" = completed ] && [ -n "$c1" ] && cond=1
  expect "completed" 200 ok "$cond"
  echo "      state=$(jstr state) started_at=$s1 completed_at=$c1"

  lower=$(printf '%s' "$ENTITY" | tr '[:upper:]' '[:lower:]')
  request POST /api/member-status "$(status_body "$UID_HEX" "$lower" Started)"
  cond=0; [ "$(jstr state)" = completed ] && [ "$(jstr started_at)" = "$s1" ] && [ "$(jstr completed_at)" = "$c1" ] && cond=1
  expect "late 'Started' on '$lower': stays completed, same times" 200 ok "$cond"
  echo "      state=$(jstr state) started_at=$(jstr started_at) completed_at=$(jstr completed_at)"

  request POST /api/member-status "$(status_body "$UID_HEX" "Unrecognized Survey Check" started)"
  cond=0; [ "$(jraw known_entity)" = false ] && cond=1
  expect "unrecognized survey name: stored and flagged" 200 ok "$cond"
else
  echo
  echo "(no student id given: started/completed/late-started/unrecognized-name checks skipped)"
fi

echo
echo "$PASS passed, $FAIL failed."
if [ -n "$EVENT_IDS" ]; then
  echo "Event ids to look up in Survey Events (▸ Details shows each request as received):"
  for e in $EVENT_IDS; do echo "  $e"; done
fi
[ "$FAIL" = 0 ]
