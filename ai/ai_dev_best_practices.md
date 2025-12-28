# Best Practices for AI-Assisted Software Development

A guide to effective collaboration patterns between developers and AI coding assistants.

---

## Core Principles

### 1. You Are Still the Developer

AI accelerates execution but doesn't replace expertise. You provide:
- Technical vision and architectural decisions
- Quality judgment and pattern recognition
- Domain knowledge and requirements understanding
- Recognition of when something is wrong

AI provides:
- Rapid implementation of well-defined pieces
- Pattern consistency across the codebase
- Tireless exploration and search
- Parallel execution at scale

The developer who understands their system deeply and directs AI effectively outperforms both the developer working alone and the one who delegates without understanding.

### 2. Context Is Everything

AI starts fresh every session. Without context, it makes assumptions that may not match your project's patterns, conventions, or constraints.

**Best practice:** Establish context before requesting work.

### 3. Iteration Is Normal

First attempts are rarely final. Expect the cycle: implement → observe → refine → confirm. This isn't failure—it's the normal development process, just faster.

### 4. Verify Everything

AI can be confidently wrong. Build frequently, test often, and manually verify results. Never assume correctness.

---

## Context Management

### Maintain a Context Document

Create a living document that captures:
- Project overview and tech stack
- Directory structure and organization
- Patterns and conventions used
- Shared utilities and when to use them
- Configuration and environment details
- Lessons learned during development
- Current state and pending work

**Example structure:**
```
ai/context.md
├── Project Overview
├── Tech Stack
├── Project Structure
├── Patterns & Conventions
│   ├── Code Organization
│   ├── Naming Conventions
│   ├── Error Handling
│   └── Testing Patterns
├── Shared Utilities
├── Configuration
├── Lessons Learned
└── Current State
```

### Start Every Session Right

Begin with context, not requests:
```
Read ai/context.md to understand the project.
```

Then proceed with work:
```
Now add a notifications feature following the materials pattern.
```

### Update Context Continuously

When you learn something important, capture it:
```
Add to the lessons learned: DocumentDB doesn't support $lookup with pipeline.
```

When completing significant work:
```
Update the context document with the new notifications feature.
```

### Session Handoff

When ending a session that will continue later:
```
Summarize what we've done, what remains, and any decisions made.
Update the context document for the next session.
```

---

## Communication Patterns

### Keep Messages Short and Focused

**Good:**
```
Add a logo upload field to the settings page.
```

**Less effective:**
```
I want to add a logo to the site. The logo should appear in the header
and users should be able to upload it from the settings page. Make sure
it works with dark mode and handles errors gracefully. Also consider
mobile responsiveness and...
```

One thing at a time. Build incrementally.

### Focus on What, Not How

Describe the outcome, not the implementation:
```
When I click download, the file should save with its original filename.
```

Not:
```
Add a Content-Disposition header with attachment and filename parameters
to the response in the download handler function.
```

Let AI figure out the how. Step in with specifics only when needed.

### Be Direct About Problems

When something is wrong, say so clearly:
```
The spinner never stops after upload completes.
```

```
The logo and title aren't aligned. See screenshot.png.
```

Don't soften or hedge—direct communication is efficient.

### Correct Immediately

When AI does something wrong or unnecessary:
```
We're not doing rate limiting on login.
```

```
Use the pattern from materials, not resources.
```

Don't let incorrect assumptions persist—they compound.

### Confirm When Satisfied

Signal completion clearly:
```
That looks good.
```

```
The layout is correct now. Move on to the next piece.
```

This helps AI understand what "done" looks like.

### Use Pattern References

Point to existing code as the specification:
```
Make it work like the groups member assignment page.
```

```
Follow the handler pattern in internal/app/features/materials/.
```

Existing code is the most precise specification you have.

### Use Visual Feedback

For UI work, screenshots are more precise than descriptions:
```
See sidebar.png - the menu items need more spacing.
```

AI can analyze images and understand visual issues directly.

---

## Development Workflow

### Read Before Writing

Always have AI read relevant code before making changes:
```
Read the handler before modifying it.
```

Understanding existing code prevents assumptions that break things.

### Work Incrementally

For complex changes:
1. Make one logical change
2. Build/test to verify
3. Move to the next change

```
Start with the model. Once that compiles, add the store.
```

Don't batch too many unverified changes—debugging becomes harder.

### Build and Test Frequently

After each significant change:
```
Run go build to check for errors.
```

```
Run the tests for this package.
```

```
Run the browser tests to verify end-to-end.
```

Catch problems close to their source.

### Use Plan Mode for Complex Features

For substantial work with architectural decisions:
```
I want to add a notifications system. Let's plan this first.
```

AI will:
- Explore the codebase
- Propose an approach
- Create a written plan
- Wait for your approval

Planning prevents wasted implementation effort.

