# MHS Grader

## 1. Purpose and Overview

The MHS Grader is the automated assessment engine for the Mission HydroSci project. It continuously polls and processes gameplay event data captured by StrataLog, evaluates student performance against curriculum-aligned criteria, and produces structured progress evaluations that power the teacher-facing MHS Dashboard in StrataHub. The progress evaluations shown on the dashboard, whether a student passed, was flagged for attention, or is actively working on a progress point, originate from the MHS Grader.

The grader evaluates 26 progress points distributed across the five units of the Mission HydroSci curriculum. Each progress point corresponds to a specific learning activity within the game, and each has its own grading rule that defines what constitutes successful performance. Some rules are simple completion checks. Others involve counting mistakes, computing weighted scores, measuring response times, or evaluating whether a student used specific reasoning strategies. This range of rule complexity reflects the diversity of skills assessed across the curriculum, from basic game orientation to constructing persuasive scientific arguments.

The MHS Grader was purpose-built because the project requires an assessment system that can process high-volume gameplay event data in near-real-time, apply domain-specific grading logic that varies by activity type, produce detailed metrics beyond simple pass/fail outcomes, and integrate tightly with the StrataLog event pipeline and the StrataHub dashboard. No existing assessment platform the project evaluated provided this combination of capabilities for educational game analytics.

## 2. What the Grader Evaluates

### Curriculum Structure

Mission HydroSci is organized into five units, each focused on a core set of hydrology and scientific argumentation concepts. Within each unit, students complete a series of activities that teach and assess specific skills. The grader maps each assessable activity to a progress point.

**Unit 1: Orientation and Scientific Argumentation Basics (4 progress points).** Students learn game controls, meet crew members, and are introduced to the structure of scientific arguments. Activities include orientation tasks, crew introductions, and identifying claims within arguments.

**Unit 2: Topographic Maps and Watersheds (7 progress points).** Students learn to read topographic maps, understand the relationship between watershed size and water flow, and practice identifying parts of scientific arguments. Activities include matching topographic maps to elevation profiles, navigating using map features, investigating watershed properties, classifying argument components, and constructing a comparison argument about watershed size.

**Unit 3: Water Flow Direction and Dissolved Materials (5 progress points).** Students learn how water flows through watersheds and how dissolved materials move with water. Activities include identifying flow direction from watershed maps, predicting pollutant spread, constructing arguments with reasoning and supporting evidence, navigating puzzle environments, and predicting nutrient distribution by planting seeds in appropriate locations.

**Unit 4: Groundwater, Soil Infiltration, and Water Table (6 progress points).** Students explore underground water systems, soil properties, and infiltration rates. Activities include water table concept puzzles, soil infiltration experiments across multiple difficulty levels, multi-component synthesis tasks, constructing persuasive scientific arguments, and matching plant water needs to soil properties.

**Unit 5: Water Cycle, Evaporation, and Condensation (4 progress points).** Students investigate evaporation rates, condensation, and the water cycle. Activities include evaporation rate puzzles, water chamber manipulation experiments, countering faulty scientific claims about the water cycle, and fixing a solar still with zero tolerance for errors.

### Progress Points

Each of the 26 progress points is identified by a unit and point number (for example, U2P3 refers to Unit 2, Progress Point 3). The grader maintains a rule for each progress point that defines:

- **Start events:** the gameplay events that indicate a student has begun the activity, at which point the grader sets the progress point status to "active"
- **Trigger events:** the gameplay events that indicate a student has completed the activity, at which point the grader evaluates the rule and produces a grade
- **Evaluation logic:** the specific criteria applied to determine whether the student passed or should be flagged for attention

## 3. Grading Outcomes

### Status Values

For each progress point, the grader assigns one of three statuses:

- **Passed:** the student completed the activity and demonstrated competency according to the grading criteria. On the dashboard, this appears as a green indicator.
- **Flagged:** the student completed the activity but their performance warrants teacher attention. The grade includes a reason code explaining what aspect of performance triggered the flag. On the dashboard, this appears as a yellow indicator.
- **Active:** the student has started the activity but has not yet reached the completion point. On the dashboard, this appears as a blue indicator.

A progress point with no status at all indicates the student has not yet begun that activity, which appears as a gray indicator on the dashboard.

### Reason Codes

When a student is flagged, the grader includes a reason code that provides specific information about why the flag was raised. Teachers and researchers can use these codes to understand the nature of a student's difficulty:

