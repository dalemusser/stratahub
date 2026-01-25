// internal/app/features/mhsdashboard2/config.go
package mhsdashboard2

import (
	"encoding/json"
	"sync"

	appresources "github.com/dalemusser/stratahub/internal/app/resources"
)

var (
	progressConfig     *ProgressConfig
	progressConfigOnce sync.Once
	progressConfigErr  error
)

// LoadProgressConfig loads and caches the progress points configuration.
// The configuration is loaded once and cached for the lifetime of the application.
func LoadProgressConfig() (*ProgressConfig, error) {
	progressConfigOnce.Do(func() {
		data, err := appresources.FS.ReadFile("mhs_progress_points.json")
		if err != nil {
			progressConfigErr = err
			return
		}

		var cfg ProgressConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			progressConfigErr = err
			return
		}

		progressConfig = &cfg
	})

	return progressConfig, progressConfigErr
}

// TotalProgressPoints returns the total number of progress points across all units.
func (c *ProgressConfig) TotalProgressPoints() int {
	total := 0
	for _, u := range c.Units {
		total += len(u.ProgressPoints)
	}
	return total
}
