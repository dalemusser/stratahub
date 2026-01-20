# StrataHub Production Deployment Guide

This guide covers deploying StrataHub to a production environment.

---

## Production Checklist

Use this checklist before going live:

### Security

- [ ] **Session key**: Generate a strong, random 32+ character key
  ```bash
  openssl rand -base64 32
  ```
- [ ] **CSRF key**: Generate a strong, random 32+ character key
- [ ] **Environment**: Set `env = "prod"`
- [ ] **HTTPS**: Enable `use_https = true` with valid certificates
- [ ] **Session domain**: Set appropriately (auto-derived in multi-workspace mode)
- [ ] **Default credentials**: Remove or change any default passwords
- [ ] **SuperAdmin**: Configure `superadmin_email` for initial superadmin user

### Database

- [ ] **MongoDB URI**: Use authenticated connection string
  ```
  mongodb://user:password@host:27017/strata_hub?authSource=admin
  ```
- [ ] **Connection pooling**: Tune `mongo_max_pool_size` and `mongo_min_pool_size`
- [ ] **Replica set**: Consider using a replica set for high availability
- [ ] **Backups**: Configure automated MongoDB backups
- [ ] **Indexes**: Verify indexes are created on startup (check logs)

### Multi-Workspace Mode (if enabled)

- [ ] **Primary domain**: Set `primary_domain` (e.g., "example.com")
- [ ] **Wildcard TLS**: Configure wildcard certificate for `*.example.com`
- [ ] **DNS**: Set up wildcard DNS record `*.example.com` pointing to your server
- [ ] **Session domain**: Auto-derived as `.example.com` (or set manually)
- [ ] **OAuth redirect**: Configure OAuth app to accept redirects from primary domain
- [ ] **Default workspace**: Configure `default_workspace_name` and `default_workspace_subdomain`

### Email

- [ ] **SMTP credentials**: Configure production SMTP server
- [ ] **From address**: Use a valid, monitored email address
- [ ] **SPF/DKIM/DMARC**: Configure DNS records for email deliverability
- [ ] **Base URL**: Set to your production domain for email links

### File Storage

- [ ] **Storage backend**: Choose `local` or `s3`
- [ ] **Local storage**: Ensure directory exists and has correct permissions
- [ ] **S3 storage**: Configure bucket, region, and IAM credentials
- [ ] **CloudFront**: Optional CDN for S3 with signed URLs
- [ ] **Backup**: Include uploaded files in backup strategy

### OAuth (if using Google OAuth)

- [ ] **Client credentials**: Configure production OAuth app
- [ ] **Redirect URIs**: Add production domain (and subdomains for multi-workspace)
- [ ] **Consent screen**: Configure OAuth consent screen for production

### Monitoring

- [ ] **Health endpoint**: Verify `/health` returns 200
- [ ] **Logging**: Set `log_level = "info"` or `"warn"` for production
- [ ] **Audit logging**: Enable `audit_log_auth` and `audit_log_admin`
- [ ] **Error tracking**: Consider integrating error tracking service

### Performance

- [ ] **Compression**: Enable `enable_compression = true`
- [ ] **Timeouts**: Review and adjust HTTP timeouts for your use case
- [ ] **Idle logout**: Configure if required for compliance

### Infrastructure

- [ ] **Reverse proxy**: Configure nginx/caddy/traefik in front of app
- [ ] **Firewall**: Restrict access to necessary ports only
- [ ] **Docker**: Use non-root user (already configured in Dockerfile)
- [ ] **Resources**: Set appropriate CPU/memory limits
- [ ] **Restart policy**: Configure automatic restarts on failure

---

## Environment Variables

All configuration can be set via environment variables with the `STRATAHUB_` prefix:

