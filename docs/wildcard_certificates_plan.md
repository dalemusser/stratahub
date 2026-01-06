# Plan: Multi-Domain and Wildcard Certificate Support

## Overview

Enhance the DNS-01 Let's Encrypt implementation to support multiple domains on a single certificate (e.g., `adroit.games` + `*.adroit.games`) and add proactive background certificate renewal.

## Current State

- DNS-01 implementation exists in `waffle/server/acmedns.go`
- Supports single domain (including wildcards like `*.example.com`)
- Renewal happens lazily during TLS handshakes (can cause latency)
- Uses AWS Route 53 for automated DNS record management

## Goals

1. Support multiple domains on a single certificate
2. Common pattern: apex domain + wildcard (e.g., `adroit.games` + `*.adroit.games`)
3. Proactive background renewal to avoid latency during user requests
4. Maintain backward compatibility with single-domain configuration

---

## Implementation Plan

### 1. Configuration Changes

**File:** `waffle/config/config.go`

Add support for multiple domains while maintaining backward compatibility:

```go
// Existing (keep for backward compatibility)
Domain string `toml:"domain"`

// New
Domains []string `toml:"domains"`
```

Configuration examples:

```toml
# Single domain (backward compatible)
domain = "adroit.games"

# Multiple domains (new)
domains = ["adroit.games", "*.adroit.games"]
```

**Validation logic:**
- If both `domain` and `domains` are set, return error
- If `domain` is set, convert to single-element `Domains` slice internally
- Validate each domain in the list

---

### 2. DNS01Manager Struct Changes

**File:** `waffle/server/acmedns.go`

```go
type DNS01Manager struct {
    Domains          []string  // Changed from Domain string
    Email            string
    CacheDir         string
    HostedZoneID     string
    ACMEDirectoryURL string
    Logger           *zap.Logger

    // ... existing fields ...

    // New: background renewal
    renewalTicker *time.Ticker
    stopRenewal   chan struct{}
}
```

**Constructor change:**

```go
// Current
func NewDNS01Manager(domain, email, cacheDir, hostedZoneID, acmeDirectoryURL string, logger *zap.Logger) (*DNS01Manager, error)

// New
func NewDNS01Manager(domains []string, email, cacheDir, hostedZoneID, acmeDirectoryURL string, logger *zap.Logger) (*DNS01Manager, error)
```

---

### 3. ACME Order Creation

**Location:** `doObtainCertificate()` around line 285

```go
// Current
order, err := m.client.AuthorizeOrder(ctx, acme.DomainIDs(m.Domain))

// New
order, err := m.client.AuthorizeOrder(ctx, acme.DomainIDs(m.Domains...))
```

---

### 4. Authorization Loop Changes

**Location:** `doObtainCertificate()` lines 294-388

The loop already iterates over `order.AuthzURLs`. Change the challenge domain extraction:

```go
// Current - uses config domain
challengeDomain := m.Domain
if strings.HasPrefix(challengeDomain, "*.") {
    challengeDomain = challengeDomain[2:]
}

// New - extract from authorization identifier
challengeDomain := authz.Identifier.Value
if strings.HasPrefix(challengeDomain, "*.") {
    challengeDomain = challengeDomain[2:]
}
```

**Important:** For `adroit.games` + `*.adroit.games`, both authorizations use the same challenge record (`_acme-challenge.adroit.games`). The ACME server may:
- Issue two separate challenges with different tokens, OR
- Reuse the authorization if already valid

Handle both cases by checking `authz.Status == acme.StatusValid` before creating DNS records.

---

### 5. CSR Creation

**Location:** `doObtainCertificate()` lines 397-402

```go
// Current
csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
    DNSNames: []string{m.Domain},
}, certKey)

// New
csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
    DNSNames: m.Domains,
}, certKey)
```

---

### 6. Cache File Naming

**Location:** `loadCachedCert()` and `cacheCert()`

Use the primary (first) domain for cache filenames:

```go
func (m *DNS01Manager) cachePrefix() string {
    // Use first domain as cache key
    // Replace wildcard asterisk with "wildcard" for filesystem safety
    name := m.Domains[0]
    name = strings.Replace(name, "*.", "wildcard.", 1)
    return name
}

// Usage
certPath, err := m.safeCachePath(m.cachePrefix() + ".crt")
keyPath, err := m.safeCachePath(m.cachePrefix() + ".key")
```

---

### 7. Certificate Validation

**Location:** `certMatchesDomain()` - rename to `certMatchesDomains()`

```go
// Check that certificate covers ALL configured domains
func (m *DNS01Manager) certMatchesDomains(leaf *x509.Certificate) bool {
    for _, expectedDomain := range m.Domains {
        if !m.domainCoveredByCert(leaf, expectedDomain) {
            return false
        }
    }
    return true
}

// Check if a single domain is covered by the certificate
func (m *DNS01Manager) domainCoveredByCert(leaf *x509.Certificate, domain string) bool {
    domainBase := strings.TrimPrefix(domain, "*.")

    for _, dnsName := range leaf.DNSNames {
        // Exact match
        if dnsName == domain {
            return true
        }
        // Wildcard in cert covers our domain
        if strings.HasPrefix(dnsName, "*.") {
            certBase := dnsName[2:]
            if certBase == domainBase {
                return true
            }
        }
        // Our wildcard domain matches cert's base
        if strings.HasPrefix(domain, "*.") && dnsName == domainBase {
            return true
        }
    }
    return false
}
```

