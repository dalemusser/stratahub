# stratahub
A modular Go web platform for managing organizations, groups, users, and resource access across a wide range of domains.

## Requirements

- Go 1.21+
- MongoDB 6.0+
- Python 3.8+ (for E2E tests)

## Quick Start

```bash
# Build the application
make build

# Run the application (requires MongoDB on localhost:27017)
make run
```

## Makefile Targets

Run `make help` to see all available targets.

### Build & Run

| Target | Description |
|--------|-------------|
| `make build` | Build the application binary to `bin/stratahub` |
| `make run` | Run the application |

### Go Tests

| Target | Description |
|--------|-------------|
| `make test` | Run all Go tests |
| `make test-v` | Run all tests with verbose output |
| `make test-cover` | Run tests and generate HTML coverage report |
| `make test-store` | Run only database store tests |
| `make test-handlers` | Run only HTTP handler tests |
| `make test-auth` | Run only auth middleware tests |
| `make test-safe` | Run tests sequentially (avoids MongoDB resource issues) |
| `make test-fresh` | Run tests without cache |

### E2E Browser Tests

E2E tests use Playwright to test complete user journeys in a real browser.

| Target | Description |
|--------|-------------|
| `make test-e2e-setup` | **Run once:** Set up Python venv and install Playwright |
| `make test-e2e` | Run E2E tests headless (fast, for CI) |
| `make test-e2e-headed` | Run E2E tests with visible browser |
| `make test-e2e-slow` | Run E2E tests with visible browser in slow motion (for demos) |

#### E2E Test Setup

Before running E2E tests for the first time:

```bash
# 1. Set up the E2E test environment (creates venv, installs dependencies)
make test-e2e-setup

# 2. Start the application in one terminal
make run

# 3. Run E2E tests in another terminal
make test-e2e
```

**Requirements for E2E tests:**
- Python 3.8+ installed on system
- Application running on localhost:8080
- MongoDB running

### Tailwind CSS

| Target | Description |
|--------|-------------|
| `make css` | Build Tailwind CSS |
| `make css-watch` | Watch and rebuild CSS on changes (development) |
| `make css-prod` | Build minified CSS (production) |

#### Tailwind Setup

The project uses the [Tailwind standalone CLI](https://tailwindcss.com/blog/standalone-cli) (no Node.js required).

**First-time setup** - download the CLI for your platform:

```bash
# macOS (Apple Silicon)
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
chmod +x tailwindcss-macos-arm64
mv tailwindcss-macos-arm64 tailwindcss

# macOS (Intel)
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64
chmod +x tailwindcss-macos-x64
mv tailwindcss-macos-x64 tailwindcss

# Linux (x64)
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
chmod +x tailwindcss-linux-x64
mv tailwindcss-linux-x64 tailwindcss
```

Then build CSS with `make css-prod`.

### Code Quality

| Target | Description |
|--------|-------------|
| `make fmt` | Format Go code |
| `make vet` | Run go vet |
| `make tidy` | Tidy go.mod dependencies |
| `make verify` | Verify dependencies |

### Cleanup

| Target | Description |
|--------|-------------|
| `make clean` | Remove build artifacts and coverage files |

## Documentation

- [Testing Guide](test-info.md) - Comprehensive guide to writing and running tests
