# Research Platform Positioning: Strategic Analysis

This document explores positioning StrataHub as a research study management platform for education researchers, rather than as a general-purpose LMS.

---

## Executive Summary

StrataHub's origin as infrastructure for an educational research project (Mission HydroSci) positions it uniquely in an underserved market. Rather than competing in the crowded LMS space, there's an opportunity to own the niche of **education research study management** — a market with no purpose-built solution, real funding, and growing demand for evidence-based education.

---

## The Market Opportunity

### Why Education Research Needs This

Education research studies — particularly those involving K-12 students using digital interventions — face a common set of problems:

1. **No purpose-built platform exists**
   - Researchers cobble together Google Classroom + Qualtrics + spreadsheets
   - Or they build custom solutions that die when the grant ends
   - Or they awkwardly use Canvas (designed for instruction, not research)

2. **Technical barriers waste research time**
   - Graduate students become reluctant sysadmins
   - PIs spend grant money on custom development
   - Technical problems delay studies and threaten timelines

3. **Compliance is complex**
   - COPPA for K-12 participants
   - FERPA considerations
   - IRB data security requirements
   - Consent tracking and documentation

4. **Research design needs are specific**
   - Treatment vs control group assignment
   - Randomization at student, class, or school level
   - Blinding (who sees what condition)
   - Fidelity of implementation tracking

### Market Size and Funding

**Federal Funding (Annual):**
| Agency | Education Research Funding | Notes |
|--------|---------------------------|-------|
| IES (Dept of Education) | ~$700M | Primary education research funder |
| NSF (Education & Human Resources) | ~$900M | STEM education focus |
| NIH (various) | ~$200M | Health education, behavior |

**Active Projects (Estimated):**
- IES: ~500 active grants at any time
- NSF Education: ~1,000+ active projects
- Many include technology/platform budget lines

**Per-Project Technology Budgets:**
- Small studies: $5,000-15,000 total
- Medium studies (multi-year): $20,000-50,000
- Large studies (multi-site): $50,000-150,000

**Market Calculation:**
- Conservative: 100 projects × $10K = $1M addressable annually
- Moderate: 300 projects × $15K = $4.5M addressable annually
- Optimistic: 500 projects × $20K = $10M addressable annually

This is a niche market, but it's real and fundable.

---

## Target Customers

### Primary: University Research Labs

**Profile:**
- Principal Investigator (PI) running education studies
- Graduate students and postdocs doing implementation
- Studies involving K-12 or undergraduate students
- Typically 1-5 year grant-funded projects

**Pain Points:**
- "We're researchers, not software developers"
- "Our last study, we spent 6 months building infrastructure"
- "The grad student who built our system graduated"
- "We need COPPA compliance but don't know how to do it"

**Decision Makers:**
- PI (writes the grant, controls the budget)
- Project Manager (if large project)
- University IT (sometimes has veto power)

**Buying Process:**
- Often included in grant proposal (planned in advance)
- Or purchased during project when problems arise
- Grant funds are already allocated

**Examples:**
- Learning sciences departments
- Educational psychology researchers
- Curriculum & instruction faculty
- STEM education researchers

---

### Secondary: Education Evaluation Firms

**Profile:**
- Professional research firms hired to evaluate programs
- Work with districts, states, foundations
- Larger budgets, more sophisticated needs

**Players:**
- WestEd
- American Institutes for Research (AIR)
- RAND Education
- Mathematica
- RTI International
- MDRC
- SRI International

**Pain Points:**
- "Every project we build custom infrastructure"
- "We need to run 10 studies, each slightly different"
- "Our clients expect professional platforms"

**Opportunity:**
- Larger contracts ($50K-200K)
- Repeat business across projects
- Could become referral sources

---

### Tertiary: EdTech Companies

**Profile:**
- Companies that need efficacy evidence for their products
- Investors and districts demand "research-backed" claims
- Running studies is not their core business

