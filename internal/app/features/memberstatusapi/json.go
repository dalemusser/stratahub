// internal/app/features/memberstatusapi/json.go
package memberstatusapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON encodes v as the response body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError encodes a standard error body.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{OK: false, Error: code, Message: message})
}
