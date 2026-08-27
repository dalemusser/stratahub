// internal/app/features/mhsdashboard/surveys.go
//
// Surveys tab data: turns member_status documents (written by the Member
// Status API and by survey-resource launches) into fully formatted per-student
// cells, one per configured survey. See docs/member-status-api/plan.md.
package mhsdashboard

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// defaultSurveyTabTitle is used when the configuration does not name the tab.
const defaultSurveyTabTitle = "Surveys"

const (
	surveyShortDateLayout = "Jan 2"
	surveyFullTimeLayout  = "Jan 2, 2006 3:04 PM MST"
)

// surveyPresentation maps a state to how a cell shows it. The glyph carries
// the meaning so the status reads correctly in every colorblind-friendly
// theme and in grayscale; color is secondary (CSS class per state).
var surveyPresentation = map[string]struct{ Label, Glyph, Class string }{
	models.MemberStatusNotStarted: {"Not started", "○", "mhs-survey-none"},
	models.MemberStatusOpened:     {"Opened", "◔", "mhs-survey-opened"},
	models.MemberStatusStarted:    {"Started", "◐", "mhs-survey-started"},
	models.MemberStatusCompleted:  {"Completed", "✓", "mhs-survey-completed"},
}

// surveyTabTitle returns the configured tab title, or the default.
func (h *Handler) surveyTabTitle() string {
	if h.SurveyConfig != nil && strings.TrimSpace(h.SurveyConfig.TabTitle) != "" {
		return h.SurveyConfig.TabTitle
	}
	return defaultSurveyTabTitle
}

// surveyHeaders returns the Surveys tab columns in configured order, or nil
// when no surveys are configured (which hides the tab).
func (h *Handler) surveyHeaders() []SurveyHeader {
	if h.SurveyConfig == nil || len(h.SurveyConfig.Items) == 0 {
		return nil
	}
	headers := make([]SurveyHeader, 0, len(h.SurveyConfig.Items))
	for _, item := range h.SurveyConfig.Items {
		headers = append(headers, SurveyHeader{
			ID:          item.ID,
			Title:       item.Title,
			ShortName:   item.ShortName,
			Description: item.Description,
		})
	}
	return headers
}

// loadSurveyCells returns, per member (keyed by user id hex), one cell per
// configured survey in header order. Timestamps are rendered in loc (the
// group's organization time zone). A store failure degrades to "not started"
// cells rather than failing the dashboard, matching how grades are loaded.
func (h *Handler) loadSurveyCells(ctx context.Context, wsID primitive.ObjectID, members []models.User, loc *time.Location) map[string][]SurveyCell {
	if h.SurveyConfig == nil || len(h.SurveyConfig.Items) == 0 || h.MemberStatusStore == nil || len(members) == 0 {
		return nil
	}

	userIDs := make([]primitive.ObjectID, len(members))
	for i, m := range members {
		userIDs[i] = m.ID
	}
	byUser, err := h.MemberStatusStore.ListByUserIDs(ctx, wsID, userIDs)
	if err != nil {
		h.Log.Error("failed to load member survey status", zap.Error(err))
		byUser = nil
	}

	out := make(map[string][]SurveyCell, len(members))
	for _, m := range members {
		hex := m.ID.Hex()
		docs := byUser[hex]
		cells := make([]SurveyCell, 0, len(h.SurveyConfig.Items))
		for _, item := range h.SurveyConfig.Items {
			cell := buildSurveyCell(item, findSurveyDoc(docs, item), loc, m.FullName)
			cell.UserID = hex
			cells = append(cells, cell)
		}
		out[hex] = cells
	}
	return out
}

// findSurveyDoc locates a member's document for a survey: by the item id (the
// canonical key both writers use), then — for events recorded under a name
// before it was added to the configuration — by the unknown-name key of any
// of the item's names. Returns nil when the member has no status for it.
func findSurveyDoc(docs map[string]models.MemberStatus, item memberstatuscfg.Item) *models.MemberStatus {
	if docs == nil {
		return nil
	}
	if d, ok := docs[item.ID]; ok {
		return &d
	}
	names := append([]string{item.Title}, item.APINames...)
	for _, name := range names {
		if d, ok := docs[memberstatuscfg.UnknownKey(name)]; ok {
			return &d
		}
	}
	return nil
}

// buildSurveyCell formats one member's status on one survey. doc may be nil
// (not started). loc may be nil (UTC).
func buildSurveyCell(item memberstatuscfg.Item, doc *models.MemberStatus, loc *time.Location, studentName string) SurveyCell {
	if loc == nil {
		loc = time.UTC
	}
	cell := SurveyCell{
		EntityID:    item.ID,
		Title:       item.Title,
		ShortName:   item.ShortName,
		Description: item.Description,
		StudentName: studentName,
		State:       models.MemberStatusNotStarted,
	}
	if doc != nil && models.IsValidMemberStatusState(doc.State) {
		cell.State = doc.State
	}
	p := surveyPresentation[cell.State]
	cell.Label, cell.Glyph, cell.CellClass = p.Label, p.Glyph, p.Class

	if doc == nil || cell.State == models.MemberStatusNotStarted {
		cell.Tooltip = item.Title + ": not started"
		return cell
	}

	format := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.In(loc).Format(surveyFullTimeLayout)
	}
	cell.OpenedAt = format(doc.OpenedAt)
	cell.StartedAt = format(doc.StartedAt)
	cell.CompletedAt = format(doc.CompletedAt)
	cell.Source = doc.Source

	// The date in the cell is when the current (highest) state was reached.
	if ts := doc.TimestampFor(cell.State); ts != nil {
		cell.ShortDate = ts.In(loc).Format(surveyShortDateLayout)
		cell.StateAt = format(ts)
	}

	// Tooltip lists every reached state in ladder order.
	parts := make([]string, 0, 3)
	for _, state := range []string{models.MemberStatusOpened, models.MemberStatusStarted, models.MemberStatusCompleted} {
		if ts := doc.TimestampFor(state); ts != nil {
			parts = append(parts, surveyPresentation[state].Label+" "+format(ts))
		}
	}
	cell.Tooltip = item.Title + ": " + strings.Join(parts, " · ")
	return cell
}
