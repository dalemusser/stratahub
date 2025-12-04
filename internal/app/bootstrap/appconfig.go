// internal/app/bootstrap/appconfig.go
package bootstrap

// AppConfig holds service-specific configuration for this WAFFLE app.
// Extend this struct as your app grows.
type AppConfig struct {
	Greeting string

	StrataHubMongoURI      string
	StrataHubMongoDatabase string

	SessionKey    string
	SessionDomain string
}
