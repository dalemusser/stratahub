# Claude API Request: Student Summary Feature

This document describes what is sent to the Anthropic Claude API when a leader, coordinator, or admin clicks the summary icon for a student on the MHS Dashboard.

---

## 1. System Prompt (context + instructions)

The system prompt has two parts: the instructions for how to write the summary, and the full curriculum context document embedded within it.

### Instructions

```
You are an educational assessment specialist analyzing student performance data
from Mission HydroSci, a science adventure game that teaches hydrology and
scientific argumentation.

Below is the curriculum context document that explains each progress point's
learning objectives, what is assessed, and what flagged results indicate:

<curriculum_context>
[... full curriculum context — see below ...]
</curriculum_context>

Your task is to write a clear, professional performance summary for a teacher
or instructional leader. The summary should:

1. Describe the student's overall progress through the game
2. Highlight areas of strength (passed with low mistake counts, fast completion)
3. Identify areas of concern (flagged results, high mistake counts, many attempts)
4. Connect flagged results to specific learning gaps using the curriculum context
5. Suggest instructional focus areas based on patterns in the data
6. Refer to the student as "this student" or "the student" (no name is provided for privacy)

Write in 2-4 paragraphs. Be specific about which concepts the student
understands or struggles with. Avoid generic statements. Reference specific
progress points by their descriptive name (not IDs like "u2p3"). Do not include
raw metrics numbers unless they help illustrate a point.
```

### Curriculum Context

The curriculum context is the full contents of `curriculum_context.md` (~294 lines, ~3,400 words, ~4,500 tokens). It is embedded into the Go binary at compile time and included in every request. It covers:

- Game overview
- All 5 units and 26 progress points
- For each progress point: learning objectives, assessed skills, what flagged results mean, scoring rules, and relevant metrics
- Cross-cutting skills progression (scientific argumentation and hydrology concepts)
- How to interpret attempt counts, durations, mistake counts, and reason codes

View the full document: [Curriculum Context and Learning Objectives](curriculum_context.html)

---

## 2. Student Data (user message)

The user message contains the student's grade data formatted as structured text. No student name or other personally identifiable information is sent to the API.

### Format

```
Please write a performance summary based on the following grade data:

Current Unit: [unit ID]

## [Unit ID]: [Unit Title]
- [Point ID] ([Short Name]): [status] [reason] [duration] [active duration] [metrics] [attempts] [history]
- [Point ID] ([Short Name]): Not started
...
```

### Example

```
Please write a performance summary based on the following grade data:

Current Unit: unit3

## unit1: Orientation and Scientific Argumentation Basics
- u1p1 (Space Legs): passed [duration: 245s] [active: 230s] [metrics: mistakeCount=0]
- u1p2 (Info & Intros): passed [duration: 180s] [active: 170s] [metrics: mistakeCount=0]
- u1p3 (Defend Expedition): passed [duration: 120s] [active: 115s] [metrics: mistakeCount=0]
- u1p4 (What Was That?): passed [duration: 95s] [active: 90s]

## unit2: Topographic Maps and Watersheds
- u2p1 (Escape the Ruin): passed [duration: 310s] [active: 290s] [metrics: mistakeCount=0]
- u2p2 (Foraged Forging): flagged [reason: BAD_FEEDBACK] [duration: 420s] [active: 400s] [metrics: mistakeCount=6]
- u2p3 (Band Together II): passed [duration: 350s] [active: 330s] [metrics: mistakeCount=2]
- u2p4 (Investigate Temple): passed [duration: 280s] [active: 260s] [metrics: mistakeCount=1]
- u2p5 (Classified Info): passed [duration: 200s] [active: 190s] [metrics: posCount=5, mistakeCount=1, score=4.67]
- u2p6 (Which Watershed? I): flagged [reason: MISSING_SUCCESS_NODE] [duration: 150s] [active: 140s] [metrics: mistakeCount=3]
- u2p7 (Which Watershed? II): passed [duration: 180s] [active: 170s] [metrics: mistakeCount=2]

## unit3: Water Flow Direction and Dissolved Materials
- u3p1 (Supply Run): flagged [reason: TOO_MANY_NEGATIVES] [duration: 195s] [active: 185s] [metrics: count=0, mistakeCount=3]
- u3p2 (Pollution Solution I): Not started
- u3p3 (Pollution Solution II): Not started
- u3p4 (Forsaken Facility): Not started
- u3p5 (Balanced Ecosystem): Not started

## unit4: Groundwater, Soil Infiltration, and the Water Table
- u4p1 (Well What Have We Here?): Not started
- u4p2 (Power Play 1&2): Not started
- u4p3 (Power Play 3&4): Not started
- u4p4 (Power Play 5 + Drill): Not started
- u4p5 (Saving Anderson): Not started
- u4p6 (Desert Delicacies): Not started

## unit5: The Water Cycle
- u5p1 (Nickel 1&2): Not started
- u5p2 (Nickel 3&4): Not started
- u5p3 (What Happened Here?): Not started
- u5p4 (Water Solutions): Not started
```

