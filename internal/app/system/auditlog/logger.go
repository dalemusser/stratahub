// internal/app/system/auditlog/logger.go
package auditlog

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"
	"strconv"

	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// Config holds audit logging configuration.
type Config struct {
	// Auth controls logging for authentication events (login, logout, password, verification).
	// Values: "all" (MongoDB + zap), "db" (MongoDB only), "log" (zap only), "off" (disabled)
	Auth string
	// Admin controls logging for admin action events (user/group/org CRUD, membership changes).
	// Values: "all" (MongoDB + zap), "db" (MongoDB only), "log" (zap only), "off" (disabled)
	Admin string
}

// Logger provides convenience methods for logging audit events.
// It logs to both MongoDB (via audit.Store) and structured logs (via zap).
type Logger struct {
	store  *audit.Store
	zapLog *zap.Logger
	config Config
}

// New creates a new audit Logger.
func New(store *audit.Store, zapLog *zap.Logger, config Config) *Logger {
	return &Logger{
		store:  store,
		zapLog: zapLog,
		config: config,
	}
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// logToZap logs the event to zap with consistent structure.
func (l *Logger) logToZap(event audit.Event) {
	fields := []zap.Field{
		zap.Bool("audit", true),
		zap.String("category", event.Category),
		zap.String("event_type", event.EventType),
		zap.Bool("success", event.Success),
		zap.String("ip", event.IP),
	}

	if event.UserID != nil {
		fields = append(fields, zap.String("user_id", event.UserID.Hex()))
	}
	if event.ActorID != nil {
		fields = append(fields, zap.String("actor_id", event.ActorID.Hex()))
	}
	if event.OrganizationID != nil {
		fields = append(fields, zap.String("organization_id", event.OrganizationID.Hex()))
	}
	if event.FailureReason != "" {
		fields = append(fields, zap.String("failure_reason", event.FailureReason))
	}
	for k, v := range event.Details {
		fields = append(fields, zap.String("detail_"+k, v))
	}

	if event.Success {
		l.zapLog.Info("audit event", fields...)
	} else {
		l.zapLog.Warn("audit event", fields...)
	}
}

// Log records an audit event based on configuration.
// If the logger is nil, this is a no-op (allows tests to use nil audit logger).
// Logging destination is controlled by config: "all", "db", "log", or "off".
func (l *Logger) Log(ctx context.Context, event audit.Event) {
	if l == nil {
		return
	}

	// Determine which config setting applies based on event category
	var setting string
	switch event.Category {
	case audit.CategoryAuth:
		setting = l.config.Auth
	case audit.CategoryAdmin:
		setting = l.config.Admin
	default:
		setting = "all" // Default to logging everything for unknown categories
	}

	// Check if logging is disabled for this category
	if setting == "off" {
		return
	}

	// Log to zap if configured
	if setting == "all" || setting == "log" {
		l.logToZap(event)
	}

	// Log to MongoDB if configured
	if setting == "all" || setting == "db" {
		if err := l.store.Log(ctx, event); err != nil {
			l.zapLog.Error("failed to store audit event",
				zap.Error(err),
				zap.String("event_type", event.EventType),
			)
		}
	}
}

// --- Authentication Events ---

// LoginSuccess logs a successful login.
func (l *Logger) LoginSuccess(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, authMethod, loginID string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginSuccess,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"auth_method": authMethod,
			"login_id":    loginID,
		},
	})
}

// LoginFailedUserNotFound logs a failed login due to user not found.
func (l *Logger) LoginFailedUserNotFound(ctx context.Context, r *http.Request, attemptedLoginID string) {
	l.Log(ctx, audit.Event{
		Category:      audit.CategoryAuth,
		EventType:     audit.EventLoginFailedUserNotFound,
		IP:            getClientIP(r),
		UserAgent:     r.UserAgent(),
		Success:       false,
		FailureReason: "user not found",
		Details: map[string]string{
			"attempted_login_id": attemptedLoginID,
		},
	})
}

// LoginFailedWrongPassword logs a failed login due to wrong password.
func (l *Logger) LoginFailedWrongPassword(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, loginID string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginFailedWrongPassword,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        false,
		FailureReason:  "wrong password",
		Details: map[string]string{
			"login_id": loginID,
		},
	})
}