**Pain Points:**
- "We need to prove our game works"
- "Districts won't buy without evidence"
- "We're a game company, not a research company"

**Opportunity:**
- Platform + research design consulting
- Ongoing relationship as they run multiple studies
- Higher willingness to pay

---

## Competitive Landscape

### Direct Competitors: None

There is no purpose-built platform for managing education research studies. This is the key insight.

### What Researchers Use Now

| Solution | Pros | Cons |
|----------|------|------|
| **Google Classroom** | Free, familiar | Not for research, no randomization, no data export, no COPPA |
| **Canvas/LMS** | Institutions have it | Overkill, not designed for research, complex setup |
| **Custom Development** | Tailored to study | Expensive, dies after grant, maintenance burden |
| **Qualtrics/Survey Tools** | Good for surveys | Not a delivery platform, doesn't manage participants |
| **Spreadsheets + Email** | Free | Doesn't scale, error-prone, no delivery |

### Why No One Has Built This

1. **Market seems small** — VCs want billion-dollar markets
2. **Customers are dispersed** — Hard to find and sell to
3. **Complex requirements** — Each study is different
4. **Academic pricing expectations** — Lower than enterprise

### Defensibility

If successful, the moat would be:
1. **Network effects** — Researchers recommend to peers
2. **Specialization** — Deep features for this use case
3. **Switching costs** — Studies can't easily migrate mid-project
4. **Reputation** — "The platform serious researchers use"

---

## Product Positioning

### Positioning Statement

> **StrataHub** is a research study management platform that lets education researchers run participant studies without building custom software. Designed for the specific needs of randomized controlled trials, quasi-experimental designs, and intervention studies, StrataHub handles participant management, treatment/control assignment, content delivery, and data collection — with built-in COPPA compliance for K-12 research.

### Key Messages

**For PIs:**
> "Stop spending grant money on custom development. Run your study on StrataHub and focus on research."

**For Project Managers:**
> "Manage participants, assign conditions, deliver interventions, and export data — all in one place."

**For IRBs:**
> "Built-in COPPA compliance, consent tracking, and data security designed for human subjects research."

### Differentiators

| Claim | Proof Point |
|-------|-------------|
| "Built for research" | Treatment/control assignment, randomization, condition blinding |
| "COPPA compliant" | ClassLink and Clever integration, proper consent flows |
| "Easy to deploy" | Single binary, runs anywhere, no IT team needed |
| "Multi-study capable" | Workspaces isolate studies, one platform for a lab |
| "Research-friendly export" | CSV, API, designed for R/SPSS/Stata analysis |

### Naming Consideration

"StrataHub" works, but for this positioning might consider:
- **StrataHub for Research**
- **StrataHub Research Platform**
- **StudyHub** (if rebranding)
- **ResearchDesk**

The name should signal "research infrastructure" not "learning management."

---

## Feature Requirements

### Already Built (StrataHub Today)

| Feature | Research Application |
|---------|---------------------|
| Workspaces | Isolate different studies |
| Organizations | Schools/sites in multi-site studies |
| Groups | Treatment and control conditions |
| Members | Study participants |
| Leaders | Teachers implementing intervention |
| Resources | Intervention content (games, materials) |
| Materials | Teacher training content |
| Visibility windows | Control when content is accessible |
| COPPA auth | ClassLink, Clever for K-12 |
| File storage | Intervention files, materials |
| Multi-tenant | Multiple studies on one deployment |

### Needed: Phase 1 (Core Research Features)

| Feature | Description | Priority |
|---------|-------------|----------|
| **Randomization** | Assign participants to conditions (individual, cluster) | Critical |
| **Condition management** | Define treatment/control/multiple arms | Critical |
| **Data export** | CSV export of all study data | Critical |
| **Access logging** | Who accessed what, when, for how long | Critical |
| **Consent tracking** | Record consent status per participant | High |
| **Study timeline** | Define phases, auto-change visibility | High |
| **External survey links** | Link to Qualtrics/SurveyMonkey per condition | High |

