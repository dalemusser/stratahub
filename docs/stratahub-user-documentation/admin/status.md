# Status

**System Status** is a server health and diagnostics screen for administrators. It's
a read-only view used to confirm the service is running normally and to gather
details when troubleshooting.

_(No screenshot is included here: the live screen displays real server configuration
specific to the deployment.)_

## What it shows

- **Health summary** — at-a-glance indicators for the **TLS certificate** (validity
  and expiry), the **database** connection, server **uptime**, and **memory** usage.
- **TLS certificate** — certificate status, host, expiry, and issuer.
- **Database** — connection status and version.
- **System** — runtime details such as uptime and memory.
- **Configuration** — the server's current settings (environment, HTTP, TLS, CORS,
  and database connection), shown for diagnostic purposes.

Because the configuration section reflects the actual production setup, treat this
screen as sensitive and don't share screenshots of it outside your organization.
