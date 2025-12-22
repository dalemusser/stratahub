// internal/app/bootstrap/routes.go
package bootstrap

import (
	"net/http"

	aboutfeature "github.com/dalemusser/stratahub/internal/app/features/about"
	contactfeature "github.com/dalemusser/stratahub/internal/app/features/contact"
	dashboardfeature "github.com/dalemusser/stratahub/internal/app/features/dashboard"
	errorsfeature "github.com/dalemusser/stratahub/internal/app/features/errors"
	groupsfeature "github.com/dalemusser/stratahub/internal/app/features/groups"
	healthfeature "github.com/dalemusser/stratahub/internal/app/features/health"
	homefeature "github.com/dalemusser/stratahub/internal/app/features/home"
	leadersfeature "github.com/dalemusser/stratahub/internal/app/features/leaders"
	loginfeature "github.com/dalemusser/stratahub/internal/app/features/login"
	logoutfeature "github.com/dalemusser/stratahub/internal/app/features/logout"
	membersfeature "github.com/dalemusser/stratahub/internal/app/features/members"
	organizationsfeature "github.com/dalemusser/stratahub/internal/app/features/organizations"
	reportsfeature "github.com/dalemusser/stratahub/internal/app/features/reports"
	resourcesfeature "github.com/dalemusser/stratahub/internal/app/features/resources"
	systemusersfeature "github.com/dalemusser/stratahub/internal/app/features/systemusers"
	termsfeature "github.com/dalemusser/stratahub/internal/app/features/terms"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/pantry/fileserver"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// BuildHandler constructs the root HTTP handler (router) for this WAFFLE app.
//
// WAFFLE calls this after configuration, DB connections, schema setup, and
// any Startup hooks have completed. At this point you have access to:
//   - coreCfg: WAFFLE core configuration (ports, env, timeouts, etc.)
//   - appCfg: app-specific configuration defined in AppConfig
//   - deps: any DB or backend clients bundled in DBDeps
//   - logger: the fully configured zap.Logger for this app
//
// This function should:
//  1. Create a router (chi, standard mux, etc.)
//  2. Mount feature routers for different parts of your application
//  3. Add any additional middleware needed for specific routes
//  4. Return the configured router as an http.Handler
//
// StrataHub initializes the template engine, applies session middleware,
// and mounts feature routers for all application areas: home, login,
// dashboard, groups, members, organizations, resources, and reports.
func BuildHandler(coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) (http.Handler, error) {
	// Create the session manager using app config.
	// Secure cookies are enabled in production mode.
	secure := coreCfg.Env == "prod"
	sessionMgr, err := auth.NewSessionManager(appCfg.SessionKey, appCfg.SessionName, appCfg.SessionDomain, secure, logger)
	if err != nil {
		logger.Error("session manager init failed", zap.Error(err))
		return nil, err
	}

	// Set up the UserFetcher so LoadSessionUser fetches fresh user data on each request.
	// This ensures role changes, disabled accounts, and profile updates take effect immediately.
	sessionMgr.SetUserFetcher(userstore.NewFetcher(deps.StrataHubMongoDatabase))

	// Initialize and boot the template engine once at startup.
	// Dev mode enables template reloading for faster iteration.
	eng := templates.New(coreCfg.Env == "dev")
	if err := eng.Boot(logger); err != nil {
		logger.Error("template engine boot failed", zap.Error(err))
		return nil, err
	}
	templates.UseEngine(eng, logger)

	// Create error logger for handlers.
	errLog := errorsfeature.NewErrorLogger(logger)

	r := chi.NewRouter()

	// Global auth middleware: loads SessionUser into context if logged in.
	// This makes the current user available to all handlers via auth.CurrentUser(r).
	r.Use(sessionMgr.LoadSessionUser)

	// Health check endpoint for load balancers and orchestrators
	healthHandler := healthfeature.NewHandler(deps.StrataHubMongoClient, logger)
	r.Mount("/health", healthfeature.Routes(healthHandler))

	// Static assets with pre-compressed file support (gzip/brotli)
	r.Handle("/static/*", fileserver.Handler("/static", "public"))

	// Public pages
	homeHandler := homefeature.NewHandler(logger)
	r.Mount("/", homefeature.Routes(homeHandler))

	aboutHandler := aboutfeature.NewHandler(logger)
	r.Mount("/about", aboutfeature.Routes(aboutHandler))

	contactHandler := contactfeature.NewHandler(logger)
	r.Mount("/contact", contactfeature.Routes(contactHandler))

	termsHandler := termsfeature.NewHandler(logger)
	r.Mount("/terms", termsfeature.Routes(termsHandler))

	// Authentication
	loginHandler := loginfeature.NewHandler(deps.StrataHubMongoDatabase, sessionMgr, errLog, logger)
	r.Mount("/login", loginfeature.Routes(loginHandler))

	logoutHandler := logoutfeature.NewHandler(sessionMgr, logger)
	r.Mount("/logout", logoutfeature.Routes(logoutHandler, sessionMgr))

	// Error pages
	errorsHandler := errorsfeature.NewHandler()
	r.Get("/forbidden", errorsHandler.Forbidden)
	r.Get("/unauthorized", errorsHandler.Unauthorized)

	// Role-based dashboards
	dashboardHandler := dashboardfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/dashboard", dashboardfeature.Routes(dashboardHandler, sessionMgr))

	// Organization management
	orgHandler := organizationsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/organizations", organizationsfeature.Routes(orgHandler, sessionMgr))

	// Group management
	groupsHandler := groupsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/groups", groupsfeature.Routes(groupsHandler, sessionMgr))

	// User management
	leadersHandler := leadersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/leaders", leadersfeature.Routes(leadersHandler, sessionMgr))

	membersHandler := membersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/members", membersfeature.Routes(membersHandler, sessionMgr))

	sysUsersHandler := systemusersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/system-users", systemusersfeature.Routes(sysUsersHandler, sessionMgr))

	// Resource management (admin and member views)
	adminResHandler := resourcesfeature.NewAdminHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/resources", resourcesfeature.AdminRoutes(adminResHandler, sessionMgr))

	memberResHandler := resourcesfeature.NewMemberHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/member/resources", resourcesfeature.MemberRoutes(memberResHandler, sessionMgr))

	// Reports
	reportsHandler := reportsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/reports", reportsfeature.Routes(reportsHandler, sessionMgr))

	return r, nil
}
