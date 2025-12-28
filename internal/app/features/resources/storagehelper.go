package resources

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/google/uuid"
)

// UploadInfo contains metadata about an uploaded file.
type UploadInfo struct {
	Path        string
	FileName    string
	Size        int64
	ContentType string
}

// uploadFile stores a file with a unique path and returns upload info.
// The path is generated as: resources/YYYY/MM/uuid-filename
func uploadFile(ctx context.Context, store storage.Store, filename string, reader io.Reader, size int64, contentType string) (UploadInfo, error) {
	// Generate unique path: resources/YYYY/MM/uuid-filename
	now := time.Now().UTC()
	dateDir := fmt.Sprintf("resources/%04d/%02d", now.Year(), now.Month())
	uniqueName := fmt.Sprintf("%s-%s", uuid.New().String()[:8], sanitizeFilename(filename))
	path := filepath.Join(dateDir, uniqueName)

	// Normalize path separators for storage
	path = filepath.ToSlash(path)

	// Upload to storage
	opts := &storage.PutOptions{
		ContentType: contentType,
	}
	if err := store.Put(ctx, path, reader, opts); err != nil {
		return UploadInfo{}, fmt.Errorf("failed to upload file: %w", err)
	}

	return UploadInfo{
		Path:        path,
		FileName:    filename,
		Size:        size,
		ContentType: contentType,
	}, nil
}

// sanitizeFilename removes or replaces characters that could be problematic in filenames.
func sanitizeFilename(filename string) string {
	// Get just the filename, not any path components
	filename = filepath.Base(filename)

	// Replace problematic characters
	result := make([]byte, 0, len(filename))
	for i := 0; i < len(filename); i++ {
		c := filename[i]
		if isAllowedFilenameChar(c) {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}

	// Ensure we have a reasonable filename
	if len(result) == 0 {
		return "file"
	}
	if len(result) > 100 {
		// Truncate but preserve extension if present
		ext := filepath.Ext(string(result))
		if len(ext) > 0 && len(ext) < 10 {
			result = append(result[:100-len(ext)], ext...)
		} else {
			result = result[:100]
		}
	}

	return string(result)
}

func isAllowedFilenameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.'
}
