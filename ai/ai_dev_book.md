# AI-Assisted Software Development: A Practical Guide

## Book Outline

Based on real-world collaborative development experience between a human developer and AI.

---

## Part I: Foundations

### Chapter 1: Using Claude Code in the Terminal

**1.1 What is Claude Code**
- The command-line interface for AI-assisted development
- Running Claude in your terminal, in your project directory
- Direct access to your filesystem and development tools
- A fundamentally different experience from web or desktop chat

**1.2 Getting Started**
- Installation and setup
- Starting Claude Code in your project
- The conversational interface in your terminal
- Basic commands and interactions

**1.3 What Claude Code Can Do**
- Reading and editing files directly
- Executing shell commands (build, test, run)
- Searching across large codebases
- Git operations and pull requests
- Spawning parallel agents for bulk work
- Analyzing screenshots and images

**1.4 Advantages Over Web and Desktop Interfaces**
- Direct file access: no copy/paste workflow
- Integrated command execution: build and test in the conversation
- Your development environment: terminal, project directory, your tools
- Large codebase handling: reads what it needs, when it needs it
- Parallel agents: bulk changes across many files simultaneously
- The build-test-fix loop in one continuous session

**1.5 The Terminal Workflow**
- The tight loop: read, edit, build, test, refine
- Staying in your development flow
- Context documents for session continuity
- Example: a complete development interaction

---

### Chapter 2: A New Way of Building Software

**2.1 What AI-Assisted Development Is**
- The developer remains the developer
- AI as an accelerator, not a replacement
- Piece-by-piece development at higher speed
- Maintaining creative and technical control

**2.2 What AI-Assisted Development Is Not**
- Not "describe an app, receive an app"
- Not abandoning your expertise
- Not blind trust in generated code
- Not a shortcut around understanding your system

**2.3 The Collaborative Model**
- Two complementary intelligences
- Human: vision, judgment, domain knowledge, quality assessment
- AI: rapid execution, pattern consistency, tireless exploration
- The steering and accelerating metaphor

**2.4 Who This Book Is For**
- Experienced developers looking to multiply their output
- Technical leads managing complex codebases
- Anyone who writes code and wants to write more, faster

---

### Chapter 3: The Developer's Role Reimagined

**3.1 From Writer to Director**
- Shifting from typing code to directing its creation
- The importance of knowing what you want
- Recognizing problems without necessarily knowing solutions

**3.2 Technical Expertise Still Matters**
- Why deep knowledge makes AI more useful, not less
- Spotting when AI goes wrong
- Providing the constraints AI can't know
- Architectural decisions remain yours

**3.3 Quality Judgment**
- You are the quality gate
- Knowing when something doesn't look right
- Knowing when something doesn't feel right
- Trusting your instincts, verifying with evidence

**3.4 The Feedback Loop**
- Observe, direct, refine, confirm
- Short cycles, continuous adjustment
- The conversation that builds software

---

## Part II: Establishing Context

### Chapter 4: The Context Problem

**4.1 AI Starts Fresh Every Time**
- No memory between sessions
- The cost of rebuilding understanding
- Why context documentation matters

**4.2 What Context Includes**
- Project structure and architecture
- Patterns and conventions
- Technology choices and constraints
- Current state and history
- Lessons learned

**4.3 The Context Document**
- Creating a living reference
- Structure for quick comprehension
- Updating as the project evolves
- Example: A complete context document

**4.4 Starting a Session Right**
- The first message matters
- Pointing to context before asking for work
- Establishing shared understanding

---

### Chapter 5: Patterns and Conventions

**5.1 Why Consistency Matters**
- AI follows patterns it sees
- Inconsistent input yields inconsistent output
- The compounding cost of divergent code

**5.2 Documenting Your Patterns**
- Code organization patterns
- Naming conventions
- Error handling approaches
- Testing patterns

**5.3 Teaching by Example**
- "Look at how X feature works"
- Reference implementations as templates
- The power of "do it like this"

**5.4 Shared Utilities and When to Use Them**
- Avoiding reinvented wheels
- Pointing AI to existing solutions
- Building a utility library over time

---

## Part III: The Development Conversation

### Chapter 6: How to Communicate

**6.1 The Anatomy of Effective Messages**
- Short, focused, single-purpose
- What you want, not how to do it
- Constraints and context when relevant
- Examples and references when helpful