- **MISSING_SUCCESS_NODE:** the student did not reach a required success checkpoint within the activity, indicating they may not have completed a key step independently
- **TOO_MANY_NEGATIVES:** the student made more errors than the threshold allows, indicating repeated difficulty with the core concept
- **WRONG_ARG_SELECTED:** the student made too many incorrect selections when identifying argument components, indicating confusion about argument structure
- **BAD_FEEDBACK:** the student received negative feedback from the game more times than the threshold allows, indicating persistent difficulty navigating or solving the activity
- **SCORE_BELOW_THRESHOLD:** the student's computed score did not meet the minimum passing value, based on a weighted combination of correct and incorrect actions
- **HIT_YELLOW_NODE:** the student triggered a specific incorrect choice marker on their first attempt, indicating a fundamental misconception about the concept being assessed

### Metrics

Beyond the pass or flag status, each grade includes a set of metrics that capture quantitative details about the student's performance. These metrics vary by progress point but commonly include:

- **Mistake count:** the number of errors the student made during the activity
- **Score:** a computed value based on weighted correct and incorrect actions
- **Duration:** total elapsed time from the start of the activity to completion
- **Active duration:** elapsed time with extended idle periods excluded, providing a more accurate measure of time on task
- **Attempt counts:** for multi-part activities, the number of attempts at each component
- **Specific indicators:** whether the student selected the correct answer, whether they used supporting evidence in an argument, and similar binary outcome measures

These metrics give teachers and researchers a detailed picture of student performance that goes beyond a simple pass or fail determination.

## 4. How the Grader Works

### Architecture

The MHS Grader runs as a standalone background service, separate from StrataHub and StrataLog. It connects to the same database cluster but operates independently, reading gameplay events from the StrataLog database and writing grades to its own database. This separation ensures that the grader can be updated, restarted, or scaled without affecting the other components of the Strata system.

The grader has no web interface or HTTP server. It is a continuous processing engine that starts, connects to its data sources, and runs until stopped.

### Scan and Evaluate Cycle

The grader operates on a continuous polling cycle:

1. **Scan for new events.** The grader maintains a cursor that tracks the last event it processed. On each cycle, it queries StrataLog for gameplay events that match any of its registered trigger or start event keys, beginning from the cursor position. Events are processed in the order they were received.

2. **Handle unit starts.** Certain events indicate that a student has entered a new unit. When the grader encounters one of these events, it updates its record of the most recently detected unit for that student. This information is stored alongside the student's progress evaluations and displayed on the dashboard.

3. **Set active status.** When an event matches the start keys for a progress point, the grader creates or updates a grade record with status "active" for that student and progress point. This signals on the dashboard that the student has begun the activity.

4. **Evaluate on trigger.** When an event matches the trigger keys for a progress point, the grader runs the evaluation rule for that progress point. The rule examines the student's gameplay events within the relevant time window and produces a grade with a status, an optional reason code, and metrics.

5. **Advance the cursor.** After processing a batch of events, the grader updates its cursor so that the next scan starts from the correct position. This ensures no events are processed twice and no events are skipped.

### Attempt Windows

A key design feature of the grader is its use of attempt windows to isolate the events relevant to each evaluation. When a student triggers evaluation for a progress point, the grader looks back through the event history to find the boundaries of the current attempt:

- The end boundary is the trigger event itself.
- The start boundary is the previous trigger event for the same progress point, or the beginning of the student's event history if no previous trigger exists.

Only events within this window are considered during evaluation. This design makes the grader replay-safe: if a student replays an activity, only the most recent attempt is evaluated, and prior attempts do not interfere with the current grade.

### Idempotent Processing

The cursor-based scanning system ensures that the grader produces consistent results regardless of when it runs or how often it restarts. If the grader is stopped and restarted, it resumes from its last recorded cursor position and processes only events it has not yet seen. This property is essential for a production assessment system where reliability and consistency are critical.

## 5. Grading Rule Types

The 26 grading rules span a range of complexity, reflecting the variety of skills assessed across the curriculum. The following categories describe the major rule types used.

### Completion Rules

The simplest rules check only whether a student reached the completion point of an activity. If the trigger event fires, the student passes. These rules are used for orientation and narrative activities where the learning objective is exposure to content rather than demonstration of a specific skill. Unit 1 uses completion rules for its introductory activities.

### Threshold Rules

