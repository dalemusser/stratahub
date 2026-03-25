# Claude API Cost Estimate: Student Summary Feature

## How the feature works

Each time a leader or admin clicks the summary icon for a student, one API call is made to Claude. There is no caching — each click generates a fresh summary.

The model can be selected per workspace in Settings. The default is Sonnet 4.

## Token breakdown per request

| Component | Description | Estimated Tokens |
|-----------|-------------|-----------------|
| System prompt instructions | ~350 words of instructions | ~500 |
| Curriculum context (embedded) | ~7,500 words (294 lines) | ~9,500 |
| User prompt + student data | Student name + grade summary (varies by progress) | ~500–2,000 |
| **Total input** | | **~10,500–12,000** |
| Output (max_tokens=1024) | 2–4 paragraph summary, typically 400–800 tokens | **~400–800** |

## Model comparison

Admins can choose which Claude model to use in Workspace Settings. Each model offers a different trade-off between quality and cost.

### Haiku 4.5 — Fast, basic analysis (~$0.014/summary)

- **Speed**: Fastest response time (~2–4 seconds)
- **Model ID**: `claude-haiku-4-5-20251001`
- **Strengths**: Correctly identifies what's passed vs flagged, lists progress status accurately
- **Weaknesses**: Summaries tend to be more surface-level and formulaic. May list observations without synthesizing them into actionable patterns. Less likely to connect struggles across units (e.g., linking topo map difficulty in Unit 2 with flow direction issues in Unit 3). Instructional suggestions can feel generic ("provide additional support")
- **Best for**: Quick status checks, high-volume usage where cost matters more than depth, or deployments where teachers primarily want a snapshot rather than detailed analysis

### Sonnet 4 — Recommended, strong analysis (~$0.04/summary)

- **Speed**: Moderate response time (~5–10 seconds)
- **Model ID**: `claude-sonnet-4-20250514`
- **Strengths**: Produces well-written, professional summaries. Connects patterns across progress points — for example, noticing that a student who struggled with topographic maps (U2P1–U2P3) also had difficulty with flow direction (U3P1), suggesting a spatial reasoning gap. References specific progress points by descriptive name. Instructional suggestions are specific and actionable
- **Weaknesses**: Slightly slower and ~3x more expensive than Haiku
- **Best for**: Most deployments. Good balance of insight, quality, and cost. Produces summaries that teachers can act on directly

### Opus 4 — Highest quality, highest cost (~$0.40/summary)

- **Speed**: Slowest response time (~15–30 seconds)
- **Model ID**: `claude-opus-4-20250514`
- **Strengths**: Most nuanced analysis. Better at identifying subtle patterns, distinguishing between a student who struggled and self-corrected vs one who needed extensive help. May provide more sophisticated instructional recommendations that consider the student's learning trajectory. Writing quality is the most natural and professional
- **Weaknesses**: ~10x more expensive than Sonnet and noticeably slower. For most student data, the quality improvement over Sonnet is incremental rather than dramatic
- **Best for**: Small deployments, detailed individual student reviews, or cases where summaries will be shared with parents or included in formal reports

### Recommendation

**Sonnet 4 is the best default for most deployments.** It provides meaningfully better analysis than Haiku at a modest cost. Opus is worth considering only for small-scale use or high-stakes contexts where summary quality is critical.

## Pricing by model

| | Haiku 4.5 | Sonnet 4 (default) | Opus 4 |
|--|-----------|-------------------|--------|
| Input (per M tokens) | $1.00 | $3.00 | $15.00 |
| Output (per M tokens) | $5.00 | $15.00 | $75.00 |
| **Est. per summary** | **~$0.014** | **~$0.04** | **~$0.40** |

## Cost formula

```
Monthly cost = (summaries per month) x (cost per summary for selected model)
```

## Example scenarios

