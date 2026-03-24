# Mission HydroSci: Curriculum Context and Learning Objectives

> **Status**: Draft — extrapolated from game mechanics and grading rules.
> Send to curriculum team for validation and corrections.

This document maps each progress point to its learning objectives, assessed skills, and what flagged results indicate about student understanding. It is used as context when generating AI-powered student performance summaries.

---

## Game Overview

Mission HydroSci is a science adventure game where students play as cadets on an alien planet. Through narrative-driven missions, students learn hydrology and Earth science concepts while developing scientific argumentation skills. The game spans 5 units with 26 progress points, progressing from basic orientation through watersheds, water flow, soil infiltration, and the water cycle.

---

## Unit 1: Orientation and Scientific Argumentation Basics

**Theme**: Introduction to the game world, controls, and the fundamentals of scientific argumentation.

### U1P1 — Getting Your Space Legs
- **Learning Objective**: Familiarize with game navigation and controls.
- **Assessed Skills**: None (completion-based). This is an onboarding activity.
- **What Flagged Means**: N/A — always passes on completion.
- **Duration Insight**: Very long durations may indicate difficulty with game controls or navigation.

### U1P2 — Info and Intros
- **Learning Objective**: Meet crew members and begin understanding what constitutes a scientific argument (claim, evidence, reasoning).
- **Assessed Skills**: None (completion-based). Students collect argument components through dialogue.
- **What Flagged Means**: N/A — always passes on completion.
- **Duration Insight**: Extended time may indicate the student is carefully reading dialogue or having difficulty following the narrative.

### U1P3 — Defend the Expedition
- **Learning Objective**: Use the argumentation engine to identify the correct claim in a scientific argument.
- **Assessed Skills**: Distinguishing valid claims from invalid ones; basic argumentation structure.
- **What Flagged Means**: `WRONG_ARG_SELECTED` — The student selected an incorrect argument component. This suggests difficulty distinguishing between claims, evidence, and reasoning, or trouble identifying which claim is supported by the available evidence.
- **Metrics**: `mistakeCount` — number of incorrect argument selections.

### U1P4 — What Was That?
- **Learning Objective**: Complete a narrative transition involving first contact with ancient aliens.
- **Assessed Skills**: None (completion-based). This is a story-driven transition.
- **What Flagged Means**: N/A — always passes on completion.

---

## Unit 2: Topographic Maps and Watersheds

**Theme**: Reading topographic maps, understanding elevation, watersheds, and constructing scientific arguments about watershed characteristics.

### U2P1 — Escape the Ruin
- **Learning Objective**: Match topographic maps to elevation profiles; understand how contour lines represent terrain.
- **Assessed Skills**: Reading topographic maps; translating 2D contour maps into 3D elevation understanding; spatial reasoning.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student did not successfully match the maps, indicating fundamental difficulty reading topographic contour lines.
  - `TOO_MANY_NEGATIVES` — The student made errors during the matching process, suggesting partial but inconsistent understanding of contour line interpretation.
- **Metrics**: `mistakeCount` — number of incorrect map-matching attempts.

