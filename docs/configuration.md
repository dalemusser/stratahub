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
| `domain` | string | `""` | Single domain for TLS certificate (backward compatible) |
| `domains` | []string | `[]` | Multiple domains for TLS certificate (e.g., `["example.com", "*.example.com"]`) |
| `route53_hosted_zone_id` | string | `""` | AWS Route53 zone ID (for dns-01 challenge) |
| `acme_directory_url` | string | `""` | Custom ACME directory URL |

> **Note:** Use either `domain` (single domain) or `domains` (multiple domains), not both. Wildcard certificates (e.g., `*.example.com`) require `lets_encrypt_challenge = "dns-01"`.

#### Multi-Domain and Wildcard Certificates

For certificates covering multiple domains (e.g., apex domain + wildcard), use the `domains` array with DNS-01 challenge:

```toml
# Multi-domain certificate with wildcard
use_https = true
use_lets_encrypt = true
lets_encrypt_email = "admin@example.com"
lets_encrypt_challenge = "dns-01"
domains = ["example.com", "*.example.com"]
route53_hosted_zone_id = "Z1234567890ABC"
```

This configuration:
- Covers both `example.com` and all subdomains (`*.example.com`)
- Uses DNS-01 challenge via AWS Route 53 (required for wildcards)
- Automatically renews certificates 30 days before expiry via background renewal

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

## Email/SMTP Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mail_smtp_host` | string | `"localhost"` | SMTP server hostname |
| `mail_smtp_port` | int | `1025` | SMTP server port |
| `mail_smtp_user` | string | `""` | SMTP username |
| `mail_smtp_pass` | string | `""` | SMTP password |
| `mail_from` | string | `"noreply@stratahub.com"` | From email address |
| `mail_from_name` | string | `"StrataHub"` | From display name |
| `base_url` | string | `"http://localhost:3000"` | Base URL for magic links |
| `email_verify_expiry` | duration | `"10m"` | Email verification code/link expiry (e.g., 10m, 1h, 90s) |

### Email Configuration for Development

