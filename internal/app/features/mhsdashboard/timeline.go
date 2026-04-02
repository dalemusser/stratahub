package mhsdashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/logdata"
)

// defaultActiveGapThreshold matches the mhsgrader default: gaps longer than
// this are excluded from active duration calculations.
const defaultActiveGapThreshold = 2 * time.Minute

// buildTimeline annotates raw log events with grading rule information.
func buildTimeline(
	events []logdata.LogEntry,
	rules *GradingRulesConfig,
	loc *time.Location,
	gapThreshold time.Duration,
) []TimelineEntry {
	if gapThreshold <= 0 {
		gapThreshold = defaultActiveGapThreshold
	}
	keyIndex := rules.EventKeyIndex()
	timeline := make([]TimelineEntry, 0, len(events))

	var prevTime time.Time
	var prevDate string
	var firstTime time.Time
	var activeTotal time.Duration

	for _, e := range events {
		localTime := e.ServerTimestamp.In(loc)

		entry := TimelineEntry{
			ID:              e.ID.Hex(),
			EventType:       e.EventType,
			EventKey:        e.EventKey,
			SceneName:       e.SceneName,
			ServerTimestamp:  e.ServerTimestamp,
			TimestampStr:    localTime.Format("15:04:05"),
			Data:            e.Data,
			DataSummary:     summarizeData(e.EventType, e.Data),
			Unit:            rules.SceneToUnit[e.SceneName],
			Category:        classifyEvent(e.EventType),
		}

		// Date separator: set DateStr when date changes
		dateStr := localTime.Format("Jan 2, 2006")
		if dateStr != prevDate {
			entry.DateStr = dateStr
			prevDate = dateStr
		}

		// Annotate from grading rules
		if e.EventKey != "" {
			if annotations, ok := keyIndex[e.EventKey]; ok {
				var pointIDs []string
				var labels []string
				for _, ann := range annotations {
					pointIDs = append(pointIDs, ann.PointID)
					switch ann.Role {
					case "start":
						entry.IsStartEvent = true
						labels = append(labels, fmt.Sprintf("START %s: %s", ann.PointID, ann.ActivityName))
					case "end":
						entry.IsEndEvent = true
						labels = append(labels, fmt.Sprintf("END %s: %s", ann.PointID, ann.ActivityName))
					default:
						labels = append(labels, fmt.Sprintf("%s %s (%s)", strings.ToUpper(ann.Role), ann.PointID, ann.ActivityName))
					}
					entry.KeyRole = ann.Role
				}
				entry.PointIDs = pointIDs
				entry.Annotation = strings.Join(labels, "; ")
				if entry.IsStartEvent || entry.IsEndEvent {
					entry.Category = "waypoint"
				}
			}
		}

		// Gap from previous event
		if !prevTime.IsZero() {
			gap := e.ServerTimestamp.Sub(prevTime)
			entry.GapSeconds = gap.Seconds()
			if gap.Seconds() > 30 {
				entry.GapDisplay = formatGapDuration(gap.Seconds())
			}
			// Accumulate active time (exclude gaps > threshold)
			if gap > 0 && gap <= gapThreshold {
				activeTotal += gap
			}
		}

		// Elapsed times
		if firstTime.IsZero() {
			firstTime = e.ServerTimestamp
		}
		elapsed := e.ServerTimestamp.Sub(firstTime)
		entry.ElapsedDisplay = formatElapsed(elapsed)
		entry.ActiveElapsedDisplay = formatElapsed(activeTotal)

		prevTime = e.ServerTimestamp

		timeline = append(timeline, entry)
	}

	// Mark duplicate events
	markDuplicates(timeline)

	return timeline
}