// LoginFailedUserDisabled logs a failed login due to disabled account.
func (l *Logger) LoginFailedUserDisabled(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, loginID string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginFailedUserDisabled,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        false,
		FailureReason:  "user disabled",
		Details: map[string]string{
			"login_id": loginID,
		},
	})
}

// LoginFailedAuthMethodDisabled logs a failed login due to auth method not enabled for workspace.
func (l *Logger) LoginFailedAuthMethodDisabled(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, loginID, authMethod string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginFailedAuthMethodDisabled,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        false,
		FailureReason:  "auth method disabled for workspace",
		Details: map[string]string{
			"login_id":    loginID,
			"auth_method": authMethod,
		},
	})
}

// LoginFailedRateLimit logs a failed login due to rate limiting.
func (l *Logger) LoginFailedRateLimit(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, loginID, limitType string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginFailedRateLimit,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        false,
		FailureReason:  "rate limit exceeded",
		Details: map[string]string{
			"login_id":   loginID,
			"limit_type": limitType,
		},
	})
}

// Logout logs a user logout.
// Accepts string IDs from SessionUser and converts them to ObjectIDs.
func (l *Logger) Logout(ctx context.Context, r *http.Request, userIDStr, orgIDStr string) {
	var userID *primitive.ObjectID
	var orgID *primitive.ObjectID

	if oid, err := primitive.ObjectIDFromHex(userIDStr); err == nil {
		userID = &oid
	}
	if oid, err := primitive.ObjectIDFromHex(orgIDStr); err == nil {
		orgID = &oid
	}

	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLogout,
		UserID:         userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
	})
}

// PasswordChanged logs a password change.
func (l *Logger) PasswordChanged(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, wasTemporary bool) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventPasswordChanged,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"was_temporary": boolToString(wasTemporary),
		},
	})
}

// VerificationCodeSent logs when a verification code is sent.
func (l *Logger) VerificationCodeSent(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, email string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventVerificationCodeSent,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"email": email,
		},
	})
}

// VerificationCodeResent logs when a verification code is resent.
func (l *Logger) VerificationCodeResent(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, email string, resendCount int) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventVerificationCodeResent,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"email":        email,
			"resend_count": intToString(resendCount),
		},
	})
}

// VerificationCodeFailed logs a failed verification code attempt.
func (l *Logger) VerificationCodeFailed(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, reason string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventVerificationCodeFailed,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        false,
		FailureReason:  reason,
	})
}

// MagicLinkUsed logs when a magic link is used for login.
func (l *Logger) MagicLinkUsed(ctx context.Context, r *http.Request, userID primitive.ObjectID, orgID *primitive.ObjectID, email string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventMagicLinkUsed,
		UserID:         &userID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"email": email,
		},
	})
}

// --- Admin Events ---

// UserCreated logs when an admin creates a user.
func (l *Logger) UserCreated(ctx context.Context, r *http.Request, actorID, targetUserID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, role, authMethod string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserCreated,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":  actorRole,
			"role":        role,
			"auth_method": authMethod,
		},
	})
}

// UserUpdated logs when an admin updates a user.
func (l *Logger) UserUpdated(ctx context.Context, r *http.Request, actorID, targetUserID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, fieldsChanged string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserUpdated,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"fields_changed": fieldsChanged,
		},
	})
}

// UserDisabled logs when an admin disables a user account.
func (l *Logger) UserDisabled(ctx context.Context, r *http.Request, actorID, targetUserID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserDisabled,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
		},
	})
}

// UserEnabled logs when an admin enables a user account.
func (l *Logger) UserEnabled(ctx context.Context, r *http.Request, actorID, targetUserID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserEnabled,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
		},
	})
}

// UserDeleted logs when an admin deletes a user.
func (l *Logger) UserDeleted(ctx context.Context, r *http.Request, actorID, targetUserID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, role string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserDeleted,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"role":       role,
		},
	})
}

// GroupCreated logs when an admin creates a group.
func (l *Logger) GroupCreated(ctx context.Context, r *http.Request, actorID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, groupName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventGroupCreated,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"group_id":   groupID.Hex(),
			"group_name": groupName,
		},
	})
}

