// internal/app/features/mhsdashboard/handler.go
package mhsdashboard

import (
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the MHS dashboard feature.
type Handler struct {
	DB                    *mongo.Database // StrataHub database for users, groups, etc.
	GradesDB              *mongo.Database // MHSGrader database for progress grades
	LogDB                 *mongo.Database // Stratalog database for game log data
	LogStore              *logdata.Store  // Read-only access to stratalog logdata collection
	Log                   *zap.Logger
	ErrLog                *uierrors.ErrorLogger
	DeviceStatusStore     *mhsdevicestatus.Store
	ProgressStore         *mhsuserprogress.Store
	ClaudeAPIKey          string        // Anthropic API key for AI summaries
	ClaudeModel           string        // Claude model ID
	ActiveGapThreshold    time.Duration // Gaps longer than this excluded from active duration (default: 2m)
}

// NewHandler constructs a new Handler.
func NewHandler(db, gradesDB, logDB *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	h := &Handler{
		DB:                db,
		GradesDB:          gradesDB,
		LogDB:             logDB,
		Log:               logger,
		ErrLog:            errLog,
		DeviceStatusStore: mhsdevicestatus.New(db),
		ProgressStore:     mhsuserprogress.New(db),
	}
	if logDB != nil {
		h.LogStore = logdata.New(logDB)
	}
	return h
}
