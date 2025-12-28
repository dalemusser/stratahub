# AI-Assisted Development Guide

A practical guide for developers on how to effectively use Claude Code for application development, based on real-world collaborative development patterns.

## Using Claude Code in the Terminal

This guide is based on using **Claude Code**, the CLI (command-line interface) version of Claude that runs directly in your terminal. This is different from using Claude through a web browser or desktop application.

### How to Use Claude Code

Claude Code runs in your terminal alongside your development tools:

```bash
# Start Claude Code in your project directory
claude

# Or start with a specific task
claude "Read the README and explain this project"
```

Once running, you have a conversational interface in your terminal where you type messages and Claude responds, with full access to your project files and development tools.

### What Claude Code Can Do

Because Claude Code runs in your terminal with access to your filesystem and shell:

**File Operations:**
- Read any file in your project
- Edit files directly (with your approval)
- Create new files when needed
- Search across your codebase with glob patterns and grep

**Command Execution:**
- Run build commands (`go build`, `npm run build`, etc.)
- Execute tests (`go test`, `pytest`, etc.)
- Run any shell command
- Start and monitor processes

**Development Workflow:**
- Make git commits with proper messages
- Create pull requests via GitHub CLI
- Run linters and formatters
- Execute database migrations

**Parallel Processing:**
- Spawn multiple agents to work on different parts of the codebase simultaneously
- Bulk changes across many files at once
- Explore different areas of the codebase in parallel

**Visual Understanding:**
- Read and analyze screenshots you save to your filesystem
- Understand images, diagrams, and UI mockups
- Compare visual output to expected designs

### Advantages Over Web/Desktop Interfaces

**Direct File Access:**
Instead of copying and pasting code snippets, Claude Code reads and writes files directly. When you say "read the handler.go file," it reads the actual file. When it makes changes, it edits the actual file.

**Integrated Command Execution:**
Claude Code can build your project, run tests, and verify changes work—all within the same conversation. No switching windows to run commands and then describing the output.

**No Copy/Paste Workflow:**
With web or desktop Claude, you copy code into the chat, get suggestions, and copy them back. With Claude Code, the entire codebase is accessible. Changes happen in place.

**Your Development Environment:**
Claude Code runs where you work—in your terminal, in your project directory, with your tools. It's part of your development flow, not a separate application.

**Handles Large Codebases:**
Claude Code can explore, search, and understand projects with hundreds of files. It reads what it needs, when it needs it, rather than requiring you to paste relevant code.

**Parallel Agents:**
For bulk changes (like adding dark mode to 20 templates), Claude Code spawns parallel agents that work simultaneously, completing in minutes what would take hours sequentially.

**Persistent Project Context:**
While Claude still starts fresh each session, Claude Code can read your context documents at the start of each session, quickly rebuilding full project understanding.

**Build-Test-Fix Loop:**
The natural development loop—make a change, build, test, fix issues—happens in one continuous conversation. Claude Code runs the build, sees the errors, and fixes them.

### Terminal Workflow Example

```
You: Read the settings handler and add a field for site description

Claude: [Reads internal/app/features/settings/handler.go]
        [Reads the model and template]
        [Makes edits to add the field]

        I've added the site_description field. Let me build to check for errors.

        [Runs: go build ./...]

        Build successful. Would you like me to run the tests?

You: Yes

Claude: [Runs: go test ./internal/app/features/settings/...]

        All tests pass. The field is now available in the settings form.

You: I'm looking at the page. The field needs helper text explaining what it's for.

Claude: [Edits the template to add helper text]

        Done. Refresh the page to see the helper text.
```

This tight loop—reading, editing, building, testing, refining—happens in one continuous terminal session.

---

## Overview

This guide captures the workflow, communication patterns, and best practices that make AI-assisted development effective. It's designed to help developers get the most out of working with Claude Code on complex projects.

## The Nature of AI-Assisted Development

### You Are Still the Developer

