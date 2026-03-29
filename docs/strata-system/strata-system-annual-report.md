# Strata System

## 1. Overview

The Strata System is the integrated data and application platform developed to support the Mission HydroSci project. It provides authentication, gameplay delivery, data collection, state persistence, automated assessment, and teacher-facing dashboards within a unified architecture designed for both classroom use and educational research.

The system enables students to interact with the Mission HydroSci game while preserving their progress, capturing gameplay interactions as structured data, and automatically evaluating and presenting performance to teachers in near-real-time. It collects only gameplay interaction data and game state information necessary for assessment, research, and session continuity.

The Strata System consists of four primary components:

- **StrataHub:** application layer for authentication, user management, game delivery, and dashboards
- **StrataLog:** gameplay event collection and analytics data store
- **StrataSave:** game state and player settings persistence service
- **MHS Grader:** automated assessment engine for curriculum-aligned evaluation

Together, these components form a continuous pipeline from student gameplay to actionable classroom and research insights.

## 2. System Architecture

The Strata System is designed as a set of independent but coordinated services:

- Students access the system through StrataHub, which manages authentication and launches the game
- The Mission HydroSci game runs in the browser as a Progressive Web App (PWA)
- The game communicates with backend services during gameplay:
  - StrataLog for event collection
  - StrataSave for state persistence
- The MHS Grader processes event data and produces structured evaluations
- Results are stored in a dedicated grading database and displayed in StrataHub dashboards

Each component operates independently but shares a common authentication model and interoperates through well-defined data flows. This separation allows services to be updated, restarted, or scaled without disrupting the rest of the system.

The system is deployed on cloud infrastructure (Amazon Web Services) and is designed to scale to support multiple schools and large numbers of concurrent students.

## 3. End-to-End Data Flow

The Strata System operates as a continuous pipeline connecting student activity to teacher-visible results:

**1. Student access and launch.** A student logs into StrataHub using an approved authentication provider (e.g., Google, Clever, ClassLink). StrataHub verifies identity and launches the game with secure, session-scoped API credentials.

**2. Gameplay interaction.** The student plays Mission HydroSci in the browser, interacting with simulations, investigations, dialogues, and navigation tasks.

**3. Event logging (StrataLog).** Gameplay actions are sent to StrataLog as structured, timestamped events. Each event includes both client and server timestamps, forming a precise, time-ordered behavioral record.

**4. State persistence (StrataSave).** The game periodically saves progress and settings to StrataSave, allowing students to resume exactly where they left off across sessions.

**5. Automated assessment (MHS Grader).** The MHS Grader continuously polls StrataLog for relevant events. When trigger events occur, it evaluates performance within defined attempt windows and produces structured progress evaluations (passed, flagged, active) with detailed metrics.

**6. Dashboard presentation (StrataHub).** The MHS Dashboard retrieves evaluation data and displays it in a color-coded grid, allowing teachers and researchers to monitor progress in near-real-time.

This pipeline operates continuously and automatically, with no manual grading required.

## 4. Core Components

### StrataHub (Application Layer)

StrataHub is the central interface for all users. It manages authentication, user access, game delivery, and the teacher-facing dashboard.

It organizes users into a hierarchy aligned with classroom and research deployment:

- Workspaces (top-level environments)
- Organizations (schools or sites)
- Groups (classes or sections)
- Members (students)

Role-based access includes administrators, analysts, coordinators, teachers (leaders), and students.

StrataHub delivers Mission HydroSci as a Progressive Web App (PWA), eliminating the need for app store distribution or device-level installation. It also hosts the MHS Dashboard, which provides near-real-time visibility into student progress across 26 curriculum-aligned progress points.

### StrataLog (Event Collection)

StrataLog captures detailed gameplay events in near-real-time. Each event represents a meaningful student action (e.g., completing a task, selecting an answer, interacting with a simulation).

Key characteristics:

- Dual timestamps (client and server) for accurate timing analysis
- Flexible schema allowing evolving instrumentation
- Secure, authenticated event submission
- High-throughput ingestion for large-scale deployments

StrataLog serves as both:

- The data source for automated assessment, and
- The research dataset for analyzing learning behaviors

Over 700,000 gameplay events have been recorded to date, prior to the primary study period, demonstrating sustained system operation under real usage conditions.

### StrataSave (State Persistence)

StrataSave ensures continuity across gameplay sessions by storing:

- Game state: player position, progress, inventory, and activity state
- Player settings: audio preferences, controls, and customization

Key features:

- Multiple save states per player with configurable retention
- Automatic cleanup of older saves
- Upsert model for player settings (always current)
- Fast, non-blocking save operations

This allows students to resume exactly where they left off, which is essential in classroom environments with limited session time.

StrataSave has handled hundreds of thousands of API requests, with high-frequency operations such as settings loads occurring at the start of every session.

### MHS Grader (Assessment Engine)

The MHS Grader is a standalone background service that continuously evaluates student performance.

It assesses 26 progress points across five curriculum units, using rule types including:

- Completion checks
- Threshold-based evaluation
- Weighted scoring
- Time-based metrics
- Multi-component aggregation
- Zero-tolerance rules

Each evaluation produces:

- A status (passed, flagged, active)
- A reason code (if flagged)
- Detailed performance metrics (mistakes, score, duration, attempts)

Key design features:

- Attempt window isolation ensures replay-safe grading
- Cursor-based processing guarantees no event loss or duplication
- Near-real-time updates (typically within seconds of activity completion)
- Rule versioning supports longitudinal research analysis

## 5. Key System Properties

The Strata System is designed to support both instruction and research:

- **Near-real-time feedback:** evaluations appear within seconds of activity completion
- **Seamless student experience:** progress resumes exactly where it left off
- **Automated assessment:** no manual grading required
- **Detailed behavioral data:** supports analysis of learning processes, not just outcomes
- **Scalable architecture:** supports large, multi-school deployments
- **Secure access control:** authenticated sessions and role-based permissions
- **Data minimization:** only essential gameplay and state data are stored

## 6. Security and Data Practices

The system implements multiple layers of security:

- **Encrypted communication:** all data transmitted via TLS
- **Session-scoped credentials:** issued by StrataHub for all game-service interactions
- **Role-based access control:** limits data visibility by user role
- **Data isolation:** enforced at workspace, organization, group, and player levels
- **Session security:** encrypted cookies, configurable idle timeouts, and CSRF protection
- **Audit logging:** administrative actions are recorded for accountability

No unnecessary personal or unrelated student data is collected. The system stores only gameplay interaction data and state data required for its educational and research functions. These controls support secure handling of student data in accordance with institutional and research requirements.

## 7. Educational and Research Value

The Strata System provides capabilities not achievable with traditional educational tools.

For teachers:

- Near-real-time visibility into student progress
- Immediate identification of students who may benefit from additional support
- Detailed insight into performance beyond pass/fail

For researchers:

- High-resolution behavioral data across full gameplay sessions
- Ability to analyze timing, sequence, and patterns of learning
- Support for studying how learning occurs, not just outcomes

By integrating gameplay, data collection, assessment, and visualization into a single platform, the system enables both instructional support and rigorous research analysis.

## 8. Why the System Was Custom-Built

The Strata System was developed because no existing solution provides the required combination of capabilities:

- Learning management systems do not support near-real-time game-based assessment
- Game analytics platforms do not evaluate individual student learning outcomes
- Standardized assessment tools cannot assess process-based, interactive learning
- General-purpose storage and rule engines lack domain-specific integration

The system combines near-real-time event capture, curriculum-aligned automated assessment, seamless state persistence, and teacher-facing visualization into a unified platform under full project control, suitable for a federally funded educational research environment.

## 9. Summary

The Strata System transforms student gameplay into near-real-time assessment and research insights.

- **StrataHub** manages authentication, access, and dashboards
- **StrataLog** captures detailed gameplay events
- **StrataSave** preserves progress and settings
- **MHS Grader** evaluates performance across 26 progress points

Together, these components form a scalable, secure, and purpose-built platform that supports both classroom instruction and the research goals of the Mission HydroSci project.
