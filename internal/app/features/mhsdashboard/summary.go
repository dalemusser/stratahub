// internal/app/features/mhsdashboard/summary.go
package mhsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	_ "embed"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

//go:embed curriculum_context.md
var curriculumContext string

// ServeSummary generates an AI-powered student performance summary.
// GET /mhsdashboard/summary?user_id=<mongodb_object_id_hex>
func (h *Handler) ServeSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.ClaudeAPIKey == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "AI summaries are not configured. Set STRATAHUB_CLAUDE_API_KEY.")
		return
	}

	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Authentication required.")
		return
	}
	if role != "leader" && role != "admin" && role != "coordinator" && role != "superadmin" {
		writeJSONError(w, http.StatusForbidden, "You do not have access to student summaries.")
		return
	}

	userIDHex := r.URL.Query().Get("user_id")
	if userIDHex == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing user_id parameter.")
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid user_id parameter.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	// Look up the student's name and login_id from the users collection by _id
	studentName, loginID, err := h.lookupUser(ctx, userOID)
	if err != nil {
		h.Log.Warn("failed to look up user", zap.String("userID", userIDHex), zap.Error(err))
		writeJSONError(w, http.StatusNotFound, "Student not found.")
		return
	}
	if loginID == "" {
		writeJSONError(w, http.StatusBadRequest, "Student has no login ID configured.")
		return
	}

	// Load the student's full grade history using login_id
	gradeDoc, err := h.loadPlayerGrades(ctx, loginID)
	if err != nil {
		h.Log.Error("failed to load grades for summary", zap.String("userID", userIDHex), zap.Error(err))
		writeJSONError(w, http.StatusInternalServerError, "Failed to load student data.")
		return
	}
	if gradeDoc == nil {
		writeJSONError(w, http.StatusNotFound, "No grade data found for this student.")
		return
	}

	// Load progress config for point names
	cfg, err := LoadProgressConfig()
	if err != nil {
		h.Log.Error("failed to load progress config for summary", zap.Error(err))
		writeJSONError(w, http.StatusInternalServerError, "Configuration error.")
		return
	}

	// Build the student data summary for the prompt
	studentData := buildStudentDataSummary(gradeDoc, cfg)

	// Determine the model: per-workspace setting overrides env var default
	model := h.ClaudeModel
	wsID := workspace.IDFromRequest(r)
	if siteSettings, sErr := settingsstore.New(h.DB).Get(ctx, wsID); sErr == nil && siteSettings.ClaudeModel != "" {
		model = siteSettings.ClaudeModel
	}

	// Build and send the Claude API request
	summary, err := h.callClaudeAPI(ctx, studentName, studentData, model)
	if err != nil {
		h.Log.Error("Claude API call failed", zap.String("userID", userIDHex), zap.String("model", model), zap.Error(err))
		writeJSONError(w, http.StatusBadGateway, "AI summary generation failed. Please try again.")
		return
	}

	resp := map[string]string{
		"summary": summary,
		"user_id": userIDHex,
		"name":    studentName,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// lookupUser finds the student's full name and login_id from the users collection by _id.
func (h *Handler) lookupUser(ctx context.Context, userID primitive.ObjectID) (fullName, loginID string, err error) {
	var result struct {
		FullName string  `bson:"full_name"`
		LoginID  *string `bson:"login_id"`
	}
	err = h.DB.Collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&result)
	if err != nil {
		return "", "", err
	}
	if result.LoginID != nil {
		loginID = *result.LoginID
	}
	return result.FullName, loginID, nil
}

