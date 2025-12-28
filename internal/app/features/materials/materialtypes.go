package materials

import (
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// materialTypeOptions returns the canonical list of material types as
// ID/Label pairs for use in templates.
//
// The IDs come from models.MaterialTypes, and labels are simple
// human-friendly versions (first letter capitalized).
func materialTypeOptions() []MaterialTypeOption {
	opts := make([]MaterialTypeOption, 0, len(models.MaterialTypes))
	for _, id := range models.MaterialTypes {
		label := id
		if len(id) > 0 {
			// Capitalize first letter for display (e.g., "document" -> "Document").
			label = strings.ToUpper(id[:1]) + id[1:]
		}
		opts = append(opts, MaterialTypeOption{ID: id, Label: label})
	}
	return opts
}