**6.2 Task Initiation**
- Scoping work appropriately
- "Add X" vs "Build the entire Y system"
- Breaking large work into pieces
- Setting up for incremental progress

**6.3 Problem Identification**
- Describing what's wrong clearly
- Expected vs actual behavior
- Visual problems: using screenshots
- Pointing to relevant code

**6.4 Refinement and Direction**
- Guiding toward correct solutions
- "Use flexbox instead"
- "Make it work like the other feature"
- Incremental adjustment

**6.5 Confirmation and Rejection**
- Signaling when things are right
- Signaling when things are wrong
- The importance of immediate feedback

---

### Chapter 7: The Iterative Refinement Loop

**7.1 First Attempts Are Rarely Final**
- Expecting iteration, not perfection
- The normal flow: implement, observe, refine
- Patience with the process

**7.2 Observing Results**
- Actually running the code
- Actually looking at the UI
- Actually testing the behavior
- Not assuming correctness

**7.3 Identifying Gaps**
- What's missing?
- What's wrong?
- What doesn't match your vision?
- What doesn't follow the pattern?

**7.4 Directing Refinement**
- Clear, specific feedback
- One issue at a time
- Building toward completion

**7.5 Knowing When You're Done**
- Matching your mental model
- Passing your quality bar
- Ready for the next piece

---

### Chapter 8: Verification Practices

**8.1 Build Early, Build Often**
- Catching errors quickly
- Not accumulating broken code
- The compile check as sanity check

**8.2 Running Tests**
- Unit tests after changes
- Integration tests for features
- Browser tests for UI work
- Making tests part of the flow

**8.3 Manual Verification**
- Looking at what was built
- Clicking through the UI
- Trying the edge cases
- Trusting but verifying

**8.4 The Verification Rhythm**
- Small change, verify
- Not batching too much unverified work
- Catching problems close to their source

---

## Part IV: Managing Complexity

### Chapter 9: Planning Large Features

**9.1 When to Plan First**
- Features that span multiple components
- Architectural decisions to make
- Uncertainty about approach

**9.2 The Planning Conversation**
- Exploring the codebase together
- Discussing approaches
- Making decisions explicit
- Creating a written plan

**9.3 Executing the Plan**
- One piece at a time
- Verifying each piece
- Adjusting as you learn
- Knowing when to re-plan

**9.4 Documenting Decisions**
- Why you chose this approach
- What alternatives you rejected
- Future considerations

---

### Chapter 10: Parallel Execution

**10.1 When Work Can Be Parallelized**
- Independent changes across files
- Bulk updates following a pattern
- Exploration of multiple areas

**10.2 Spawning Parallel Work**
- Describing the pattern once
- Letting AI apply it everywhere
- Reviewing consolidated results

**10.3 Coordinating Results**
- Merging parallel changes
- Handling conflicts
- Verifying consistency

**10.4 Real Example: Adding Dark Mode**
- The challenge: many templates
- The approach: parallel agents
- The result: consistent updates

---

### Chapter 11: Session Management

**11.1 Context Limits**
- Sessions don't last forever
- Recognizing when limits approach
- Planning for continuity

**11.2 Session Handoff**
- Capturing current state
- Documenting what's done and pending
- Updating context documents
- Setting up the next session

**11.3 Resuming Work**
- Starting fresh with full context
- Rebuilding shared understanding quickly
- Continuing without losing momentum

**11.4 Long-Running Projects**
- Context documents as project memory
- Evolving documentation over time
- Multiple sessions, continuous progress

---

## Part V: Patterns and Practices

### Chapter 12: Patterns That Work

**12.1 Context First**
- Always establish context before work
- Point to documentation
- Reference existing patterns

**12.2 Incremental Development**
- Small pieces, verified frequently
- Building on solid foundations
- Not getting ahead of verification

**12.3 Direct Communication**
- Say what you mean
- Correct immediately
- Confirm when satisfied

**12.4 Pattern References**
- "Do it like X"
- Using existing code as templates
- Maintaining consistency

**12.5 Visual Feedback**
- Screenshots for UI work
- Showing rather than describing
- Closing the visual loop

---

### Chapter 13: Anti-Patterns to Avoid

**13.1 Skipping Context**
- The cost of assumptions
- Inconsistent code generation
- Time lost to misunderstanding

**13.2 Batching Too Much**
- Unverified changes accumulating
- Harder debugging
- Lost track of what changed

**13.3 Accepting Without Verification**
- Bugs shipped unknowingly
- Technical debt accumulating
- False confidence