// loadPlayerGrades fetches grades for a single player from the mhsgrader database.
func (h *Handler) loadPlayerGrades(ctx context.Context, playerID string) (*ProgressGradeDoc, error) {
	if h.GradesDB == nil {
		return nil, fmt.Errorf("mhsgrader database not configured")
	}

	var doc ProgressGradeDoc
	err := h.GradesDB.Collection("progress_point_grades").FindOne(ctx, bson.M{
		"game":     "mhs",
		"playerId": playerID,
	}).Decode(&doc)

	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

// buildStudentDataSummary converts grade data into a structured text summary for the prompt.
func buildStudentDataSummary(doc *ProgressGradeDoc, cfg *ProgressConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Current Unit: %s\n\n", doc.CurrentUnit))

	for _, unit := range cfg.Units {
		sb.WriteString(fmt.Sprintf("## %s: %s\n", unit.ID, unit.Title))

		for _, point := range unit.ProgressPoints {
			items, exists := doc.Grades[point.ID]
			if !exists || len(items) == 0 {
				sb.WriteString(fmt.Sprintf("- %s (%s): Not started\n", point.ID, point.ShortName))
				continue
			}

			latest := items[len(items)-1]
			sb.WriteString(fmt.Sprintf("- %s (%s): %s", point.ID, point.ShortName, latest.Status))

			if latest.ReasonCode != "" {
				sb.WriteString(fmt.Sprintf(" [reason: %s]", latest.ReasonCode))
			}

			if latest.DurationSecs != nil {
				sb.WriteString(fmt.Sprintf(" [duration: %.0fs]", *latest.DurationSecs))
			}
			if latest.ActiveDurationSecs != nil {
				sb.WriteString(fmt.Sprintf(" [active: %.0fs]", *latest.ActiveDurationSecs))
			}

			// Include metrics
			if len(latest.Metrics) > 0 {
				sb.WriteString(" [metrics: ")
				first := true
				for k, v := range latest.Metrics {
					if !first {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%s=%v", k, v))
					first = false
				}
				sb.WriteString("]")
			}

			// Show attempt count if multiple
			if len(items) > 1 {
				sb.WriteString(fmt.Sprintf(" [attempts: %d]", len(items)))
				// Show previous attempt statuses
				var prevStatuses []string
				for _, item := range items[:len(items)-1] {
					s := item.Status
					if item.ReasonCode != "" {
						s += "(" + item.ReasonCode + ")"
					}
					prevStatuses = append(prevStatuses, s)
				}
				sb.WriteString(fmt.Sprintf(" [history: %s]", strings.Join(prevStatuses, " → ")))
			}

			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// claudeRequest is the request body for the Anthropic Messages API.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
	System    string          `json:"system"`
}

// claudeMessage is a single message in the conversation.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the response from the Anthropic Messages API.
type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// callClaudeAPI sends the student data to Claude and returns the summary text.
func (h *Handler) callClaudeAPI(ctx context.Context, studentName, studentData, model string) (string, error) {
	systemPrompt := fmt.Sprintf(`You are an educational assessment specialist analyzing student performance data from Mission HydroSci, a science adventure game that teaches hydrology and scientific argumentation.

Below is the curriculum context document that explains each progress point's learning objectives, what is assessed, and what flagged results indicate:

<curriculum_context>
%s
</curriculum_context>

Your task is to write a clear, professional performance summary for a teacher or instructional leader. The summary should:

1. Describe the student's overall progress through the game
2. Highlight areas of strength (passed with low mistake counts, fast completion)
3. Identify areas of concern (flagged results, high mistake counts, many attempts)
4. Connect flagged results to specific learning gaps using the curriculum context
5. Suggest instructional focus areas based on patterns in the data
6. Use the student's name naturally (not "the student")

Write in 2-4 paragraphs. Be specific about which concepts the student understands or struggles with. Avoid generic statements. Reference specific progress points by their descriptive name (not IDs like "u2p3"). Do not include raw metrics numbers unless they help illustrate a point.`, curriculumContext)

	userPrompt := fmt.Sprintf("Please write a performance summary for %s based on the following grade data:\n\n%s", studentName, studentData)

	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	reqBody := claudeRequest{
		Model:     model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", h.ClaudeAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		h.Log.Error("Claude API error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)),
		)
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if claudeResp.Error != nil {
		return "", fmt.Errorf("API error: %s", claudeResp.Error.Message)
	}

	// Extract text from content blocks
	var texts []string
	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}

	if len(texts) == 0 {
		return "", fmt.Errorf("no text content in response")
	}

	return strings.Join(texts, "\n"), nil
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