Threshold rules count specific types of events within the attempt window and compare the count against a limit. For example, a rule might flag a student if they received more than six instances of negative feedback during a navigation task, or if they made any errors at all in a zero-tolerance activity. The threshold and the type of event counted vary by progress point.

### Scoring Rules

Scoring rules compute a numerical score based on weighted positive and negative events. A correct action might add one point while an incorrect action might subtract a fraction of a point, with a minimum passing score defined for the rule. Some scoring rules use capped penalties, where each category of error can reduce the score by at most a fixed amount, preventing a single type of repeated mistake from overwhelming the overall assessment. Others use bonus systems that award additional credit for using advanced strategies such as including supporting evidence in an argument.

### Time-Based Rules

Some rules incorporate response time as a factor in the grade. For example, an activity might award a duration bonus if the student answered within 30 seconds, a partial bonus for answers within 90 seconds, and no bonus for longer response times. The time component is combined with accuracy to produce the final score. Active duration calculations exclude extended idle periods (gaps longer than two minutes by default) to avoid penalizing students for interruptions.

### Multi-Component Rules

Complex activities that span multiple game elements use multi-component rules that aggregate results across several sub-activities. Each component contributes to the overall score independently. For example, a rule might award points for completing a top-row puzzle, additional points for a machine manipulation task, and further points for successfully navigating a dialogue sequence. The component scores are summed and compared against an overall threshold.

### Zero-Tolerance Rules

The strictest rule type requires perfect performance. If any negative event occurs during the attempt, the student is flagged. This rule type is used sparingly, only for activities where the learning objective demands precise execution and any error represents a meaningful misunderstanding of the concept.

## 6. Integration with the Dashboard

The grades produced by the MHS Grader are stored in a database that the MHS Dashboard reads when rendering the progress grid for a teacher or researcher.

### Grade Storage

Each student's grades are stored as a single document containing all 26 progress points. For each progress point, the document records the status (passed, flagged, or active), the reason code if flagged, the computed metrics, the timestamps of the start and end events, the duration measurements, and the grading rule version that produced the grade. If a student has multiple attempts at a progress point, each attempt is recorded, providing a complete history of the student's performance trajectory.

### Dashboard Display

When a teacher opens the MHS Dashboard, it queries the grade database for all students in the selected group. The dashboard renders a grid with students as rows and progress points as columns. Each cell displays the color-coded status: green for passed, yellow for flagged, blue for active, and gray for not started. Teachers can examine individual grades to see the reason code and metrics, giving them specific, actionable information about each student's performance.

### Current Unit Tracking

The grader also records the most recently detected unit for each student, based on the most recent unit-start event encountered. This information appears on the dashboard, allowing teachers to see at a glance which unit each student is working on, even between graded activities.

## 7. Data Flow Pipeline

The complete data flow from student gameplay to teacher-visible grades follows this pipeline:

1. A student plays Mission HydroSci in their browser, launched through StrataHub.
2. The game sends gameplay events to StrataLog as the student interacts with activities, dialogues, simulations, and puzzles.
3. The MHS Grader, running as a background service, continuously polls StrataLog for events that match its registered trigger and start keys.
4. When a start event is detected, the grader sets the corresponding progress point to "active" for that student.
5. When a trigger event is detected, the grader retrieves the relevant events from the attempt window, applies the grading rule, and stores the resulting grade with its status, reason code, and metrics.
6. The MHS Dashboard in StrataHub queries the grade database and displays the color-coded progress grid to the teacher.

This pipeline operates continuously and automatically. No manual intervention is required to grade student work or update the dashboard. Teachers see near-real-time progress evaluations as students play.

## 8. Reliability and Operational Design

### Cursor-Based Processing

The grader maintains a persistent cursor (the ID of the last processed event) in its database. This cursor survives service restarts, ensuring that no events are skipped or reprocessed after a restart. The cursor-based approach also means the grader can be stopped for maintenance and resumed without any data loss or inconsistency.

### Configurable Scanning

The polling interval, batch size, and database connections are configurable through environment variables. The default polling interval is five seconds, meaning that new grades typically appear on the dashboard within seconds of a student completing an activity. The batch size controls how many events are processed per cycle, allowing the system to handle bursts of activity without falling behind.

### Active Duration Calculation

The grader's active duration metric excludes idle gaps longer than a configurable threshold (two minutes by default). This ensures that time-on-task measurements accurately reflect engaged work time rather than clock time, which is important for research analysis and for rules that incorporate timing into their scoring.

