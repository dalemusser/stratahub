// internal/app/features/mhsdashboard/handler.go
package mhsdashboard

import (
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the MHS dashboard feature.
type Handler struct {
	DB                 *mongo.Database // StrataHub database for users, groups, etc.
	GradesDB           *mongo.Database // MHSGrader database for progress grades
	LogDB              *mongo.Database // Stratalog database for game log data
	LogStore           *logdata.Store  // Read-only access to stratalog logdata collection
	Log                *zap.Logger
	ErrLog             *uierrors.ErrorLogger
	DeviceStatusStore  *mhsdevicestatus.Store
	ProgressStore      *mhsuserprogress.Store
	CollectionStore    *mhscollections.Store
	MemberStatusStore  *memberstatus.Store     // survey started/completed status (Member Status API)
	SurveyConfig       *memberstatuscfg.Config // tracked surveys; nil = no Surveys tab
	ClaudeAPIKey       string                  // Anthropic API key for AI summaries
	ClaudeModel        string                  // Claude model ID
	ActiveGapThreshold time.Duration           // Gaps longer than this excluded from active duration (default: 2m)
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
		CollectionStore:   mhscollections.New(db),
		MemberStatusStore: memberstatus.New(db),
	}
	if logDB != nil {
		h.LogStore = logdata.New(logDB)
	}
	// The survey list is optional for the dashboard: a bad configuration is
	// logged and the Surveys tab is simply absent (the API, which needs the
	// same file, fails startup instead).
	if cfg, err := memberstatuscfg.Load(); err != nil {
		logger.Error("mhs dashboard: survey configuration unavailable", zap.Error(err))
	} else {
		h.SurveyConfig = cfg
	}
	return h
}