Working with Claude doesn't mean handing over requirements and receiving a finished application. You still develop the application piece by piece, exactly as you would when working alone. The difference is that Claude can execute those pieces more quickly while you focus on architecture, design decisions, and quality.

**You provide:**
- Technical vision and direction
- Recognition of when something isn't right
- Domain knowledge and requirements
- Quality judgment and refinement guidance

**Claude provides:**
- Rapid implementation of well-defined pieces
- Pattern consistency across the codebase
- Exploration and research across many files quickly
- Execution of repetitive changes at scale

### Iterative Refinement

Development is rarely one-shot. You describe what you want, Claude implements it, and then you refine through conversation:

```
Developer: Add a logo upload field to the settings page.

Claude: [Implements logo upload]

Developer: When I click View on the logo I get a 404.

Claude: [Investigates, finds missing route, fixes it]

Developer: Now the download works but I get a perpetual spinner.

Claude: [Investigates, finds download links trigger loader, fixes it]

Developer: The logo and title aren't aligned. See menubar.png.

Claude: [Adjusts layout]

Developer: Use flexbox instead, that's not centering properly.

Claude: [Switches to flexbox approach]

Developer: Better. Now reduce the whitespace between title and the line below.

Claude: [Adjusts spacing]
```

This back-and-forth is normal and expected. Each message refines the result until it matches your vision.

### Your Technical Expertise Drives the Conversation

You bring the expertise to recognize when something is wrong, even if you're not writing every line of code:

- **Spotting incorrect approaches:** "That won't work with DocumentDB, we need to support both MongoDB and DocumentDB."
- **Recognizing missing pieces:** "There's no way to replace the logo without first removing it."
- **Identifying visual issues:** "The logo and title aren't lining up."
- **Catching unnecessary additions:** "We are not doing rate limiting on login."
- **Directing architectural decisions:** "Use the pattern from the materials feature, not resources."

You don't need to know the exact fix - you need to recognize the problem and direct attention to it.

### What Your Messages Typically Contain

**Task initiation** - Clear, scoped requests for specific pieces of work:
```
Add a footer HTML field to the settings page with a TipTap editor.
```

**Problem identification** - Describing what's wrong when you see it:
```
When I click download on the logo I get a perpetual spinner.
```

**Refinement direction** - Guiding toward the correct solution:
```
The title should be centered. And the user info block should have
a line above and below it.
```

**Acceptance or rejection** - Confirming when something is right or wrong:
```
That looks good.
```
```
No, I want it above the Dark Mode section, not below it.
```

**Context and constraints** - Providing information Claude needs:
```
All code has to support both DocumentDB and MongoDB.
```

**Pattern references** - Pointing to examples of how things should work:
```
Make it work like the groups member assignment page.
```

### What Your Messages Focus On

Your messages focus on **what** and **why**, not necessarily **how**:

- What you want to achieve: "Add dark mode support"
- What's wrong: "The spinner never stops"
- What it should look like: "See this screenshot"
- What constraints exist: "It needs to work on DocumentDB"
- What patterns to follow: "Like the materials feature"

Claude figures out the how - the specific code changes, file modifications, and implementation details.

### What Your Messages Achieve

Each message moves the work forward:

1. **Initiating work** - Starting a new piece of functionality
2. **Unblocking progress** - Providing information Claude needs
3. **Correcting course** - Redirecting when implementation diverges from intent
4. **Refining results** - Polishing until the result matches your vision
5. **Confirming completion** - Signaling when something is done correctly

### A Realistic Conversation Flow

Here's what a real development conversation looks like:

```
Developer: I want to add site settings where admins can change
the site name. Look at how other features are structured first.

Claude: [Explores codebase, proposes approach with model, store, handler]

Developer: That looks right. Implement the model and store first.

Claude: [Implements model and store]

Developer: Run go build.

Claude: [Builds, finds error, fixes it, builds clean]

Developer: Now add the handler and template.

Claude: [Implements handler and template]

Developer: I'm looking at the page. The site name field needs
a helper text explaining where it appears.

Claude: [Adds helper text]

Developer: Now I want to add a logo upload. The logo should
appear in the menu header.

Claude: [Adds logo upload to settings, updates menu template]

Developer: When I view the logo I get 404.

Claude: [Investigates, adds file serving route]

Developer: The logo works but it's not centered with the title.

Claude: [Adjusts CSS]

Developer: Use flexbox. And remove the margin between them.

Claude: [Switches to flexbox, removes margin]

Developer: Good. Now add all this to the context document so
the next session knows about it.

Claude: [Updates context.md]
```

Notice the pattern:
- Short, focused messages
- Each message addresses one thing
- Problems are stated directly when noticed
- Direction is given when Claude needs guidance
- Confirmation when things are correct

This is collaborative development - you're steering, Claude is accelerating.

## Getting Started with a New Session

### 1. Establish Context First

At the start of a new session, provide context about your project. Point Claude to your context document:

```
Read /path/to/project/ai/context.md to understand the project.
```

Or describe the project briefly:

```
This is a Go web application using MongoDB, Chi router, and HTMX.
The codebase follows a feature-based structure in internal/app/features/.
```

**Why this matters:** Claude starts each session fresh. Without context, it will make assumptions that may not match your project's patterns and conventions.

### 2. Reference Existing Patterns

When implementing new features, ask Claude to look at similar existing implementations:

```
Before implementing the new "notifications" feature, look at how the
"materials" feature is structured in internal/app/features/materials/.
```

This ensures new code follows established patterns rather than introducing inconsistencies.

## Communication Patterns

### Be Direct and Specific

**Good:**
```
The logo isn't centered. It should align with the title below it.
```

**Less effective:**
```
The layout looks a bit off, can you fix it?
```

### Correct Mistakes Immediately

If Claude does something wrong or unnecessary, say so directly:

```
We are not doing rate limiting on login.
```

```
Remove the word 'entirely' from "Remove logo entirely"
```

Don't let incorrect assumptions persist - they compound.

### Use Screenshots for UI Issues

For visual problems, share a screenshot:

```
See menubar.png - the logo and title aren't lining up properly.
```

Claude can analyze images and understand visual layout issues better than text descriptions.

### Confirm Understanding

When Claude proposes something, confirm or redirect:

```
Yes, that approach works.
```

```
No, I want it to work like the groups feature, not the resources feature.
```

## Development Workflow

### 1. Read Before Writing

Always have Claude read relevant code before making changes:

```
Read the handler.go file before making changes to it.
```

Claude will refuse to edit files it hasn't read, but proactively asking ensures it understands the full context.

### 2. Incremental Changes

For complex changes, work incrementally:

1. Make one logical change
2. Build/test to verify
3. Move to next change

```
Let's start by adding the new model. Once that compiles, we'll add the store.
```

### 3. Build and Test Frequently

Ask Claude to verify changes work:

```
Run go build to check for compile errors.
```

```
Run the tests for this package.
```

```
Run the browser tests to verify the feature works.
```

### 4. Use Plan Mode for Complex Features

For substantial new features, use plan mode:

```
I want to add a notifications system. Let's plan this first.
```

Claude will:
- Explore the codebase
- Design the implementation approach
- Create a written plan
- Get your approval before implementing

### 5. Parallel Agents for Bulk Changes

When the same change needs to apply across many files, Claude can spawn parallel agents:

```
Add dark mode support to all the list templates in features/.
```

```
Add the FooterHTML field to all feature handlers.
```

This is much faster than sequential changes.

## Effective Prompting Patterns

### For New Features

