# Systemd Configuration for StrataHub

This document describes how to configure systemd to run StrataHub as a service on Ubuntu Linux.

## Overview

Systemd is the init system used by Ubuntu (and most modern Linux distributions) to manage services. Running StrataHub as a systemd service provides:

- Automatic startup on boot
- Automatic restart on crash/panic
- Centralized logging via journald
- Process supervision and resource management
- Graceful shutdown handling

## Service Unit File

Create the service file at `/etc/systemd/system/stratahub.service`:

```ini
[Unit]
Description=StrataHub Education Platform
Documentation=https://github.com/dalemusser/stratahub
After=network.target mongodb.service
Wants=mongodb.service

[Service]
Type=simple
User=stratahub
Group=stratahub
WorkingDirectory=/opt/stratahub
ExecStart=/opt/stratahub/stratahub
ExecReload=/bin/kill -HUP $MAINPID

# Restart behavior
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=3

# Environment
Environment=WAFFLE_ENV=prod
EnvironmentFile=-/opt/stratahub/.env

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/stratahub/uploads

# Resource limits
LimitNOFILE=65535
MemoryMax=1G

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=stratahub

[Install]
WantedBy=multi-user.target
```

## Section Reference

### [Unit] Section

| Directive | Description |
|-----------|-------------|
| `Description` | Human-readable description shown in `systemctl status` |
| `Documentation` | URL to documentation |
| `After` | Start this service after the listed units |
| `Wants` | Weak dependency - try to start these units, but don't fail if they don't start |
| `Requires` | Strong dependency - fail if these units don't start (use sparingly) |

### [Service] Section

#### Basic Settings

| Directive | Value | Description |
|-----------|-------|-------------|
| `Type` | `simple` | The process started by `ExecStart` is the main process |
| `User` | `stratahub` | Run as this user (never run as root) |
| `Group` | `stratahub` | Run as this group |
| `WorkingDirectory` | `/opt/stratahub` | Working directory for the process |
| `ExecStart` | `/opt/stratahub/stratahub` | Command to start the service |
| `ExecReload` | `/bin/kill -HUP $MAINPID` | Command to reload configuration |

#### Restart Behavior

| Directive | Value | Description |
|-----------|-------|-------------|
| `Restart` | `on-failure` | When to restart (see table below) |
| `RestartSec` | `5` | Wait 5 seconds before restarting |
| `StartLimitIntervalSec` | `60` | Time window for counting restart attempts |
| `StartLimitBurst` | `3` | Max restarts within the interval before giving up |

**Restart Options:**

| Value | Restarts On |
|-------|-------------|
| `no` | Never (default) |
| `always` | Always, regardless of exit status |
| `on-success` | Clean exit (exit code 0) only |
| `on-failure` | Non-zero exit code, signal, timeout, or watchdog |
| `on-abnormal` | Signal, timeout, or watchdog (not non-zero exit) |
| `on-abort` | Unclean signal (SIGABRT, SIGBUS, etc.) |

**Go Panic Behavior:**
- A Go panic that isn't recovered exits with code 2
- `Restart=on-failure` will restart after a panic
- `Restart=always` will also restart after a panic

**Restart Limiting:**
With `StartLimitBurst=3` and `StartLimitIntervalSec=60`:
- If the service crashes 3 times within 60 seconds, systemd stops trying
- This prevents infinite restart loops from a persistent bug
- After the interval passes, the counter resets

#### Environment Variables

| Directive | Description |
|-----------|-------------|
| `Environment` | Set individual environment variables inline |
| `EnvironmentFile` | Load variables from a file (prefix with `-` to ignore if missing) |

Example inline variables:
```ini
Environment=WAFFLE_ENV=prod
Environment=WAFFLE_LOG_LEVEL=info
Environment=STRATAHUB_MONGO_URI=mongodb://localhost:27017
```

Example environment file (`/opt/stratahub/.env`):
```bash
WAFFLE_ENV=prod
WAFFLE_LOG_LEVEL=info
STRATAHUB_MONGO_URI=mongodb://user:password@localhost:27017/strata_hub
STRATAHUB_SESSION_KEY=your-production-secret-key
STRATAHUB_MAIL_SMTP_HOST=email-smtp.us-east-1.amazonaws.com
STRATAHUB_MAIL_SMTP_USER=your-ses-user
STRATAHUB_MAIL_SMTP_PASS=your-ses-password
```

**Security Note:** Protect the environment file:
```bash
chmod 600 /opt/stratahub/.env
chown stratahub:stratahub /opt/stratahub/.env
```

#### Security Hardening

| Directive | Description |
|-----------|-------------|
| `NoNewPrivileges=true` | Prevent privilege escalation |
| `ProtectSystem=strict` | Mount /usr, /boot, /etc as read-only |
| `ProtectHome=true` | Make /home, /root, /run/user inaccessible |
| `PrivateTmp=true` | Use private /tmp and /var/tmp |
| `ReadWritePaths=` | Directories the service can write to |

If using local file storage for uploads:
```ini
ReadWritePaths=/opt/stratahub/uploads
```

#### Resource Limits

| Directive | Description |
|-----------|-------------|
| `LimitNOFILE=65535` | Max open file descriptors (important for many connections) |
| `MemoryMax=1G` | Maximum memory usage (kills process if exceeded) |
| `MemoryHigh=800M` | Throttle when memory exceeds this |
| `CPUQuota=200%` | Max CPU (200% = 2 cores) |

#### Logging

