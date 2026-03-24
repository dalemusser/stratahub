package staffauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/mailer"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// formatExpiryDuration formats a duration as a human-readable string.
func formatExpiryDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes < 60 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	hours := minutes / 60
	if hours == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", hours)
}

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDisabled       = errors.New("user account is disabled")
	ErrUserNotStaff       = errors.New("user is not a leader, coordinator, admin, or superadmin")
	ErrUserWrongWorkspace = errors.New("user does not belong to this workspace")
	ErrNoEmail            = errors.New("this user has email authentication but no email address could be determined")
	ErrInvalidCredential  = errors.New("invalid credential")
	ErrCodeExpired        = errors.New("verification code expired")
	ErrTooManyAttempts    = errors.New("too many verification attempts")
)

// staffRoles are roles that qualify as "staff" for supervisor override.
var staffRoles = map[string]bool{
	"leader":      true,
	"admin":       true,
	"coordinator": true,
	"superadmin":  true,
}

// StartResult is returned from StartAuth with the next step info.
type StartResult struct {
	Method      string // "trust", "password", "email"
	ChallengeID string // Empty for trust (token returned directly)
	Token       string // Only set for trust (immediate auth)
	StaffName   string
}

// VerifyResult is returned from VerifyAuth on success.
type VerifyResult struct {
	Token     string
	StaffName string
}

// Verifier handles staff authentication verification flows.
type Verifier struct {
	DB          *mongo.Database
	Store       *Store
	EmailVerify *emailverify.Store
	Mailer      *mailer.Mailer
	Log         *zap.Logger
	SiteName    string
}

// StartAuth begins a staff authentication flow.
// It looks up the user by login ID, validates they are staff, and returns
// the authentication method and challenge info.
func (v *Verifier) StartAuth(ctx context.Context, wsID primitive.ObjectID, loginID string) (*StartResult, error) {
	// Look up user by folded login ID
	foldedID := text.Fold(loginID)
	if foldedID == "" {
		return nil, ErrUserNotFound
	}

	// Query by login_id_ci, scoped to the current workspace OR superadmins
	// (who have no workspace_id). This matches the login handler's pattern.
	filter := bson.M{
		"login_id_ci": foldedID,
		"$or": []bson.M{
			{"workspace_id": wsID},
			{"workspace_id": bson.M{"$exists": false}},
			{"workspace_id": nil},
		},
	}

	var user models.User
	err := v.DB.Collection("users").FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	// Validate user status
	if user.Status == "disabled" {
		return nil, ErrUserDisabled
	}

	// Validate role is staff
	if !staffRoles[user.Role] {
		return nil, ErrUserNotStaff
	}

	method := user.AuthMethod

	switch method {
	case "trust":
		token, err := v.Store.CreateVerifiedToken(ctx, wsID, user.ID, user.FullName)
		if err != nil {
			return nil, fmt.Errorf("create trust token: %w", err)
		}
		return &StartResult{
			Method:    "trust",
			Token:     token,
			StaffName: user.FullName,
		}, nil

	case "password":
		challengeID, err := v.Store.CreateChallenge(ctx, wsID, user.ID, user.FullName, "password")
		if err != nil {
			return nil, fmt.Errorf("create password challenge: %w", err)
		}
		return &StartResult{
			Method:      "password",
			ChallengeID: challengeID,
			StaffName:   user.FullName,
		}, nil

	case "email":
		// For email auth, the login ID IS the email (same pattern as login handler)
		email := ""
		if user.LoginID != nil {
			email = *user.LoginID
		}
		if email == "" {
			return nil, ErrNoEmail
		}
		// Create the email verification
		result, err := v.EmailVerify.Create(ctx, user.ID, email, false)
		if err != nil {
			return nil, fmt.Errorf("create email verification: %w", err)
		}
		// Send the verification email (code only, no magic link for staff auth)
		emailData := mailer.VerificationEmailData{
			SiteName:  v.SiteName,
			Code:      result.Code,
			MagicLink: "", // No magic link for staff auth
			ExpiresIn: formatExpiryDuration(v.EmailVerify.Expiry()),
		}
		mailMsg := mailer.BuildVerificationEmail(emailData)
		mailMsg.To = email
		if err := v.Mailer.Send(mailMsg); err != nil {
			v.Log.Error("failed to send staff auth verification email", zap.Error(err))
			return nil, fmt.Errorf("send verification email: %w", err)
		}
		// Create challenge to track the flow
		challengeID, err := v.Store.CreateChallenge(ctx, wsID, user.ID, user.FullName, "email")
		if err != nil {
			return nil, fmt.Errorf("create email challenge: %w", err)
		}
		return &StartResult{
			Method:      "email",
			ChallengeID: challengeID,
			StaffName:   user.FullName,
		}, nil

	default:
		// For SSO methods (google, microsoft, classlink, clever), fall back to
		// simple confirmation since we can't initiate an OAuth flow inline.
		token, err := v.Store.CreateVerifiedToken(ctx, wsID, user.ID, user.FullName)
		if err != nil {
			return nil, fmt.Errorf("create sso trust token: %w", err)
		}
		return &StartResult{
			Method:    "trust",
			Token:     token,
			StaffName: user.FullName,
		}, nil
	}
}

