// internal/app/features/missionhydrosci/ceremony.go
//
// The end-of-game ceremony (docs/mission-hydrosci/mhs-end-ceremony-plan.md):
// a host page that mounts the ceremony bundle from the resolved collection's
// ceremony version, the student's EA checkpoint scores for it, and the
// viewed marks on their progress. The bundle's contract is
// mhs-gameplay-end/docs/embed-api.md.
package missionhydrosci

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const (
	// eaFinalPointID is the last graded progress point of the game. A finished
	// grade for it means the grader has processed the end of Unit 5, so the
	// scores are as complete as they will get.
	eaFinalPointID = "u5p4"

	// eaPendingWindow bounds how long after finishing Unit 5 the scores stay
	// "pending" while the final grade is missing. The embed polls only while
	// pending; a student replaying weeks later whose last grade never arrived
	// (a lost log) should not keep polling for the whole show.
	eaPendingWindow = 30 * time.Minute

	ceremonyExitLabel = "Back to Mission HydroSci"
)

// eaGradeDoc is the slice of mhsgrader's progress_point_grades document the
// ceremony needs (the dashboard has its own fuller read model). EA scores
// live on each attempt (Grade.eaScores, per mhsgrader/docs/updates/ea-scores.md);
// the per-unit stars on the document (eaStars, written by the grader when a
// unit's checkpoints are complete).
type eaGradeDoc struct {
	Game        string                   `bson:"game"`
	UserID      string                   `bson:"user_id"`
	Grades      map[string][]eaGradeItem `bson:"grades"`
	CurrentUnit string                   `bson:"currentUnit,omitempty"`
	EAStars     map[string]int           `bson:"eaStars,omitempty"`
	LastUpdated time.Time                `bson:"lastUpdated"`
}

type eaGradeItem struct {
	Status     string             `bson:"status"` // "active", "passed", or "flagged"
	ComputedAt time.Time          `bson:"computedAt"`
	EAScores   map[string]eaScore `bson:"eaScores,omitempty"` // "U2.C2" -> {score, max}
}

type eaScore struct {
	Score float64 `bson:"score" json:"score"`
	Max   float64 `bson:"max" json:"max"`
}

// eaScoresResponse is the contract the ceremony embed consumes
// (mhs-gameplay-end/docs/embed-api.md §"The scores contract").
type eaScoresResponse struct {
	Game           string             `json:"game"`
	UserID         string             `json:"user_id"`
	GeneratedAt    time.Time          `json:"generatedAt"`
	Status         string             `json:"status"` // "ready" | "pending"
	CurrentUnit    string             `json:"currentUnit"`
	CompletedUnits []string           `json:"completedUnits"`
	Items          map[string]eaScore `json:"items"`
	Stars          map[string]int     `json:"stars,omitempty"`
}

// ServeCeremony renders the ceremony host page.
//
// Members may open it once their progress is "complete" (Unit 5 finished
// through the play page); they always see their own ceremony. Staff (leader,
// coordinator, admin) may open it any time, and with ?user_id=<hex> preview a
// student within their reach — a preview writes no marks and no step log.
// Without a ceremony in the resolved collection the page does not exist:
// the student goes back to the units page.
func (h *Handler) ServeCeremony(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	manifest, _ := h.resolveManifest(r)
	if manifest.Ceremony == nil {
		http.Redirect(w, r, "/missionhydrosci/units", http.StatusSeeOther)
		return
	}

	preview := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if preview == user.ID {
		preview = ""
	}
	isMember := user.Role == "member"
	wsID := workspace.IDFromRequest(r)

	if isMember {
		if preview != "" {
			http.Redirect(w, r, "/missionhydrosci/units", http.StatusSeeOther)
			return
		}
		userOID, err := primitive.ObjectIDFromHex(user.ID)
		if err != nil {
			http.Error(w, "invalid user", http.StatusBadRequest)
			return
		}
		progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userOID)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "load MHS progress failed (ceremony gate)", err, "Couldn't load Mission HydroSci. Please try again.", "/missionhydrosci/units")
			return
		}
		if !progress.GameEnded() {
			http.Redirect(w, r, "/missionhydrosci/units", http.StatusSeeOther)
			return
		}
	} else if preview != "" {
		allowed, err := h.staffCanView(r, preview)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "ceremony preview scope check failed", err, "Couldn't open the ceremony. Please try again.", "/mhsdashboard")
			return
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	scoresURL := "/missionhydrosci/api/ea-scores"
	returnURL := "/missionhydrosci/units"
	if preview != "" {
		scoresURL += "?user_id=" + url.QueryEscape(preview)
		returnURL = "/mhsdashboard"
	}

	data := CeremonyData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Mission Complete", returnURL).AsBare(),
		Base:      ceremonyBase(manifest.Ceremony.Version),
		Version:   manifest.Ceremony.Version,
		ScoresURL: scoresURL,
		ReturnURL: returnURL,
		ExitLabel: ceremonyExitLabel,
		Dev:       !isMember && r.URL.Query().Has("dev"),
		Preview:   preview != "",
	}
	w.Header().Set("Cache-Control", "no-cache, no-store")
	templates.Render(w, r, "missionhydrosci_ceremony", data)
}

