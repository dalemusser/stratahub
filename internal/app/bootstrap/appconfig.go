// internal/app/bootstrap/appconfig.go
package bootstrap

import "time"

// AppConfig holds service-specific configuration for this WAFFLE app.
//
// These values come from environment variables, configuration files, or
// command-line flags (loaded in LoadConfig). They represent *app-level*
// configuration, not WAFFLE core configuration.
//
// WAFFLE's CoreConfig handles framework-level settings like:
//   - HTTP/HTTPS ports and TLS configuration
//   - Logging level and format
//   - CORS settings
//   - Request body size limits
//   - Database connection timeouts
//
// AppConfig is where you put everything specific to YOUR application:
//   - Database connection strings (MongoDB URI, Postgres DSN, etc.)
//   - External service API keys and endpoints
//   - Feature flags and application modes
//   - Business logic configuration
//   - Default values for your domain
//
// Add fields here as your application grows. The struct is passed to
// most lifecycle hooks, so any configuration needed during startup,
// request handling, or shutdown should live here.
type AppConfig struct {
	// MongoDB connection configuration
	MongoURI         string // MongoDB connection string (e.g., mongodb://localhost:27017)
	MongoDatabase    string // Database name within MongoDB
	MongoMaxPoolSize uint64 // Maximum connections in pool (default: 100)
	MongoMinPoolSize uint64 // Minimum connections to keep warm (default: 10)

	// MHSGrader database (for reading progress grades - same cluster, different database)
	MHSGraderDatabase string // Database name for MHSGrader grades (default: mhsgrader)

	// Session management configuration
	SessionKey    string        // Secret key for signing session cookies (must be strong in production)
	SessionName   string        // Cookie name for sessions (default: stratahub-session)
	SessionDomain string        // Cookie domain (blank means current host)
	SessionMaxAge time.Duration // Maximum session cookie lifetime (default: 24h)

	// Idle logout configuration
	IdleLogoutEnabled bool          // Enable automatic logout after idle time
	IdleLogoutTimeout time.Duration // Duration of inactivity before logout (default: 30m)
	IdleLogoutWarning time.Duration // Time before logout to show warning (default: 5m)

	// CSRF protection configuration
	CSRFKey string // Secret key for CSRF token signing (32+ chars in production)

	// File storage configuration
	StorageType      string // Storage backend: "local" or "s3"
	StorageLocalPath string // Local storage path (e.g., "./uploads/materials")
	StorageLocalURL  string // URL prefix for serving local files (e.g., "/files/materials")

	// S3/CloudFront configuration (only used if StorageType is "s3")
	StorageS3Region string // AWS region
	StorageS3Bucket string // S3 bucket name
	StorageS3Prefix string // Key prefix (e.g., "materials/")
	StorageS3ACL    string // Default ACL for uploaded objects (e.g., "public-read", "private")
	StorageCFURL       string // CloudFront distribution URL
	StorageCFKeyPairID string // CloudFront key pair ID
	StorageCFKeyPath   string // Path to CloudFront private key file

	// Email/SMTP configuration
	MailSMTPHost string // SMTP server host (e.g., localhost for Mailpit, email-smtp.us-east-1.amazonaws.com for SES)
	MailSMTPPort int    // SMTP server port (e.g., 1025 for Mailpit, 587 for SES)
	MailSMTPUser string // SMTP username (empty for Mailpit, SES SMTP credentials for AWS)
	MailSMTPPass string // SMTP password
	MailFrom     string // From email address (e.g., noreply@stratahub.com)
	MailFromName string // From display name (e.g., StrataHub)

	// Base URL for email links (magic links, password reset, etc.)
	BaseURL string // e.g., "https://stratahub.com" or "http://localhost:3000"

	// Email verification settings
	EmailVerifyExpiry time.Duration // How long email verification codes/links are valid (default: 10m)

	// Audit logging configuration
	// Values: "all" (MongoDB + zap), "db" (MongoDB only), "log" (zap only), "off" (disabled)
	AuditLogAuth  string // Phase 1: authentication events (login, logout, password, verification)
	AuditLogAdmin string // Phase 2: admin actions (user/group/org CRUD, membership changes)

	// Google OAuth configuration
	GoogleClientID     string // Google OAuth2 client ID
	GoogleClientSecret string // Google OAuth2 client secret

	// Multi-workspace configuration
	MultiWorkspace bool   // true = subdomain-based workspaces, false = single workspace mode
	PrimaryDomain  string // Apex domain (e.g., "adroit.games") for OAuth callbacks and workspace selector

	// Default workspace configuration (used when bootstrapping first workspace)
	DefaultWorkspaceName      string // Display name for default workspace (default: "Default")
	DefaultWorkspaceSubdomain string // Subdomain for default workspace (default: "app")

	// SuperAdmin bootstrap configuration
	SuperAdminEmail string // Email of the superadmin user (if set, promotes/creates this user on startup)

	// MHS Content Delivery
	MHSCDNBaseURL string // CDN base URL for MHS game builds (e.g., "https://cdn.adroit.games/mhs")
}