### Use Parallel Agents for Bulk Changes

When the same pattern applies across many files:
```
Add dark mode support to all the templates in features/.
```

```
Add the FooterHTML field to all feature handlers.
```

This completes in minutes what would take hours sequentially.

---

## Quality Practices

### Check for Existing Solutions

Before implementing something new:
```
Check if there's already a pagination utility in system/.
```

```
See if this pattern exists elsewhere in the codebase.
```

Don't reinvent wheels.

### Maintain Consistency

Reference existing patterns:
```
Make sure this follows the same pattern as the other handlers.
```

```
Use the shared utilities correctly.
```

Consistency compounds—inconsistency compounds too.

### Review Generated Code

AI can introduce:
- Security vulnerabilities (injection, XSS, etc.)
- Performance issues (N+1 queries, missing indexes)
- Subtle logic errors
- Pattern violations

Your expertise catches what AI misses.

### Don't Over-Engineer

AI tends toward comprehensive solutions. Push back:
```
Keep it simple. No need for the extra abstraction.
```

```
We don't need error handling for cases that can't happen.
```

The right amount of complexity is the minimum needed.

---

## Anti-Patterns to Avoid

### Skipping Context

**Problem:** AI makes assumptions that don't match your project.
**Result:** Inconsistent code, time lost to misunderstanding.
**Solution:** Always establish context first.

### Mega-Prompts

**Problem:** Trying to specify an entire feature in one message.
**Result:** Misinterpretation, wasted effort, frustration.
**Solution:** Work incrementally, one piece at a time.

### Fire and Forget

**Problem:** Accepting output without verification.
**Result:** Bugs shipped, technical debt accumulated.
**Solution:** Build, test, and verify every change.

### Batching Too Much

**Problem:** Multiple changes without verification between them.
**Result:** Harder debugging, lost track of what changed.
**Solution:** Verify each logical change before the next.

### Letting Mistakes Compound

**Problem:** Not correcting errors immediately.
**Result:** Wrong assumptions building on wrong assumptions.
**Solution:** Correct as soon as you notice something wrong.

### Over-Delegating

**Problem:** Losing touch with your codebase.
**Result:** Can't catch AI errors, can't make good decisions.
**Solution:** Stay engaged, understand what's being built.

### Assuming AI Remembers

**Problem:** Expecting context to persist between sessions.
**Result:** Repeated explanations, inconsistent work.
**Solution:** Document everything in context files.

---

## Effective Prompting Templates

### For New Features

```
Implement [feature] that:
- Does X
- Follows the pattern in [existing similar feature]
- Includes [specific requirements]

Look at [reference files] before implementing.
```

### For Bug Fixes

```
When I [action], [unexpected behavior] happens.
Expected: [what should happen]

The relevant code is in [file/area].
```

### For Refactoring

```
Refactor [component] to:
- [specific improvement]
- Keep the same external behavior
- Follow the pattern used in [reference]
```

### For UI Changes

```
See [screenshot]. Change [element] to:
- [specific visual change]
- Match the style of [reference element]
```

---

## Session Management

### Recognize Context Limits

Long sessions accumulate context. When you notice:
- Slower responses
- Repeated explanations needed
- Context window warnings

It's time to start fresh.

### End Sessions Cleanly

```
Summarize what we've done and what remains.
Update the context document.
```

### Start Sessions Efficiently

```
Read ai/context.md.
Continue with [pending work from last session].
```

### Use Compaction

When context grows but you want to continue:
```
/compact
```

This summarizes the conversation and frees context space.

---

## Advanced Techniques

### Slash Commands for Repetitive Workflows

Create custom commands for common operations:
```
.claude/commands/review.md → /review
.claude/commands/test.md → /test
```

### MCP Servers for External Tools

Connect AI to external systems:
- Database queries
- API calls
- Custom tooling

### Hooks for Automation

Automate checks on AI actions:
- Pre-commit validation
- Code style enforcement
- Security scanning

---

## Measuring Effectiveness

### What Should Speed Up

- Boilerplate and repetitive code
- Pattern application across files
- Exploration and search
- Initial implementations

### What Stays About the Same

- Architectural decisions
- Design thinking
- Quality judgment
- Complex debugging

### Where You Add Unique Value

- Recognizing when something is wrong
- Knowing the domain
- Making trade-off decisions
- Understanding user needs

---

## Summary

Effective AI-assisted development is:

1. **Context-first** - Establish understanding before requesting work
2. **Incremental** - Small pieces, verified frequently
3. **Directive** - Clear, focused communication
4. **Verified** - Build and test after changes
5. **Documented** - Capture lessons and state for continuity
6. **Collaborative** - You steer, AI accelerates

The goal isn't to write less code—it's to build more software, faster, while maintaining the quality and coherence that comes from human judgment and expertise.