// VerifyAuth verifies a credential against a pending challenge.
func (v *Verifier) VerifyAuth(ctx context.Context, challengeID, credential string) (*VerifyResult, error) {
	ch, err := v.Store.GetChallenge(ctx, challengeID)
	if err != nil {
		return nil, err
	}

	switch ch.Method {
	case "password":
		// Load user to get password hash
		var user struct {
			PasswordHash *string `bson:"password_hash"`
		}
		err := v.DB.Collection("users").FindOne(ctx, bson.M{"_id": ch.UserID}).Decode(&user)
		if err != nil {
			return nil, fmt.Errorf("lookup user for password check: %w", err)
		}
		if user.PasswordHash == nil || !authutil.CheckPassword(credential, *user.PasswordHash) {
			return nil, ErrInvalidCredential
		}

	case "email":
		_, err := v.EmailVerify.VerifyCode(ctx, ch.UserID, credential)
		if err != nil {
			switch {
			case errors.Is(err, emailverify.ErrExpired):
				return nil, ErrCodeExpired
			case errors.Is(err, emailverify.ErrTooManyAttempts):
				return nil, ErrTooManyAttempts
			default:
				return nil, ErrInvalidCredential
			}
		}

	default:
		return nil, fmt.Errorf("unsupported challenge method: %s", ch.Method)
	}

	// Mark verified and get token
	token, err := v.Store.MarkVerified(ctx, challengeID)
	if err != nil {
		return nil, fmt.Errorf("mark verified: %w", err)
	}

	return &VerifyResult{
		Token:     token,
		StaffName: ch.StaffName,
	}, nil
}

// ResendEmail resends the verification email for an email challenge.
func (v *Verifier) ResendEmail(ctx context.Context, challengeID string) error {
	ch, err := v.Store.GetChallenge(ctx, challengeID)
	if err != nil {
		return err
	}
	if ch.Method != "email" {
		return fmt.Errorf("challenge is not email method")
	}

	// Load user to get login_id (for email auth, login_id IS the email)
	var user struct {
		LoginID *string `bson:"login_id"`
	}
	err = v.DB.Collection("users").FindOne(ctx, bson.M{"_id": ch.UserID}).Decode(&user)
	if err != nil {
		return fmt.Errorf("lookup user: %w", err)
	}
	email := ""
	if user.LoginID != nil {
		email = *user.LoginID
	}
	if email == "" {
		return ErrNoEmail
	}

	// Create new email verification (isResend=true)
	result, err := v.EmailVerify.Create(ctx, ch.UserID, email, true)
	if err != nil {
		return fmt.Errorf("create resend verification: %w", err)
	}

	emailData := mailer.VerificationEmailData{
		SiteName:  v.SiteName,
		Code:      result.Code,
		MagicLink: "",
		ExpiresIn: formatExpiryDuration(v.EmailVerify.Expiry()),
	}
	mailMsg := mailer.BuildVerificationEmail(emailData)
	mailMsg.To = email
	if err := v.Mailer.Send(mailMsg); err != nil {
		v.Log.Error("failed to resend staff auth verification email", zap.Error(err))
		return fmt.Errorf("send verification email: %w", err)
	}

	return nil
}