// formatElapsed formats a duration as h:mm:ss or m:ss.
func formatElapsed(d time.Duration) string {
	totalSec := int(d.Seconds())
	if totalSec < 0 {
		totalSec = 0
	}
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// classifyEvent returns a category string for display filtering.
func classifyEvent(eventType string) string {
	switch eventType {
	case "PlayerPositionEvent":
		return "position"
	case "DialogueEvent":
		return "dialogue"
	case "questEvent":
		return "quest"
	case "argumentationEvent", "argumentationNodeEvent", "argumentationToolEvent":
		return "argumentation"
	case "Topographic Map Event", "TopographicMapEvent", "WaterChamberEvent",
		"soilMachine", "TerasGardenBox", "Soil Key Puzzle":
		return "gameplay"
	case "EndOfUnit":
		return "system"
	default:
		return "other"
	}
}

// summarizeData creates a compact string from the event data map.
func summarizeData(eventType string, data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}

	switch eventType {
	case "DialogueEvent":
		parts := make([]string, 0, 3)
		if v, ok := data["dialogueEventType"]; ok {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		if v, ok := data["conversationId"]; ok {
			parts = append(parts, fmt.Sprintf("conv:%v", v))
		}
		if v, ok := data["nodeId"]; ok {
			parts = append(parts, fmt.Sprintf("node:%v", v))
		}
		return strings.Join(parts, " ")

	case "questEvent":
		parts := make([]string, 0, 3)
		if v, ok := data["questName"]; ok {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		if v, ok := data["questEventType"]; ok {
			s := fmt.Sprintf("%v", v)
			s = strings.TrimSuffix(s, "Event")
			parts = append(parts, s)
		}
		if v, ok := data["questSuccessOrFailure"]; ok {
			parts = append(parts, fmt.Sprintf("→ %v", v))
		}
		return strings.Join(parts, " ")

	case "EndOfUnit":
		if v, ok := data["Unit"]; ok {
			return fmt.Sprintf("Unit %v", v)
		}

	case "PlayerPositionEvent":
		if pos, ok := data["position"].(map[string]interface{}); ok {
			x, _ := pos["x"]
			z, _ := pos["z"]
			return fmt.Sprintf("pos(%.1f, %.1f)", toFloat(x), toFloat(z))
		}

	case "soilMachine":
		parts := make([]string, 0, 3)
		if v, ok := data["floor"]; ok {
			parts = append(parts, fmt.Sprintf("floor:%v", v))
		}
		if v, ok := data["machine"]; ok {
			parts = append(parts, fmt.Sprintf("machine:%v", v))
		}
		if v, ok := data["row"]; ok {
			parts = append(parts, fmt.Sprintf("row:%v", v))
		}
		return strings.Join(parts, " ")

	case "WaterChamberEvent":
		parts := make([]string, 0, 2)
		if v, ok := data["floor"]; ok {
			parts = append(parts, fmt.Sprintf("floor:%v", v))
		}
		if v, ok := data["machineType"]; ok {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		return strings.Join(parts, " ")

	case "TerasGardenBox":
		parts := make([]string, 0, 3)
		if v, ok := data["actionType"]; ok {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		if v, ok := data["boxId"]; ok {
			parts = append(parts, fmt.Sprintf("box:%v", v))
		}
		if v, ok := data["soilType"]; ok {
			parts = append(parts, fmt.Sprintf("soil:%v", v))
		}
		return strings.Join(parts, " ")

	case "argumentationToolEvent":
		if v, ok := data["toolName"]; ok {
			return fmt.Sprintf("%v", v)
		}
	}

	// Generic fallback: show up to 3 key=value pairs
	parts := make([]string, 0, 3)
	i := 0
	for k, v := range data {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%v", k, v))
		i++
	}
	return strings.Join(parts, " ")
}

// markDuplicates flags events that fire multiple times within 30 seconds.
func markDuplicates(timeline []TimelineEntry) {
	// Group events by eventKey within 30-second windows
	type keyTime struct {
		key  string
		time time.Time
		idx  int
	}
	recent := make(map[string][]keyTime) // eventKey → recent occurrences

	for i := range timeline {
		ek := timeline[i].EventKey
		if ek == "" {
			continue
		}

		// Check for duplicates
		if prev, ok := recent[ek]; ok {
			for _, p := range prev {
				gap := timeline[i].ServerTimestamp.Sub(p.time)
				if gap < 30*time.Second && gap > 0 {
					timeline[i].IsAnomaly = true
					timeline[i].AnomalyNote = fmt.Sprintf("Duplicate %s (%.1fs after previous)", ek, gap.Seconds())
					// Also mark the earlier one if not already marked
					if !timeline[p.idx].IsAnomaly {
						timeline[p.idx].IsAnomaly = true
						timeline[p.idx].AnomalyNote = "First of duplicate " + ek
					}
				}
			}
		}

		recent[ek] = append(recent[ek], keyTime{key: ek, time: timeline[i].ServerTimestamp, idx: i})
	}
}

// formatGapDuration formats seconds into a human-readable gap string.
func formatGapDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s > 0 {
			return fmt.Sprintf("%dm %ds", m, s)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// toFloat converts an interface{} to float64.
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
