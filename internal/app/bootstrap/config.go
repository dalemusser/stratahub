// internal/app/bootstrap/config.go
package bootstrap

import (
	"fmt"
	"time"

	"github.com/dalemusser/waffle/config"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.uber.org/zap"
)

// appConfigKeys defines the configuration keys for StrataHub.
// These are loaded via WAFFLE's config system with support for:
//   - Config files: mongo_uri, session_name, etc.
//   - Environment variables: STRATAHUB_MONGO_URI, STRATAHUB_SESSION_NAME, etc.
//   - Command-line flags: --mongo_uri, --session_name, etc.
var appConfigKeys = []config.AppKey{
	{Name: "mongo_uri", Default: "mongodb://localhost:27017", Desc: "MongoDB connection URI"},
	{Name: "mongo_database", Default: "strata_hub", Desc: "MongoDB database name"},
	{Name: "mongo_max_pool_size", Default: 100, Desc: "MongoDB max connection pool size (default: 100)"},
	{Name: "mongo_min_pool_size", Default: 10, Desc: "MongoDB min connection pool size (default: 10)"},
	{Name: "session_key", Default: "dev-only-change-me-please-0123456789ABCDEF", Desc: "Session signing key (must be strong in production)"},
	{Name: "session_name", Default: "stratahub-session", Desc: "Session cookie name"},
	{Name: "session_domain", Default: "", Desc: "Session cookie domain (blank means current host)"},

	// File storage configuration
	{Name: "storage_type", Default: "local", Desc: "Storage backend: 'local' or 's3'"},
	{Name: "storage_local_path", Default: "./uploads/materials", Desc: "Local storage path for uploaded files"},
	{Name: "storage_local_url", Default: "/files/materials", Desc: "URL prefix for serving local files"},

	// S3/CloudFront configuration
	{Name: "storage_s3_region", Default: "", Desc: "AWS region for S3"},
	{Name: "storage_s3_bucket", Default: "", Desc: "S3 bucket name"},
	{Name: "storage_s3_prefix", Default: "materials/", Desc: "S3 key prefix"},
	{Name: "storage_cf_url", Default: "", Desc: "CloudFront distribution URL"},
	{Name: "storage_cf_keypair_id", Default: "", Desc: "CloudFront key pair ID"},
	{Name: "storage_cf_key_path", Default: "", Desc: "Path to CloudFront private key file"},

	// Email/SMTP configuration
	{Name: "mail_smtp_host", Default: "localhost", Desc: "SMTP server host"},
	{Name: "mail_smtp_port", Default: 1025, Desc: "SMTP server port"},
	{Name: "mail_smtp_user", Default: "", Desc: "SMTP username"},
	{Name: "mail_smtp_pass", Default: "", Desc: "SMTP password"},
	{Name: "mail_from", Default: "noreply@stratahub.com", Desc: "From email address"},
	{Name: "mail_from_name", Default: "StrataHub", Desc: "From display name"},

	// Base URL for email links (magic links, etc.)
	{Name: "base_url", Default: "http://localhost:3000", Desc: "Base URL for email links"},

	// Email verification settings
	{Name: "email_verify_expiry", Default: "10m", Desc: "Email verification code/link expiry (e.g., 10m, 1h, 90s)"},

	// Audit logging settings
	{Name: "audit_log_auth", Default: "all", Desc: "Auth event logging: 'all' (db+log), 'db', 'log', or 'off'"},
	{Name: "audit_log_admin", Default: "all", Desc: "Admin event logging: 'all' (db+log), 'db', 'log', or 'off'"},

	// Google OAuth configuration
	{Name: "google_client_id", Default: "", Desc: "Google OAuth2 client ID"},
	{Name: "google_client_secret", Default: "", Desc: "Google OAuth2 client secret"},

	// Multi-workspace configuration
	{Name: "multi_workspace", Default: false, Desc: "Enable multi-workspace mode (subdomain-based tenancy)"},
	{Name: "primary_domain", Default: "", Desc: "Primary domain for OAuth callbacks and workspace selector (e.g., adroit.games)"},

	// Default workspace configuration
	{Name: "default_workspace_name", Default: "Default", Desc: "Display name for default workspace"},
	{Name: "default_workspace_subdomain", Default: "app", Desc: "Subdomain for default workspace"},

	// SuperAdmin bootstrap
	{Name: "superadmin_email", Default: "", Desc: "Email of the superadmin user (promotes/creates on startup)"},
}

