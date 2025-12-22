package resources

import (
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// resourceTypeOptions returns the canonical list of resource types as
// ID/Label pairs for use in templates.
//
// The IDs come from models.ResourceTypes, and labels are simple
// human-friendly versions (first letter capitalized).
func resourceTypeOptions() []ResourceTypeOption {
	opts := make([]ResourceTypeOption, 0, len(models.ResourceTypes))
	for _, id := range models.ResourceTypes {
		label := id
		if len(id) > 0 {
			// Capitalize first letter for display (e.g., "game" -> "Game").
			label = strings.ToUpper(id[:1]) + id[1:]
		}
		opts = append(opts, ResourceTypeOption{ID: id, Label: label})
	}
	return opts
}
