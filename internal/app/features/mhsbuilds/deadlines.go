// internal/app/features/mhsbuilds/deadlines.go
package mhsbuilds

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Build zips are hundreds of megabytes (up to maxUploadSize). The server's
// read_timeout (15s by default) covers the whole request body, so an upload
// that takes longer is cut off mid-body: the browser waits forever and nothing
// is created. These two posts get their own, longer deadlines; every other
// request keeps the server's.
const (
	// uploadReadTimeout: time to receive the zip. 1.5 GB at about 1 MB/s.
	uploadReadTimeout = 30 * time.Minute
	// uploadWriteTimeout: the server's write deadline runs from the end of the
	// request headers, so it has to outlast the body plus the work after it
	// (analysis, or the CDN upload on confirm).
	uploadWriteTimeout = 45 * time.Minute
)

// longDeadlinePaths are the posts that carry or process a build zip.
var longDeadlinePaths = map[string]bool{
	"/mhsbuilds/upload":         true,
	"/mhsbuilds/upload/confirm": true,
}

// LongUploadDeadlines extends the connection deadlines for the build upload
// posts. It must run before anything that reads the body: the CSRF check
// parses the multipart form, so it goes ahead of csrf.Protect.
func LongUploadDeadlines(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && longDeadlinePaths[r.URL.Path] {
				now := time.Now()
				rc := http.NewResponseController(w)
				if err := rc.SetReadDeadline(now.Add(uploadReadTimeout)); err != nil {
					logger.Warn("mhsbuilds: could not extend the read deadline", zap.String("path", r.URL.Path), zap.Error(err))
				}
				if err := rc.SetWriteDeadline(now.Add(uploadWriteTimeout)); err != nil {
					logger.Warn("mhsbuilds: could not extend the write deadline", zap.String("path", r.URL.Path), zap.Error(err))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
