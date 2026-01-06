// internal/app/features/leaders/handler.go
package leaders

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type Handler struct {
	DB       *mongo.Database
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
	AuditLog *auditlog.Logger
}

func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, audit *auditlog.Logger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:       db,
		Log:      logger,
		ErrLog:   errLog,
		AuditLog: audit,
	}
}
