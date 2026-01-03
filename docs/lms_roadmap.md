# LMS Roadmap: From StrataHub to Learning Management System

This document outlines a potential path for evolving StrataHub into a full-featured Learning Management System (LMS) that could serve as an alternative to platforms like Canvas.

---

## Vision

Build a modern, lightweight, fast LMS that:
- Deploys as a single binary (no complex infrastructure)
- Runs efficiently (fraction of Canvas's resource requirements)
- Provides excellent user experience (clean, intuitive, not bloated)
- Supports multi-tenancy from the ground up
- Is accessible and COPPA-compliant
- Can compete with Canvas for institutions that want simplicity over feature bloat

---

## Current State: StrataHub Foundation

### What Exists Today

| Component | Status | Notes |
|-----------|--------|-------|
| Multi-tenancy | ✓ | Workspaces with workspace_id on all entities |
| Organizations | ✓ | Schools, departments, companies |
| Groups | ✓ | Classes, cohorts, sections |
| Users & Roles | ✓ | Admin, analyst, leader, member (coordinator planned) |
| Content Delivery | ✓ | Resources and materials with visibility windows |
| File Storage | ✓ | Local and S3/CloudFront abstraction |
| Authentication | ✓ | Internal, Google, ClassLink, Clever |
| Rich Text | ✓ | TipTap editor for HTML content |
| Dark Mode | ✓ | Full dark mode support |
| Responsive UI | ✓ | Mobile-friendly design |

### Technical Stack

| Layer | Technology | Advantage |
|-------|------------|-----------|
| Backend | Go 1.24 | Fast, efficient, great concurrency |
| Database | MongoDB | Flexible schema, document-oriented |
| Frontend | HTMX + Tailwind | Lightweight, fast development |
| Framework | Waffle | Storage, templates, sessions |
| Deployment | Single binary (embed) | Trivial ops, low cost |

---

## Competitive Analysis

### Canvas Weaknesses (Opportunities)

1. **Heavy resource requirements** - Needs significant infrastructure
2. **Slow performance** - Ruby on Rails shows its age
3. **Complex deployment** - Docker, Redis, PostgreSQL, background jobs
4. **Bloated UX** - 15 years of accumulated features
5. **Expensive** - Enterprise pricing
6. **Overwhelming for simple use cases** - Too much for many needs

### Market Gaps

| Segment | Pain Point | Opportunity |
|---------|------------|-------------|
| Small colleges | Canvas is overkill and expensive | Simpler, cheaper alternative |
| K-12 | Complex setup, COPPA concerns | Easy deployment, built-in COPPA |
| Corporate training | Don't need academic features | Focused feature set |
| International | Canvas expensive, US-centric | Lower cost, simpler |
| Self-hosted | Canvas is hard to self-host | Single binary deployment |

---

## Gap Analysis: StrataHub → LMS

### Tier 1: Core LMS (Must Have)

| Feature | Current State | Effort | Priority |
|---------|---------------|--------|----------|
| Progress tracking | Not implemented | Medium | High |
| Simple quizzes | Not implemented | High | High |
| Gradebook | Not implemented | High | High |
| Announcements | Not implemented | Low | High |
| Email notifications | Not implemented | Medium | High |
| Due dates | Partial (visibility windows) | Low | High |
| Student dashboard | Basic | Medium | High |

### Tier 2: Enhanced LMS (Should Have)

| Feature | Current State | Effort | Priority |
|---------|---------------|--------|----------|
| Full quiz engine | Not implemented | Very High | Medium |
| Discussion forums | Not implemented | Medium | Medium |
| Learning pathways/modules | Not implemented | Medium | Medium |
| Rubrics | Not implemented | Medium | Medium |
| Calendar view | Not implemented | Medium | Medium |
| File submissions | Not implemented | Medium | Medium |
| Peer review | Not implemented | Medium | Low |

### Tier 3: Advanced LMS (Nice to Have)

| Feature | Current State | Effort | Priority |
|---------|---------------|--------|----------|
| LTI 1.3 integration | Not implemented | Very High | Medium |
| SIS import/export | Not implemented | High | Medium |
| Outcomes/competencies | Not implemented | High | Low |
| SCORM player | Not implemented | Very High | Low |
| Analytics dashboards | Basic reports | Medium | Medium |
| Mobile apps (native) | Not implemented | Very High | Low |
| Video conferencing | Not implemented | High | Low |

---

## Phased Roadmap

### Phase 1: Tracking & Communication (3-4 months)

**Goal:** Transform from content delivery to learning management.

#### Features

1. **Progress Tracking**
   - Log when members access resources
   - Track completion (manual or automatic for some types)
   - Store: access_logs collection
   - Show progress indicators on member dashboard

2. **Announcements**
   - Leaders/admins can post announcements to groups or orgs
   - Display on member dashboard
   - Mark as read/unread
   - Store: announcements collection

3. **Email Notifications**
   - New resource assigned
   - New announcement posted
   - Upcoming due dates
   - Configurable preferences per user
   - Queue-based sending (background job)

4. **Due Dates**
   - Add due_date to resource assignments (in addition to visibility windows)
   - Show upcoming items on dashboard
   - "Overdue" indicators

5. **Enhanced Dashboard**
   - Member: Upcoming items, recent announcements, progress summary
   - Leader: Class progress overview, who's behind
   - Admin: Workspace-wide activity

#### Technical Work

- Add access_logs collection and store
- Add announcements collection and store
- Email sending infrastructure (SMTP config, templates, queue)
- Dashboard aggregation queries

---

### Phase 2: Assessments (4-6 months)

**Goal:** Add quiz/test capability with grading.

#### Features

1. **Quiz Builder**
   - Create quizzes with questions
   - Question types (start simple, expand):
     - Multiple choice (single answer)
     - Multiple choice (multiple answers)
     - True/False
     - Short answer (exact match)
     - Essay (manual grading)
   - Question bank per workspace
   - Randomization options (order, select N from bank)

2. **Quiz Taking**
   - Time limits (optional)
   - One attempt vs multiple attempts
   - Show results immediately vs after due date
   - Review correct answers (configurable)

3. **Auto-Grading**
   - Immediate grading for objective questions
   - Partial credit options
   - Manual grading queue for essays

4. **Gradebook (Basic)**
   - View all grades for a group
   - Manual grade entry
   - Export to CSV
   - Grade passback to assignments

#### Data Model

```
quizzes:
  - id, workspace_id, title, description
  - time_limit, attempts_allowed
  - show_results_when (immediate, after_due, manual)
  - created_by, created_at

questions:
  - id, workspace_id, quiz_id (or null for bank)
  - type (mc_single, mc_multi, tf, short, essay)
  - text, options[], correct_answer
  - points, partial_credit

quiz_assignments:
  - id, group_id, quiz_id
  - due_date, available_from, available_until

quiz_attempts:
  - id, user_id, quiz_assignment_id
  - started_at, submitted_at
  - answers[], score, graded_by
```

---

### Phase 3: Pathways & Discussions (3-4 months)

**Goal:** Structured learning sequences and collaboration.

#### Features

1. **Learning Modules**
   - Ordered container for resources, quizzes, discussions
   - Sequential unlock (complete item N to see item N+1)
   - Prerequisites between modules
   - Completion requirements (all items, minimum score, etc.)

2. **Discussion Forums**
   - Per-group discussion boards
   - Threaded replies
   - Instructor can mark as "must post before seeing replies"
   - Graded discussions (participation points)
   - Moderation tools (pin, lock, delete)

3. **Assignment Submissions**
   - Upload file for assignment
   - Submission status (not submitted, submitted, graded)
   - Resubmission options
   - Download all submissions (for grading)

4. **Gradebook (Enhanced)**
   - Weighted grade categories
   - Drop lowest N scores
   - Late penalty policies
   - Grade history/audit trail

---

### Phase 4: Integration & Scale (4-6 months)

**Goal:** Enterprise readiness and interoperability.

#### Features

1. **LTI 1.3 (Tool Consumer)**
   - Embed external tools in courses
   - Deep linking
   - Assignment and grade services
   - This is complex but table stakes for higher ed

2. **LTI 1.3 (Tool Provider)**
   - Allow StrataHub content to be embedded in other LMS
   - Useful for gradual adoption

3. **SIS Integration**
   - Import rosters from Student Information Systems
   - CSV import with mapping
   - API for programmatic sync
   - SCIM for user provisioning

4. **Advanced Analytics**
   - Learning analytics dashboard
   - At-risk student identification
   - Course comparison across terms
   - Export to BI tools

5. **API**
   - REST API for all major operations
   - API keys and OAuth for third parties
   - Webhooks for events

---

### Phase 5: Polish & Parity (Ongoing)

**Goal:** Feature parity for competitive positioning.

#### Features (As Needed)

1. **SCORM Player** - Run packaged e-learning content
2. **xAPI/Caliper** - Learning analytics standards
3. **Outcomes/Competencies** - Standards-based grading
4. **Peer Review** - Student-to-student feedback
5. **Groups within Groups** - Student workgroups
6. **Video** - Built-in recording, video assignments
7. **Mobile Apps** - Native iOS/Android (or enhanced PWA)
8. **Accessibility Audit** - Full WCAG 2.1 AA compliance
9. **Multi-language** - i18n support
10. **White-labeling** - Full branding customization

---

## Technical Architecture Evolution

### New Collections

```
Phase 1:
- access_logs (user, resource, timestamp, duration)
- announcements (workspace, org/group, title, body, author)
- notification_queue (user, type, payload, status)
- user_preferences (user, email_prefs, ui_prefs)

Phase 2:
- quizzes (workspace, title, settings)
- questions (workspace, quiz or bank, type, content)
- quiz_assignments (group, quiz, dates)
- quiz_attempts (user, assignment, answers, score)
- grades (user, group, item_type, item_id, score)

Phase 3:
- modules (workspace, group, title, order, requirements)
- module_items (module, item_type, item_id, order, requirements)
- discussions (group, title, settings)
- discussion_posts (discussion, user, parent, content)
- submissions (user, assignment, files, status)

Phase 4:
- lti_tools (workspace, name, config, keys)
- lti_launches (tool, user, context, timestamp)
- api_keys (workspace, user, key, permissions)
- webhooks (workspace, events, url, secret)
```

### Background Jobs

Starting Phase 1, need background job processing for:
- Email sending
- Notification delivery
- Grade calculations
- Analytics aggregation
- SIS sync

Options:
- Simple: Goroutine-based in-process queue
- Scalable: Redis-based queue (adds dependency)
- Recommendation: Start simple, add Redis when needed

### Search

As content grows, need full-text search:
- MongoDB text indexes (built-in, good enough to start)
- Later: Elasticsearch/Meilisearch for advanced search

---

## Effort Estimates

| Phase | Duration | Team Size | Notes |
|-------|----------|-----------|-------|
| Phase 1 | 3-4 months | 1-2 devs | Foundation for LMS |
| Phase 2 | 4-6 months | 2-3 devs | Assessment is complex |
| Phase 3 | 3-4 months | 2 devs | Builds on Phase 1-2 |
| Phase 4 | 4-6 months | 2-3 devs | LTI is complex |
| Phase 5 | Ongoing | 1-2 devs | Based on market needs |

**Total to "LMS Lite" (Phase 1-3):** 10-14 months with small team
**Total to "Full LMS" (Phase 1-4):** 14-20 months with small team

*Note: These are rough estimates. Actual time depends on scope decisions, testing requirements, and how polished the UX needs to be.*

---

## Go-to-Market Considerations

### Positioning Options

1. **"The Simple LMS"**
   - For those who find Canvas overwhelming
   - Focus: ease of use, fast setup, clean UX
   - Market: Small colleges, K-12, training programs

2. **"The Efficient LMS"**
   - Runs on minimal infrastructure
   - Self-hosted friendly
   - Market: Cost-conscious institutions, international

3. **"The Modern LMS"**
   - Built with modern tech, not legacy
   - Fast, responsive, mobile-first
   - Market: Forward-thinking institutions

### Pricing Models

| Model | Approach |
|-------|----------|
| Open Source + Support | Free to use, paid support/hosting |
| SaaS | Per-user or per-workspace pricing |
| Enterprise | On-premise license + support |
| Freemium | Free tier (limited users), paid for more |

### Competitive Advantages

1. **Deployment simplicity** - Single binary vs. Canvas's complexity
2. **Performance** - Go is 10-50x faster than Ruby
3. **Resource efficiency** - Runs on modest hardware
4. **Modern codebase** - No legacy cruft
5. **Multi-tenant native** - Not bolted on
6. **COPPA built-in** - K-12 ready from start

---

## Key Decisions Needed

Before proceeding, clarify:

1. **Target market priority:**
   - K-12? Higher Ed? Corporate? All?
   - Each has different feature priorities

2. **Assessment depth:**
   - Simple quizzes only?
   - Full testing engine with question banks?
   - Proctoring/anti-cheating?

3. **Integration requirements:**
   - Is LTI required for your market?
   - SIS integration needed?

4. **Open source strategy:**
   - Fully open source?
   - Open core (free + paid features)?
   - Proprietary SaaS?

5. **Mobile strategy:**
   - PWA sufficient?
   - Native apps needed?

6. **Accessibility requirements:**
   - WCAG level (A, AA, AAA)?
   - Timeline for compliance?

---

## Risk Factors

| Risk | Mitigation |
|------|------------|
| Scope creep | Strict phasing, MVP mindset |
| Assessment complexity | Start simple, iterate |
| LTI complexity | Consider third-party library |
| Market timing | Ship early phases, get feedback |
| Competition | Focus on differentiation (simplicity, speed) |
| Single developer risk | Document well, consistent patterns |

---

## Success Metrics

### Phase 1 Success
- [ ] Members can see their progress
- [ ] Leaders can see class progress
- [ ] Announcements delivered to members
- [ ] Email notifications working
- [ ] Dashboard shows upcoming items

### Phase 2 Success
- [ ] Can create and assign quizzes
- [ ] Auto-grading works for objective questions
- [ ] Gradebook shows all scores
- [ ] At least 3 question types supported

### Phase 3 Success
- [ ] Modules with sequential unlock work
- [ ] Discussions with threading work
- [ ] File submissions work
- [ ] Gradebook has weighted categories

### Phase 4 Success
- [ ] Can embed LTI tools
- [ ] Can import rosters from CSV
- [ ] API available for integrations
- [ ] Analytics dashboard for admins

---

## Appendix: Feature Comparison Matrix

| Feature | Canvas | StrataHub Now | After Phase 3 |
|---------|--------|---------------|---------------|
| Courses/Groups | ✓ | ✓ | ✓ |
| Assignments | ✓ | ✓ (resources) | ✓ |
| Quizzes | ✓ | ✗ | ✓ |
| Discussions | ✓ | ✗ | ✓ |
| Grades | ✓ | ✗ | ✓ |
| Modules | ✓ | ✗ | ✓ |
| Announcements | ✓ | ✗ | ✓ |
| Calendar | ✓ | ✗ | Partial |
| Files | ✓ | ✓ | ✓ |
| Progress Tracking | ✓ | ✗ | ✓ |
| LTI | ✓ | ✗ | Phase 4 |
| Mobile App | ✓ | PWA | PWA |
| Analytics | ✓ | Basic | ✓ |
| Multi-tenant | ✓ | ✓ | ✓ |
| Easy Deploy | ✗ | ✓ | ✓ |
| Fast | ✗ | ✓ | ✓ |

---

*Document created: December 2024*
*Status: Strategic planning document. Not yet committed to implementation.*
