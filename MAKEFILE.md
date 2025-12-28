# Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the application to `bin/stratahub` |
| `make run` | Run the application |
| `make test` | Run all tests |
| `make test-v` | Run tests with verbose output |
| `make test-cover` | Run tests and generate `coverage.html` |
| `make test-store` | Run only store tests (requires MongoDB) |
| `make test-handlers` | Run handler tests (requires MongoDB) |
| `make test-auth` | Run auth middleware tests |
| `make test-safe` | Run all tests sequentially (avoids MongoDB resource contention) |
| `make test-fresh` | Run tests without cache |
| `make test-e2e` | Run Playwright E2E tests (requires app running on localhost:8080) |
| `make test-e2e-headed` | Run E2E tests with visible browser |
| `make clean` | Remove build artifacts |
| `make tidy` | Tidy go.mod dependencies |
| `make verify` | Verify dependencies |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make help` | Show available targets |