### Needed: Phase 2 (Enhanced Research Features)

| Feature | Description | Priority |
|---------|-------------|----------|
| **Dosage tracking** | Time-on-task measurement | High |
| **Fidelity tracking** | Did teachers implement as intended | Medium |
| **Blinding options** | Control what different roles see | Medium |
| **Cohort management** | Rolling enrollment, multiple cohorts | Medium |
| **API** | Programmatic access for analysis pipelines | Medium |
| **Automated reminders** | Email participants/teachers | Medium |

### Needed: Phase 3 (Advanced Features)

| Feature | Description | Priority |
|---------|-------------|----------|
| **Built-in assessments** | Pre/post tests within platform | Medium |
| **Adaptive randomization** | Balance on covariates | Low |
| **Multi-site dashboard** | Aggregate view across sites | Medium |
| **IRB documentation export** | Generate protocol-ready docs | Low |
| **Integration with analysis tools** | Direct R/Python connection | Low |

---

## Go-to-Market Strategy

### Phase 1: Proof Points (Now - 6 months)

**Goal:** Get 3-5 paying research projects running on StrataHub.

**Actions:**
1. **Leverage MHS**
   - Case study and testimonials
   - PI as reference customer

2. **Warm outreach at University of Missouri**
   - College of Education
   - Learning sciences programs
   - Psychology department (educational psych)
   - Other research projects that might need this

3. **Direct outreach to IES grantees**
   - All funded projects are public record
   - Identify those with digital interventions
   - Email PIs directly with specific value prop

4. **Simple website**
   - Clear research positioning
   - MHS case study
   - Feature list for researchers
   - "Schedule a demo" CTA

### Phase 2: Network Effects (6-18 months)

**Goal:** Establish reputation in research community.

**Actions:**
1. **Conference presence**
   - AERA (American Educational Research Association)
   - SREE (Society for Research on Educational Effectiveness)
   - Games+Learning+Society
   - ISLS (Learning Sciences)
   - Set up booth, give demos, sponsor sessions

2. **Academic partnerships**
   - Formal partnership with 1-2 university labs
   - "Preferred platform" status
   - Joint grant applications

3. **Content marketing**
   - Blog posts on research methodology
   - Guides: "Running COPPA-Compliant K-12 Studies"
   - Webinars on research study logistics

4. **Word of mouth**
   - Referral incentives for researchers
   - Graduate student discounts (they become PIs later)

### Phase 3: Scale (18+ months)

**Goal:** Become the default platform for education research studies.

**Actions:**
1. **Evaluation firm partnerships**
   - Enterprise deals with WestEd, AIR, etc.
   - Volume pricing

2. **Grant writing integration**
   - Templates for including StrataHub in proposals
   - Budget justification language
   - Pre-approval with major funders

3. **Training and certification**
   - "StrataHub Certified" research assistants
   - Integration in research methods courses

---

## Pricing Strategy

### Model: Per-Study Subscription

| Tier | Participants | Features | Price |
|------|-------------|----------|-------|
| **Pilot** | Up to 100 | Core features | $200/month |
| **Standard** | Up to 500 | + Advanced randomization | $500/month |
| **Professional** | Up to 2,000 | + API, priority support | $1,000/month |
| **Enterprise** | Unlimited | + Custom, SLA, dedicated support | Custom |

**Annual prepay:** 2 months free (10 months for price of 12)

### Alternative: Per-Participant Pricing

- $5-10 per participant per study
- Simpler to understand
- Aligns cost with study size
- Risk: Large studies become expensive

### Academic Considerations

- Accept grant/PO payments (net 30-60)
- Provide formal quotes for grant budgets
- Offer multi-year pricing for multi-year grants
- Consider university-wide site licenses

### Free Tier?

**Option A: No free tier**
- Research has funding; free devalues product
- Pilot tier is affordable enough

