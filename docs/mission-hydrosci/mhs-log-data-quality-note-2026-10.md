# Mission HydroSci game log data: a data-quality note

**Date:** 2026-10-02
**For:** the research team and the research partner's analysts.
**About:** the game-event log data collected from Mission HydroSci (the `mhs` entries in the log service), from February 2026 to the present.

## Summary

Every game-event entry has a small set of fields that were captured when the event happened and are reliable in every build: the event type, the student, the event's own timestamp, the scene, the build version, and, where the type has one, the event key. The event's **details** (the `data` object) are reliable in builds from October 2026 onwards, but in every build from February 2026 through the one students played in September 2026 they can belong to a *later* event from the same part of the game. This happened to a small share of most event types in normal play, to a large share of puzzle-piece events, and to almost every entry that was sent late (after a connection problem). The true details of an affected entry cannot be recovered, but for three event types an affected entry can be recognised exactly, and for the rest the conditions under which it happened are known. This note says what to trust, how to recognise the affected entries, the measured rates, and what the October builds add to help.

## 1. The shape of an entry

Each entry is one JSON document with an envelope and a details object:

| Field | Set when | Reliable in all builds? |
|---|---|---|
| `eventType` | the event happens | yes |
| `user_id` | the event happens | yes (see §6 for the one exception) |
| `timestamp` | the event happens (the game's clock, ISO 8601, UTC) | yes |
| `sceneName` | the event happens | yes |
| `version` | the build's version string | yes |
| `eventKey` | the event happens; present for dialogue-node and quest events (`DialogueNodeEvent:<conversation>:<node>`, `<questEventType>:<questId>`) | yes |
| `data` | the event happens, **but serialized only when the entry is sent** | **no, in builds before October 2026** (§2) |
| `device` | attached when sent | yes |
| `serverTimestamp` | added by the log service on receipt | yes |

## 2. What went wrong with `data`

Many events are produced by reusable logging components in the game. In the builds of February to September 2026, each such component kept **one** details dictionary: on every event it cleared it, refilled it, and handed that same object to the logger, which stored a reference to it in the queued entry and turned it into text only when the entry was sent. The schools' build also sent one entry per request. So whenever events from one component came faster than they could be sent, the entries still waiting were serialized with the **latest** event's details. The entry's envelope was unaffected, because those values were copied at logging time.

Two consequences follow. In normal play on a working connection, entries wait only a fraction of a second, so only events that come in quick bursts from one component were affected. Anything that waited longer, because the connection was slow or down and the game saved its queue to send later, was affected almost completely. The position event had the same problem through a different route (two reused dictionaries), and the components that log the game's shared variables stored the variable objects rather than their values, so those entries show the variable's value at sending time.

The cause was found in the game's source on 2026-09-30 and is fixed in the builds the game team made on 2026-10-01 (version string `20261001-12464` and later; §7). Nothing can restore the true details of entries recorded before that.

## 3. Which event types are affected

Affected (produced through the reusable components, or through the reused position dictionaries): `DialogueEvent` (including the dialogue-node events), `questEvent`, `argumentationEvent`, `argumentationNodeEvent`, `argumentationToolEvent`, `argumentationAnswerEvent`, `InputEvent`, `PuzzlePieceVisibleEvent`, `ObjectInterEvent`, `Soil Key Puzzle`, `soilMachine`, `WaterChamberEvent`, `TerasGardenBox`, `EndOfUnit`, `DEBUGMenu`, and `PlayerPositionEvent` (in builds from August 2026 onwards).

Not affected: `TopographicMapEvent`, and the dialogue and quest events logged through the game's wrapper functions, which built fresh details for every call.

## 4. How to recognise an affected entry

For three types the entry carries an independent copy of one detail in its envelope, so a disagreement is proof that the details were overwritten:

| Event type | Compare | An affected entry shows |
|---|---|---|
| dialogue-node events (`eventType: DialogueEvent`, `eventKey` beginning `DialogueNodeEvent:`) | `eventKey` = `DialogueNodeEvent:<conversation>:<node>` against `data.conversationId` and `data.nodeId` | different conversation or node ids |
| `questEvent` | `eventKey` = `<questEventType>:<questId>` against `data.questEventType` and `data.questID` | different type or quest id |
| `PuzzlePieceVisibleEvent` | the entry's `timestamp` against `data.timestamp` (the component writes its own time into the details, and the logger copied it to the envelope when the event happened) | different timestamps; `data.pieceId` and `data.actionType` then belong to a later piece |

For all other affected types there is no independent copy, so there is no exact test. Two signs identify most cases:

- **Late arrival.** `serverTimestamp` minus `timestamp` of more than about five seconds means the entry waited in the game's queue; more than two minutes means it was saved on the device and sent later. The longer the wait, the more likely the details are a later event's (§5).
- **Runs of identical details.** Several consecutive entries from one student with the same `data` but different `timestamp`s are the overwritten run: all of them carry the last event's details.

## 5. Measured rates

Measured on 2026-09-28 over the log service's data for the schools' build of September 2026 (`version` `20260914-`, all accounts since 2026-09-14), using the exact tests of §4:

| Event type | Sent within about 5 s (normal play) | 5 s to 2 min late | More than 2 min late (a saved backlog) |
|---|---|---|---|
| dialogue-node events | 2.5% (1,357 entries, 123 accounts) | 52% | 99.9% |
| `questEvent` | 2.9% (101 entries) | 9% | 98% |
| `PuzzlePieceVisibleEvent` | 43% (86,872 entries, 110 accounts) | 94% | 96% |

Puzzle-piece events are so much worse because the pieces change visibility in bursts, several in one frame as the camera moves, and the old sender posted one entry per request, so most of a burst was still waiting when the next piece overwrote the shared details.

Across builds: every build since `20260209` shows about 1% to 5% of dialogue-node events overwritten in normal play, including the spring study's main build (`20260313-10763`: 3.1%, 10,026 entries). Builds before February 2026 used a different entry format and are not comparable by these tests. The game team's interim builds of September 2026 (`20260921-12438`, `20260925-12446`) do not have the fix either (85.8% for build 12446).

The rates for the other affected types cannot be measured exactly. For normal play they are likely in the low single digits, like dialogue and quest events, except where a component logs in bursts.

## 6. Two other things to know about the pre-October data

- **Each session's first event is missing.** The schools' builds logged one event (a debug-menu state change) before the student's identity was known; the log service refused it as having no `user_id`, so it was never stored. Nothing else was lost by this; the first *stored* event of a session is a few hundred milliseconds later.
- **Duplicate entries from testing, dev accounts only.** On 2026-09-25 and 2026-09-28 the interim builds `20260921-12438` and `20260925-12446` resent the same events in a loop and wrote about 700,000 duplicate entries from three dev accounts. Those builds were only ever available on the development site, never to students, so excluding entries whose `version` is `20260921-12438` or `20260925-12446` removes them. If they are kept, de-duplicate on the quadruple (`user_id`, `eventType`, `timestamp`, `data`).
- **Recovered backlogs.** Since 2026-09-28 the game's host page repairs a device whose saved queue was stuck, so backlogs that would have been lost now arrive, with the right type, time, student and key, but with details that are almost all overwritten (the last row of §5). They are recognisable by lateness: `serverTimestamp` more than two minutes after `timestamp`, `version` `20260914-`.

## 7. What the October 2026 builds change

From the game team's builds of 2026-10-01 (version string `20261001-12464` and later), every event gets its own details at the moment it happens, the variable-backed entries carry values, and the position event builds fresh details, so `data` is reliable. On the development site these builds were checked by the §4 tests: 0 disagreements in 8 dialogue-node, 2 quest and 36 puzzle-piece events, and every event arrived exactly once.

The same builds add fields that make analysis easier:

| New field | Meaning | Use |
|---|---|---|
| `session_id` | one id per launch of the game | group a session's events; join with the session's first events |
| `seq` | the event's number within its session, starting at 1 | order events without relying on the device clock; a gap means an event was lost or dropped on the device |
| `entry_id` | `<session_id>:<seq>` | a unique key per entry; de-duplicate on it |
| `recovered` | `true` when the entry was saved on the device and sent after a reload or a connection problem | the details are correct in these builds, but the entry arrived late; absent on live entries |
| `sent_at` | the game's clock when the entry was sent | with `serverTimestamp`, the device's clock skew, and a lateness measure that does not depend on the event time |

Entries saved by an older build and sent by a newer one carry `recovered: true` but none of the ids.

## 8. Practical recommendations

1. Trust `eventType`, `user_id`, `timestamp`, `sceneName`, `version` and `eventKey` in every entry; count and order events with them.
2. For dialogue-node, quest and puzzle-piece events from builds before `20261001-12464`, drop or flag the entries that fail the §4 test; this is exact.
3. For the other affected types from those builds, treat `data` as unreliable in any entry that arrived more than about five seconds after its `timestamp`, and in runs of identical details; weigh results that depend on those details accordingly.
4. Exclude `version` `20260921-12438` and `20260925-12446` (dev-only test builds), or de-duplicate as in §6.
5. From `20261001-12464` onwards, use `entry_id` as the key and `recovered` to separate live from late entries.

## 9. Questions

Please send questions about the data to the project lead; the measurements in §5 can be re-run on request for any build, event type or date range.
