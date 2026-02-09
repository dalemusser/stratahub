# Incorporating the Claude API into StrataHub

This document outlines practical, safe, and high-leverage ways the Claude API could be incorporated into StrataHub. The focus is on augmenting understanding and intent, not automating authority or decision-making.

- StrataHub remains the source of truth.
- Claude acts as an interpretive and explanatory layer, always constrained and advisory.

---

## Why Claude?

Claude offers several capabilities that align well with StrataHub's needs:

- **Extended context windows** — Claude can process large amounts of data in a single request (up to 200K tokens), enabling analysis of extensive logs, configurations, or historical data without chunking.
- **Structured outputs** — Native support for JSON mode and tool use ensures responses conform to expected schemas.
- **Tool use** — Claude can be given tools that map directly to StrataHub's API, enabling validated intent extraction.
- **Vision capabilities** — Can analyze screenshots, charts, and visual data for dashboard interpretation or UI assistance.
- **Safety-focused design** — Constitutional AI principles align with StrataHub's need for advisory-only, non-authoritative AI assistance.

---

## Guiding Principles

Any use of Claude within StrataHub should adhere to the following principles:

- **Advisory, not authoritative** — Claude may suggest, explain, summarize, or interpret — never execute irreversible actions.
- **Tool-constrained outputs** — Use Claude's tool use feature to ensure responses map to valid StrataHub actions.
- **Human confirmation required** — All state-changing actions remain explicit, reviewable, and user-initiated.
- **Contextual and role-aware** — System prompts are grounded in StrataHub's data model and the user's role (leader, admin, developer).
- **No student-facing open chat** — AI features are scoped to administrative, instructional, or diagnostic use cases.

---

## Core Integration Ideas

### Natural Language → Structured Administrative Intent via Tool Use

Claude's tool use feature enables precise mapping of natural language to StrataHub actions.

**Define tools that mirror StrataHub's capabilities:**

```json
{
  "name": "assign_resource",
  "description": "Assign a game resource to a group with optional constraints",
  "input_schema": {
    "type": "object",
    "properties": {
      "resource_id": { "type": "string" },
      "group_id": { "type": "string" },
      "start_date": { "type": "string", "format": "date" },
      "end_date": { "type": "string", "format": "date" },
      "unit_filter": { "type": "array", "items": { "type": "integer" } }
    },
    "required": ["resource_id", "group_id"]
  }
}
```

**Example input:**

> "Give Group 3 access to Zombie Outbreak starting next Monday, Unit 2 only."

**Claude responds with a tool call:**

```json
{
  "name": "assign_resource",
  "input": {
    "resource_id": "zombie-outbreak",
    "group_id": "group-3",
    "start_date": "2026-02-02",
    "unit_filter": [2]
  }
}
```

StrataHub then:

- Resolves IDs to actual entities
- Validates the action against policies
- Previews the change to the user
- Requests confirmation
- Applies the action on approval

This ensures Claude's output is always valid and actionable.

---

### Bulk Operations via Extended Context

Claude's large context window enables operations that would be impractical with smaller models.

**Use cases:**

- Analyze all groups in an organization and suggest optimal resource assignments
- Review an entire semester's worth of access rules for conflicts
- Process hundreds of log entries to identify patterns
- Compare configurations across multiple organizations

**Example prompt:**

> "Here are all 47 groups in this organization with their current resource assignments. Identify any groups that have no resources assigned, groups with expired assignments, and groups with overlapping access to the same resource."

Claude can process this in a single request and return structured findings.

---

### Dashboard Insights with Vision

Claude's vision capabilities can analyze dashboard screenshots or rendered charts.

**Use cases:**

- Interpret complex visualizations for non-technical users
- Identify anomalies in usage graphs
- Compare multiple dashboard states
- Generate accessible descriptions of visual data

**Example:**

Upload a screenshot of the MHS dashboard and ask:

> "Summarize what this dashboard shows. Are there any concerning patterns?"

**Response:**

> "This dashboard shows save activity over the past week. There's a significant spike on Wednesday that's 3x the normal volume, followed by elevated error rates. This pattern often indicates a bulk import or a class-wide assignment deadline."

---

### Conversational Configuration Assistance

Instead of a generic chatbot, provide context-anchored assistance using Claude's system prompt.

**System prompt includes:**

- Current page/feature context
- User's role and permissions
- Available actions on this page
- Relevant documentation excerpts
- Current entity being viewed (group, game, organization)

**Example interaction:**

User is on the Group Settings page.

> User: "What happens if I change the save frequency?"

> Claude: "Changing the save frequency affects how often player progress is automatically saved for this group. The current setting is every 60 seconds. Lower values (e.g., 30 seconds) provide more granular recovery points but increase server load. Higher values reduce load but risk more lost progress if a session ends unexpectedly. This change takes effect immediately for all active sessions in this group."

---

### Policy Explanation and Reasoning

Claude can explain why certain actions are allowed or denied based on StrataHub's policy model.

**Provide Claude with:**

- The attempted action
- The user's role and permissions
- The relevant policy rules
- The current state of affected entities

**Example:**