For local development, use [Mailpit](https://github.com/axllent/mailpit) to capture emails:

```bash
# Install Mailpit (macOS)
brew install mailpit

# Run Mailpit
mailpit
# or
brew services start mailpit
```

- **SMTP**: localhost:1025 (default, no config needed)
- **Web UI**: http://localhost:8025

### Email Configuration for Production

For production, configure a real SMTP server:

```toml
mail_smtp_host = "email-smtp.us-east-1.amazonaws.com"
mail_smtp_port = 587
mail_smtp_user = "your-ses-smtp-user"
mail_smtp_pass = "your-ses-smtp-password"
mail_from = "noreply@yourdomain.com"
mail_from_name = "Your App Name"
base_url = "https://yourdomain.com"
email_verify_expiry = "10m"  # or "15m", "1h", etc.
```

Or via environment variables:

```bash
export STRATAHUB_MAIL_SMTP_HOST=email-smtp.us-east-1.amazonaws.com
export STRATAHUB_MAIL_SMTP_PORT=587
export STRATAHUB_MAIL_SMTP_USER=your-ses-smtp-user
export STRATAHUB_MAIL_SMTP_PASS=your-ses-smtp-password
export STRATAHUB_MAIL_FROM=noreply@yourdomain.com
export STRATAHUB_MAIL_FROM_NAME="Your App Name"
export STRATAHUB_BASE_URL=https://yourdomain.com
export STRATAHUB_EMAIL_VERIFY_EXPIRY=10m
```

---

## File Storage Configuration

StrataHub supports two storage backends for uploaded files (e.g., Materials):

1. **Local storage** - Files stored on the local filesystem and served by the application
2. **S3/CloudFront** - Files stored in AWS S3 with signed CloudFront URLs for secure delivery

### Storage Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `storage_type` | string | `"local"` | Storage backend: `"local"` or `"s3"` |
| `storage_local_path` | string | `"./uploads/materials"` | Local filesystem path for uploaded files |
| `storage_local_url` | string | `"/files/materials"` | URL prefix for serving local files |

### S3/CloudFront Settings

Required when `storage_type = "s3"`:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `storage_s3_region` | string | `""` | AWS region (e.g., `"us-east-1"`) |
| `storage_s3_bucket` | string | `""` | S3 bucket name |
| `storage_s3_prefix` | string | `"materials/"` | Key prefix for uploaded files |
| `storage_cf_url` | string | `""` | CloudFront distribution URL (e.g., `"https://d1234.cloudfront.net"`) |
| `storage_cf_keypair_id` | string | `""` | CloudFront key pair ID for signed URLs |
| `storage_cf_key_path` | string | `""` | Path to CloudFront private key file (.pem) |

### Local Storage (Development)

Local storage is the default and requires no additional configuration. Files are stored in the `storage_local_path` directory and served at the `storage_local_url` prefix.

```toml
# Local storage (default - no config needed, or explicit):
storage_type = "local"
storage_local_path = "./uploads/materials"
storage_local_url = "/files/materials"
```

The application automatically creates the storage directory if it doesn't exist.

### S3/CloudFront Storage (Production)

For production deployments, S3 with CloudFront provides:

- Scalable, durable file storage
- Global CDN distribution
- Time-limited signed URLs for secure access
- Reduced load on the application server

#### AWS Setup Requirements

1. **S3 Bucket** - Create a bucket for file storage (can be private)
2. **CloudFront Distribution** - Create a distribution with the S3 bucket as origin
3. **CloudFront Key Pair** - Create a key pair for signing URLs (in AWS account settings, not IAM)
4. **IAM Credentials** - The application needs `s3:PutObject`, `s3:GetObject`, and `s3:DeleteObject` permissions

#### Configuration Example

```toml
# S3/CloudFront storage
storage_type = "s3"
storage_s3_region = "us-east-1"
storage_s3_bucket = "my-stratahub-files"
storage_s3_prefix = "materials/"
storage_cf_url = "https://d1234abcd.cloudfront.net"
storage_cf_keypair_id = "APKAEIBAERJR2EXAMPLE"
storage_cf_key_path = "/etc/stratahub/cloudfront-private-key.pem"
```

#### Environment Variables for S3/CloudFront

For production, use environment variables for sensitive configuration:

```bash
export STRATAHUB_STORAGE_TYPE=s3
export STRATAHUB_STORAGE_S3_REGION=us-east-1
export STRATAHUB_STORAGE_S3_BUCKET=my-stratahub-files
export STRATAHUB_STORAGE_S3_PREFIX=materials/
export STRATAHUB_STORAGE_CF_URL=https://d1234abcd.cloudfront.net
export STRATAHUB_STORAGE_CF_KEYPAIR_ID=APKAEIBAERJR2EXAMPLE
export STRATAHUB_STORAGE_CF_KEY_PATH=/etc/stratahub/cloudfront-private-key.pem

# AWS credentials (if not using IAM roles)
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
export AWS_REGION=us-east-1
```

> **Note:** When running on AWS infrastructure (EC2, ECS, Lambda), use IAM roles instead of static credentials for better security.

---

## Setting Up AWS S3 and CloudFront for File Storage

This section provides step-by-step instructions for configuring AWS S3 and CloudFront to work with StrataHub's file storage system.

### Overview

The architecture uses:
- **S3** - Private bucket for secure file storage
- **CloudFront** - CDN for fast, global file delivery with signed URLs
- **Origin Access Control (OAC)** - Secure connection between CloudFront and S3 (no public bucket access)
- **Signed URLs** - Time-limited access to files for authenticated users

### Step 1: Create an S3 Bucket

1. Go to **S3** in the AWS Console
2. Click **Create bucket**
3. Configure the bucket:

| Setting | Value |
|---------|-------|
| Bucket name | Choose a unique name (e.g., `stratahub-files-prod`) |
| AWS Region | Select your preferred region (e.g., `us-east-1`) |
| Object Ownership | ACLs disabled (recommended) |
| Block Public Access | **Block all public access** (checked) |
| Bucket Versioning | Enable (recommended for data protection) |
| Default encryption | SSE-S3 or SSE-KMS |

4. Click **Create bucket**

> **Important:** Keep the bucket private. CloudFront will access it via Origin Access Control, not public URLs.

### Step 2: Create a CloudFront Distribution

1. Go to **CloudFront** in the AWS Console
2. Click **Create distribution**
3. Configure the origin:

| Setting | Value |
|---------|-------|
| Origin domain | Select your S3 bucket from the dropdown |
| Origin path | Leave empty (prefix is handled by StrataHub) |
| Name | Auto-generated or custom name |
| Origin access | **Origin access control settings (recommended)** |

4. Click **Create new OAC** when prompted:

| Setting | Value |
|---------|-------|
| Name | `stratahub-oac` (or your preference) |
| Signing behavior | Sign requests (recommended) |
| Origin type | S3 |

5. Configure default cache behavior:

| Setting | Value |
|---------|-------|
| Viewer protocol policy | Redirect HTTP to HTTPS |
| Allowed HTTP methods | GET, HEAD |
| Restrict viewer access | **Yes** |
| Trusted authorization type | **Trusted key groups** (configure in Step 3) |

6. Configure distribution settings:

| Setting | Value |
|---------|-------|
| Price class | Choose based on your needs |
| Alternate domain name (CNAME) | Optional: your custom domain |
| Custom SSL certificate | Required if using custom domain |
| Default root object | Leave empty |
| Standard logging | Enable (recommended) |

7. Click **Create distribution**

8. **Copy the S3 bucket policy** that CloudFront provides and apply it to your bucket (see Step 4)

### Step 3: Create a CloudFront Key Pair for Signed URLs

CloudFront signed URLs require a public/private key pair. This is different from IAM credentials.

#### Generate the Key Pair

```bash
# Generate a 2048-bit RSA private key
openssl genrsa -out cloudfront-private-key.pem 2048

# Extract the public key
openssl rsa -pubout -in cloudfront-private-key.pem -out cloudfront-public-key.pem
```

#### Add the Public Key to CloudFront

1. Go to **CloudFront** → **Key management** → **Public keys**
2. Click **Create public key**
3. Enter a name (e.g., `stratahub-signing-key`)
4. Paste the contents of `cloudfront-public-key.pem`
5. Click **Create public key**
6. **Copy the Key ID** - you'll need this for `storage_cf_keypair_id`

#### Create a Key Group

1. Go to **CloudFront** → **Key management** → **Key groups**
2. Click **Create key group**
3. Enter a name (e.g., `stratahub-key-group`)
4. Select the public key you just created
5. Click **Create key group**

#### Update the Distribution

1. Go to your CloudFront distribution → **Behaviors** tab
2. Edit the default behavior
3. Under **Restrict viewer access**, select **Yes**
4. Under **Trusted key groups**, select the key group you created
5. Save changes

#### Secure the Private Key

Store the private key securely on your server:

```bash
# Copy to a secure location
sudo mkdir -p /etc/stratahub
sudo cp cloudfront-private-key.pem /etc/stratahub/
sudo chmod 600 /etc/stratahub/cloudfront-private-key.pem
sudo chown stratahub:stratahub /etc/stratahub/cloudfront-private-key.pem

# Remove the local copy
rm cloudfront-private-key.pem cloudfront-public-key.pem
```

### Step 4: Configure S3 Bucket Policy

Apply this policy to allow CloudFront to access your S3 bucket. Replace the placeholders with your values.

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowCloudFrontServicePrincipal",
            "Effect": "Allow",
            "Principal": {
                "Service": "cloudfront.amazonaws.com"
            },
            "Action": "s3:GetObject",
            "Resource": "arn:aws:s3:::YOUR-BUCKET-NAME/*",
            "Condition": {
                "StringEquals": {
                    "AWS:SourceArn": "arn:aws:cloudfront::YOUR-ACCOUNT-ID:distribution/YOUR-DISTRIBUTION-ID"
                }
            }
        }
    ]
}
```

To apply the policy:
1. Go to **S3** → Your bucket → **Permissions** tab
2. Edit **Bucket policy**
3. Paste the policy (with your values)
4. Save changes

### Step 5: Create IAM Policy for StrataHub

Create an IAM policy that grants StrataHub the minimum permissions needed:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "StrataHubS3Access",
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:GetObject",
                "s3:DeleteObject"
            ],
            "Resource": "arn:aws:s3:::YOUR-BUCKET-NAME/materials/*"
        },
        {
            "Sid": "StrataHubS3List",
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket"
            ],
            "Resource": "arn:aws:s3:::YOUR-BUCKET-NAME",
            "Condition": {
                "StringLike": {
                    "s3:prefix": "materials/*"
                }
            }
        }
    ]
}
```

