// internal/app/features/missionhydrosci/signedin.go
package missionhydrosci

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

// SignedInPath is where the play page's "Sign in" link lands once the
// student has signed in again (it is the login page's return address).
const SignedInPath = "/missionhydrosci/signed-in"

// ServeSignedIn tells a student who signed in again from a running game to
// go back to the game's tab. The play page shows a bar when its sign-in has
// ended and opens the login page in another tab; landing on the launcher
// there would offer a second Play button next to a game that is still
// running. This page offers nothing but the way back, and tells the game's
// tab to check its sign-in at once.
func (h *Handler) ServeSignedIn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store")
	data := struct{ viewdata.BaseVM }{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Signed in", "/missionhydrosci/units").AsBare(),
	}
	templates.Render(w, r, "missionhydrosci_signedin", data)
}
