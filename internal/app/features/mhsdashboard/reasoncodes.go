// internal/app/features/mhsdashboard/reasoncodes.go
package mhsdashboard

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	appresources "github.com/dalemusser/stratahub/internal/app/resources"
)

// ReasonCatalog is the teacher-facing text behind flagged cells, extracted
// from the grading specification (mhsgrading/grading-logic) by mhsgrader's
// `mhsreasoncodes` command into mhs_reason_codes.json: per progress point,
// each reason code's instructor-message template (with {placeholders} that
// the grader fills from Reason.Variables) and the point's Teacher Guidance.
type ReasonCatalog struct {
	Source string                        `json:"source"`
	Points map[string]ReasonCatalogPoint `json:"points"` // "u2p1" -> ...
}

// ReasonCatalogPoint is one point's reason codes and guidance.
type ReasonCatalogPoint struct {
	Codes    []ReasonCatalogCode `json:"codes"`
	Guidance string              `json:"guidance,omitempty"`
	Note     string              `json:"note,omitempty"`
}

// ReasonCatalogCode is one reason code's message template.
type ReasonCatalogCode struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	reasonCatalog     *ReasonCatalog
	reasonCatalogOnce sync.Once
	reasonCatalogErr  error
	placeholderRe     = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)
)

// LoadReasonCatalog loads and caches mhs_reason_codes.json.
func LoadReasonCatalog() (*ReasonCatalog, error) {
	reasonCatalogOnce.Do(func() {
		data, err := appresources.FS.ReadFile("mhs_reason_codes.json")
		if err != nil {
			reasonCatalogErr = err
			return
		}
		var c ReasonCatalog
		if err := json.Unmarshal(data, &c); err != nil {
			reasonCatalogErr = err
			return
		}
		reasonCatalog = &c
	})
	return reasonCatalog, reasonCatalogErr
}

// Template returns the message template for a point's reason code.
func (c *ReasonCatalog) Template(pointID, code string) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, rc := range c.Points[pointID].Codes {
		if rc.Code == code {
			return rc.Message, true
		}
	}
	return "", false
}

// RenderReasonMessage fills a template's {placeholders} from the grader's
// variables. Numbers render without trailing zeros; unknown placeholders are
// left visible so a mismatch is noticed rather than hidden.
func RenderReasonMessage(template string, variables map[string]any) string {
	return placeholderRe.ReplaceAllStringFunc(template, func(ph string) string {
		name := ph[1 : len(ph)-1]
		v, ok := variables[name]
		if !ok {
			return ph
		}
		switch x := v.(type) {
		case nil:
			return ""
		case float64:
			if x == float64(int64(x)) {
				return fmt.Sprintf("%d", int64(x))
			}
			return fmt.Sprintf("%.2f", x)
		case float32:
			return RenderReasonMessage(ph, map[string]any{name: float64(x)})
		default:
			return fmt.Sprint(v)
		}
	})
}

// noReasonMessage is shown for a flagged point whose grade carries no
// reason the specification explains (an unreached success node with no
// other evidence, or a grade from before the September 2026 grader).
const noReasonMessage = "The game did not record enough detail to say why this task was flagged. Ask the student how it went, or look at the Debug timeline."

// reviewText builds the pop-up text for a flagged grade: one rendered
// instructor message per triggered reason code (in the spec's order), and
// the point's teacher guidance. Legacy grades that carry only a reasonCode
// fall back to the old fixed descriptions.
func reviewText(catalog *ReasonCatalog, pointID string, g *ProgressGradeItem) (messages []string, guidance string) {
	if catalog != nil {
		guidance = catalog.Points[pointID].Guidance
	}
	for _, r := range g.Reasons {
		if tmpl, ok := catalog.Template(pointID, r.Code); ok {
			messages = append(messages, RenderReasonMessage(tmpl, r.Variables))
		} else {
			messages = append(messages, "Needs review: "+r.Code)
		}
	}
	if len(messages) == 0 {
		switch {
		case g.ReasonCode == "":
			messages = append(messages, noReasonMessage)
		default:
			if tmpl, ok := catalog.Template(pointID, g.ReasonCode); ok {
				messages = append(messages, RenderReasonMessage(tmpl, g.Metrics))
			} else if msg, ok := reasonCodeToMessage[g.ReasonCode]; ok {
				messages = append(messages, msg)
			} else {
				messages = append(messages, "Needs review: "+g.ReasonCode)
			}
		}
	}
	return messages, strings.TrimSpace(guidance)
}
