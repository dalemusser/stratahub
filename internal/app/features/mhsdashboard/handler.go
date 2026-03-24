// internal/app/features/mhsdashboard/handler.go
package mhsdashboard

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the MHS dashboard feature.
type Handler struct {
	DB                *mongo.Database // StrataHub database for users, groups, etc.
	GradesDB          *mongo.Database // MHSGrader database for progress grades
	Log               *zap.Logger
	ErrLog            *uierrors.ErrorLogger
	DeviceStatusStore *mhsdevicestatus.Store
	ProgressStore     *mhsuserprogress.Store
	ClaudeAPIKey      string // Anthropic API key for AI summaries
	ClaudeModel       string // Claude model ID
}

// NewHandler constructs a new Handler.
func NewHandler(db, gradesDB *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:                db,
		GradesDB:          gradesDB,
		Log:               logger,
		ErrLog:            errLog,
		DeviceStatusStore: mhsdevicestatus.New(db),
		ProgressStore:     mhsuserprogress.New(db),
	}
}