// LoadConfig loads WAFFLE core config and app-specific config.
//
// It is called early in startup so that both WAFFLE and the app have
// access to configuration before any backends or handlers are built.
// CoreConfig comes from the shared WAFFLE layer; AppConfig is specific
// to this app and can be extended as the app grows.
//
// WAFFLE's config.LoadWithAppConfig handles:
//   - Loading from .env files
//   - Loading from config.yaml/json/toml files
//   - Reading environment variables (WAFFLE_* for core, STRATAHUB_* for app)
//   - Parsing command-line flags
//   - Merging with precedence: flags > env > files > defaults
func LoadConfig(logger *zap.Logger) (*config.CoreConfig, AppConfig, error) {
	coreCfg, appValues, err := config.LoadWithAppConfig(logger, "STRATAHUB", appConfigKeys)
	if err != nil {
		return nil, AppConfig{}, err
	}

	appCfg := AppConfig{
		MongoURI:         appValues.String("mongo_uri"),
		MongoDatabase:    appValues.String("mongo_database"),
		MongoMaxPoolSize: uint64(appValues.Int("mongo_max_pool_size")),
		MongoMinPoolSize: uint64(appValues.Int("mongo_min_pool_size")),
		SessionKey:       appValues.String("session_key"),
		SessionName:   appValues.String("session_name"),
		SessionDomain: appValues.String("session_domain"),

		// File storage
		StorageType:      appValues.String("storage_type"),
		StorageLocalPath: appValues.String("storage_local_path"),
		StorageLocalURL:  appValues.String("storage_local_url"),

		// S3/CloudFront
		StorageS3Region:    appValues.String("storage_s3_region"),
		StorageS3Bucket:    appValues.String("storage_s3_bucket"),
		StorageS3Prefix:    appValues.String("storage_s3_prefix"),
		StorageCFURL:       appValues.String("storage_cf_url"),
		StorageCFKeyPairID: appValues.String("storage_cf_keypair_id"),
		StorageCFKeyPath:   appValues.String("storage_cf_key_path"),

		// Email/SMTP
		MailSMTPHost: appValues.String("mail_smtp_host"),
		MailSMTPPort: appValues.Int("mail_smtp_port"),
		MailSMTPUser: appValues.String("mail_smtp_user"),
		MailSMTPPass: appValues.String("mail_smtp_pass"),
		MailFrom:     appValues.String("mail_from"),
		MailFromName: appValues.String("mail_from_name"),

		// Base URL
		BaseURL: appValues.String("base_url"),

		// Email verification
		EmailVerifyExpiry: appValues.Duration("email_verify_expiry", 10*time.Minute),

		// Audit logging
		AuditLogAuth:  appValues.String("audit_log_auth"),
		AuditLogAdmin: appValues.String("audit_log_admin"),

		// Google OAuth
		GoogleClientID:     appValues.String("google_client_id"),
		GoogleClientSecret: appValues.String("google_client_secret"),

		// Multi-workspace
		MultiWorkspace: appValues.Bool("multi_workspace"),
		PrimaryDomain:  appValues.String("primary_domain"),

		// Default workspace
		DefaultWorkspaceName:      appValues.String("default_workspace_name"),
		DefaultWorkspaceSubdomain: appValues.String("default_workspace_subdomain"),

		// SuperAdmin
		SuperAdminEmail: appValues.String("superadmin_email"),
	}

	// Auto-derive session domain in multi-workspace mode if not explicitly set.
	// For cross-subdomain cookies to work, domain must be ".adroit.games" (with leading dot).
	if appCfg.MultiWorkspace && appCfg.SessionDomain == "" && appCfg.PrimaryDomain != "" {
		appCfg.SessionDomain = "." + appCfg.PrimaryDomain
		logger.Info("auto-derived session domain for multi-workspace mode",
			zap.String("session_domain", appCfg.SessionDomain))
	}

	return coreCfg, appCfg, nil
}

// ValidateConfig performs app-specific config validation.
//
// Return nil to accept the loaded config, or an error to abort startup.
// This is the right place to enforce required fields or invariants that
// involve both the core and app configs.
//
// StrataHub validates the MongoDB URI format to catch configuration
// errors early, before attempting to connect.
func ValidateConfig(coreCfg *config.CoreConfig, appCfg AppConfig, logger *zap.Logger) error {
	if err := wafflemongo.ValidateURI(appCfg.MongoURI); err != nil {
		logger.Error("invalid MongoDB URI", zap.Error(err))
		return fmt.Errorf("invalid MongoDB URI: %w", err)
	}

	// Multi-workspace mode requires primary_domain to be set
	if appCfg.MultiWorkspace && appCfg.PrimaryDomain == "" {
		return fmt.Errorf("multi_workspace mode requires primary_domain to be set (e.g., 'adroit.games')")
	}

	return nil
}
