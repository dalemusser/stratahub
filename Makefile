.PHONY: build build-linux test test-v test-cover test-store test-handlers test-auth test-safe test-fresh test-e2e test-e2e-setup test-e2e-headed test-e2e-slow css css-watch css-prod clean tidy run help setup setup-tailwind

# Build variables
BINARY_NAME=stratahub
BUILD_DIR=bin
CMD_PATH=./cmd/stratahub

# Default target
all: build

# Build the application
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

# Build for Linux 386 (production server)
build-linux:
	GOOS=linux GOARCH=386 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-386 $(CMD_PATH)

# Run the application
run:
	go run $(CMD_PATH)

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage report
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run only store tests (requires MongoDB)
test-store:
	go test ./internal/app/store/...

# Run handler tests (requires MongoDB)
test-handlers:
	go test ./internal/app/features/...

# Run auth middleware tests
test-auth:
	go test ./internal/app/system/auth/...

# Run all tests sequentially (avoids MongoDB resource contention)
test-safe:
	go test -p=1 -count=1 ./...

# Run tests without cache
test-fresh:
	go test -count=1 ./...

# Set up E2E test environment (run once before running E2E tests)
test-e2e-setup:
	cd tests/e2e && python3 -m venv venv
	cd tests/e2e && source venv/bin/activate && pip install pytest pytest-playwright playwright
	cd tests/e2e && source venv/bin/activate && playwright install chromium

# Run Playwright E2E tests (requires app running on localhost:8080)
test-e2e:
	cd tests/e2e && source venv/bin/activate && pytest -v

# Run Playwright E2E tests with visible browser
test-e2e-headed:
	cd tests/e2e && source venv/bin/activate && pytest -v --headed

# Run Playwright E2E tests with visible browser in slow motion (for demos/recording)
test-e2e-slow:
	cd tests/e2e && source venv/bin/activate && pytest -v --headed --slowmo=500

# Tailwind CSS variables
CSS_INPUT  = ./internal/app/resources/assets/css/src/input.css
CSS_OUTPUT = ./internal/app/resources/assets/css/tailwind.css

# Build Tailwind CSS
css:
	./tailwindcss -i $(CSS_INPUT) -o $(CSS_OUTPUT)

# Watch Tailwind CSS for changes (development)
css-watch:
	./tailwindcss -i $(CSS_INPUT) -o $(CSS_OUTPUT) --watch

# Build minified Tailwind CSS (production)
css-prod:
	./tailwindcss -i $(CSS_INPUT) -o $(CSS_OUTPUT) --minify

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Tidy dependencies
tidy:
	go mod tidy

# Verify dependencies
verify:
	go mod verify

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Download Tailwind CSS standalone CLI for current platform
setup-tailwind:
	@echo "Detecting platform..."
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	ARCH=$$(uname -m); \
	if [ "$$OS" = "darwin" ]; then \
		if [ "$$ARCH" = "arm64" ]; then \
			URL="https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64"; \
		else \
			URL="https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64"; \
		fi; \
	elif [ "$$OS" = "linux" ]; then \
		if [ "$$ARCH" = "aarch64" ] || [ "$$ARCH" = "arm64" ]; then \
			URL="https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-arm64"; \
		else \
			URL="https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64"; \
		fi; \
	else \
		echo "Unsupported OS: $$OS"; exit 1; \
	fi; \
	echo "Downloading Tailwind CSS from $$URL..."; \
	curl -sL "$$URL" -o tailwindcss && chmod +x tailwindcss
	@echo "Tailwind CSS installed successfully"

# Set up development environment (run after git clone)
setup: setup-tailwind
	@echo "Development environment setup complete"
	@echo ""
	@echo "Optional: To set up E2E tests, run: make test-e2e-setup"

# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "Setup (run after git clone):"
	@echo "  setup           - Set up development environment (downloads Tailwind)"
	@echo "  setup-tailwind  - Download Tailwind CSS standalone CLI"
	@echo "  test-e2e-setup  - Set up E2E test environment (Python + Playwright)"
	@echo ""
	@echo "Build & Run:"
	@echo "  build           - Build the application"
	@echo "  build-linux     - Build for Linux 386 (production server)"
	@echo "  run             - Run the application"
	@echo ""
	@echo "Testing:"
	@echo "  test            - Run all tests"
	@echo "  test-v          - Run all tests with verbose output"
	@echo "  test-cover      - Run tests with coverage report"
	@echo "  test-store      - Run only store tests (requires MongoDB)"
	@echo "  test-handlers   - Run handler tests (requires MongoDB)"
	@echo "  test-auth       - Run auth middleware tests"
	@echo "  test-safe       - Run all tests sequentially (avoids MongoDB issues)"
	@echo "  test-fresh      - Run tests without cache"
	@echo "  test-e2e        - Run Playwright E2E tests (requires app running)"
	@echo "  test-e2e-headed - Run E2E tests with visible browser"
	@echo "  test-e2e-slow   - Run E2E tests with visible browser in slow motion"
	@echo ""
	@echo "CSS:"
	@echo "  css             - Build Tailwind CSS"
	@echo "  css-watch       - Watch and rebuild Tailwind CSS on changes"
	@echo "  css-prod        - Build minified Tailwind CSS for production"
	@echo ""
	@echo "Maintenance:"
	@echo "  clean           - Remove build artifacts"
	@echo "  tidy            - Tidy go.mod dependencies"
	@echo "  verify          - Verify dependencies"
	@echo "  fmt             - Format code"
	@echo "  vet             - Run go vet"
