package mhsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// requireAdmin checks that the user has admin/coordinator/superadmin role.
func requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return "", false
	}
	if role != "admin" && role != "coordinator" && role != "superadmin" {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}
	return role, true
}

// ServeDebugStudents serves the debug student list with anomaly counts.
func (h *Handler) ServeDebugStudents(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	groupID := r.URL.Query().Get("group")
	if groupID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	gid, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	members, err := h.getMembersForGroup(ctx, r, gid, "asc")
	if err != nil {
		h.Log.Error("failed to load members for debug", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Load progress config and grades
	progressCfg, err := LoadProgressConfig()
	if err != nil {
		h.Log.Error("failed to load progress config", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	playerIDs := make([]string, 0, len(members))
	for _, m := range members {
		if m.LoginID != nil && *m.LoginID != "" {
			playerIDs = append(playerIDs, *m.LoginID)
		}
	}

	gradesMap, err := h.loadProgressGrades(ctx, playerIDs)
	if err != nil {
		h.Log.Error("failed to load grades for debug", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Build student rows with anomaly counts
	rows := make([]DebugStudentRow, 0, len(members))
	for _, m := range members {
		loginID := ""
		if m.LoginID != nil {
			loginID = *m.LoginID
		}
		row := DebugStudentRow{
			ID:      m.ID.Hex(),
			Name:    m.FullName,
			LoginID: loginID,
		}

		if loginID != "" {
			if doc, ok := gradesMap[loginID]; ok && doc.Grades != nil {
				row.PencilCount, row.EmptyCount = detectAnomaliesFromGrades(doc.Grades, progressCfg)
			}
		}
		row.TotalAnomalies = row.PencilCount + row.EmptyCount + row.DuplicateCount

		// Get event count if log store is available
		if h.LogStore != nil && loginID != "" {
			count, err := h.LogStore.CountForPlayer(ctx, "mhs", loginID)
			if err != nil {
				h.Log.Warn("failed to count events", zap.String("player", loginID), zap.Error(err))
			} else {
				row.TotalEvents = count
			}
		}

		rows = append(rows, row)
	}

	data := DebugStudentsData{
		GroupName: groupID,
		Students:  rows,
	}

	templates.Render(w, r, "mhsdashboard_debug_students", data)
}

// ServeDebugDetail serves the full debug detail view for a single student.
func (h *Handler) ServeDebugDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	userIDHex := chi.URLParam(r, "userID")
	uid, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the user
	var user models.User
	if err := h.lookupUserByID(ctx, uid, &user); err != nil {
		h.ErrLog.LogServerError(w, r, "failed to look up user", err, "User not found.", "/mhsdashboard")
		return
	}

	// Load configs
	progressCfg, err := LoadProgressConfig()
	if err != nil {
		h.Log.Error("failed to load progress config", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	rules, err := LoadGradingRules()
	if err != nil {
		h.Log.Error("failed to load grading rules", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Resolve login ID
	loginID := ""
	if user.LoginID != nil {
		loginID = *user.LoginID
	}

	// Load grades
	gradesMap, err := h.loadProgressGrades(ctx, []string{loginID})
	if err != nil {
		h.Log.Error("failed to load grades", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var grades map[string][]ProgressGradeItem
	if doc, ok := gradesMap[loginID]; ok {
		grades = doc.Grades
	}

	// Load log events
	selectedUnit := r.URL.Query().Get("unit")
	var events []logEntryWrapper
	if h.LogStore != nil && loginID != "" {
		if selectedUnit != "" {
			scenes := rules.ScenesForUnit(selectedUnit)
			if len(scenes) > 0 {
				raw, err := h.LogStore.ListForPlayerByScenes(ctx, "mhs", loginID, scenes)
				if err != nil {
					h.Log.Error("failed to load events", zap.Error(err))
				} else {
					events = wrapEntries(raw)
				}
			}
		} else {
			raw, err := h.LogStore.ListForPlayer(ctx, "mhs", loginID)
			if err != nil {
				h.Log.Error("failed to load events", zap.Error(err))
			} else {
				events = wrapEntries(raw)
			}
		}
	}

	// Get org timezone
	loc := time.UTC
	// Use org timezone if available
	orgTZ := h.getOrgTimezoneForUser(ctx, r, user)
	if orgTZ != nil {
		loc = orgTZ
	}

	// Get timezone abbreviation for display
	tzAbbr := "UTC"
	if orgTZ != nil {
		tzAbbr, _ = time.Now().In(loc).Zone()
	}

	// Build timeline and detect anomalies
	var rawEntries = unwrapEntries(events)
	timeline := buildTimeline(rawEntries, rules, loc, h.ActiveGapThreshold)
	anomalies := detectAnomalies(grades, rawEntries, rules, progressCfg)

	data := DebugDetailData{
		StudentID:    userIDHex,
		StudentName:  user.FullName,
		LoginID:      loginID,
		SelectedUnit: selectedUnit,
		Anomalies:    anomalies,
		Timeline:     timeline,
		TotalEvents:  len(timeline),
		UnitOptions:  []string{"unit1", "unit2", "unit3", "unit4", "unit5"},
		GroupID:      r.URL.Query().Get("group"),
		TimezoneAbbr: tzAbbr,
	}

	templates.Render(w, r, "mhsdashboard_debug_detail", data)
}

// ServeMapPositions returns position data as JSON for a single scene.
func (h *Handler) ServeMapPositions(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	userIDHex := r.URL.Query().Get("user")
	if userIDHex == "" {
		http.Error(w, "user parameter required", http.StatusBadRequest)
		return
	}
	uid, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	sceneName := r.URL.Query().Get("scene")
	if sceneName == "" {
		http.Error(w, "scene parameter required", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.lookupUserByID(ctx, uid, &user); err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if h.LogStore == nil {
		http.Error(w, "log store not available", http.StatusServiceUnavailable)
		return
	}

	loginID := ""
	if user.LoginID != nil {
		loginID = *user.LoginID
	}
	if loginID == "" {
		http.Error(w, "user has no login ID", http.StatusBadRequest)
		return
	}

	entries, err := h.LogStore.ListForPlayerByScenes(ctx, "mhs", loginID, []string{sceneName})
	if err != nil {
		http.Error(w, "failed to load events", http.StatusInternalServerError)
		return
	}

	type posPoint struct {
		X    float64 `json:"x"`
		Z    float64 `json:"z"`
		Time string  `json:"time"`
	}
	type bounds struct {
		MinX float64 `json:"minX"`
		MaxX float64 `json:"maxX"`
		MinZ float64 `json:"minZ"`
		MaxZ float64 `json:"maxZ"`
	}
	type posResponse struct {
		Scene     string     `json:"scene"`
		Count     int        `json:"count"`
		Positions []posPoint `json:"positions"`
		Bounds    bounds     `json:"bounds"`
	}

	var positions []posPoint
	minX, maxX := 1e9, -1e9
	minZ, maxZ := 1e9, -1e9

	for _, e := range entries {
		if e.EventType != "PlayerPositionEvent" {
			continue
		}
		pos, ok := e.Data["position"].(map[string]interface{})
		if !ok {
			continue
		}
		x := toFloat(pos["x"])
		z := toFloat(pos["z"])

		positions = append(positions, posPoint{
			X:    x,
			Z:    z,
			Time: e.ServerTimestamp.Format(time.RFC3339),
		})

		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if z < minZ {
			minZ = z
		}
		if z > maxZ {
			maxZ = z
		}
	}

	// Add padding to bounds
	padX := (maxX - minX) * 0.05
	padZ := (maxZ - minZ) * 0.05
	if padX < 1 {
		padX = 1
	}
	if padZ < 1 {
		padZ = 1
	}

	resp := posResponse{
		Scene: sceneName,
		Count: len(positions),
		Positions: positions,
		Bounds: bounds{
			MinX: minX - padX,
			MaxX: maxX + padX,
			MinZ: minZ - padZ,
			MaxZ: maxZ + padZ,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ServeMapScenes returns the list of known scenes as JSON.
func (h *Handler) ServeMapScenes(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	rules, err := LoadGradingRules()
	if err != nil {
		http.Error(w, "failed to load rules", http.StatusInternalServerError)
		return
	}

	type sceneInfo struct {
		Name string `json:"name"`
		Unit string `json:"unit"`
	}

	scenes := make([]sceneInfo, 0, len(rules.SceneToUnit))
	for name, unit := range rules.SceneToUnit {
		scenes = append(scenes, sceneInfo{Name: name, Unit: unit})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scenes)
}

// ServeMapMembers returns the group members as JSON for the Maps tab student dropdown.
func (h *Handler) ServeMapMembers(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	groupID := r.URL.Query().Get("group")
	if groupID == "" {
		http.Error(w, "group parameter required", http.StatusBadRequest)
		return
	}

	gid, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		http.Error(w, "invalid group ID", http.StatusBadRequest)
		return
	}

	members, err := h.getMembersForGroup(ctx, r, gid, "asc")
	if err != nil {
		h.Log.Error("failed to load members for maps", zap.Error(err))
		http.Error(w, "failed to load members", http.StatusInternalServerError)
		return
	}

	type memberInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	result := make([]memberInfo, 0, len(members))
	for _, m := range members {
		result = append(result, memberInfo{
			ID:   m.ID.Hex(),
			Name: m.FullName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// logEntryWrapper wraps logdata.LogEntry for internal use.
type logEntryWrapper struct {
	entry logdata.LogEntry
}

func wrapEntries(entries []logdata.LogEntry) []logEntryWrapper {
	wrapped := make([]logEntryWrapper, len(entries))
	for i := range entries {
		wrapped[i] = logEntryWrapper{entry: entries[i]}
	}
	return wrapped
}

func unwrapEntries(wrapped []logEntryWrapper) []logdata.LogEntry {
	entries := make([]logdata.LogEntry, len(wrapped))
	for i := range wrapped {
		entries[i] = wrapped[i].entry
	}
	return entries
}

// lookupUserByID loads a user by their MongoDB ObjectID.
func (h *Handler) lookupUserByID(ctx context.Context, id primitive.ObjectID, user *models.User) error {
	return h.DB.Collection("users").FindOne(ctx, bson.M{"_id": id}).Decode(user)
}

// getOrgTimezoneForUser attempts to get the timezone for a user's organization.
func (h *Handler) getOrgTimezoneForUser(ctx context.Context, r *http.Request, user models.User) *time.Location {
	// This is a best-effort helper — returns nil if timezone can't be determined
	// We look up the user's group membership to find their org timezone
	wsID := workspace.IDFromRequest(r)
	if wsID.IsZero() {
		return nil
	}

	// Find a group membership for this user
	type membership struct {
		GroupID primitive.ObjectID `bson:"group_id"`
	}
	var mem membership
	err := h.DB.Collection("group_memberships").FindOne(ctx, bson.M{
		"workspace_id": wsID,
		"user_id":      user.ID,
		"role":         "member",
	}).Decode(&mem)
	if err != nil {
		return nil
	}

	// Find the group to get org_id
	type group struct {
		OrgID primitive.ObjectID `bson:"organization_id"`
	}
	var g group
	err = h.DB.Collection("groups").FindOne(ctx, bson.M{"_id": mem.GroupID}).Decode(&g)
	if err != nil || g.OrgID.IsZero() {
		return nil
	}

	// Find the org to get timezone
	type org struct {
		TimeZone string `bson:"time_zone"`
	}
	var o org
	err = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": g.OrgID}).Decode(&o)
	if err != nil || o.TimeZone == "" {
		return nil
	}

	loc, err := time.LoadLocation(o.TimeZone)
	if err != nil {
		return nil
	}
	return loc
}
