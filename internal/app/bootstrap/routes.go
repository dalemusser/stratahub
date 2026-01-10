// internal/app/bootstrap/routes.go
package bootstrap

import (
	"net/http"

	activityfeature "github.com/dalemusser/stratahub/internal/app/features/activity"
	auditlogfeature "github.com/dalemusser/stratahub/internal/app/features/auditlog"
	authgooglefeature "github.com/dalemusser/stratahub/internal/app/features/authgoogle"
	dashboardfeature "github.com/dalemusser/stratahub/internal/app/features/dashboard"
	errorsfeature "github.com/dalemusser/stratahub/internal/app/features/errors"
	groupsfeature "github.com/dalemusser/stratahub/internal/app/features/groups"
	healthfeature "github.com/dalemusser/stratahub/internal/app/features/health"
	heartbeatfeature "github.com/dalemusser/stratahub/internal/app/features/heartbeat"
	homefeature "github.com/dalemusser/stratahub/internal/app/features/home"
	leadersfeature "github.com/dalemusser/stratahub/internal/app/features/leaders"
	loginfeature "github.com/dalemusser/stratahub/internal/app/features/login"
	logoutfeature "github.com/dalemusser/stratahub/internal/app/features/logout"
	profilefeature "github.com/dalemusser/stratahub/internal/app/features/profile"
	materialsfeature "github.com/dalemusser/stratahub/internal/app/features/materials"
	membersfeature "github.com/dalemusser/stratahub/internal/app/features/members"
	organizationsfeature "github.com/dalemusser/stratahub/internal/app/features/organizations"
	pagesfeature "github.com/dalemusser/stratahub/internal/app/features/pages"
	reportsfeature "github.com/dalemusser/stratahub/internal/app/features/reports"
	resourcesfeature "github.com/dalemusser/stratahub/internal/app/features/resources"
	settingsfeature "github.com/dalemusser/stratahub/internal/app/features/settings"
	systemusersfeature "github.com/dalemusser/stratahub/internal/app/features/systemusers"
	uploadcsvfeature "github.com/dalemusser/stratahub/internal/app/features/uploadcsv"
	userinfofeature "github.com/dalemusser/stratahub/internal/app/features/userinfo"
	workspacesfeature "github.com/dalemusser/stratahub/internal/app/features/workspaces"
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/store/oauthstate"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/middleware"
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
	sessionMgr.SetUserFetcher(userstore.NewFetcher(deps.StrataHubMongoDatabase, logger))

	// Initialize and boot the template engine once at startup.
	// Dev mode enables template reloading for faster iteration.
	eng := templates.New(coreCfg.Env == "dev")
	if err := eng.Boot(logger); err != nil {
		logger.Error("template engine boot failed", zap.Error(err))
		return nil, err
	}
	templates.UseEngine(eng, logger)

	// Initialize viewdata with storage for logo URLs.
	viewdata.Init(deps.FileStorage)

	// Create error logger for handlers.
	errLog := errorsfeature.NewErrorLogger(logger)

	// Create audit store and logger for security event tracking.
	auditStore := audit.New(deps.StrataHubMongoDatabase)
	auditConfig := auditlog.Config{
		Auth:  appCfg.AuditLogAuth,
		Admin: appCfg.AuditLogAdmin,
	}
	auditLogger := auditlog.New(auditStore, logger, auditConfig)

	// Create sessions store for activity tracking.
	sessionsStore := sessions.New(deps.StrataHubMongoDatabase)

	r := chi.NewRouter()

	// CORS middleware: must be early in the chain to handle preflight requests.
	// Only active when enable_cors=true in config.
	r.Use(middleware.CORSFromConfig(coreCfg))

	// Workspace middleware: extracts workspace context from host/subdomain.
	// In single-workspace mode, uses the default workspace for all requests.
	// In multi-workspace mode, extracts from subdomain (e.g., mhs.adroit.games).
	wsStore := workspacestore.New(deps.StrataHubMongoDatabase)
	r.Use(workspace.Middleware(appCfg.PrimaryDomain, wsStore, appCfg.MultiWorkspace, logger))

	// Global auth middleware: loads SessionUser into context if logged in.
	// This makes the current user available to all handlers via auth.CurrentUser(r).
	r.Use(sessionMgr.LoadSessionUser)

	// Health check endpoint for load balancers and orchestrators
	healthHandler := healthfeature.NewHandler(deps.StrataHubMongoClient, logger)
	r.Mount("/health", healthfeature.Routes(healthHandler))

	// Static assets with pre-compressed file support (gzip/brotli)
	// /static/* serves files from disk (static directory)
	r.Handle("/static/*", fileserver.Handler("/static", "static"))

	// /assets/* serves embedded assets (bundled into the binary)
	r.Handle("/assets/*", appresources.AssetsHandler("/assets"))

	// Uploaded files (local storage only)
	// When using local storage, serve files from the configured path
	if appCfg.StorageType == "local" || appCfg.StorageType == "" {
		r.Handle(appCfg.StorageLocalURL+"/*", fileserver.Handler(appCfg.StorageLocalURL, appCfg.StorageLocalPath))
	}

	// Public pages
	homeHandler := homefeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/", homefeature.Routes(homeHandler))

	// Dynamic content pages (about, contact, terms, privacy)
	pagesHandler := pagesfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/about", pagesHandler.AboutRouter())
	r.Mount("/contact", pagesHandler.ContactRouter())
	r.Mount("/terms", pagesHandler.TermsRouter())
	r.Mount("/privacy", pagesHandler.PrivacyRouter())
	r.Mount("/pages", pagesfeature.EditRoutes(pagesHandler, sessionMgr))

	// Authentication
	googleEnabled := appCfg.GoogleClientID != "" && appCfg.GoogleClientSecret != ""
	loginHandler := loginfeature.NewHandler(
		deps.StrataHubMongoDatabase,
		sessionMgr,
		errLog,
		deps.Mailer,
		auditLogger,
		sessionsStore,
		workspacestore.New(deps.StrataHubMongoDatabase),
		appCfg.BaseURL,
		appCfg.EmailVerifyExpiry,
		googleEnabled,
		appCfg.MultiWorkspace,
		appCfg.PrimaryDomain,
		logger,
	)
	r.Mount("/login", loginfeature.Routes(loginHandler))

	logoutHandler := logoutfeature.NewHandler(sessionMgr, auditLogger, sessionsStore, logger)
	r.Mount("/logout", logoutfeature.Routes(logoutHandler, sessionMgr))

	// Google OAuth (only mount if configured)
	if appCfg.GoogleClientID != "" && appCfg.GoogleClientSecret != "" {
		oauthStateStore := oauthstate.New(deps.StrataHubMongoDatabase)
		googleHandler := authgooglefeature.NewHandler(
			deps.StrataHubMongoDatabase,
			sessionMgr,
			errLog,
			auditLogger,
			sessionsStore,
			oauthStateStore,
			workspacestore.New(deps.StrataHubMongoDatabase),
			appCfg.GoogleClientID,
			appCfg.GoogleClientSecret,
			appCfg.BaseURL,
			appCfg.MultiWorkspace,
			appCfg.PrimaryDomain,
			logger,
		)
		r.Mount("/auth/google", authgooglefeature.Routes(googleHandler))
		logger.Info("Google OAuth enabled", zap.String("redirect_url", appCfg.BaseURL+"/auth/google/callback"))
	}

	// User profile (any logged-in user)
	profileHandler := profilefeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Route("/profile", func(sr chi.Router) {
		sr.Use(sessionMgr.RequireRole("superadmin", "admin", "analyst", "coordinator", "leader", "member"))
		sr.Mount("/", profilefeature.Routes(profileHandler))
	})

	// Error pages
	errorsHandler := errorsfeature.NewHandler()
	r.Get("/forbidden", errorsHandler.Forbidden)
	r.Get("/unauthorized", errorsHandler.Unauthorized)

	// Role-based dashboards
	dashboardHandler := dashboardfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/dashboard", dashboardfeature.Routes(dashboardHandler, sessionMgr))

	// Organization management
	orgHandler := organizationsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
	r.Mount("/organizations", organizationsfeature.Routes(orgHandler, sessionMgr))

	// Group management
	groupsHandler := groupsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
	r.Mount("/groups", groupsfeature.Routes(groupsHandler, sessionMgr))

	// User management
	leadersHandler := leadersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
	r.Mount("/leaders", leadersfeature.Routes(leadersHandler, sessionMgr))

	membersHandler := membersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
	r.Mount("/members", membersfeature.Routes(membersHandler, sessionMgr))

	// CSV upload (standalone feature accessible from members, groups, organizations)
	uploadCSVHandler := &uploadcsvfeature.Handler{DB: deps.StrataHubMongoDatabase, Log: logger, ErrLog: errLog}
	r.Mount("/upload_csv", uploadcsvfeature.Routes(uploadCSVHandler, sessionMgr))

	sysUsersHandler := systemusersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
	r.Mount("/system-users", systemusersfeature.Routes(sysUsersHandler, sessionMgr))

	// Audit log (admin and coordinator access)
	auditLogHandler := auditlogfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/audit", auditlogfeature.Routes(auditLogHandler, sessionMgr))

	// Workspace management (superadmin only, apex domain)
	workspacesHandler := workspacesfeature.NewHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, appCfg.PrimaryDomain, logger)
	r.Mount("/workspaces", workspacesfeature.Routes(workspacesHandler, sessionMgr))

	// Resource management (admin and member views)
	adminResHandler := resourcesfeature.NewAdminHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, logger)
	r.Mount("/resources", resourcesfeature.AdminRoutes(adminResHandler, sessionMgr))

	activityStore := activity.New(deps.StrataHubMongoDatabase)
	memberResHandler := resourcesfeature.NewMemberHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, activityStore, sessionMgr, logger)
	r.Mount("/member/resources", resourcesfeature.MemberRoutes(memberResHandler, sessionMgr))

	// Material management (admin and leader views)
	adminMatHandler := materialsfeature.NewAdminHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, logger)
	r.Mount("/materials", materialsfeature.AdminRoutes(adminMatHandler, sessionMgr))

	leaderMatHandler := materialsfeature.NewLeaderHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, logger)
	r.Mount("/leader/materials", materialsfeature.LeaderRoutes(leaderMatHandler, sessionMgr))

	// Reports
	reportsHandler := reportsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Mount("/reports", reportsfeature.Routes(reportsHandler, sessionMgr))

	// Site Settings (admin and superadmin)
	settingsHandler := settingsfeature.NewHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, logger)
	r.Route("/settings", func(sr chi.Router) {
		sr.Use(sessionMgr.RequireRole("superadmin", "admin"))
		settingsHandler.MountRoutes(sr)
	})

	// User info API (for games to identify the current player)
	userInfoHandler := userinfofeature.NewHandler()
	userinfofeature.MountRoutes(r, userInfoHandler)

	// Activity dashboard (for leaders to monitor student activity)
	activityHandler := activityfeature.NewHandler(deps.StrataHubMongoDatabase, sessionsStore, activityStore, sessionMgr, errLog, logger)
	r.Mount("/activity", activityfeature.Routes(activityHandler, sessionMgr))

	// Heartbeat API (for activity tracking)
	heartbeatHandler := heartbeatfeature.NewHandler(sessionsStore, activityStore, sessionMgr, logger)
	r.Mount("/api/heartbeat", heartbeatfeature.Routes(heartbeatHandler, sessionMgr))

	return r, nil
}
