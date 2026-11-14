package mhsbuilds

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// slowPost sends a body in pieces over about 2.5 s, longer than the server's
// 1 s read timeout, and returns the status (0 if the connection was cut).
func slowPost(t *testing.T, url string) (int, string) {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		for range 5 {
			if _, err := pw.Write([]byte(strings.Repeat("x", 1024))); err != nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		pw.Close()
	}()
	resp, err := http.Post(url, "application/octet-stream", pr)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestLongUploadDeadlines(t *testing.T) {
	read := func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("ok"))
	}
	srv := httptest.NewUnstartedServer(LongUploadDeadlines(zap.NewNop())(http.HandlerFunc(read)))
	srv.Config.ReadTimeout = time.Second
	srv.Config.WriteTimeout = time.Second
	srv.Start()
	defer srv.Close()

	if code, body := slowPost(t, srv.URL+"/mhsbuilds/upload"); code != http.StatusOK {
		t.Fatalf("upload post: got %d %q, want 200 (the slow body must get through)", code, body)
	}
	if code, body := slowPost(t, srv.URL+"/mhsbuilds/upload/confirm"); code != http.StatusOK {
		t.Fatalf("confirm post: got %d %q, want 200", code, body)
	}
	if code, _ := slowPost(t, srv.URL+"/somewhere/else"); code == http.StatusOK {
		t.Fatalf("other post: got 200, want the server's 1 s read timeout to cut it off")
	}
}