**13.4 Letting Mistakes Compound**
- Wrong assumptions building on wrong assumptions
- The importance of immediate correction
- Cutting losses early

**13.5 Over-Delegating**
- Losing touch with your codebase
- Abdicating architectural decisions
- Becoming dependent rather than empowered

---

## Part VI: Real-World Practice

### Chapter 14: Case Study: Building a Feature

**14.1 The Feature: Site Settings**
- Requirements: site name, logo, footer
- Starting point: existing codebase with patterns

**14.2 The Development Flow**
- Initial implementation
- Discovering missing pieces (file serving)
- Fixing issues (spinner, alignment)
- Iterative refinement
- Final polish

**14.3 Lessons from the Process**
- What worked well
- What required correction
- How iteration solved problems

---

### Chapter 15: Case Study: Cross-Cutting Changes

**15.1 The Challenge: Dark Mode Everywhere**
- Many templates needing updates
- Consistent pattern required
- Scale of the change

**15.2 The Approach: Parallel Agents**
- Defining the pattern
- Distributing the work
- Collecting and verifying results

**15.3 The Outcome**
- Time saved
- Consistency achieved
- Lessons for future bulk changes

---

### Chapter 16: Case Study: Complex Feature

**16.1 The Feature: Materials Management**
- Domain complexity: assignments, visibility, file storage
- Technical complexity: multiple handlers, stores, templates
- Integration complexity: fits into existing system

**16.2 Planning Phase**
- Exploring existing patterns
- Designing the approach
- Creating a detailed plan

**16.3 Execution Phase**
- Building piece by piece
- Integrating with existing code
- Testing throughout

**16.4 Refinement Phase**
- UI adjustments
- Edge cases
- Documentation

---

## Part VII: Growing Your Practice

### Chapter 17: Getting Started

**17.1 Starting Small**
- First tasks to try
- Building confidence
- Learning the rhythm

**17.2 Building Context Documentation**
- Starting your context document
- What to include first
- Growing it over time

**17.3 Developing Communication Skills**
- Learning to direct effectively
- Recognizing when to intervene
- Finding your style

**17.4 Measuring Your Progress**
- What speeds up
- What stays the same
- Where you add value

---

### Chapter 18: Deepening Expertise

**18.1 Knowing When AI Struggles**
- Types of problems that need human insight
- Recognizing limitations
- Stepping in appropriately

**18.2 Developing Pattern Recognition**
- Seeing what's wrong quickly
- Knowing what good looks like
- Trusting your judgment

**18.3 Architectural Thinking**
- AI executes; you architect
- Making decisions that compound well
- Thinking beyond the current task

**18.4 Teaching AI Your Codebase**
- Building better context documents
- Creating reference implementations
- Investing in patterns that pay off

---

### Chapter 19: The Future of Development

**19.1 What Changes**
- Speed of execution
- Scale of individual contribution
- Nature of development work

**19.2 What Stays the Same**
- Need for understanding
- Importance of quality judgment
- Value of experience

**19.3 The Evolving Partnership**
- AI capabilities will grow
- Human role will evolve
- The partnership deepens

**19.4 Staying Relevant**
- Skills that matter more
- Skills that matter differently
- Continuous learning

---

## Appendices

### Appendix A: Sample Context Document
- Complete example with annotations
- Customization guidance

### Appendix B: Message Templates
- Task initiation templates
- Problem reporting templates
- Refinement templates

### Appendix C: Checklists
- Session start checklist
- Feature development checklist
- Session end checklist

### Appendix D: Troubleshooting
- Common problems and solutions
- When AI goes wrong
- Recovery strategies

---

## About This Book

This book emerged from extensive real-world experience building software with AI assistance. Every pattern, anti-pattern, and piece of advice comes from actual development work—building features, fixing bugs, refining interfaces, and shipping working software.

The goal is not to make you dependent on AI, but to make you more capable with it. The developer who understands their system deeply and can direct AI effectively will outperform both the developer working alone and the one who delegates without understanding.

AI-assisted development is not about writing less code. It's about building more software, faster, while maintaining the quality and coherence that comes from human judgment and expertise.

---

*Working title: "AI-Assisted Software Development: A Practical Guide"*
*Alternative titles:*
- *"Collaborative Code: The Developer's Guide to AI-Assisted Development"*
- *"Steering and Accelerating: How to Build Software with AI"*
- *"The AI-Augmented Developer"*