Each progress point includes (when available):
- **Status**: `passed`, `flagged`, `active`, or `Not started`
- **Reason code**: Only present for flagged items (e.g., `BAD_FEEDBACK`, `TOO_MANY_NEGATIVES`)
- **Duration**: Wall-clock time in seconds
- **Active duration**: Engaged time in seconds (excludes idle gaps >5 min)
- **Metrics**: Key-value pairs specific to each progress point's grading rule
- **Attempt count**: Shown when the student has attempted a point more than once
- **History**: Previous attempt statuses when multiple attempts exist (e.g., `flagged(BAD_FEEDBACK) → passed`)

---

## 3. API Configuration

| Parameter | Value |
|-----------|-------|
| **API endpoint** | `POST https://api.anthropic.com/v1/messages` |
| **API version header** | `anthropic-version: 2023-06-01` |
| **Model** | Configurable per workspace in Settings. Default: `claude-sonnet-4-20250514`. Options: Haiku 4.5 (`claude-haiku-4-5-20251001`), Sonnet 4 (`claude-sonnet-4-20250514`), Opus 4 (`claude-opus-4-20250514`) |
| **max_tokens** | `1024` |
| **Request timeout** | 90 seconds (context timeout and HTTP client timeout) |
| **Authentication** | `x-api-key` header with the Anthropic API key from `STRATAHUB_CLAUDE_API_KEY` env var |

---

## 4. Response Handling

The API returns a JSON response with a `content` array of text blocks. The text from all blocks is joined and returned to the browser as:

```json
{
  "summary": "This student has made strong progress through the initial units...",
  "user_id": "6612a3f1c2e4b5d6e7f89012",
  "name": "Chase Creason"
}
```

The `name` field is used by the modal header to display the student's name above the summary. The summary text itself does not contain the student's name. The summary is in Markdown format and the browser converts it to HTML for display, handling headings (`##`, `###`), bold (`**text**`), italic (`*text*`), and paragraph breaks.

### Privacy

No personally identifiable information (name, login ID, or user ID) is sent to the Anthropic API. The request contains only the anonymous grade data and curriculum context. The student's name is displayed only in the browser-side modal header.

---

## 5. Token Estimates

| Component | Estimated Tokens |
|-----------|-----------------|
| System prompt instructions | ~500 |
| Curriculum context | ~4,500 |
| Student grade data | ~500–2,000 (varies by progress) |
| **Total input** | **~5,500–7,000** |
| Output (max_tokens=1024) | ~400–800 typical |

See `docs/costs/claude-api-cost-estimate.md` for detailed cost analysis by model and usage scenario.

---

## 6. Source Code

- **API call and prompt construction**: `internal/app/features/mhsdashboard/summary.go`
- **Curriculum context**: `internal/app/features/mhsdashboard/curriculum_context.md`
- **Progress point config**: `internal/app/resources/mhs_progress_points.json`
- **Dashboard UI (modal + JS)**: `internal/app/features/mhsdashboard/templates/mhsdashboard_view.gohtml`
