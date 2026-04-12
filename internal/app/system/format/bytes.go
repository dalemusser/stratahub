// internal/app/system/format/bytes.go
package format

import "fmt"

// Bytes converts a byte count to a human-readable string (e.g., "104.3 MB").
func Bytes(b int64) string {
	const mb = 1024 * 1024
	const gb = 1024 * mb
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	default:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
}
