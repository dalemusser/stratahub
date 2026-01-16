// internal/app/features/announcements/handler.go
package announcements

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/announcement"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler owns all Announcements handlers.
type Handler struct {
	DB     *mongo.Database
	Store  *announcement.Store
	Log    *zap.Logger
	ErrLog *uierrors.ErrorLogger
}

// NewHandler constructs an Announcements Handler.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:     db,
		Store:  announcement.New(db),
		Log:    logger,
		ErrLog: errLog,
	}
}
