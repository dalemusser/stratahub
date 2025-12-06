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
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/templates"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// BuildHandler constructs the HTTP handler for the service.
func BuildHandler(coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) (http.Handler, error) {
	// Initialize and boot the template engine once at startup.
	eng := templates.New(coreCfg.Env == "dev")
	if err := eng.Boot(logger); err != nil {
		logger.Error("template engine boot failed", zap.Error(err))
		return nil, err
	}
	templates.UseEngine(eng, logger)

	r := chi.NewRouter()

	// Global auth middleware: loads SessionUser into context if logged in.
	r.Use(auth.LoadSessionUser)

	// Health feature
	healthHandler := healthfeature.NewHandler(deps.StrataHubMongoClient, logger)
	r.Mount("/health", healthfeature.Routes(healthHandler))

	// Home feature
	homeHandler := homefeature.NewHandler(logger)
	r.Mount("/", homefeature.Routes(homeHandler))

	// About feature
	aboutHandler := aboutfeature.NewHandler(logger)
	r.Mount("/about", aboutfeature.Routes(aboutHandler))

	contactHandler := contactfeature.NewHandler(logger)
	r.Mount("/contact", contactfeature.Routes(contactHandler))

	// Terms feature
	termsHandler := termsfeature.NewHandler(logger)
	r.Mount("/terms", termsfeature.Routes(termsHandler))

	// Login feature
	loginHandler := loginfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/login", loginfeature.Routes(loginHandler))

	// Logout feature
	logoutHandler := logoutfeature.NewHandler(logger)
	r.Mount("/logout", logoutfeature.Routes(logoutHandler))

	// Dashboard feature
	dashboardHandler := dashboardfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/dashboard", dashboardfeature.Routes(dashboardHandler))

	// Leaders feature
	leadersHandler := leadersfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/leaders", leadersfeature.Routes(leadersHandler))

	// Errors feature
	errorsHandler := errorsfeature.NewHandler()
	r.Get("/forbidden", errorsHandler.Forbidden)
	r.Get("/unauthorized", errorsHandler.Unauthorized)

	// Groups feature
	groupsHandler := groupsfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/groups", groupsfeature.Routes(groupsHandler))

	// Members feature
	membersHandler := membersfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/members", membersfeature.Routes(membersHandler))

	// System Users feature
	sysUsersHandler := systemusersfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/system-users", systemusersfeature.Routes(sysUsersHandler))

	// Organizations feature
	orgHandler := organizationsfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/organizations", organizationsfeature.Routes(orgHandler))

	// Resource Admin feature
	adminResHandler := resourcesfeature.NewAdminHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/resources", resourcesfeature.AdminRoutes(adminResHandler))

	// Member Resources feature
	memberResHandler := resourcesfeature.NewMemberHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/member/resources", resourcesfeature.MemberRoutes(memberResHandler))

	reportsHandler := reportsfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/reports", reportsfeature.Routes(reportsHandler))

	return r, nil
}
