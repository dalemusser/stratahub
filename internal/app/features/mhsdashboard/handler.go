// internal/app/features/mhsdashboard/handler.go
package mhsdashboard

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the MHS dashboard feature.
type Handler struct {
	DB       *mongo.Database // StrataHub database for users, groups, etc.
	GradesDB *mongo.Database // MHSGrader database for progress grades
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
}

// NewHandler constructs a new Handler.
func NewHandler(db, gradesDB *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:       db,
		GradesDB: gradesDB,
		Log:      logger,
		ErrLog:   errLog,
	}
}
