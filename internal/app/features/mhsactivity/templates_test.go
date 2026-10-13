package mhsactivity

import "testing"

// Every template of the feature compiles with the shared layout. This needs
// no database, so it runs anywhere; the handler tests exercise the pages.
func TestTemplatesBoot(t *testing.T) {
	bootTemplates(t)
}
