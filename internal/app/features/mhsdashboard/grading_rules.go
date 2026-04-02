// internal/app/features/mhsdashboard/grading_rules.go
package mhsdashboard

import (
	"encoding/json"
	"sync"

	appresources "github.com/dalemusser/stratahub/internal/app/resources"
)

// GradingRulesConfig holds the grading rules loaded from JSON.
type GradingRulesConfig struct {
	UnitStartEvents map[string]string `json:"unit_start_events"`
	SceneToUnit     map[string]string `json:"scene_to_unit"`
	Rules           []GradingRule     `json:"rules"`
}

// GradingRule defines a single grading rule with its eventKey triggers.
type GradingRule struct {
	RuleID        string              `json:"rule_id"`
	PointID       string              `json:"point_id"`
	Unit          int                 `json:"unit"`
	Point         int                 `json:"point"`
	ActivityName  string              `json:"activity_name"`
	StartKeys     []string            `json:"start_keys"`
	TriggerKeys   []string            `json:"trigger_keys"`
	GradingType   string              `json:"grading_type"`
	EvaluatedKeys map[string][]string `json:"evaluated_keys"`
}

var (
	gradingRules     *GradingRulesConfig
	gradingRulesOnce sync.Once
	gradingRulesErr  error
)

// LoadGradingRules loads and caches the grading rules configuration.
func LoadGradingRules() (*GradingRulesConfig, error) {
	gradingRulesOnce.Do(func() {
		data, err := appresources.FS.ReadFile("mhs_grading_rules.json")
		if err != nil {
			gradingRulesErr = err
			return
		}

		var cfg GradingRulesConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			gradingRulesErr = err
			return
		}

		gradingRules = &cfg
	})

	return gradingRules, gradingRulesErr
}

// EventKeyIndex builds a map from eventKey to the grading rules that reference it.
// This covers start keys, trigger keys, and all evaluated keys.
func (c *GradingRulesConfig) EventKeyIndex() map[string][]EventKeyAnnotation {
	idx := make(map[string][]EventKeyAnnotation)

	for _, r := range c.Rules {
		for _, k := range r.StartKeys {
			idx[k] = append(idx[k], EventKeyAnnotation{
				PointID:      r.PointID,
				ActivityName: r.ActivityName,
				Role:         "start",
			})
		}
		for _, k := range r.TriggerKeys {
			idx[k] = append(idx[k], EventKeyAnnotation{
				PointID:      r.PointID,
				ActivityName: r.ActivityName,
				Role:         "end",
			})
		}
		for category, keys := range r.EvaluatedKeys {
			for _, k := range keys {
				idx[k] = append(idx[k], EventKeyAnnotation{
					PointID:      r.PointID,
					ActivityName: r.ActivityName,
					Role:         category,
				})
			}
		}
	}

	return idx
}

// RuleByPointID returns the rule for a given point ID, or nil if not found.
func (c *GradingRulesConfig) RuleByPointID(pointID string) *GradingRule {
	for i := range c.Rules {
		if c.Rules[i].PointID == pointID {
			return &c.Rules[i]
		}
	}
	return nil
}

// ScenesForUnit returns all scene names that map to the given unit ID.
func (c *GradingRulesConfig) ScenesForUnit(unitID string) []string {
	var scenes []string
	for scene, unit := range c.SceneToUnit {
		if unit == unitID {
			scenes = append(scenes, scene)
		}
	}
	return scenes
}

// EventKeyAnnotation describes why an eventKey is significant.
type EventKeyAnnotation struct {
	PointID      string // e.g., "u1p3"
	ActivityName string // e.g., "Defend the Expedition"
	Role         string // "start", "end", "yellow", "positive", "negative", "success", "gate", etc.
}