### Rule Versioning

Each grading rule carries a version identifier that is recorded with every grade it produces. If a rule is updated to reflect a curriculum change or a refinement in grading criteria, the version identifier changes, allowing researchers to distinguish grades produced under different rule versions when analyzing data.

## 9. Relationship to Other Strata Components

The MHS Grader operates as part of the broader Strata system, with specific relationships to each other component:

**StrataLog** is the grader's primary data source. Every event the grader processes originates from StrataLog. The grader reads from the StrataLog database but never writes to it, maintaining a clean separation between event collection and event analysis.

**StrataHub** hosts the MHS Dashboard, which is the primary consumer of the grades the grader produces. StrataHub also manages the student accounts and classroom groups that give structure to the grade data. The grader does not communicate with StrataHub directly; they share data through the grade database.

**StrataSave** provides game state persistence so students can resume where they left off. While the grader does not interact with StrataSave, the continuity StrataSave provides is valuable for grading: because students resume at their last save point rather than replaying content, their event streams in StrataLog are clean and sequential, making the grader's attempt window logic reliable.

Together, these components form a pipeline in which StrataLog collects the raw data, the MHS Grader transforms it into structured assessments, and the MHS Dashboard presents those assessments to teachers and researchers.

## 10. Why a Custom Grading Engine Was Necessary

Several categories of existing assessment tools were considered before building the MHS Grader:

**Learning management system (LMS) gradebooks** such as those in Canvas, Blackboard, or Google Classroom are designed for manually entered grades, quiz scores, and assignment submissions. They cannot process near-real-time gameplay event streams, apply complex scoring rules with attempt windows, or produce the detailed metrics required for research-grade assessment of game-based learning activities.

**Game analytics platforms** such as GameAnalytics or Unity Analytics provide aggregate metrics about player behavior (session length, retention, feature usage) but are not designed to evaluate individual student performance against curriculum-specific criteria. They do not support custom grading rules or integration with educational dashboards.

**Standardized assessment platforms** such as those used for state testing are designed for fixed-form assessments with predetermined questions and answer keys. They cannot assess the open-ended, process-oriented activities in Mission HydroSci, where the evaluation depends on the sequence, timing, and pattern of student actions across an extended gameplay session.

**Rule engine frameworks** such as Drools or business rule management systems provide generic rule evaluation capabilities. However, they are designed for business process automation, not for processing ordered gameplay event streams with attempt windows, cursor-based scanning, and duration calculations. Adapting a generic rule engine to this domain would require extensive customization while introducing unnecessary complexity and dependencies.

The MHS Grader was built because the project needed an assessment engine that combines near-real-time event stream processing, curriculum-specific grading rules with configurable thresholds, attempt window isolation for replay safety, detailed performance metrics beyond pass/fail, cursor-based idempotent processing, and tight integration with the StrataLog event pipeline and the StrataHub dashboard. No existing tool provides this combination.

## 11. Summary of Capabilities

The MHS Grader provides the following capabilities in support of the Mission HydroSci research project:

- **Automated assessment:** 26 progress points evaluated continuously without manual intervention
- **Curriculum alignment:** each progress point maps to a specific learning activity and assessed skill within the five-unit curriculum
- **Three-status grading:** passed, flagged, and active statuses provide teachers with clear, actionable information
- **Reason codes:** flagged grades include specific codes explaining the nature of the performance concern
- **Detailed metrics:** mistake counts, scores, durations, attempt counts, and specific indicators recorded for each grade
- **Diverse rule types:** completion checks, threshold rules, weighted scoring, time-based scoring, multi-component evaluation, and zero-tolerance rules
- **Attempt window isolation:** only the most recent attempt is evaluated, making the grader safe for students who replay activities
- **Cursor-based processing:** persistent cursor ensures no events are skipped or duplicated across restarts
- **Near-real-time grading:** default five-second polling interval delivers grades to the dashboard within seconds of activity completion
- **Active duration tracking:** idle periods excluded from time measurements for accurate time-on-task analysis
- **Current unit tracking:** records which unit each student is working on for dashboard display
- **Rule versioning:** each grade records the rule version that produced it, supporting longitudinal research analysis
- **Standalone operation:** runs independently of StrataHub and StrataLog, connecting only through shared databases
- **Integration with the Strata system:** purpose-built to read from StrataLog and produce grades for the MHS Dashboard as part of the unified assessment pipeline
