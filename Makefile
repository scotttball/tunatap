# Tunatap Makefile
# Development, testing, and release automation

BINARY_NAME := tunatap
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt

# Directories
COVERAGE_DIR := .coverage

.PHONY: all build clean test lint fmt help

# Default target
all: lint test build

##@ Development

build: ## Build the binary
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

build-all: ## Build for all platforms
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

clean: ## Remove build artifacts
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	rm -rf $(COVERAGE_DIR)
	rm -rf dist/

run: build ## Build and run
	./$(BINARY_NAME)

##@ Testing

test: ## Run all tests
	$(GOTEST) -v ./...

test-short: ## Run tests (short mode, skip slow tests)
	$(GOTEST) -v -short ./...

test-race: ## Run tests with race detector
	$(GOTEST) -v -race ./...

test-coverage: ## Run tests with coverage report
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

test-integration: ## Run integration tests (requires TEST_INTEGRATION=1)
	TEST_INTEGRATION=1 $(GOTEST) -v -tags=integration ./...

test-bench: ## Run benchmarks
	$(GOTEST) -v -bench=. -benchmem ./...

##@ Code Quality

lint: ## Run linters
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

lint-fix: ## Run linters and fix issues
	golangci-lint run --fix

fmt: ## Format code
	$(GOFMT) -s -w .

fmt-check: ## Check code formatting
	@test -z "$$($(GOFMT) -l .)" || (echo "Code not formatted. Run 'make fmt'" && $(GOFMT) -l . && exit 1)

vet: ## Run go vet
	$(GOCMD) vet ./...

security: ## Run security scanner (gosec)
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)
	gosec -quiet ./...

##@ Profiling & Debugging

pprof-cpu: build ## Run with CPU profiling
	./$(BINARY_NAME) --pprof-cpu

pprof-mem: build ## Run with memory profiling
	./$(BINARY_NAME) --pprof-mem

debug: ## Build with debug symbols (for delve)
	$(GOBUILD) -gcflags="all=-N -l" -o $(BINARY_NAME)-debug .
	@echo "Debug binary: $(BINARY_NAME)-debug"
	@echo "Run: dlv exec ./$(BINARY_NAME)-debug -- [args]"

debug-test: ## Debug tests with delve
	@echo "Run: dlv test ./internal/pool -- -test.run TestConnectionPool"

goroutine-check: ## Check for goroutine leaks in tests
	@which goleak > /dev/null || (echo "Note: Add uber-go/goleak to detect goroutine leaks in tests")
	$(GOTEST) -v ./... -count=1

##@ Dependencies

deps: ## Download dependencies
	$(GOMOD) download

deps-tidy: ## Tidy dependencies
	$(GOMOD) tidy

deps-update: ## Update all dependencies
	$(GOGET) -u ./...
	$(GOMOD) tidy

deps-verify: ## Verify dependencies
	$(GOMOD) verify

##@ Tools Installation

tools: ## Install development tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/go-delve/delve/cmd/dlv@latest
	go install github.com/goreleaser/goreleaser@latest
	go install go.uber.org/mock/mockgen@latest
	@echo "Tools installed to $(shell go env GOPATH)/bin"

##@ Release

release-snapshot: ## Create a snapshot release (for testing)
	@which goreleaser > /dev/null || (echo "Installing goreleaser..." && go install github.com/goreleaser/goreleaser@latest)
	goreleaser release --snapshot --clean

release: ## Create a release (requires GITHUB_TOKEN and git tag)
	@which goreleaser > /dev/null || (echo "Installing goreleaser..." && go install github.com/goreleaser/goreleaser@latest)
	goreleaser release --clean

##@ CI/CD

ci: deps-verify fmt-check lint test-race test-coverage ## Run all CI checks

##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