```
Implement a [feature name] that:
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

## Managing Long Sessions

### Context Limits

Claude sessions have context limits. When approaching limits:

1. **Create a context document** capturing the current state
2. **Start a new session** pointing to that document
3. **Continue work** with full context restored

```
Create a context document in ai/context.md with everything needed
to continue this work in a new session.
```

### Session Handoff

When ending a session that will continue later:

```
Summarize what we've done and what remains to be done.
```

```
Document any decisions we made and why.
```

## Code Quality Practices

### Ask for Existing Pattern Review

Before implementing, have Claude check for existing utilities:

```
Before writing custom pagination, check if there's already a
paging utility in internal/app/system/.
```

### Request Consistency Checks

```
Make sure this follows the same pattern as the other handlers.
```

```
Check that I'm using the shared utilities correctly.
```

### Dark Mode Reminder

For UI work:

```
Remember to include dark mode variants for all new styles.
```

## Testing Integration

### Unit Tests

```
Run the tests for this package and fix any failures.
```

```
Add tests for the new functionality.
```

### Browser Tests

```
Run the browser tests to verify the feature works end-to-end.
```

```
Add a new test case to test_admin_journey.py for this feature.
```

### Build Verification

```
Build the project and fix any compile errors.
```

## Documentation

### Capture Lessons Learned

When you discover something important:

```
Add this to the lessons learned section of the context document.
```

### Update Context for Future Sessions

```
Update the context document with information about [new feature/pattern].
```

### Record Decisions

```
Document why we chose [approach] over [alternative].
```

## Common Anti-Patterns to Avoid

### Don't Skip Context

**Problem:** Starting work without context leads to inconsistent code.

**Solution:** Always point Claude to context documents or describe key patterns first.

### Don't Batch Too Many Changes

**Problem:** Large batches of changes are hard to verify and debug.

**Solution:** Make incremental changes with verification steps between.

### Don't Assume Claude Remembers

**Problem:** Claude doesn't retain information between sessions.

**Solution:** Document everything important in context files.

### Don't Accept Without Verification

**Problem:** Changes might not compile or might break tests.

**Solution:** Always build and test after significant changes.

### Don't Let Mistakes Compound

**Problem:** Incorrect assumptions early lead to more wrong code later.

**Solution:** Correct mistakes immediately when you notice them.

## Example Development Session

Here's how a typical development session might flow:

```
Developer: Read ai/context.md to understand the project.

Claude: [Reads and confirms understanding]

Developer: I want to add a "notifications" feature for alerting
users about new resources. Look at how materials works first.

Claude: [Reads materials feature, proposes approach]

Developer: That looks good, but notifications should be simpler -
no file uploads, just title and message.

Claude: [Revises approach]

Developer: Go ahead and implement the model and store first.

Claude: [Implements model and store]

Developer: Run go build to check for errors.

Claude: [Runs build, fixes any issues]

Developer: Now add the handler following the materials pattern.

Claude: [Implements handler]

Developer: Run the tests.

Claude: [Runs tests, all pass]

Developer: See notification_ui.png - the layout needs to match
the resources list page.

Claude: [Adjusts UI based on screenshot]

Developer: Add this feature to the context document.

Claude: [Updates context.md]
```

## Quick Reference

| Task | Approach |
|------|----------|
| New session | Point to context document first |
| New feature | Look at similar feature, plan, implement incrementally |
| Bug fix | Describe expected vs actual, point to relevant code |
| UI change | Share screenshot, describe specific changes |
| Bulk changes | Use parallel agents |
| Complex feature | Use plan mode |
| Session ending | Update context document |
| Verify changes | Build and test frequently |

## Building Your Context Document

Your context document should include:

1. **Project overview** - What the project does, tech stack
2. **Project structure** - Key directories and their purposes
3. **Patterns and conventions** - How things are done in this codebase
4. **Shared utilities** - Reusable code that should be used
5. **Configuration** - Environment variables, settings
6. **Lessons learned** - Hard-won insights from development
7. **Current state** - What's been done, what's pending

See `ai/context.md` in this project for a comprehensive example.

## Final Notes

The key to effective AI-assisted development is treating Claude as a capable collaborator who needs context and guidance. Provide clear direction, verify results, and maintain documentation for continuity across sessions.

The more context you provide upfront, and the more consistently you follow established patterns, the more effective the collaboration becomes.
