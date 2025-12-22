// internal/app/system/csvutil/limits.go
package csvutil

// Upload size and row limits for CSV processing.
const (
	MaxUploadSize = 5 << 20 // 5 MB
	MaxRows       = 20000
)
