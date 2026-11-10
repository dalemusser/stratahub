// internal/app/features/missionhydrosci/services_health.go
package missionhydrosci

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// The play gate (docs/mission-hydrosci/mhs-issues-102826.md, "Play gate"):
// a student whose game cannot log or save must not play, but only when the
// evidence points at the school. A device that cannot reach a service the
// server reaches is being blocked on its side; when the server cannot reach
// it either, the service is down for everyone and classes play on (the game
// keeps its activity and sends it later). This is the server's view: each
// service's probe route (ManifestProbe.Post), sent the way the game sends,
// checked at most once a minute.
const (
	servicesHealthTTL     = time.Minute
	servicesHealthTimeout = 4 * time.Second
)

type servicesHealth struct {
	mu      sync.Mutex
	at      time.Time
	state   map[string]string // service name -> "ok" | "down"
	client  *http.Client
	probeFn func(ctx context.Context, url string) bool // tests replace it
}

// ServicesHealthState is the endpoint's reply.
type ServicesHealthState struct {
	Services  map[string]string `json:"services"` // name -> "ok" | "down"
	CheckedAt time.Time         `json:"checked_at"`
}

func (h *Handler) servicesHealthNow(ctx context.Context) ServicesHealthState {
	sh := &h.svcHealth
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.state != nil && time.Since(sh.at) < servicesHealthTTL {
		return ServicesHealthState{Services: copyState(sh.state), CheckedAt: sh.at}
	}
	probe := sh.probeFn
	if probe == nil {
		if sh.client == nil {
			sh.client = &http.Client{Timeout: servicesHealthTimeout}
		}
		probe = func(ctx context.Context, url string) bool { return postProbe(ctx, sh.client, url) }
	}
	state := map[string]string{}
	var wg sync.WaitGroup
	var smu sync.Mutex
	for _, p := range h.Probes {
		if p.Post == "" {
			continue
		}
		wg.Add(1)
		go func(p ManifestProbe) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, servicesHealthTimeout)
			defer cancel()
			v := "down"
			if probe(cctx, p.Post) {
				v = "ok"
			}
			smu.Lock()
			state[p.Name] = v
			smu.Unlock()
		}(p)
	}
	wg.Wait()
	sh.state, sh.at = state, time.Now().UTC()
	for name, v := range state {
		if v != "ok" && h.Log != nil {
			h.Log.Warn("game service probe failed from the server", zap.String("service", name))
		}
	}
	return ServicesHealthState{Services: copyState(state), CheckedAt: sh.at}
}

func copyState(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// postProbe sends one probe the way the game sends its requests and reports
// whether the service itself answered {"ok":true}.
func postProbe(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"probe":true}`))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer probe")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return false
	}
	return body.OK
}

// ServeServicesHealth answers GET /missionhydrosci/api/services-health: the
// server's view of each game service ("ok" or "down"). Public (device-test
// pages have no account); it says nothing about any person.
func (h *Handler) ServeServicesHealth(w http.ResponseWriter, r *http.Request) {
	st := h.servicesHealthNow(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(st)
}
