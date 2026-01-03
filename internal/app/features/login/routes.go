// internal/app/features/login/routes.go
package login

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeLogin)
	r.Post("/", h.HandleLoginPost)

	// Password authentication flow
	r.Get("/password", h.ServePasswordPage)
	r.Post("/password", h.HandlePasswordSubmit)

	// Change password (for temporary passwords)
	r.Get("/change-password", h.ServeChangePassword)
	r.Post("/change-password", h.HandleChangePassword)

	// Email verification flow
	r.Get("/verify-email", h.ServeVerifyEmail)
	r.Post("/verify-email", h.HandleVerifyEmailSubmit)
	r.Post("/resend-code", h.HandleResendCode)

	return r
}
