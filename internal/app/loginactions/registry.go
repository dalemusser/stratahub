// internal/app/loginactions/registry.go
package loginactions

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// LoginContext provides user information for client-side login actions.
// This is used instead of auth.CurrentUser because the session middleware
// hasn't run yet during login — the login handler builds this from the
// *models.User it already has.
type LoginContext struct {
	UserID      string
	LoginID     string
	Role        string
	EnabledApps []string
}

// ServerAction runs on the server during login, after session creation.
// It receives the request context and the HTTP request.
// It must not write to the response.
type ServerAction struct {
	Name        string
	Description string
	Run         func(ctx context.Context, r *http.Request) error
}

// ClientAction produces JavaScript to run in the browser after login.
// ShouldRun determines whether this action applies to the current user.
// Script returns the JavaScript code to execute (will be wrapped in
// an async IIFE for isolation).
type ClientAction struct {
	Name        string
	Description string
	ShouldRun   func(lc *LoginContext) bool
	Script      func(lc *LoginContext) string
}

// Registry holds all registered login actions.
type Registry struct {
	serverActions []ServerAction
	clientActions []ClientAction
	logger        *zap.Logger
}

// New creates a new login actions registry.
func New(logger *zap.Logger) *Registry {
	return &Registry{logger: logger}
}

// RegisterServer adds a server-side action to run on login.
func (reg *Registry) RegisterServer(action ServerAction) {
	reg.serverActions = append(reg.serverActions, action)
}

// RegisterClient adds a client-side action to run after login.
func (reg *Registry) RegisterClient(action ClientAction) {
	reg.clientActions = append(reg.clientActions, action)
}

// RunServerActions executes all registered server-side actions.
// Each action runs independently — a failure in one does not affect others.
// Panics are recovered and logged.
func (reg *Registry) RunServerActions(ctx context.Context, r *http.Request) {
	for _, action := range reg.serverActions {
		func() {
			defer func() {
				if rv := recover(); rv != nil {
					reg.logger.Error("login action panicked",
						zap.String("action", action.Name),
						zap.Any("panic", rv))
				}
			}()
			if err := action.Run(ctx, r); err != nil {
				reg.logger.Error("login action failed",
					zap.String("action", action.Name),
					zap.Error(err))
			}
		}()
	}
}

// ClientScripts returns the JavaScript to include in the post-login page.
// Each applicable action's script is wrapped in an async IIFE for isolation.
// Returns empty string if no client actions apply.
func (reg *Registry) ClientScripts(lc *LoginContext) string {
	var scripts []string
	for _, action := range reg.clientActions {
		if action.ShouldRun(lc) {
			js := action.Script(lc)
			if js != "" {
				wrapped := fmt.Sprintf(
					"/* login action: %s */\n(async function() { try { %s } catch(e) { console.error('Login action %s failed:', e); } })();",
					action.Name, js, action.Name,
				)
				scripts = append(scripts, wrapped)
			}
		}
	}
	return strings.Join(scripts, "\n")
}
