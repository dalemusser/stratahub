// internal/app/system/limits/limits.go
package limits

// Request body size limits for various features.
// These limits help prevent memory exhaustion from oversized requests.
const (
	// MaxPageContentSize is the maximum size for page edit form submissions.
	MaxPageContentSize = 1 << 20 // 1 MB

	// MaxSettingsFormSize is the maximum size for settings form submissions.
	// Note: Logo uploads use ParseMultipartForm with a separate limit.
	MaxSettingsFormSize = 1 << 20 // 1 MB
)