// GroupUpdated logs when an admin updates a group.
func (l *Logger) GroupUpdated(ctx context.Context, r *http.Request, actorID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, fieldsChanged string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventGroupUpdated,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"group_id":       groupID.Hex(),
			"fields_changed": fieldsChanged,
		},
	})
}

// GroupDeleted logs when an admin deletes a group.
func (l *Logger) GroupDeleted(ctx context.Context, r *http.Request, actorID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, groupName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventGroupDeleted,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"group_id":   groupID.Hex(),
			"group_name": groupName,
		},
	})
}

// MemberAddedToGroup logs when a user is added to a group.
func (l *Logger) MemberAddedToGroup(ctx context.Context, r *http.Request, actorID, targetUserID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, memberRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventMemberAddedToGroup,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":  actorRole,
			"group_id":    groupID.Hex(),
			"member_role": memberRole,
		},
	})
}

// MemberRemovedFromGroup logs when a user is removed from a group.
func (l *Logger) MemberRemovedFromGroup(ctx context.Context, r *http.Request, actorID, targetUserID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventMemberRemovedFromGroup,
		UserID:         &targetUserID,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"group_id":   groupID.Hex(),
		},
	})
}

// --- Organization Events ---

// OrgCreated logs when an admin creates an organization.
func (l *Logger) OrgCreated(ctx context.Context, r *http.Request, actorID, orgID primitive.ObjectID, actorRole, orgName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventOrgCreated,
		ActorID:        &actorID,
		OrganizationID: &orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"org_name":   orgName,
		},
	})
}

// OrgUpdated logs when an admin or coordinator updates an organization.
func (l *Logger) OrgUpdated(ctx context.Context, r *http.Request, actorID, orgID primitive.ObjectID, actorRole, fieldsChanged string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventOrgUpdated,
		ActorID:        &actorID,
		OrganizationID: &orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"fields_changed": fieldsChanged,
		},
	})
}

// OrgDeleted logs when an admin deletes an organization.
func (l *Logger) OrgDeleted(ctx context.Context, r *http.Request, actorID, orgID primitive.ObjectID, actorRole, orgName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventOrgDeleted,
		ActorID:        &actorID,
		OrganizationID: &orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"org_name":   orgName,
		},
	})
}

// --- Resource Events ---

// ResourceCreated logs when a resource is created in the library.
func (l *Logger) ResourceCreated(ctx context.Context, r *http.Request, actorID, resourceID primitive.ObjectID, actorRole, resourceTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventResourceCreated,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"resource_id":    resourceID.Hex(),
			"resource_title": resourceTitle,
		},
	})
}

// ResourceUpdated logs when a resource is updated.
func (l *Logger) ResourceUpdated(ctx context.Context, r *http.Request, actorID, resourceID primitive.ObjectID, actorRole, resourceTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventResourceUpdated,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"resource_id":    resourceID.Hex(),
			"resource_title": resourceTitle,
		},
	})
}

// ResourceDeleted logs when a resource is deleted from the library.
func (l *Logger) ResourceDeleted(ctx context.Context, r *http.Request, actorID, resourceID primitive.ObjectID, actorRole, resourceTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventResourceDeleted,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"resource_id":    resourceID.Hex(),
			"resource_title": resourceTitle,
		},
	})
}

// --- Material Events ---

// MaterialCreated logs when a material is created in the library.
func (l *Logger) MaterialCreated(ctx context.Context, r *http.Request, actorID, materialID primitive.ObjectID, actorRole, materialTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventMaterialCreated,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"material_id":    materialID.Hex(),
			"material_title": materialTitle,
		},
	})
}

// MaterialUpdated logs when a material is updated.
func (l *Logger) MaterialUpdated(ctx context.Context, r *http.Request, actorID, materialID primitive.ObjectID, actorRole, materialTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventMaterialUpdated,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"material_id":    materialID.Hex(),
			"material_title": materialTitle,
		},
	})
}

// MaterialDeleted logs when a material is deleted from the library.
func (l *Logger) MaterialDeleted(ctx context.Context, r *http.Request, actorID, materialID primitive.ObjectID, actorRole, materialTitle string) {
	l.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventMaterialDeleted,
		ActorID:   &actorID,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Success:   true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"material_id":    materialID.Hex(),
			"material_title": materialTitle,
		},
	})
}

