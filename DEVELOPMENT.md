# Development Guide

This guide covers development workflows for tunatap.

## Prerequisites

- Go 1.21 or later
- golangci-lint (installed via `make tools`)
- delve debugger (installed via `make tools`)

Install all development tools:

```bash
make tools
```

## Quick Start

```bash
# Build and test
make build
make test

# Run linting
make lint

# Run full CI check
make ci
```

## Testing

### Unit Tests

Run all unit tests with race detection (recommended):

```bash
make test-race
```

This detects concurrency bugs in the connection pool and tunnel code.

### Test Coverage

Generate a coverage report:

```bash
make test-coverage
open .coverage/coverage.html
```

### Integration Tests

Integration tests require real infrastructure (SSH endpoints). They are skipped by default:

```bash
# Run integration tests
TEST_INTEGRATION=1 go test -v -tags=integration ./...

# Or use make
make test-integration
```

The integration tests in `internal/tunnel/tunnel_integration_test.go` start ephemeral SSH servers to test tunnel functionality without requiring external infrastructure.

### Mocking OCI APIs

Use the mock client for tests that need OCI behavior without real credentials:

```go
import "github.com/scott/tunatap/internal/client"

func TestMyFunction(t *testing.T) {
    mock := client.NewMockOCIClient()

    // Set up mock data
    mock.AddCompartment("infra/k8s", "ocid1.compartment.oc1..abc")
    mock.AddCluster(&containerengine.Cluster{
        Id:   ptr("ocid1.cluster.oc1..xyz"),
        Name: ptr("prod-cluster"),
    })

    // Configure behavior
    mock.ShouldFailSession = true  // Simulate failures
    mock.SessionActiveDelay = 2 * time.Second  // Add delays

    // Your test code here...

    // Verify calls
    calls := mock.GetCalls()
    if len(calls) != 3 {
        t.Error("Expected 3 API calls")
    }
}
```

## Linting

### Running Linters

```bash
# Run all linters
make lint

# Auto-fix issues
make lint-fix
```

### Linter Configuration

The `.golangci.yml` file configures:

- **Security linters**: gosec, bodyclose
- **Bug detection**: nilerr, nilnil, contextcheck
- **Code quality**: gocritic, gofmt, goimports, misspell
- **Concurrency**: copyloopvar (Go 1.22+)
- **Style**: revive, whitespace

### Suppressing False Positives

Use `//nolint` comments when needed:

```go
//nolint:gosec // G304: Path is validated before use
data, err := os.ReadFile(configPath)
```

## Debugging

### Interactive Debugging with Delve

Build with debug symbols:

```bash
make debug
```

Start debugging:

```bash
# Debug the CLI
dlv exec ./tunatap-debug -- connect my-cluster

# Debug a test
dlv test ./internal/pool -- -test.run TestConnectionPool
```

Common delve commands:
- `b main.main` - Set breakpoint
- `c` - Continue
- `n` - Next line
- `s` - Step into
- `p variable` - Print variable
- `goroutines` - List goroutines
- `stack` - Show stack trace

### VS Code Debugging

Add to `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug tunatap",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}",
            "args": ["connect", "my-cluster"]
        },
        {
            "name": "Debug Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/internal/pool",
            "args": ["-test.run", "TestConnectionPool"]
        }
    ]
}
```

## Profiling

### CPU Profiling

```bash
# Add to your code temporarily:
import "runtime/pprof"

f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()

# Analyze:
go tool pprof cpu.prof
```

### Memory Profiling

```bash
# Add to your code:
f, _ := os.Create("mem.prof")
pprof.WriteHeapProfile(f)

# Analyze:
go tool pprof mem.prof
```

### Goroutine Leak Detection

Add uber-go/goleak to your tests:

```go
import "go.uber.org/goleak"

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

## Releases

### Versioning

Use semantic versioning: `vMAJOR.MINOR.PATCH`

- MAJOR: Breaking changes
- MINOR: New features, backward compatible
- PATCH: Bug fixes

### Creating a Release

1. Update version references if needed
2. Create and push a tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

3. GitHub Actions will automatically:
   - Run tests
   - Build binaries for all platforms
   - Create a GitHub release
   - Upload artifacts

### Local Release Testing

Test the release process locally:

```bash
make release-snapshot
ls dist/
```

## CI/CD

### GitHub Actions

Two workflows are configured:

1. **ci.yml**: Runs on every push and PR
   - Linting (golangci-lint)
   - Tests with race detection
   - Security scanning (gosec)
   - Multi-platform builds

2. **release.yml**: Runs on version tags
   - Full test suite
   - GoReleaser for binary creation
   - GitHub Release creation

### Running CI Locally

```bash
make ci
```

This runs the same checks as CI: dependency verification, formatting, linting, race-detected tests, and coverage.

## Project Structure

```
tunatap/
├── cmd/                    # CLI commands (Cobra)
├── internal/
│   ├── client/             # OCI SDK wrapper + mock for testing
│   ├── pool/               # SSH connection pooling
│   ├── tunnel/             # SSH tunnel implementation
│   ├── bastion/            # OCI Bastion integration
│   └── ...                 # Other packages
├── pkg/utils/              # Shared utilities
├── .github/workflows/      # CI/CD configuration
├── .golangci.yml           # Linter configuration
├── .goreleaser.yml         # Release configuration
├── Makefile                # Development commands
└── DEVELOPMENT.md          # This file
```

## Common Tasks

### Adding a New Command

1. Create `cmd/newcmd.go`
2. Add command to root in `init()`
3. Add tests in `cmd/newcmd_test.go`

### Adding a New Package

1. Create `internal/newpkg/`
2. Follow existing patterns for interfaces
3. Add tests in `internal/newpkg/newpkg_test.go`
4. Update `architecture.md`

### Updating Dependencies

```bash
make deps-update
make deps-tidy
make test
```
