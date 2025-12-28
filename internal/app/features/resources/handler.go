// internal/app/features/resources/handler.go
package resources

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// AdminHandler owns all admin/library-facing Resource handlers
// (list, new, edit, view, delete, manage modal).
//
// It is constructed once at startup in bootstrap, using the
// shared Mongo database handle, file storage, and logger.
type AdminHandler struct {
	DB      *mongo.Database
	Storage storage.Store
	Log     *zap.Logger
	ErrLog  *uierrors.ErrorLogger
}

// MemberHandler owns the member-facing Resource handlers
// ("My Resources" list and individual resource view/download).
//
// It shares the same underlying DB, storage, and logger pattern as
// AdminHandler but keeps the responsibilities clearly separated.
type MemberHandler struct {
	DB      *mongo.Database
	Storage storage.Store
	Log     *zap.Logger
	ErrLog  *uierrors.ErrorLogger
}

// NewAdminHandler constructs an AdminHandler bound to the
// given Mongo database, file storage, and logger.
func NewAdminHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		DB:      db,
		Storage: store,
		Log:     logger,
		ErrLog:  errLog,
	}
}

// NewMemberHandler constructs a MemberHandler bound to the
// given Mongo database, file storage, and logger.
func NewMemberHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, logger *zap.Logger) *MemberHandler {
	return &MemberHandler{
		DB:      db,
		Storage: store,
		Log:     logger,
		ErrLog:  errLog,
	}
}