```bash
# Required for production
export STRATAHUB_ENV=prod
export STRATAHUB_SESSION_KEY="your-secure-32-char-session-key-here"
export STRATAHUB_CSRF_KEY="your-secure-32-char-csrf-key-here"
export STRATAHUB_MONGO_URI="mongodb://user:pass@host:27017/strata_hub"
export STRATAHUB_BASE_URL="https://yourdomain.com"

# HTTPS (Let's Encrypt with wildcard for multi-workspace)
export STRATAHUB_USE_HTTPS=true
export STRATAHUB_USE_LETS_ENCRYPT=true
export STRATAHUB_LETS_ENCRYPT_EMAIL="admin@yourdomain.com"
export STRATAHUB_LETS_ENCRYPT_CHALLENGE=dns-01
export STRATAHUB_ROUTE53_HOSTED_ZONE_ID="Z1234567890ABC"
export STRATAHUB_DOMAINS='["yourdomain.com", "*.yourdomain.com"]'

# Multi-workspace mode
export STRATAHUB_MULTI_WORKSPACE=true
export STRATAHUB_PRIMARY_DOMAIN="yourdomain.com"
export STRATAHUB_DEFAULT_WORKSPACE_NAME="Main"
export STRATAHUB_DEFAULT_WORKSPACE_SUBDOMAIN="app"

# Email
export STRATAHUB_MAIL_SMTP_HOST="smtp.example.com"
export STRATAHUB_MAIL_SMTP_PORT=587
export STRATAHUB_MAIL_SMTP_USER="smtp-user"
export STRATAHUB_MAIL_SMTP_PASS="smtp-password"
export STRATAHUB_MAIL_FROM="noreply@yourdomain.com"

# Optional: Google OAuth
export STRATAHUB_GOOGLE_CLIENT_ID="your-client-id"
export STRATAHUB_GOOGLE_CLIENT_SECRET="your-client-secret"

# Optional: S3 Storage
export STRATAHUB_STORAGE_TYPE=s3
export STRATAHUB_STORAGE_S3_REGION="us-east-1"
export STRATAHUB_STORAGE_S3_BUCKET="your-bucket"

# Optional: Initial superadmin
export STRATAHUB_SUPERADMIN_EMAIL="admin@yourdomain.com"
```

---

## Docker Deployment

### Build the Image

```bash
docker build -t stratahub:latest .
```

### Run with Docker

```bash
docker run -d \
  --name stratahub \
  --restart unless-stopped \
  -p 8080:8080 \
  -e STRATAHUB_ENV=prod \
  -e STRATAHUB_SESSION_KEY="your-session-key" \
  -e STRATAHUB_CSRF_KEY="your-csrf-key" \
  -e STRATAHUB_MONGO_URI="mongodb://host:27017/strata_hub" \
  -e STRATAHUB_BASE_URL="https://yourdomain.com" \
  -v /path/to/uploads:/app/uploads \
  stratahub:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  stratahub:
    build: .
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      STRATAHUB_ENV: prod
      STRATAHUB_SESSION_KEY: ${SESSION_KEY}
      STRATAHUB_CSRF_KEY: ${CSRF_KEY}
      STRATAHUB_MONGO_URI: mongodb://mongo:27017/strata_hub
      STRATAHUB_BASE_URL: https://yourdomain.com
      STRATAHUB_MULTI_WORKSPACE: ${MULTI_WORKSPACE:-false}
      STRATAHUB_PRIMARY_DOMAIN: ${PRIMARY_DOMAIN}
      STRATAHUB_MAIL_SMTP_HOST: ${SMTP_HOST}
      STRATAHUB_MAIL_SMTP_PORT: ${SMTP_PORT}
      STRATAHUB_MAIL_SMTP_USER: ${SMTP_USER}
      STRATAHUB_MAIL_SMTP_PASS: ${SMTP_PASS}
    volumes:
      - uploads:/app/uploads
    depends_on:
      - mongo
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      retries: 3

  mongo:
    image: mongo:7
    restart: unless-stopped
    volumes:
      - mongo_data:/data/db
    # In production, add authentication:
    # environment:
    #   MONGO_INITDB_ROOT_USERNAME: admin
    #   MONGO_INITDB_ROOT_PASSWORD: ${MONGO_PASSWORD}

volumes:
  uploads:
  mongo_data:
```

---

## Multi-Workspace Deployment

Multi-workspace mode enables subdomain-based tenancy where each workspace gets its own subdomain (e.g., `team1.yourdomain.com`, `team2.yourdomain.com`).

### Requirements

1. **Wildcard DNS**: Create a DNS record for `*.yourdomain.com`
2. **Wildcard TLS**: Use Let's Encrypt with DNS-01 challenge or a wildcard certificate
3. **Session cookies**: Must work across subdomains (auto-configured)

### DNS Setup

```
# A records (or CNAME to load balancer)
yourdomain.com.     A     123.45.67.89
*.yourdomain.com.   A     123.45.67.89
```

### Let's Encrypt with Wildcard (Route 53)

```bash
export STRATAHUB_USE_HTTPS=true
export STRATAHUB_USE_LETS_ENCRYPT=true
export STRATAHUB_LETS_ENCRYPT_EMAIL="admin@yourdomain.com"
export STRATAHUB_LETS_ENCRYPT_CHALLENGE=dns-01
export STRATAHUB_ROUTE53_HOSTED_ZONE_ID="Z1234567890ABC"
export STRATAHUB_DOMAINS='["yourdomain.com", "*.yourdomain.com"]'
```

### OAuth Configuration for Multi-Workspace

When using Google OAuth with multi-workspace:

1. Add the primary domain as authorized redirect URI:
   ```
   https://yourdomain.com/auth/google/callback
   ```

2. OAuth callbacks route through the primary domain, then redirect to the appropriate workspace subdomain.

