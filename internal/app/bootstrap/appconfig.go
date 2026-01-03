// internal/app/bootstrap/appconfig.go
package bootstrap

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
	MongoURI      string // MongoDB connection string (e.g., mongodb://localhost:27017)
	MongoDatabase string // Database name within MongoDB

	// Session management configuration
	SessionKey    string // Secret key for signing session cookies (must be strong in production)
	SessionName   string // Cookie name for sessions (default: stratahub-session)
	SessionDomain string // Cookie domain (blank means current host)

	// File storage configuration
	StorageType      string // Storage backend: "local" or "s3"
	StorageLocalPath string // Local storage path (e.g., "./uploads/materials")
	StorageLocalURL  string // URL prefix for serving local files (e.g., "/files/materials")

	// S3/CloudFront configuration (only used if StorageType is "s3")
	StorageS3Region    string // AWS region
	StorageS3Bucket    string // S3 bucket name
	StorageS3Prefix    string // Key prefix (e.g., "materials/")
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
}
