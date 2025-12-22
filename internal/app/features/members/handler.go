// internal/app/features/members/handler.go
package members

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the feature-level handler for Members.
// It holds the DB handle, stores, and logger provided by WAFFLE DBDeps / Startup.
type Handler struct {
	DB       *mongo.Database
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
	Users    *userstore.Store
	Orgs     *organizationstore.Store
}

func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:     db,
		Log:    logger,
		ErrLog: errLog,
		Users:  userstore.New(db),
		Orgs:   organizationstore.New(db),
	}
}