Attach this policy to:
- An **IAM Role** (recommended for EC2, ECS, Lambda), or
- An **IAM User** (for servers outside AWS)

### Step 6: Configure StrataHub

With everything set up, configure StrataHub with your values:

```toml
# S3/CloudFront storage
storage_type = "s3"
storage_s3_region = "us-east-1"
storage_s3_bucket = "stratahub-files-prod"
storage_s3_prefix = "materials/"
storage_cf_url = "https://d1234567890abc.cloudfront.net"
storage_cf_keypair_id = "K1A2B3C4D5E6F7"
storage_cf_key_path = "/etc/stratahub/cloudfront-private-key.pem"
```

Or via environment variables:

```bash
export STRATAHUB_STORAGE_TYPE=s3
export STRATAHUB_STORAGE_S3_REGION=us-east-1
export STRATAHUB_STORAGE_S3_BUCKET=stratahub-files-prod
export STRATAHUB_STORAGE_S3_PREFIX=materials/
export STRATAHUB_STORAGE_CF_URL=https://d1234567890abc.cloudfront.net
export STRATAHUB_STORAGE_CF_KEYPAIR_ID=K1A2B3C4D5E6F7
export STRATAHUB_STORAGE_CF_KEY_PATH=/etc/stratahub/cloudfront-private-key.pem
```

### Verification

Test your setup:

1. Start StrataHub with the S3 configuration
2. Upload a material with a file attachment
3. View the material and click the download link
4. Verify the file downloads successfully

Check CloudFront logs if downloads fail. Common issues:
- Incorrect bucket policy (CloudFront can't access S3)
- Wrong key pair ID (signed URL verification fails)
- Private key file not readable (check permissions)
- Clock skew (signed URLs are time-sensitive)

### Cost Considerations

- **S3**: Pay for storage and requests (minimal for most use cases)
- **CloudFront**: Pay for data transfer and requests (often cheaper than direct S3 access)
- **Free tier**: Both services include free tier allowances for new accounts

For cost optimization:
- Use appropriate CloudFront price class (e.g., "Use only North America and Europe")
- Set reasonable signed URL expiration times (default is typically 1 hour)
- Enable S3 lifecycle rules to clean up old/unused files

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