### U2P2 — Foraged Forging
- **Learning Objective**: Read a topographic map to locate a specific position (Toppo's location).
- **Assessed Skills**: Interpreting contour lines to identify specific terrain features; applying topographic map reading to navigation.
- **What Flagged Means**: `BAD_FEEDBACK` — The student made multiple incorrect location identifications, suggesting difficulty translating contour patterns into real terrain understanding.
- **Metrics**: `mistakeCount` — number of incorrect location attempts.

### U2P3 — Getting the Band Back Together Part II
- **Learning Objective**: Read topographic maps to locate two characters (Tera and Aryn) by interpreting terrain features.
- **Assessed Skills**: Applying topographic map skills independently; identifying multiple locations from contour patterns; understanding directional movement on a map.
- **What Flagged Means**: `BAD_FEEDBACK` — More than 6 wrong-direction choices, indicating the student struggles to determine correct paths on a topographic map. This may reflect difficulty with elevation interpretation or spatial orientation.
- **Metrics**: `mistakeCount` — number of wrong-direction prompts (threshold: 6).

### U2P4 — Investigate the Temple
- **Learning Objective**: Understand the relationship between watershed size and flow rate through its main river.
- **Assessed Skills**: Connecting watershed area to water flow volume; understanding that larger watersheds collect more precipitation and produce higher flow rates.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student did not demonstrate understanding of the watershed-flow relationship.
  - `TOO_MANY_NEGATIVES` — Incorrect responses suggest confusion about how watershed characteristics affect river flow.
- **Metrics**: `mistakeCount` — number of incorrect responses.

### U2P5 — Classified Information
- **Learning Objective**: Identify the parts of a scientific argument (claim, evidence, reasoning) and correctly classify them.
- **Assessed Skills**: Scientific argumentation literacy; distinguishing between argument components; understanding how each part functions in supporting a conclusion.
- **What Flagged Means**: `TOO_MANY_NEGATIVES` — The weighted score fell below threshold, indicating the student frequently misclassified argument components. This suggests a need for more practice distinguishing claims from evidence and reasoning.
- **Scoring**: Weighted formula: `score = positiveSelections - (negativeSelections / 3)`. Threshold: 4.0. The penalty divisor allows some mistakes while ensuring overall competency.
- **Metrics**: `posCount`, `mistakeCount`, `score`.

### U2P6 — Which Watershed? Part I
- **Learning Objective**: Collect and identify relevant evidence for constructing an argument about watershed size.
- **Assessed Skills**: Evidence identification; understanding what constitutes relevant data for a watershed comparison; distinguishing useful evidence from irrelevant information.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student did not collect the necessary evidence, suggesting difficulty identifying what data is relevant to a watershed size argument.
  - `HIT_YELLOW_NODE` — The student selected incorrect or irrelevant evidence, indicating confusion about what supports a watershed comparison claim.
- **Metrics**: `mistakeCount` — number of incorrect evidence selections.

### U2P7 — Which Watershed? Part II
- **Learning Objective**: Construct a complete argument about which watershed is bigger by linking a claim with supporting evidence.
- **Assessed Skills**: Argument construction; claim-evidence alignment; synthesizing collected data into a coherent scientific argument.
- **What Flagged Means**: `WRONG_ARG_SELECTED` — The student made too many incorrect argument component selections (>3), indicating difficulty connecting claims with appropriate evidence or constructing a logically consistent argument.
- **Metrics**: `mistakeCount` — number of incorrect argument selections (threshold: 3).

---

## Unit 3: Water Flow Direction and Dissolved Materials

**Theme**: Understanding water flow direction in watersheds, how dissolved materials (pollutants and nutrients) spread through waterways, and constructing evidence-based arguments.

### U3P1 — Supply Run
- **Learning Objective**: Identify the direction of water flow based on a map of the watershed.
- **Assessed Skills**: Reading watershed maps to determine flow direction; understanding that water flows from higher to lower elevation; applying this knowledge to predict where objects carried by water will go.
- **What Flagged Means**: `TOO_MANY_NEGATIVES` — The student had difficulty sending crates in the correct direction, suggesting they cannot reliably determine water flow direction from a watershed map.
- **Scoring**: Pass if count > 1 (student needs at least 2 successful sends).
- **Metrics**: `count` (successful sends), `mistakeCount`.

### U3P2 — Pollution Solution Part I
- **Learning Objective**: Predict the spread of dissolved materials (pollutants) through a watershed to trace a pollutant back to its source.
- **Assessed Skills**: Understanding how dissolved substances move with water flow; tracing contaminant paths upstream; predicting which waterways could carry a pollutant to a given location.
- **What Flagged Means**: `BAD_FEEDBACK` — The capped-penalty score fell below 3, indicating the student made significant errors predicting pollutant movement. This suggests difficulty understanding how water carries dissolved materials through connected waterways.
- **Scoring**: Capped penalty system across 3 event types. Each category: 0 penalty if ≤1 error, 1 if ≤3, 2 if ≥4. Score = 5 - penalties. Threshold: 3.
- **Metrics**: `c27`, `c29`, `c230`, `score`, `mistakeCount`.

### U3P3 — Pollution Solution Part II
- **Learning Objective**: Construct a scientific argument about the location of a pollutant source, including reasoning that links claim with evidence.
- **Assessed Skills**: Full scientific argumentation (claim + evidence + reasoning); using watershed knowledge as reasoning to connect evidence to a claim; using backing information to strengthen arguments.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — Did not use backing information. The student may have constructed an argument but failed to support it with additional evidence, indicating incomplete argumentation skills.
  - `WRONG_ARG_SELECTED` — Despite using backing info, the score was too low. The student made too many incorrect argument selections, suggesting difficulty constructing coherent scientific arguments even with available resources.
- **Scoring**: Base score (3→0 based on mistakes) + 1 bonus for using backing info. Threshold: 3.
- **Metrics**: `baseScore`, `usedBackingInfo`, `totalScore`, `mistakeCount`.

### U3P4 — Forsaken Facility
- **Learning Objective**: Demonstrate understanding of how materials move through waterways by solving puzzles in alien ruins.
- **Assessed Skills**: Applying knowledge of water flow and material transport in practical problem-solving contexts; predicting outcomes of material movement through connected water systems.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student did not reach the required gate checkpoint, indicating they could not solve the fundamental puzzle about material movement.
  - `TOO_MANY_NEGATIVES` — Too many incorrect attempts (>2), suggesting trial-and-error rather than applying understanding of how materials move through waterways.
- **Scoring**: Gate requirement (must reach checkpoint) + score based on mistakes (2 if 0, 1 if ≤2, 0 if ≥3). Threshold: pass if ≤2 mistakes.
- **Metrics**: `score`, `mistakeCount`.

### U3P5 — Part of a Balanced Ecosystem
- **Learning Objective**: Predict the spread of a dissolved nutrient through a watershed to plant seeds in ideal locations.
- **Assessed Skills**: Understanding nutrient transport in water systems; predicting where dissolved substances will concentrate; applying watershed knowledge to ecological/agricultural decisions.
- **What Flagged Means**: `TOO_MANY_NEGATIVES` — The weighted score fell below 3.0, indicating the student placed seeds in incorrect locations. This suggests difficulty predicting where nutrients carried by water will be available for plant growth.
- **Scoring**: `score = positiveCount - (negativeCount * 0.5)`. Threshold: 3.0. Negative penalties are weighted at 0.5 (less punitive than U2P5).
- **Metrics**: `posCount`, `score`, `mistakeCount`.

---

## Unit 4: Groundwater, Soil Infiltration, and the Water Table

**Theme**: Understanding groundwater, the water table, soil types and their infiltration rates, and how these properties affect water availability. Continued development of scientific argumentation skills.

### U4P1 — Well What Have We Here?
- **Learning Objective**: Understand the concept of the water table and how to locate groundwater in a desert environment.
- **Assessed Skills**: Understanding that water exists underground (water table); knowing that wells access groundwater; understanding the relationship between soil type and water accessibility.
- **What Flagged Means**: `SCORE_BELOW_THRESHOLD` — The combined score from correct answer choice and puzzle speed was below 1.0, suggesting the student may not understand the water table concept or struggled significantly with the soil key puzzle.
- **Scoring**: +0.5 for correct dialogue choice, +1.0 if puzzle ≤30s, +0.5 if 30-90s. Threshold: 1.0.
- **Metrics**: `hasCorrectAnswer`, `puzzleDurationSecs`, `durationBonus`, `score`, `mistakeCount`.
- **Duration Insight**: Puzzle duration reflects how quickly the student can apply soil type knowledge. Fast completion (<30s) suggests strong understanding; slow completion (>90s) suggests the student is still developing this knowledge.

### U4P2 — Power Play: Floors 1 & 2
- **Learning Objective**: Understand the relationship between infiltration rate and soil type.
- **Assessed Skills**: Knowing that different soil types (sand, gravel, clay) have different infiltration rates; predicting how water moves through different soils.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student did not demonstrate understanding of infiltration-soil relationships.
  - `TOO_MANY_NEGATIVES` — Incorrect responses to soil infiltration questions suggest confusion about which soil types allow faster or slower water movement.
- **Metrics**: `mistakeCount` — number of incorrect responses.

### U4P3 — Power Play: Floors 3 & 4
- **Learning Objective**: Manipulate soil type variables to achieve target infiltration rates, demonstrating deeper understanding of the infiltration-soil relationship.
- **Assessed Skills**: Applying infiltration rate knowledge in a hands-on context; selecting appropriate soil types to achieve desired water flow outcomes; problem-solving with soil/water variables.
- **What Flagged Means**: `SCORE_BELOW_THRESHOLD` — Multiple attempts on the soil machines indicate the student needed trial-and-error rather than applying understanding. This suggests the relationship between soil type and infiltration rate is not yet firmly understood.
- **Scoring**: +1 if floor 3 solved in 1 attempt, +2 if floor 4 solved in 1 attempt, +1 if floor 4 solved in 2 attempts. Threshold: >1.
- **Metrics**: `floor3Attempts`, `floor4Attempts`, `score`, `mistakeCount`.

### U4P4 — Power Play: Floor 5 + You Know the Drill
- **Learning Objective**: Synthesize soil infiltration knowledge across multiple machine configurations and drill a well to access the water table.
- **Assessed Skills**: Advanced soil machine manipulation; understanding multiple soil layer configurations; connecting soil infiltration knowledge to practical well-drilling decisions.
- **What Flagged Means**: `SCORE_BELOW_THRESHOLD` — The student needed multiple attempts across the floor 5 machines and/or made errors in the well-drilling activity. This indicates incomplete synthesis of soil infiltration concepts.
- **Scoring**: Multi-component: +1 for top row machine (1 attempt + no bottom row errors), +1 for machine 2 (1 attempt), +2 for dialogue success with 0 negatives or +1 with 1 negative. Threshold: >2.
- **Metrics**: `topRowAttempts`, `bottomRowAttempts`, `machine2Attempts`, `successCount`, `score`, `mistakeCount`.

### U4P5 — Saving Cadet Anderson
- **Learning Objective**: Construct a complete scientific argument using evidence and reasoning to convince a character to take action.
- **Assessed Skills**: Persuasive scientific argumentation; selecting appropriate evidence and reasoning; constructing a complete argument (claim + evidence + reasoning) under narrative pressure.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student could not construct a convincing argument, indicating significant difficulty with argument structure.
  - `TOO_MANY_NEGATIVES` — More than 3 incorrect selections suggest the student struggles to identify which components make a persuasive scientific argument.
- **Metrics**: `mistakeCount` — number of incorrect argument selections (threshold: 3).

### U4P6 — Desert Delicacies
- **Learning Objective**: Demonstrate understanding of soil infiltration rates and water holding capacity by selecting the ideal soil type for different plants.
- **Assessed Skills**: Matching plant water needs to soil properties; understanding that gravel drains quickly (low holding capacity), sand has moderate drainage, and clay retains water (high holding capacity); applying soil science knowledge to agricultural decisions.
- **What Flagged Means**: `SCORE_BELOW_THRESHOLD` — The student placed fewer than 2 of 3 boxes on the correct soil type, indicating they cannot reliably match soil properties to plant needs.
- **Scoring**: 3 boxes, each requiring a specific soil type (Box 0=Gravel, Box 1=Sand, Box 2=Clay). +1 per correct placement. Threshold: ≥2.
- **Metrics**: `score`, `mistakeCount`, `box0SoilType`, `box0Correct`, `box1SoilType`, `box1Correct`, `box2SoilType`, `box2Correct`.

---

## Unit 5: The Water Cycle — Evaporation and Condensation

**Theme**: Understanding evaporation, condensation, and the water cycle. Constructing counter-arguments and applying water cycle knowledge to practical problems.

### U5P1 — If I Had a Nickel: Floors 1 & 2
- **Learning Objective**: Apply knowledge of evaporation rate and the water cycle to solve puzzles.
- **Assessed Skills**: Understanding factors that affect evaporation rate (temperature, surface area, humidity); basic water cycle knowledge; applying these concepts to predict outcomes.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student could not solve the puzzle, suggesting fundamental gaps in understanding evaporation and the water cycle.
  - `TOO_MANY_NEGATIVES` — More than 2 errors indicate the student is guessing rather than applying water cycle knowledge.
- **Metrics**: `mistakeCount` — number of incorrect attempts (threshold: 2).

### U5P2 — If I Had a Nickel: Floors 3 & 4
- **Learning Objective**: Demonstrate understanding of evaporation and condensation through hands-on puzzle solving with water chamber machines.
- **Assessed Skills**: Manipulating evaporation and condensation variables; understanding the relationship between temperature and phase changes; predicting how changes to one part of a water system affect other parts.
- **What Flagged Means**: `SCORE_BELOW_THRESHOLD` — High attempt counts on the water chamber machines (condenser and evaporator) indicate the student is not yet able to predict how manipulating evaporation/condensation variables will affect outcomes.
- **Scoring**: Floor 3: +2 if ≤6 attempts, +1 if <11. Floor 4: +2 if ≤5 attempts, +1 if <10. Threshold: ≥3.
- **Metrics**: `floor3Attempts`, `floor4Attempts`, `score`, `mistakeCount`.

### U5P3 — What Happened Here?
- **Learning Objective**: Evaluate and counter a faulty scientific claim using knowledge about the water cycle.
- **Assessed Skills**: Critical evaluation of scientific claims; constructing counter-arguments; identifying flaws in reasoning about the water cycle; distinguishing between correlation and causation in water cycle phenomena.
- **What Flagged Means**: `TOO_MANY_NEGATIVES` — 4 or more incorrect responses when trying to counter Aryn's claim. This suggests the student has difficulty identifying what makes a scientific claim faulty or cannot articulate why the claim is wrong using water cycle knowledge.
- **Metrics**: `mistakeCount` — number of incorrect counter-argument attempts (threshold: 3).

### U5P4 — Water Problems Require Water Solutions
- **Learning Objective**: Apply knowledge of evaporation and condensation to fix a solar still — a practical water purification device.
- **Assessed Skills**: Understanding how solar stills work (evaporation of contaminated water, condensation of pure water); applying water cycle knowledge to real-world technology; troubleshooting a physical system using scientific principles.
- **What Flagged Means**:
  - `MISSING_SUCCESS_NODE` — The student could not fix the solar still, indicating they do not understand how evaporation and condensation work together in a practical device.
  - `TOO_MANY_NEGATIVES` — Zero tolerance for errors. Any incorrect attempt indicates incomplete understanding of how the solar still's components relate to evaporation and condensation processes.
- **Metrics**: `mistakeCount` — number of incorrect attempts (threshold: 0 — strictest rule in the game).
- **Note**: This is the strictest rule, requiring zero mistakes. It serves as a capstone assessment for water cycle understanding.

---

## Cross-Cutting Skills

### Scientific Argumentation (assessed in U1P3, U2P5, U2P6, U2P7, U3P3, U4P5, U5P3)
Students develop argumentation skills progressively:
1. **U1P3**: Identify a claim (basic recognition)
2. **U2P5**: Classify argument parts (claim, evidence, reasoning)
3. **U2P6–U2P7**: Collect evidence and construct a complete argument
4. **U3P3**: Include reasoning and backing information in arguments
5. **U4P5**: Construct a persuasive argument under pressure
6. **U5P3**: Evaluate and counter a faulty claim (critical thinking)

### Hydrology Concepts (assessed across Units 2–5)
1. **Topographic maps** (U2P1–U2P3): Reading contour lines, spatial orientation
2. **Watersheds** (U2P4, U2P6–U2P7): Watershed size and flow rate relationship
3. **Water flow direction** (U3P1, U3P4): Predicting flow from elevation data
4. **Dissolved material transport** (U3P2, U3P5): Pollutant tracing, nutrient prediction
5. **Groundwater and water table** (U4P1, U4P4): Underground water, well drilling
6. **Soil infiltration** (U4P2–U4P4, U4P6): Soil type → infiltration rate → holding capacity
7. **Water cycle** (U5P1–U5P4): Evaporation, condensation, and practical applications

---

## Interpreting Student Performance Data

### Attempt Count
- **1 attempt**: Student demonstrated understanding on first try.
- **2 attempts**: Student needed one retry — may have had a minor misconception that self-corrected.
- **3+ attempts**: Student is struggling with the concept and may benefit from targeted instruction.

### Duration and Active Duration
- **Duration**: Wall-clock time from activity start to completion. Includes all pauses, distractions, and idle time.
- **Active Duration**: Time spent actively engaged (gaps >5 minutes excluded). Better reflects actual time-on-task.
- **Very short active durations** with passing grades may indicate the student understood the concept quickly.
- **Long active durations** with flagged results suggest the student spent significant effort but could not overcome their misconception.
- **Large gap between duration and active duration** suggests the student took breaks, was multitasking, or left the game idle.

### Mistake Count
- Normalized differently per rule. Compare within a progress point, not across them.
- A mistake count of 0 with a pass indicates mastery.
- A high mistake count even with a pass (e.g., U2P3 allows up to 6 mistakes) indicates the student eventually succeeded but needed many attempts.

### Reason Codes
| Code | Meaning |
|------|---------|
| `TOO_MANY_NEGATIVES` | Student made too many errors during the activity |
| `MISSING_SUCCESS_NODE` | Student did not reach the required success checkpoint |
| `WRONG_ARG_SELECTED` | Student chose incorrect argument components |
| `BAD_FEEDBACK` | Student received negative feedback for incorrect choices |
| `SCORE_BELOW_THRESHOLD` | Student's computed score did not meet the minimum |
| `HIT_YELLOW_NODE` | Student triggered a specific incorrect choice marker |

---

## Notes for Curriculum Team

This document was generated by analyzing the game's automated grading rules. The learning objectives and skill descriptions are *inferred* from the game mechanics and may not precisely match the intended curriculum goals. Please review and correct:

1. **Learning objectives**: Are these accurate descriptions of what each activity is designed to teach?
2. **Assessed skills**: Do these capture the key competencies being evaluated?
3. **Flagged interpretations**: Are these fair descriptions of what mistakes indicate about student understanding?
4. **Cross-cutting progressions**: Does the argumentation progression accurately reflect the curriculum design?
5. **Missing context**: Are there pedagogical goals not captured by the grading rules (e.g., collaboration, reflection, real-world connections)?
