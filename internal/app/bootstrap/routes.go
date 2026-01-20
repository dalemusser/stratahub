// internal/app/bootstrap/routes.go
package bootstrap

import (
	"context"
	"net/http"

	"github.com/gorilla/csrf"

	activityfeature "github.com/dalemusser/stratahub/internal/app/features/activity"
	announcementsfeature "github.com/dalemusser/stratahub/internal/app/features/announcements"
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
	statusfeature "github.com/dalemusser/stratahub/internal/app/features/status"
	systemusersfeature "github.com/dalemusser/stratahub/internal/app/features/systemusers"
	uploadcsvfeature "github.com/dalemusser/stratahub/internal/app/features/uploadcsv"
	userinfofeature "github.com/dalemusser/stratahub/internal/app/features/userinfo"
	workspacesfeature "github.com/dalemusser/stratahub/internal/app/features/workspaces"
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/store/activity"
	announcementstore "github.com/dalemusser/stratahub/internal/app/store/announcement"
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
	sessionMgr, err := auth.NewSessionManager(appCfg.SessionKey, appCfg.SessionName, appCfg.SessionDomain, appCfg.SessionMaxAge, secure, logger)
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

	// Set up announcement loader for viewdata (loads active announcements for all pages)
	annStore := announcementstore.New(deps.StrataHubMongoDatabase)
	viewdata.SetAnnouncementLoader(func(ctx context.Context) []viewdata.AnnouncementVM {
		active, _ := annStore.GetActive(ctx)
		vms := make([]viewdata.AnnouncementVM, len(active))
		for i, a := range active {
			vms[i] = viewdata.AnnouncementVM{
				ID:          a.ID.Hex(),
				Title:       a.Title,
				Content:     a.Content,
				Type:        string(a.Type),
				Dismissible: a.Dismissible,
			}
		}
		return vms
	})

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

	// CSRF protection: validates token on all POST/PUT/PATCH/DELETE requests.
	// Token is injected into templates via BaseVM.CSRFToken.
	// HTMX requests send token via X-CSRF-Token header.
	csrfOpts := []csrf.Option{
		csrf.Secure(secure),
		csrf.Path("/"),
		csrf.CookieName("csrf_token"),
		csrf.FieldName("csrf_token"),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Warn("CSRF validation failed",
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
				zap.String("reason", csrf.FailureReason(r).Error()),
			)
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Error(w, "CSRF token invalid or missing", http.StatusForbidden)
		})),
	}
	// In multi-workspace mode, set CSRF cookie domain to match session domain.
	// This ensures the CSRF cookie is shared across subdomains (e.g., mhs.adroit.games, adroit.games).
	if appCfg.SessionDomain != "" {
		csrfOpts = append(csrfOpts, csrf.Domain(appCfg.SessionDomain))
	}
	// In dev mode, trust localhost origins for CSRF validation.
	// gorilla/csrf validates the Origin header's Host against TrustedOrigins (not the full URL).
	if !secure {
		csrfOpts = append(csrfOpts, csrf.TrustedOrigins([]string{
			"localhost:8080",
			"localhost:3000",
			"127.0.0.1:8080",
			"127.0.0.1:3000",
		}))
	}
	csrfMiddleware := csrf.Protect([]byte(appCfg.CSRFKey), csrfOpts...)
	r.Use(csrfMiddleware)

	// Apex domain protection: redirect non-superadmins to their workspace domain.
	// This prevents workspace users from accidentally accessing apex via shared cookies.
	if appCfg.MultiWorkspace {
		r.Use(workspace.RedirectNonSuperadminFromApex(wsStore, appCfg.PrimaryDomain, logger))
	}

	// Health check endpoint for load balancers and orchestrators
	healthHandler := healthfeature.NewHandler(deps.StrataHubMongoClient, appCfg.BaseURL, logger)
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

	// Dynamic content pages (about, contact, terms, privacy) - require workspace context
	// These pages show workspace-specific content, so they shouldn't be accessible from apex
	pagesHandler := pagesfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
	r.Group(func(pr chi.Router) {
		pr.Use(workspace.RequireWorkspace)
		pr.Mount("/about", pagesHandler.AboutRouter())
		pr.Mount("/contact", pagesHandler.ContactRouter())
		pr.Mount("/terms", pagesHandler.TermsRouter())
		pr.Mount("/privacy", pagesHandler.PrivacyRouter())
		pr.Mount("/pages", pagesfeature.EditRoutes(pagesHandler, sessionMgr))
	})

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
	r.Get("/apex-denied", errorsHandler.ApexDenied)

	// Workspace management (superadmin only, apex domain - no workspace required)
	workspacesHandler := workspacesfeature.NewHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, appCfg.PrimaryDomain, logger)
	r.Mount("/workspaces", workspacesfeature.Routes(workspacesHandler, sessionMgr))

	// User info API (for games to identify the current player - no workspace required for backward compat)
	userInfoHandler := userinfofeature.NewHandler()
	userinfofeature.MountRoutes(r, userInfoHandler)

	// Activity store - used by multiple features
	activityStore := activity.New(deps.StrataHubMongoDatabase)

	// Workspace-scoped features - require workspace context (redirects to /workspaces if on apex)
	r.Group(func(wsr chi.Router) {
		wsr.Use(workspace.RequireWorkspace)

		// Role-based dashboards
		dashboardHandler := dashboardfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
		wsr.Mount("/dashboard", dashboardfeature.Routes(dashboardHandler, sessionMgr))

		// Active sessions dashboard (admin only)
		sessionsHandler := dashboardfeature.NewSessionsHandler(deps.StrataHubMongoDatabase, sessionsStore, logger)
		wsr.Mount("/dashboard/sessions", dashboardfeature.SessionsRoutes(sessionsHandler, sessionMgr))

		// Organization management
		orgHandler := organizationsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
		wsr.Mount("/organizations", organizationsfeature.Routes(orgHandler, sessionMgr))

		// Group management
		groupsHandler := groupsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
		wsr.Mount("/groups", groupsfeature.Routes(groupsHandler, sessionMgr))

		// User management
		leadersHandler := leadersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
		wsr.Mount("/leaders", leadersfeature.Routes(leadersHandler, sessionMgr))

		membersHandler := membersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
		wsr.Mount("/members", membersfeature.Routes(membersHandler, sessionMgr))

		// CSV upload (standalone feature accessible from members, groups, organizations)
		uploadCSVHandler := &uploadcsvfeature.Handler{DB: deps.StrataHubMongoDatabase, Log: logger, ErrLog: errLog}
		wsr.Mount("/upload_csv", uploadcsvfeature.Routes(uploadCSVHandler, sessionMgr))

		sysUsersHandler := systemusersfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, auditLogger, logger)
		wsr.Mount("/system-users", systemusersfeature.Routes(sysUsersHandler, sessionMgr))

		// Audit log (admin and coordinator access)
		auditLogHandler := auditlogfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
		wsr.Mount("/audit", auditlogfeature.Routes(auditLogHandler, sessionMgr))

		// Resource management (admin and member views)
		adminResHandler := resourcesfeature.NewAdminHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, logger)
		wsr.Mount("/resources", resourcesfeature.AdminRoutes(adminResHandler, sessionMgr))

		memberResHandler := resourcesfeature.NewMemberHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, activityStore, sessionsStore, sessionMgr, logger)
		wsr.Mount("/member/resources", resourcesfeature.MemberRoutes(memberResHandler, sessionMgr))

		// Material management (admin and leader views)
		adminMatHandler := materialsfeature.NewAdminHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, auditLogger, logger)
		wsr.Mount("/materials", materialsfeature.AdminRoutes(adminMatHandler, sessionMgr))

		leaderMatHandler := materialsfeature.NewLeaderHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, logger)
		wsr.Mount("/leader/materials", materialsfeature.LeaderRoutes(leaderMatHandler, sessionMgr))

		// Reports
		reportsHandler := reportsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
		wsr.Mount("/reports", reportsfeature.Routes(reportsHandler, sessionMgr))

		// Site Settings (admin and superadmin)
		settingsHandler := settingsfeature.NewHandler(deps.StrataHubMongoDatabase, deps.FileStorage, errLog, logger)
		wsr.Route("/settings", func(sr chi.Router) {
			sr.Use(sessionMgr.RequireRole("superadmin", "admin"))
			settingsHandler.MountRoutes(sr)
		})

		// Activity dashboard (for leaders to monitor member activity)
		activityHandler := activityfeature.NewHandler(deps.StrataHubMongoDatabase, sessionsStore, activityStore, sessionMgr, errLog, logger)
		wsr.Mount("/activity", activityfeature.Routes(activityHandler, sessionMgr))

		// Announcements management (admin only)
		announcementsHandler := announcementsfeature.NewHandler(deps.StrataHubMongoDatabase, errLog, logger)
		wsr.Route("/announcements", func(sr chi.Router) {
			sr.Use(sessionMgr.RequireRole("admin"))
			announcementsHandler.MountRoutes(sr)
		})

		// User-facing announcements view (authenticated users)
		wsr.Mount("/my-announcements", announcementsfeature.ViewRoutes(announcementsHandler, sessionMgr))
	})

	// System status page (admin only - no workspace required)
	statusAppCfg := statusfeature.AppConfig{
		MongoURI:                  appCfg.MongoURI,
		MongoDatabase:             appCfg.MongoDatabase,
		MongoMaxPoolSize:          appCfg.MongoMaxPoolSize,
		MongoMinPoolSize:          appCfg.MongoMinPoolSize,
		SessionKey:                appCfg.SessionKey,
		SessionName:               appCfg.SessionName,
		SessionDomain:             appCfg.SessionDomain,
		SessionMaxAge:             appCfg.SessionMaxAge,
		IdleLogoutEnabled:         appCfg.IdleLogoutEnabled,
		IdleLogoutTimeout:         appCfg.IdleLogoutTimeout,
		IdleLogoutWarning:         appCfg.IdleLogoutWarning,
		CSRFKey:                   appCfg.CSRFKey,
		StorageType:               appCfg.StorageType,
		StorageLocalPath:          appCfg.StorageLocalPath,
		StorageLocalURL:           appCfg.StorageLocalURL,
		StorageS3Region:           appCfg.StorageS3Region,
		StorageS3Bucket:           appCfg.StorageS3Bucket,
		StorageS3Prefix:           appCfg.StorageS3Prefix,
		StorageCFURL:              appCfg.StorageCFURL,
		StorageCFKeyPairID:        appCfg.StorageCFKeyPairID,
		StorageCFKeyPath:          appCfg.StorageCFKeyPath,
		MailSMTPHost:              appCfg.MailSMTPHost,
		MailSMTPPort:              appCfg.MailSMTPPort,
		MailSMTPUser:              appCfg.MailSMTPUser,
		MailSMTPPass:              appCfg.MailSMTPPass,
		MailFrom:                  appCfg.MailFrom,
		MailFromName:              appCfg.MailFromName,
		BaseURL:                   appCfg.BaseURL,
		EmailVerifyExpiry:         appCfg.EmailVerifyExpiry,
		AuditLogAuth:              appCfg.AuditLogAuth,
		AuditLogAdmin:             appCfg.AuditLogAdmin,
		GoogleClientID:            appCfg.GoogleClientID,
		GoogleClientSecret:        appCfg.GoogleClientSecret,
		MultiWorkspace:            appCfg.MultiWorkspace,
		PrimaryDomain:             appCfg.PrimaryDomain,
		DefaultWorkspaceName:      appCfg.DefaultWorkspaceName,
		DefaultWorkspaceSubdomain: appCfg.DefaultWorkspaceSubdomain,
		SuperAdminEmail:           appCfg.SuperAdminEmail,
	}
	statusHandler := statusfeature.NewHandler(deps.StrataHubMongoClient, appCfg.BaseURL, coreCfg, statusAppCfg, logger)
	r.Mount("/admin/status", statusfeature.Routes(statusHandler, sessionMgr))

	// Heartbeat API (for activity tracking - no workspace required for cross-domain tracking)
	heartbeatHandler := heartbeatfeature.NewHandler(sessionsStore, activityStore, sessionMgr, logger)
	heartbeatHandler.SetIdleLogoutConfig(appCfg.IdleLogoutEnabled, appCfg.IdleLogoutTimeout, appCfg.IdleLogoutWarning)
	r.Mount("/api/heartbeat", heartbeatfeature.Routes(heartbeatHandler, sessionMgr))

	// 404 catch-all for unmatched routes
	r.NotFound(errorsHandler.NotFound)

	return r, nil
}