| Directive | Description |
|-----------|-------------|
| `StandardOutput=journal` | Send stdout to journald |
| `StandardError=journal` | Send stderr to journald |
| `SyslogIdentifier=stratahub` | Identifier in logs |

### [Install] Section

| Directive | Description |
|-----------|-------------|
| `WantedBy=multi-user.target` | Start when system reaches multi-user mode (normal boot) |

## Installation Steps

### 1. Create the Service User

```bash
# Create a system user with no login shell
sudo useradd --system --no-create-home --shell /usr/sbin/nologin stratahub
```

### 2. Set Up the Application Directory

```bash
# Create directory structure
sudo mkdir -p /opt/stratahub/uploads

# Copy application files
sudo cp stratahub /opt/stratahub/
sudo cp config.toml /opt/stratahub/

# Set ownership
sudo chown -R stratahub:stratahub /opt/stratahub

# Set permissions
sudo chmod 755 /opt/stratahub/stratahub
sudo chmod 644 /opt/stratahub/config.toml
sudo chmod 750 /opt/stratahub/uploads
```

### 3. Create Environment File (for secrets)

```bash
sudo nano /opt/stratahub/.env
# Add your production secrets...

sudo chmod 600 /opt/stratahub/.env
sudo chown stratahub:stratahub /opt/stratahub/.env
```

### 4. Install the Service

```bash
# Copy the service file
sudo nano /etc/systemd/system/stratahub.service
# Paste the service configuration...

# Reload systemd to recognize the new service
sudo systemctl daemon-reload

# Enable the service to start on boot
sudo systemctl enable stratahub

# Start the service
sudo systemctl start stratahub

# Check status
sudo systemctl status stratahub
```

## Common Commands

### Service Management

```bash
# Start the service
sudo systemctl start stratahub

# Stop the service
sudo systemctl stop stratahub

# Restart the service
sudo systemctl restart stratahub

# Reload configuration (sends SIGHUP)
sudo systemctl reload stratahub

# Check status
sudo systemctl status stratahub

# Enable auto-start on boot
sudo systemctl enable stratahub

# Disable auto-start on boot
sudo systemctl disable stratahub
```

### Viewing Logs

```bash
# View recent logs
sudo journalctl -u stratahub -n 100

# Follow logs in real-time
sudo journalctl -u stratahub -f

# View logs since last boot
sudo journalctl -u stratahub -b

# View logs from the last hour
sudo journalctl -u stratahub --since "1 hour ago"

# View logs for a specific time range
sudo journalctl -u stratahub --since "2024-01-15 10:00" --until "2024-01-15 12:00"

# View only errors
sudo journalctl -u stratahub -p err

# Export logs to a file
sudo journalctl -u stratahub --since today > stratahub-logs.txt
```

### Troubleshooting

```bash
# Check if service is active
systemctl is-active stratahub

# Check if service is enabled
systemctl is-enabled stratahub

# View detailed service information
systemctl show stratahub

# Check for configuration errors
sudo systemd-analyze verify /etc/systemd/system/stratahub.service

# View service dependencies
systemctl list-dependencies stratahub

# Check why a service failed
sudo systemctl status stratahub
sudo journalctl -u stratahub -n 50 --no-pager

# Reset failed state (after fixing issues)
sudo systemctl reset-failed stratahub
```

## Graceful Shutdown

StrataHub (via Waffle) handles graceful shutdown when it receives SIGTERM (the default signal from `systemctl stop`):

1. Stops accepting new connections
2. Waits for in-flight requests to complete (up to `shutdown_timeout`)
3. Closes database connections
4. Exits cleanly

The default stop timeout is 90 seconds. To adjust:

```ini
[Service]
TimeoutStopSec=30
```

## Health Checks with Watchdog

For critical deployments, enable the systemd watchdog:

```ini
[Service]
WatchdogSec=30
```

This requires the application to send periodic "I'm alive" signals to systemd. Waffle doesn't currently implement this, but it could be added if needed.

## Running Multiple Instances

To run multiple instances (e.g., for different environments):

```bash
# Create instance-specific service files
sudo cp /etc/systemd/system/stratahub.service /etc/systemd/system/stratahub-staging.service

# Edit the staging version to use different:
# - WorkingDirectory
# - EnvironmentFile
# - Port (via environment variable)
```

## Updating the Application

```bash
# Build new version
go build -o stratahub ./cmd/stratahub

# Stop the service
sudo systemctl stop stratahub

# Replace the binary
sudo cp stratahub /opt/stratahub/

# Set permissions
sudo chown stratahub:stratahub /opt/stratahub/stratahub
sudo chmod 755 /opt/stratahub/stratahub

# Start the service
sudo systemctl start stratahub

# Verify it's running
sudo systemctl status stratahub
sudo journalctl -u stratahub -n 20
```

## MongoDB Dependency

If MongoDB is on the same server, ensure proper ordering:

```ini
[Unit]
After=network.target mongodb.service mongod.service
Wants=mongodb.service
```

Notes:
- `After=` ensures StrataHub starts after MongoDB
- `Wants=` attempts to start MongoDB but doesn't fail if it's not available
- Use `Requires=` instead of `Wants=` if MongoDB is mandatory and local

If MongoDB is on a separate server, you only need:
```ini
[Unit]
After=network.target
```

## See Also

- [Configuration Guide](configuration.md) - StrataHub configuration options
- [AWS S3/CloudFront Setup](configuration.md#setting-up-aws-s3-and-cloudfront-for-file-storage) - File storage configuration
- `man systemd.service` - Full systemd service documentation
- `man systemd.exec` - Execution environment options
- `man journalctl` - Log viewing options