---

### 8. Background Proactive Renewal

**New methods in `waffle/server/acmedns.go`:**

```go
const (
    // Check for renewal every 12 hours
    renewalCheckInterval = 12 * time.Hour

    // Renew 30 days before expiry (existing constant)
    renewalBuffer = 30 * 24 * time.Hour
)

// StartBackgroundRenewal starts a goroutine that proactively renews
// certificates before they expire. This prevents renewal latency
// during user TLS handshakes.
func (m *DNS01Manager) StartBackgroundRenewal(ctx context.Context) {
    m.renewalTicker = time.NewTicker(renewalCheckInterval)
    m.stopRenewal = make(chan struct{})

    go func() {
        for {
            select {
            case <-ctx.Done():
                m.renewalTicker.Stop()
                return
            case <-m.stopRenewal:
                m.renewalTicker.Stop()
                return
            case <-m.renewalTicker.C:
                m.checkAndRenew(ctx)
            }
        }
    }()

    m.Logger.Info("started background certificate renewal",
        zap.Duration("check_interval", renewalCheckInterval),
        zap.Duration("renewal_buffer", renewalBuffer))
}

// StopBackgroundRenewal stops the background renewal goroutine.
func (m *DNS01Manager) StopBackgroundRenewal() {
    if m.stopRenewal != nil {
        close(m.stopRenewal)
    }
}

// checkAndRenew checks if the certificate needs renewal and renews if necessary.
func (m *DNS01Manager) checkAndRenew(ctx context.Context) {
    m.certMu.RLock()
    expiry := m.certExpiry
    m.certMu.RUnlock()

    if time.Now().Add(renewalBuffer).Before(expiry) {
        // Certificate still valid, no renewal needed
        m.Logger.Debug("certificate still valid",
            zap.Time("expiry", expiry),
            zap.Duration("time_remaining", time.Until(expiry)))
        return
    }

    m.Logger.Info("proactively renewing certificate",
        zap.Time("expiry", expiry),
        zap.Strings("domains", m.Domains))

    // Use a reasonable timeout for renewal
    renewCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
    defer cancel()

    if _, err := m.GetCertificate(nil); err != nil {
        m.Logger.Error("background certificate renewal failed",
            zap.Error(err),
            zap.Strings("domains", m.Domains))
    } else {
        m.Logger.Info("background certificate renewal succeeded",
            zap.Strings("domains", m.Domains))
    }
}
```

---

### 9. Server Integration

**File:** `waffle/server/server.go`

Start background renewal after server starts:

```go
// In server startup, after PreWarm succeeds
if dns01Manager != nil {
    dns01Manager.StartBackgroundRenewal(ctx)

    // Ensure cleanup on shutdown
    defer dns01Manager.StopBackgroundRenewal()
}
```

---

### 10. Logging Updates

Update all log statements to show multiple domains:

```go
// Current
zap.String("domain", m.Domain)

// New
zap.Strings("domains", m.Domains)
```

---

### 11. Domain Validation

**Location:** `NewDNS01Manager()`

```go
// Validate all domains
if len(domains) == 0 {
    return nil, errors.New("dns01: at least one domain is required")
}

for _, domain := range domains {
    if err := validateDomainFormat(domain); err != nil {
        return nil, fmt.Errorf("dns01: invalid domain %q: %w", domain, err)
    }
}

// Check for duplicates
seen := make(map[string]bool)
for _, domain := range domains {
    if seen[domain] {
        return nil, fmt.Errorf("dns01: duplicate domain %q", domain)
    }
    seen[domain] = true
}
```

---

## Files to Modify

| File | Changes |
|------|---------|
| `waffle/config/config.go` | Add `Domains []string` field, validation logic |
| `waffle/server/acmedns.go` | Multi-domain support, background renewal |
| `waffle/server/server.go` | Start/stop background renewal |
| `stratahub/docs/configuration.md` | Document new `domains` config option |

---

## Testing Plan

1. **Unit tests:**
   - `validateDomainFormat()` with wildcards
   - `certMatchesDomains()` with various cert/domain combinations
   - Cache filename generation with wildcards

2. **Integration tests:**
   - Single domain (backward compatibility)
   - Multiple domains
   - Wildcard + apex combination
   - Background renewal trigger

3. **Manual testing:**
   - Configure `domains = ["adroit.games", "*.adroit.games"]`
   - Verify certificate covers both domains
   - Verify background renewal logs appear
   - Test with Let's Encrypt staging first

---

## Configuration Example

```toml
# Production configuration for wildcard + apex
use_https = true
use_lets_encrypt = true
lets_encrypt_email = "admin@adroit.games"
lets_encrypt_challenge = "dns-01"
domains = ["adroit.games", "*.adroit.games"]
route53_hosted_zone_id = "Z1234567890ABC"
```

---

## Rollout Plan

1. Implement changes in waffle framework
2. Test with Let's Encrypt staging environment
3. Update stratahub configuration
4. Deploy to production
5. Verify certificate covers both domains via browser/openssl

---

## Notes

- Let's Encrypt rate limits: 50 certificates per registered domain per week
- Wildcard certificates require DNS-01 challenge (cannot use HTTP-01)
- Both `adroit.games` and `*.adroit.games` use the same challenge record: `_acme-challenge.adroit.games`
- Background renewal runs every 12 hours, renews 30 days before expiry
