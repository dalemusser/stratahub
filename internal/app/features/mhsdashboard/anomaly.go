package mhsdashboard

import (
	"fmt"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/logdata"
)

// detectAnomaliesFromGrades detects anomalies using only grade data (no log queries).
// Used for the student list view where we need fast anomaly counts.
func detectAnomaliesFromGrades(
	grades map[string][]ProgressGradeItem,
	progressCfg *ProgressConfig,
) (pencils, empties int) {
	// Track whether we've seen any grade for later points (to detect skipped empties)
	hasLaterGrade := make(map[string]bool) // unitID → true if any point in unit has a grade
	for pointID := range grades {
		if len(grades[pointID]) > 0 {
			// Extract unit from pointID (e.g., "u3p2" → "unit3")
			if len(pointID) >= 2 {
				unitNum := pointID[1:2]
				hasLaterGrade["unit"+unitNum] = true
			}
		}
	}

	for _, u := range progressCfg.Units {
		seenGradeInUnit := false
		for i := len(u.ProgressPoints) - 1; i >= 0; i-- {
			pp := u.ProgressPoints[i]
			items, exists := grades[pp.ID]
			if exists && len(items) > 0 {
				seenGradeInUnit = true
			}
		}

		for _, pp := range u.ProgressPoints {
			items, exists := grades[pp.ID]
			if !exists || len(items) == 0 {
				// No grade — is this a meaningful empty?
				if seenGradeInUnit {
					empties++
				}
				continue
			}
			// Check latest attempt
			latest := items[len(items)-1]
			if latest.Status == "active" {
				pencils++
			}
		}
	}

	return pencils, empties
}

// detectAnomalies performs full anomaly detection using grades and log events.
func detectAnomalies(
	grades map[string][]ProgressGradeItem,
	events []logdata.LogEntry,
	rules *GradingRulesConfig,
	progressCfg *ProgressConfig,
) []DebugAnomaly {
	var anomalies []DebugAnomaly

	// Build eventKey lookup from log events
	eventKeyExists := make(map[string]bool)
	eventKeyTimes := make(map[string]time.Time) // first occurrence
	for _, e := range events {
		if e.EventKey != "" {
			eventKeyExists[e.EventKey] = true
			if _, exists := eventKeyTimes[e.EventKey]; !exists {
				eventKeyTimes[e.EventKey] = e.ServerTimestamp
			}
		}
	}

	// Check each progress point
	for _, u := range progressCfg.Units {
		seenGradeInUnit := false
		for _, pp := range u.ProgressPoints {
			if items, ok := grades[pp.ID]; ok && len(items) > 0 {
				seenGradeInUnit = true
				break
			}
		}
		// Check from end to find furthest graded point
		furthestGraded := ""
		for i := len(u.ProgressPoints) - 1; i >= 0; i-- {
			if items, ok := grades[u.ProgressPoints[i].ID]; ok && len(items) > 0 {
				furthestGraded = u.ProgressPoints[i].ID
				break
			}
		}

		for _, pp := range u.ProgressPoints {
			rule := rules.RuleByPointID(pp.ID)
			if rule == nil {
				continue
			}

			items, hasGrade := grades[pp.ID]

			if !hasGrade || len(items) == 0 {
				// No grade — check if start event exists in logs
				if seenGradeInUnit {
					startFound := false
					for _, sk := range rule.StartKeys {
						if eventKeyExists[sk] {
							startFound = true
							break
						}
					}
					if startFound {
						anomalies = append(anomalies, DebugAnomaly{
							Type:     "event_no_grade",
							Severity: "error",
							PointID:  pp.ID,
							Unit:     u.ID,
							Description: fmt.Sprintf("%s: Start event found in logs but grader has no grade. "+
								"Possible grader cursor issue or eventKey format mismatch.", pp.ShortName),
						})
					} else {
						desc := fmt.Sprintf("%s: No grade and start event not found in logs.", pp.ShortName)
						if furthestGraded != "" {
							desc += fmt.Sprintf(" Furthest graded point in %s: %s.", u.Title, furthestGraded)
						}
						anomalies = append(anomalies, DebugAnomaly{
							Type:        "empty",
							Severity:    "warning",
							PointID:     pp.ID,
							Unit:        u.ID,
							Description: desc,
						})
					}
				}
				continue
			}

			// Has grade — check for stuck active (pencil)
			latest := items[len(items)-1]
			if latest.Status == "active" {
				// Check if end event exists in logs
				endFound := false
				for _, tk := range rule.TriggerKeys {
					if eventKeyExists[tk] {
						endFound = true
						break
					}
				}
				ts := ""
				if latest.StartTime != nil {
					ts = latest.StartTime.Format("15:04:05")
				}
				desc := fmt.Sprintf("%s: Started", pp.ShortName)
				if ts != "" {
					desc += " at " + ts
				}
				if endFound {
					desc += ". End event EXISTS in logs but grade is still active — possible grader issue."
				} else {
					desc += ". End event NOT found in logs — student likely quit the activity."
				}
				anomalies = append(anomalies, DebugAnomaly{
					Type:      "pencil",
					Severity:  "error",
					PointID:   pp.ID,
					Unit:      u.ID,
					Description: desc,
					Timestamp: ts,
				})
			}
		}
	}

	// Check for duplicate EndOfUnit events
	type endOfUnitEvent struct {
		unit string
		time time.Time
	}
	var eouEvents []endOfUnitEvent
	for _, e := range events {
		if e.EventType == "EndOfUnit" {
			unit := ""
			if d, ok := e.Data["Unit"]; ok {
				unit = fmt.Sprintf("%v", d)
			}
			eouEvents = append(eouEvents, endOfUnitEvent{unit: unit, time: e.ServerTimestamp})
		}
	}
	// Group by unit and check for rapid duplicates
	eouByUnit := make(map[string][]time.Time)
	for _, eou := range eouEvents {
		eouByUnit[eou.unit] = append(eouByUnit[eou.unit], eou.time)
	}
	for unit, times := range eouByUnit {
		if len(times) <= 1 {
			continue
		}
		// Check if any two are within 60 seconds
		for i := 1; i < len(times); i++ {
			gap := times[i].Sub(times[i-1])
			if gap < 60*time.Second {
				anomalies = append(anomalies, DebugAnomaly{
					Type:     "duplicate",
					Severity: "warning",
					Unit:     "unit" + unit,
					Description: fmt.Sprintf("EndOfUnit (Unit %s): Fired %d times, %s apart. Likely game bug.",
						unit, len(times), gap.Round(time.Second)),
				})
				break // only report once per unit
			}
		}
	}

	return anomalies
}