**Option B: Limited free tier**
- Up to 20 participants, 1 study
- Good for grad students testing
- Converts to paid for real studies

**Recommendation:** Start with no free tier. Offer extended trials instead.

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Market too small | Medium | High | Expand to EdTech efficacy studies |
| Long sales cycles | High | Medium | Start with warm leads, build pipeline |
| Feature requests overwhelm | Medium | Medium | Strict prioritization, say no often |
| Single-developer concern | Medium | High | Document well, consider contractor help |
| Competition emerges | Low | Medium | Move fast, build relationships |
| Grant cycles misalign | Medium | Low | Offer flexible start dates |

### The "Just Use Canvas" Objection

**Objection:** "Our university already has Canvas."

**Response:**
- Canvas is designed for courses, not research studies
- No randomization, no condition management, no research data export
- Setting up a "course" for research is awkward and limited
- Your IRB may have concerns about student data in the LMS
- Canvas doesn't track the data you need for research

---

## Success Metrics

### Year 1

| Metric | Target |
|--------|--------|
| Paying studies | 10 |
| Annual recurring revenue | $50,000 |
| Active participants served | 2,000 |
| Reference customers | 3 |
| Conference presentations | 2 |

### Year 2

| Metric | Target |
|--------|--------|
| Paying studies | 30 |
| Annual recurring revenue | $200,000 |
| Active participants served | 10,000 |
| Evaluation firm contracts | 1 |
| Researchers recommending | 10+ |

### Year 3

| Metric | Target |
|--------|--------|
| Paying studies | 75 |
| Annual recurring revenue | $500,000 |
| Active participants served | 30,000 |
| Evaluation firm contracts | 3 |
| Mentioned in grant proposals | Regular |

---

## Appendix: IES Grant Programs (Target Customers)

IES funds education research through these programs:

| Program | Focus | Typical Budget | Study Duration |
|---------|-------|----------------|----------------|
| Exploration | New ideas | $400K-700K | 2-3 years |
| Development | Build interventions | $1.5M-3M | 3-4 years |
| Efficacy | Test interventions | $3M-6M | 4-5 years |
| Effectiveness | Scale testing | $4M-8M | 4-5 years |
| Replication | Confirm findings | $2M-4M | 4 years |

Development, Efficacy, and Effectiveness grants are prime targets — they have budget and need infrastructure.

**Where to find them:** https://ies.ed.gov/funding/grantsearch/

---

## Appendix: Sample Outreach Email

Subject: Platform for running your [TOPIC] study

Dr. [NAME],

I noticed your IES-funded project on [TOPIC] involves [digital intervention / game-based learning / online curriculum] with K-12 students.

We built StrataHub specifically for studies like yours — it handles participant management, treatment/control assignment, content delivery, and COPPA-compliant authentication, so you can focus on research rather than software.

We're currently running a multi-site RCT with 700+ students using the platform, and I'd be happy to share how it might work for your project.

Would you have 20 minutes for a quick demo?

Best,
[NAME]

---

## Appendix: Competitive Comparison Table

| Capability | StrataHub | Canvas | Google Classroom | Custom Build |
|------------|-----------|--------|------------------|--------------|
| Designed for research | ✓ | ✗ | ✗ | Maybe |
| Treatment/control groups | ✓ | Manual | ✗ | Build it |
| Randomization | ✓ | ✗ | ✗ | Build it |
| COPPA authentication | ✓ | Varies | ✗ | Build it |
| Research data export | ✓ | Limited | ✗ | Build it |
| Access logging | ✓ | Basic | ✗ | Build it |
| Multi-site studies | ✓ | Awkward | ✗ | Build it |
| Time to deploy | Days | Weeks | N/A | Months |
| Cost | $$ | $$$ | Free | $$$$ |
| Maintained after grant | ✓ | N/A | N/A | ✗ |

---

*Document created: December 2024*
*Status: Strategic positioning document for consideration.*
