# StrataHub Configuration Guide

StrataHub uses a layered configuration system powered by the Waffle framework. Configuration can be provided through config files, environment variables, or command-line flags.

## Configuration Precedence

Settings are merged with the following precedence (highest wins):

1. Command-line flags
2. Environment variables
3. Config files (`config.toml`, `config.yaml`, or `config.json`)
4. Default values

## Config File Location

Place your config file in the application's working directory:

- `config.toml` (recommended)
- `config.yaml`
- `config.json`

## Configuration Sections

StrataHub configuration is divided into two sections:

1. **Waffle Core Configuration** - Framework-level settings (HTTP, TLS, logging, etc.)
2. **StrataHub Application Configuration** - App-specific settings (MongoDB, sessions, etc.)

---

## Waffle Core Configuration

### Runtime Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `env` | string | `"dev"` | Runtime environment: `"dev"` or `"prod"` |
| `log_level` | string | `"info"` | Logging level: `debug`, `info`, `warn`, `error` |

### HTTP Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `http_port` | int | `8080` | HTTP server port |
| `https_port` | int | `8443` | HTTPS server port |
| `use_https` | bool | `false` | Enable HTTPS |
| `read_timeout` | duration | `"30s"` | Max time to read request |
| `read_header_timeout` | duration | `"10s"` | Max time to read request headers |
| `write_timeout` | duration | `"30s"` | Max time to write response |
| `idle_timeout` | duration | `"120s"` | Max time for keep-alive connections |
| `shutdown_timeout` | duration | `"30s"` | Graceful shutdown timeout |

### TLS Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cert_file` | string | `""` | Path to TLS certificate file |
| `key_file` | string | `""` | Path to TLS private key file |
| `use_lets_encrypt` | bool | `false` | Enable automatic Let's Encrypt certificates |
| `lets_encrypt_email` | string | `""` | Email for Let's Encrypt registration |
| `lets_encrypt_cache_dir` | string | `""` | Directory to cache Let's Encrypt certificates |
| `lets_encrypt_challenge` | string | `"http-01"` | ACME challenge type: `"http-01"` or `"dns-01"` |
| `domain` | string | `""` | Domain for TLS certificate |
| `route53_hosted_zone_id` | string | `""` | AWS Route53 zone ID (for dns-01 challenge) |
| `acme_directory_url` | string | `""` | Custom ACME directory URL |

### CORS Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable_cors` | bool | `false` | Enable CORS headers |
| `cors_allowed_origins` | []string | `[]` | Allowed origins |
| `cors_allowed_methods` | []string | `[]` | Allowed HTTP methods |
| `cors_allowed_headers` | []string | `[]` | Allowed request headers |
| `cors_exposed_headers` | []string | `[]` | Headers exposed to browser |
| `cors_allow_credentials` | bool | `false` | Allow credentials |
| `cors_max_age` | int | `0` | Preflight cache duration (seconds) |

### Database & Misc Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `db_connect_timeout` | duration | `"10s"` | Database connection timeout |
| `index_boot_timeout` | duration | `"30s"` | Index creation timeout at startup |
| `max_request_body_bytes` | int64 | `10485760` | Max request body size (bytes) |
| `enable_compression` | bool | `true` | Enable response compression |
| `compression_level` | int | `5` | Compression level (1-9) |

---

## StrataHub Application Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mongo_uri` | string | `"mongodb://localhost:27017"` | MongoDB connection URI |
| `mongo_database` | string | `"strata_hub"` | MongoDB database name |
| `session_key` | string | *(dev default)* | Secret key for signing session cookies |
| `session_name` | string | `"stratahub-session"` | Session cookie name |
| `session_domain` | string | `""` | Session cookie domain (blank = current host) |

> **Security Note:** The `session_key` must be a strong, random string in production. Never use the default development key in production environments.

---

## Environment Variables

Configuration can be overridden using environment variables:

- **Waffle core settings:** Use `WAFFLE_` prefix (e.g., `WAFFLE_HTTP_PORT`, `WAFFLE_LOG_LEVEL`)
- **StrataHub app settings:** Use `STRATAHUB_` prefix (e.g., `STRATAHUB_MONGO_URI`)

Examples:
```bash
export WAFFLE_HTTP_PORT=3000
export WAFFLE_LOG_LEVEL=debug
export STRATAHUB_MONGO_URI="mongodb://user:pass@dbserver:27017"
export STRATAHUB_SESSION_KEY="your-production-secret-key"
```

---

## Example: Local Development Configuration

This example is for a developer running the full stack locally on macOS or Linux with MongoDB installed on localhost.

### Prerequisites

- MongoDB running on `localhost:27017`
- Database `strata_hub` (will be created automatically)

### config.toml

```toml
# StrataHub Local Development Configuration
# Place this file in the application root directory

# =============================================================================
# Waffle Core Configuration
# =============================================================================

# Runtime
env = "dev"
log_level = "debug"

# HTTP Server
http_port = 8080
https_port = 8443
use_https = false

# Server Timeouts
read_timeout = "30s"
read_header_timeout = "10s"
write_timeout = "30s"
idle_timeout = "120s"
shutdown_timeout = "30s"

# TLS (disabled for local dev)
# cert_file = ""
# key_file = ""
# use_lets_encrypt = false

# CORS (not needed for same-origin local dev)
enable_cors = false

# Database Timeouts
db_connect_timeout = "10s"
index_boot_timeout = "30s"

# HTTP Behavior
max_request_body_bytes = 10485760  # 10 MB

# Compression
enable_compression = true
compression_level = 5

# =============================================================================
# StrataHub Application Configuration
# =============================================================================

# MongoDB - local instance
mongo_uri = "mongodb://localhost:27017"
mongo_database = "strata_hub"

# Session Configuration
# WARNING: Change session_key in production!
session_key = "dev-only-change-me-please-0123456789ABCDEF"
session_name = "stratahub-session"
session_domain = ""
```

### Running the Application

```bash
# Start MongoDB (if not already running)
brew services start mongodb-community  # macOS with Homebrew
# or
mongod --dbpath /path/to/data          # Manual start

# Run StrataHub
go run ./cmd/stratahub
```

The application will be available at `http://localhost:8080`.

---

## Production Considerations

When deploying to production:

1. **Set `env = "prod"`** - Enables production optimizations
2. **Use a strong `session_key`** - Generate with: `openssl rand -hex 32`
3. **Enable HTTPS** - Either with your own certificates or Let's Encrypt
4. **Use environment variables for secrets** - Don't commit secrets to config files
5. **Set appropriate timeouts** - Adjust based on your infrastructure
6. **Configure CORS if needed** - For API access from different origins

### Example Production Environment Variables

```bash
export WAFFLE_ENV=prod
export WAFFLE_LOG_LEVEL=info
export WAFFLE_USE_HTTPS=true
export WAFFLE_USE_LETS_ENCRYPT=true
export WAFFLE_LETS_ENCRYPT_EMAIL=admin@yourdomain.com
export WAFFLE_DOMAIN=yourdomain.com

export STRATAHUB_MONGO_URI="mongodb://user:password@mongo.yourdomain.com:27017/strata_hub?authSource=admin"
export STRATAHUB_SESSION_KEY="$(openssl rand -hex 32)"
export STRATAHUB_SESSION_DOMAIN=".yourdomain.com"
```