// staffCanView reports whether the signed-in staff user may look at the
// given student: the student must exist in this workspace and fall within
// the user's data scope (whole workspace for admins and analysts, assigned
// organizations for coordinators, own groups for leaders).
func (h *Handler) staffCanView(r *http.Request, userIDHex string) (bool, error) {
	oid, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return false, nil
	}
	ctx := r.Context()
	scope, err := viewscope.Resolve(ctx, h.DB, r, viewscope.Options{})
	if err != nil {
		if errors.Is(err, viewscope.ErrForbidden) || errors.Is(err, viewscope.ErrNoWorkspace) {
			return false, nil
		}
		return false, err
	}
	if scope.Empty() {
		return false, nil
	}
	reach, err := scope.Filter(ctx, "organization_id", "_id")
	if err != nil {
		return false, err
	}
	target := bson.M{"_id": oid, "workspace_id": scope.WorkspaceID}
	filter := target
	if len(reach) > 0 {
		filter = bson.M{"$and": []bson.M{reach, target}}
	}
	n, err := h.DB.Collection("users").CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ServeEAScores returns the EA-scores contract for the signed-in student, or
// for ?user_id=<hex> when a staff user within reach asks. Device-test ids are
// refused: they are not students.
func (h *Handler) ServeEAScores(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	target := user.ID
	if q := strings.TrimSpace(r.URL.Query().Get("user_id")); q != "" && q != user.ID {
		if user.Role == "member" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		allowed, err := h.staffCanView(r, q)
		if err != nil {
			h.Log.Error("ea-scores scope check failed", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		target = q
	}
	if models.IsMHSDeviceTestUserID(target) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	targetOID, err := primitive.ObjectIDFromHex(target)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	wsID := workspace.IDFromRequest(r)
	progress, err := h.ProgressStore.GetOrCreate(ctx, wsID, targetOID)
	if err != nil {
		h.Log.Error("ea-scores: load progress failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	doc, err := h.loadGradeDoc(ctx, target)
	if err != nil {
		// The show must go on: no grades means every line on its gentle
		// variant, which the embed handles; log and answer with what we have.
		h.Log.Warn("ea-scores: load grades failed", zap.String("user_id", target), zap.Error(err))
	}

	resp := buildEAScores(target, doc, progress, time.Now())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// loadGradeDoc reads the student's grade document from the grader's
// database; (nil, nil) when the database is not configured or the student
// has no grades yet.
func (h *Handler) loadGradeDoc(ctx context.Context, userIDHex string) (*eaGradeDoc, error) {
	if h.GradesDB == nil {
		return nil, nil
	}
	var doc eaGradeDoc
	err := h.GradesDB.Collection("progress_point_grades").FindOne(ctx, bson.M{
		"game":    "mhs",
		"user_id": userIDHex,
	}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

// buildEAScores merges a grade document into the contract: for every point,
// the latest FINISHED attempt (passed or flagged — the dashboard's rule; a
// fresh "active" replay after a finished attempt does not hide the finished
// scores) contributes its EA scores; the document's stars pass through. An
// item the grader never scored is simply absent (never a fabricated 0).
//
// Status is "pending" only while a grade for the final point is missing
// shortly after the student finished Unit 5, so the embed polls exactly when
// a grade may still land and otherwise plays with what exists.
func buildEAScores(userIDHex string, doc *eaGradeDoc, progress models.MHSUserProgress, now time.Time) eaScoresResponse {
	resp := eaScoresResponse{
		Game:           "mhs",
		UserID:         userIDHex,
		GeneratedAt:    now.UTC(),
		Status:         "ready",
		CurrentUnit:    progress.CurrentUnit,
		CompletedUnits: progress.CompletedUnits,
		Items:          map[string]eaScore{},
	}
	if resp.CompletedUnits == nil {
		resp.CompletedUnits = []string{}
	}
	finalGraded := false
	if doc != nil {
		for pointID, attempts := range doc.Grades {
			g := latestFinishedAttempt(attempts)
			if g == nil {
				continue
			}
			if pointID == eaFinalPointID {
				finalGraded = true
			}
			for k, v := range g.EAScores {
				resp.Items[k] = v
			}
		}
		if len(doc.EAStars) > 0 {
			resp.Stars = make(map[string]int, len(doc.EAStars))
			for k, v := range doc.EAStars {
				resp.Stars[k] = v
			}
		}
	}
	if progress.GameEnded() && !finalGraded {
		recent := now.Sub(*progress.GameEndedAt) < eaPendingWindow
		if doc != nil && now.Sub(doc.LastUpdated) < eaPendingWindow {
			recent = true
		}
		if recent {
			resp.Status = "pending"
		}
	}
	return resp
}

// latestFinishedAttempt returns the newest passed/flagged attempt, or nil.
func latestFinishedAttempt(attempts []eaGradeItem) *eaGradeItem {
	for i := len(attempts) - 1; i >= 0; i-- {
		if st := attempts[i].Status; st == "passed" || st == "flagged" {
			return &attempts[i]
		}
	}
	return nil
}

// ceremonyViewedRequest is the host page's body for POST /api/ceremony/viewed.
type ceremonyViewedRequest struct {
	Event   string `json:"event"`   // "started" | "finished"
	Version string `json:"version"` // ceremony version shown
}

// HandleCeremonyViewed records that the signed-in student pressed Begin or
// reached the end of the ceremony (models.MHSUserProgress ceremony marks).
// Staff previews never call it.
func (h *Handler) HandleCeremonyViewed(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req ceremonyViewedRequest
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	switch req.Event {
	case mhsuserprogress.CeremonyStarted, mhsuserprogress.CeremonyFinished:
	default:
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_event"})
		return
	}
	userOID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	version := models.ClipRunes(strings.TrimSpace(req.Version), 32)
	if err := h.ProgressStore.MarkCeremony(ctx, workspace.IDFromRequest(r), userOID, req.Event, version); err != nil {
		h.Log.Error("ceremony viewed: mark failed", zap.Error(err))
		writeDeviceTestJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "server_error"})
		return
	}
	writeDeviceTestJSON(w, http.StatusOK, map[string]any{"ok": true})
}