// --- Resource Assignment Events ---

// ResourceAssignedToGroup logs when a resource is assigned to a group.
func (l *Logger) ResourceAssignedToGroup(ctx context.Context, r *http.Request, actorID, assignmentID, resourceID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, resourceTitle, groupName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventResourceAssignedToGroup,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"assignment_id":  assignmentID.Hex(),
			"resource_id":    resourceID.Hex(),
			"resource_title": resourceTitle,
			"group_id":       groupID.Hex(),
			"group_name":     groupName,
		},
	})
}

// ResourceAssignmentUpdated logs when a resource assignment is updated.
func (l *Logger) ResourceAssignmentUpdated(ctx context.Context, r *http.Request, actorID, assignmentID, resourceID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventResourceAssignmentUpdated,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":    actorRole,
			"assignment_id": assignmentID.Hex(),
			"resource_id":   resourceID.Hex(),
			"group_id":      groupID.Hex(),
		},
	})
}

// ResourceUnassignedFromGroup logs when a resource is unassigned from a group.
func (l *Logger) ResourceUnassignedFromGroup(ctx context.Context, r *http.Request, actorID, assignmentID, resourceID, groupID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventResourceUnassignedFromGroup,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":    actorRole,
			"assignment_id": assignmentID.Hex(),
			"resource_id":   resourceID.Hex(),
			"group_id":      groupID.Hex(),
		},
	})
}

// --- Coordinator Assignment Events ---

// CoordinatorAssignedToOrg logs when a coordinator is assigned to an organization.
func (l *Logger) CoordinatorAssignedToOrg(ctx context.Context, r *http.Request, actorID, coordinatorID, orgID primitive.ObjectID, actorRole, orgName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventCoordinatorAssignedToOrg,
		ActorID:        &actorID,
		UserID:         &coordinatorID,
		OrganizationID: &orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"org_name":   orgName,
		},
	})
}

// CoordinatorUnassignedFromOrg logs when a coordinator is unassigned from an organization.
func (l *Logger) CoordinatorUnassignedFromOrg(ctx context.Context, r *http.Request, actorID, coordinatorID, orgID primitive.ObjectID, actorRole, orgName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventCoordinatorUnassignedFromOrg,
		ActorID:        &actorID,
		UserID:         &coordinatorID,
		OrganizationID: &orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role": actorRole,
			"org_name":   orgName,
		},
	})
}

// --- Material Assignment Events ---

// MaterialAssigned logs when a material is assigned to an organization or leader.
func (l *Logger) MaterialAssigned(ctx context.Context, r *http.Request, actorID, assignmentID, materialID primitive.ObjectID, orgID *primitive.ObjectID, actorRole, materialTitle, targetType, targetName string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventMaterialAssigned,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":     actorRole,
			"assignment_id":  assignmentID.Hex(),
			"material_id":    materialID.Hex(),
			"material_title": materialTitle,
			"target_type":    targetType,
			"target_name":    targetName,
		},
	})
}

// MaterialAssignmentUpdated logs when a material assignment is updated.
func (l *Logger) MaterialAssignmentUpdated(ctx context.Context, r *http.Request, actorID, assignmentID, materialID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventMaterialAssignmentUpdated,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":    actorRole,
			"assignment_id": assignmentID.Hex(),
			"material_id":   materialID.Hex(),
		},
	})
}

// MaterialUnassigned logs when a material assignment is removed.
func (l *Logger) MaterialUnassigned(ctx context.Context, r *http.Request, actorID, assignmentID, materialID primitive.ObjectID, orgID *primitive.ObjectID, actorRole string) {
	l.Log(ctx, audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventMaterialUnassigned,
		ActorID:        &actorID,
		OrganizationID: orgID,
		IP:             getClientIP(r),
		UserAgent:      r.UserAgent(),
		Success:        true,
		Details: map[string]string{
			"actor_role":    actorRole,
			"assignment_id": assignmentID.Hex(),
			"material_id":   materialID.Hex(),
		},
	})
}

// --- Helper functions ---

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intToString(i int) string {
	return strconv.Itoa(i)
}