> User: "Why can't I delete this group?"

> Claude: "This group cannot be deleted because:
> 1. It has 23 active players with save data
> 2. Your role (Leader) can only delete groups you created
> 3. This group was created by another leader (Jane Smith)
>
> **Available alternatives:**
> - Archive the group (preserves data, hides from active view)
> - Request the organization admin to transfer ownership to you
> - Contact Jane Smith to delete or transfer the group"

---

### Log Analysis and Anomaly Detection

Use Claude's extended context to analyze logs and identify issues.

**Input:**

- Sampled or filtered log entries (can include thousands of lines)
- Known good patterns
- Recent changes or deployments

**Output structured as:**

```json
{
  "anomalies": [
    {
      "type": "elevated_error_rate",
      "description": "404 errors increased 5x starting at 14:32",
      "affected_endpoints": ["/api/saves", "/api/progress"],
      "likely_cause": "Missing resource after deployment",
      "suggested_action": "Check if save-handler was deployed correctly"
    }
  ],
  "patterns": [...],
  "summary": "..."
}
```

---

### Student Progress Interpretation (Teacher-Facing)

Given scoped progression data, Claude can help teachers understand learning patterns.

**Constraints enforced via system prompt:**

- Observations only, no diagnoses or labels
- No predictions about student capability
- Age-appropriate, neutral language
- Focus on patterns, not judgments

**Example:**

> "This student completed the first three levels quickly but has attempted level 4 twelve times over two sessions. The attempts show different strategies being tried — they're actively problem-solving rather than repeating the same approach. Consider checking in to see if they'd benefit from a hint about the core mechanic introduced in level 4."

---

### Configuration Validation

Before applying changes, use Claude as a sanity-check layer.

**Input:**

- Proposed configuration change
- Current configuration state
- Known constraints and best practices

**Output:**

```json
{
  "warnings": [
    {
      "severity": "medium",
      "message": "Save frequency of 5 seconds is unusually aggressive",
      "recommendation": "Consider 30-60 seconds unless real-time sync is required"
    }
  ],
  "conflicts": [],
  "approved": true
}
```

Claude does not block changes — it informs the user of potential issues.

---

### Multi-Turn Troubleshooting

Claude can guide administrators through diagnostic workflows.

**Example session:**

> User: "Players in Group 5 can't save their progress"

> Claude: "Let me help diagnose this. First, is this affecting all players in Group 5, or specific ones?"

> User: "All of them"

> Claude: "That suggests a group-level issue rather than individual player problems. Can you check:
> 1. Is the group's save feature enabled? (Group Settings → Data)
> 2. Does the group have an active resource assignment for the game they're playing?
> 3. Are there any error messages in the player's console?"

The conversation continues until the issue is resolved or escalated.

---

## Explicitly Out of Scope

The following are intentionally excluded:

- Open-ended student chat
- AI-driven grading or evaluation
- Autonomous state changes without confirmation
- Unbounded free-form responses to students
- Decision-making without human approval
- Access to raw student PII in prompts

---

## Implementation Considerations

### API Integration

```go
type ClaudeService struct {
    client    *anthropic.Client
    rateLimit *rate.Limiter
}

func (s *ClaudeService) InterpretIntent(ctx context.Context, input string, tools []Tool) (*IntentResult, error) {
    // Build messages with system context
    // Call Claude API with tool definitions
    // Parse and validate tool use response
    // Return structured intent for confirmation
}
```

### Cost Management

- Cache common queries and responses
- Use Haiku for simple classification tasks
- Reserve Sonnet/Opus for complex analysis
- Implement token budgets per organization
- Rate limit by user role

### Prompt Management

- Store system prompts as versioned templates
- Include StrataHub schema definitions
- Inject runtime context (user role, current page, entity data)
- Log prompts and responses for debugging (with PII redaction)

---

## Architectural Positioning

Claude is not a chatbot bolted onto StrataHub.

**It functions as:**

- An intent interpreter (natural language → validated actions)
- A data narrator (metrics → plain-language insights)
- A policy explainer (rules → understandable reasoning)
- A configuration advisor (changes → risk assessment)
- A diagnostic assistant (problems → guided resolution)

Always downstream of StrataHub's data model.
Always constrained by tools and schemas.
Always requiring human confirmation for actions.

---

## Summary

StrataHub serves as the control plane for access, state, data, and policy.

The Claude API complements this by adding:

- Semantic understanding of user intent
- Large-context analysis of logs and configurations
- Visual interpretation of dashboards and charts
- Structured, validated output via tool use
- Conversational guidance for complex workflows

This increases clarity and usability without compromising correctness, safety, or trust.

---

## Next Steps

Potential implementation phases:

1. **Phase 1: Intent Interpreter** — Natural language → tool calls for common admin actions
2. **Phase 2: Contextual Help** — Page-aware assistance for leaders and admins
3. **Phase 3: Dashboard Insights** — Narrative summaries and anomaly detection
4. **Phase 4: Diagnostic Assistant** — Multi-turn troubleshooting for support cases
5. **Phase 5: Configuration Advisor** — Pre-apply validation and suggestions
