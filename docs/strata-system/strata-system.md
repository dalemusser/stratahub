# Strata System

## 1. Purpose and Overview

The Strata System is the integrated data and application platform that supports the Mission HydroSci project. It provides authentication, gameplay delivery, data collection, state persistence, automated assessment, and teacher-facing dashboards within a unified architecture.

The system is designed specifically for educational research and classroom deployment. It enables students to interact with the Mission HydroSci game while preserving their progress, capturing gameplay interactions as structured data, and automatically evaluating and presenting performance to teachers in near-real-time.

The Strata System consists of four primary components:

- **StrataHub:** the application layer that manages authentication, user access, game delivery, and the teacher-facing dashboard (see [StrataHub](stratahub.md))
- **StrataLog:** the gameplay event collection and analytics data store (see [StrataLog](stratalog.md))
- **StrataSave:** the game state and player settings persistence service (see [StrataSave](stratasave.md))
- **MHS Grader:** the automated assessment engine that evaluates student performance against curriculum-aligned criteria (see [MHS Grader](mhsgrader.md))

Together, these components form a complete pipeline from gameplay to actionable classroom insights. Each component is described in detail in its own section of this report.

The system collects only gameplay interaction data and game state information necessary for assessment, research, and session continuity.

## 2. System Architecture

At a high level, the Strata System operates as a coordinated set of independent services:

- Students access the system through StrataHub, which manages authentication and game delivery
- The Mission HydroSci game runs in the student's browser and communicates with backend services during gameplay
- Gameplay data flows to specialized services for event logging (StrataLog), state persistence (StrataSave), and automated assessment (MHS Grader)
- Assessment results are stored in a dedicated grading database and presented through the MHS Dashboard in StrataHub

Each service operates independently but is designed to interoperate through shared databases and a common authentication model. This separation allows individual components to be updated, restarted, or scaled without disrupting the rest of the system.

## 3. End-to-End Data Flow

The Strata System operates through a continuous data pipeline that connects student gameplay to teacher-visible progress evaluations:

**1. Student access and launch.** A student logs into StrataHub using an approved authentication provider. StrataHub verifies the student's identity, determines which game unit is available, and launches Mission HydroSci in the browser with secure, session-scoped API credentials.

**2. Gameplay interaction.** The student interacts with the game, completing activities that include dialogues, simulations, investigations, and navigation tasks. The game runs entirely in the browser.

**3. Event logging (StrataLog).** As the student plays, gameplay actions are sent to StrataLog as structured events in near-real-time. Each event includes timestamps, event types, and contextual data specific to the interaction. Over the course of a session, these events form a detailed, time-ordered behavioral record of the student's activity.

**4. State persistence (StrataSave).** The game periodically saves the student's progress to StrataSave. This allows the student to resume exactly where they left off in a future session. Player settings such as audio preferences and character appearance are also stored and restored independently.

**5. Automated assessment (MHS Grader).** The MHS Grader, running as a background service, continuously polls StrataLog for events that correspond to curriculum-aligned progress points. When a trigger event is detected, the grader evaluates the student's performance within the relevant attempt window and produces a structured progress evaluation (passed, flagged, or active) along with supporting metrics.

**6. Dashboard presentation (StrataHub).** The MHS Dashboard in StrataHub retrieves progress evaluations from the grader database and displays them in a color-coded grid. Teachers and researchers can view the status of every progress point for every student in a group, and can examine individual evaluations to see reason codes and detailed performance metrics.

This pipeline operates continuously and automatically. No manual grading is required. Teachers see near-real-time progress evaluations as students play.

## 4. Roles of Each Component

### StrataHub (Application Layer)

StrataHub is the central platform through which all users interact with the system. It manages authentication and user access across multiple schools and classrooms, launches the game with secure session credentials, organizes users into workspaces, organizations, and groups that mirror the structure of the research deployment, and hosts the MHS Dashboard where teachers and researchers monitor student progress.

For full details, see [StrataHub](stratahub.md).

### StrataLog (Event Collection)

StrataLog captures detailed gameplay events in near-real-time as students play Mission HydroSci. Each event carries both a client timestamp (when the action occurred on the student's device) and a server timestamp (when it was received), ensuring accurate timing analysis regardless of network conditions. StrataLog is the primary data source for both automated assessment and research analysis.

For full details, see [StrataLog](stratalog.md).

### StrataSave (State Persistence)

StrataSave saves and restores two categories of player data: game state (where the student is in the game and what they have accomplished within gameplay) and player settings (personal preferences such as audio volume and character appearance). It ensures continuity across sessions and maintains multiple save points per student with configurable retention policies.

For full details, see [StrataSave](stratasave.md).

### MHS Grader (Assessment Engine)

The MHS Grader continuously polls and processes StrataLog events to evaluate student performance at 26 curriculum-aligned progress points across the five units of Mission HydroSci. Each progress point has its own grading rule that defines the criteria for passing or being flagged. The grader produces structured progress evaluations with detailed metrics that are displayed on the MHS Dashboard.

For full details, see [MHS Grader](mhsgrader.md).

## 5. Key System Properties

The Strata System is designed to meet the needs of both classroom instruction and educational research:

- **Near-real-time feedback:** student progress evaluations appear on the dashboard within seconds of activity completion
- **Seamless student experience:** gameplay resumes exactly where it left off, preserving instructional time
- **Automated assessment:** 26 progress points evaluated continuously with no manual grading required
- **Detailed behavioral data:** structured gameplay event history supports research analysis of learning processes, not just outcomes
- **Scalable architecture:** independent services support high volumes of simultaneous student activity across multiple schools
- **Secure and controlled access:** authenticated sessions, role-based permissions, and session-scoped API credentials protect data in transit and control access to stored data
- **Data minimization:** only gameplay interaction data and game state data necessary for assessment, research, and session continuity are stored

## 6. Educational and Research Value

The Strata System enables a level of insight into student learning that is not achievable with traditional educational tools.

Teachers can monitor student progress in near-real-time and identify students who may benefit from additional support. The color-coded dashboard provides an immediate overview of an entire classroom, and detailed metrics allow teachers to understand the specific nature of a student's difficulty rather than receiving only a pass or fail indicator.

Researchers can analyze detailed learning behaviors across the full sequence of student interactions with the game. Because StrataLog captures the timing, sequence, and context of each gameplay action, the research dataset supports analysis of how students learn, not just whether they succeed. This granularity is essential for studying learning processes in game-based environments.

By integrating gameplay delivery, data collection, automated assessment, and visualization into a single platform, the Strata System provides a complete infrastructure for both supporting and studying learning in the Mission HydroSci project.

## 7. Summary

The Strata System is a purpose-built platform that transforms student gameplay into near-real-time assessment and research insights. Through its four core components, StrataHub, StrataLog, StrataSave, and the MHS Grader, it transforms gameplay interactions into structured data, evaluates performance automatically, and presents actionable results to educators.

Each component is described in detail in its own section of this report:

- [StrataHub](stratahub.md): authentication, game delivery, and the teacher dashboard
- [StrataLog](stratalog.md): gameplay event collection and analytics
- [StrataSave](stratasave.md): game state and settings persistence
- [MHS Grader](mhsgrader.md): automated curriculum-aligned assessment

Together, they serve as the foundation for both classroom use and the research objectives of the Mission HydroSci project.