| Scenario | Summaries/month | Haiku 4.5 | Sonnet 4 | Opus 4 |
|----------|----------------|-----------|----------|--------|
| 1 teacher, 30 students, once/week | 120 | $1.68 | $4.80 | $48 |
| 5 teachers, 25 students each, once/week | 500 | $7 | $20 | $200 |
| 10 teachers, 30 students each, twice/month | 600 | $8.40 | $24 | $240 |
| 20 teachers, 30 students each, once/week | 2,400 | $33.60 | $96 | $960 |
| 50 teachers, 30 students each, once/week | 6,000 | $84 | $240 | $2,400 |

## What drives cost up

- **Repeated clicks**: No caching, so clicking the same student twice = two API calls
- **Students deep into the game**: More grade data = slightly more input tokens (but curriculum context dominates, so the difference is small — maybe $0.04 vs $0.05)
- **Model selection**: Opus is ~10x more expensive than Sonnet, and ~29x more than Haiku

## What doesn't significantly affect cost

- **Students with little progress**: The curriculum context (~9,500 tokens) is always sent in full regardless of student progress. A student with 2 data points costs nearly the same as one with 26.

## Cost control options

If costs need to be managed:

1. **Settings toggle**: Admins can disable the feature entirely per workspace
2. **Model selection**: Switch to Haiku for ~3x cost reduction with acceptable quality for basic summaries
3. **Response caching**: Cache summaries for a period (e.g., 1 hour) to avoid duplicate API calls — not yet implemented
4. **Reduce curriculum context**: Summarize or trim the 294-line context document — diminishing returns since it improves summary quality significantly

Check [anthropic.com/pricing](https://www.anthropic.com/pricing) for current rates.

## Context size and cost scaling

The current curriculum context is ~3,400 words (~4,500 tokens). All three models support up to 200K input tokens, leaving significant room to expand the context with additional rubrics, pedagogical notes, example analyses, or more detailed curriculum descriptions.

The table below shows how per-summary cost scales as the curriculum context grows. Student data and system prompt add ~1,000–2,500 tokens on top of the context. Output cost (~600 tokens) stays roughly constant regardless of input size.

### Per-summary cost by context size

| Context Size | ~Words | ~Input Tokens (total) | Haiku 4.5 | Sonnet 4 | Opus 4 |
|---|---|---|---|---|---|
| Current | 3,400 | ~6,500 | $0.010 | $0.029 | $0.14 |
| 2x | 7,000 | ~11,000 | $0.014 | $0.042 | $0.21 |
| 5x | 17,000 | ~24,000 | $0.027 | $0.081 | $0.41 |
| ── | ── | ── 32K context ── | ── | ── | ── |
| 10x | 34,000 | ~46,000 | $0.049 | $0.15 | $0.74 |
| ── | ── | ── 64K context ── | ── | ── | ── |
| 25x | 85,000 | ~112,000 | $0.12 | $0.35 | $1.73 |
| 50x | 170,000 | ~222,000 | $0.23 | $0.68 | $3.42 |
| Max (~150x) | 500,000 | ~200,000 input limit | $0.20 | $0.62 | $3.05 |

*Total input tokens = context tokens + system prompt (~500) + student data (~500–2,000). Output assumes ~600 tokens at output pricing.*

### Monthly cost at 500 summaries/month by context size

| Context Size | Haiku 4.5 | Sonnet 4 | Opus 4 |
|---|---|---|---|
| Current (3,400 words) | $5 | $15 | $70 |
| 5x (17,000 words) | $14 | $41 | $205 |
| 10x (34,000 words) | $25 | $75 | $370 |
| 25x (85,000 words) | $60 | $175 | $865 |

### Practical considerations

- **Cost scales linearly** with input size. Doubling the context roughly doubles the input cost.
- **Quality does not scale linearly**. Beyond a certain point, adding more context yields diminishing returns and can introduce noise that reduces summary quality.
- **Latency increases** with context size. Larger contexts take longer to process, especially on Opus.
- **The current context size is efficient**. At ~4,500 tokens it represents a small fraction of the 200K limit and keeps costs low while providing enough detail for high-quality summaries.