---

## Reverse Proxy Configuration

### Nginx (Multi-Workspace)

```nginx
# Redirect HTTP to HTTPS for all subdomains
server {
    listen 80;
    server_name yourdomain.com *.yourdomain.com;
    return 301 https://$host$request_uri;
}

# Main server block for all subdomains
server {
    listen 443 ssl http2;
    server_name yourdomain.com *.yourdomain.com;

    # Wildcard certificate
    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (if needed)
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # Increase max upload size if needed
    client_max_body_size 32M;
}
```

### Caddy (Multi-Workspace)

```caddyfile
yourdomain.com, *.yourdomain.com {
    reverse_proxy localhost:8080

    header {
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
        Referrer-Policy "strict-origin-when-cross-origin"
    }

    # Caddy handles wildcard certs automatically with DNS challenge
    tls {
        dns route53
    }
}
```

---

## MongoDB Setup

### Production Recommendations

1. **Use authentication**:
   ```bash
   mongosh
   use admin
   db.createUser({
     user: "stratahub",
     pwd: "secure-password",
     roles: [{ role: "readWrite", db: "strata_hub" }]
   })
   ```

2. **Enable replica set** for high availability (minimum 3 nodes)

3. **Configure backups**:
   ```bash
   # Daily backup script
   mongodump --uri="mongodb://user:pass@host:27017/strata_hub" \
     --out=/backups/$(date +%Y%m%d)
   ```

4. **Monitor performance** with MongoDB tools or cloud monitoring

---

## Health Checks

The `/health` endpoint returns system status:

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "database": "connected"
}
```

Use this for:
- Load balancer health checks
- Container orchestration (Kubernetes, Docker Swarm)
- Monitoring systems

---

## Backup Strategy

### What to Back Up

1. **MongoDB database**: All user data, workspaces, settings, files metadata
2. **Uploaded files**: If using local storage, back up the uploads directory
3. **Configuration**: Keep `config.toml` or environment variables documented

### Backup Schedule

| Data | Frequency | Retention |
|------|-----------|-----------|
| MongoDB | Daily | 30 days |
| MongoDB | Weekly | 1 year |
| Uploaded files | Daily (incremental) | 30 days |
| Configuration | On change | Version controlled |

### Multi-Workspace Considerations

In multi-workspace mode, all workspaces share the same database. Ensure backups include:
- All workspace documents
- User-workspace associations
- Workspace-specific settings

---

## Troubleshooting

### Common Issues

**App won't start**
- Check MongoDB connectivity: `mongosh $STRATAHUB_MONGO_URI`
- Verify environment variables are set
- Check logs for configuration errors
- For multi-workspace: verify `primary_domain` is set

**Can't login**
- Verify session_key hasn't changed (invalidates existing sessions)
- Check session_domain is correct for your setup
- Verify CSRF key is consistent across restarts

**Subdomains not working (multi-workspace)**
- Verify wildcard DNS is configured: `dig *.yourdomain.com`
- Check wildcard TLS certificate is valid
- Verify session_domain allows cross-subdomain cookies
- Check primary_domain is set correctly

**OAuth not working**
- Verify redirect URI matches primary domain
- Check Google Cloud Console for errors
- Verify client ID and secret are correct

**Emails not sending**
- Test SMTP connection independently
- Check spam folders
- Verify SPF/DKIM/DMARC DNS records

**File uploads failing**
- Check storage directory permissions
- Verify max upload size in config and reverse proxy
- Check available disk space

### Logs

View application logs:
```bash
# Docker
docker logs stratahub

# Systemd
journalctl -u stratahub -f
```

---

## Security Hardening

### Additional Recommendations

1. **Firewall**: Only expose ports 80/443
2. **Fail2ban**: Block repeated failed login attempts at firewall level
3. **Updates**: Keep OS, Docker, and MongoDB updated
4. **Secrets management**: Use Docker secrets or Vault for sensitive config
5. **Network isolation**: Run MongoDB on private network
6. **Audit logs**: Review audit logs regularly for suspicious activity
7. **Backup encryption**: Encrypt backups at rest
8. **Session timeout**: Enable idle logout for sensitive deployments
9. **Workspace isolation**: In multi-workspace mode, verify data isolation between workspaces

---

## Scaling Considerations

### Horizontal Scaling

StrataHub can be scaled horizontally with:

1. **Load balancer**: Distribute traffic across multiple instances
2. **Session affinity**: Not required (sessions stored in MongoDB)
3. **Shared storage**: Use S3 for file storage when running multiple instances
4. **MongoDB replica set**: For database high availability

### Multi-Workspace Performance

- Each workspace query includes workspace ID filter
- Indexes are optimized for workspace-scoped queries
- Consider separate read replicas for high-traffic workspaces
