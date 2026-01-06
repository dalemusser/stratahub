// internal/app/features/organizations/handler.go
package organizations

import (
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the feature-level entry point for Organizations.
type Handler struct {
	DB       *mongo.Database
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
	AuditLog *auditlog.Logger
}

// NewHandler constructs a new Organizations handler bound to a DB and logger.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, audit *auditlog.Logger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:       db,
		Log:      logger,
		ErrLog:   errLog,
		AuditLog: audit,
	}
}

// coordinatorHasAccess checks if the coordinator user has access to the given organization.
func coordinatorHasAccess(r *http.Request, orgID primitive.ObjectID) bool {
	orgIDs := authz.UserOrgIDs(r)
	for _, id := range orgIDs {
		if id == orgID {
			return true
		}
	}
	return false
}
