# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build the project
make build                    # or: go build -o tunatap .

# Build with version info
make build                    # Includes git tag, commit, build time

# Cross-platform builds
make build-all                # Builds for linux, darwin, windows (amd64, arm64)

# Build with debug symbols (for delve)
make debug                    # Creates tunatap-debug binary
```

## Testing Commands

```bash
# Run all tests
make test                     # or: go test -v ./...

# Run tests with race detector (RECOMMENDED for concurrency bugs)
make test-race                # or: go test -v -race ./...

# Run tests with coverage
make test-coverage            # Generates .coverage/coverage.html

# Run integration tests (requires real SSH endpoints)
make test-integration         # or: TEST_INTEGRATION=1 go test -v -tags=integration ./...

# Run benchmarks
make test-bench               # or: go test -v -bench=. -benchmem ./...

# Run a specific test
go test ./internal/pool -run TestConnectionPool

# Run tests for a specific package
go test -v ./internal/pool
```

## Linting and Static Analysis

```bash
# Run all linters (golangci-lint)
make lint                     # Includes security checks via gosec

# Run linters and auto-fix issues
make lint-fix

# Check code formatting
make fmt-check

# Format code
make fmt

# Run go vet
make vet

# Run security scanner specifically
make security                 # or: gosec ./...
```

## Debugging

```bash
# Build debug binary
make debug

# Start delve debugger
dlv exec ./tunatap-debug -- connect my-cluster

# Debug a specific test
dlv test ./internal/pool -- -test.run TestConnectionPool

# Attach to running process
dlv attach <pid>
```

## Profiling

```bash
# Run with CPU profiling
./tunatap --pprof-cpu connect my-cluster
# Then: go tool pprof cpu.prof

# Run with memory profiling
./tunatap --pprof-mem connect my-cluster
# Then: go tool pprof mem.prof

# Check for goroutine leaks (in tests)
# Add uber-go/goleak to your tests
```

## Release Commands

```bash
# Create a snapshot release (local testing)
make release-snapshot

# Create a full release (requires git tag and GITHUB_TOKEN)
git tag v1.0.0
git push origin v1.0.0
# GitHub Actions will handle the release via goreleaser
```

## CI Commands

```bash
# Run full CI check locally
make ci                       # deps-verify, fmt-check, lint, test-race, test-coverage
```

## Install Development Tools

```bash
make tools                    # Installs golangci-lint, gosec, delve, goreleaser, mockgen
```

## Architecture Overview

Tunatap is an SSH tunnel manager for OCI (Oracle Cloud Infrastructure) Bastion services, primarily used for connecting to private OKE (Oracle Kubernetes Engine) clusters.

### Data Flow

1. User runs `tunatap connect <cluster>`
2. Config is loaded from `~/.tunatap/config.yaml`
3. OCI client authenticates using `~/.oci/config`
4. Bastion session is created/retrieved via OCI API
5. SSH connection pool is established to bastion host
6. Local TCP listener accepts connections and forwards through the tunnel

### Package Dependencies

```
cmd/
  └── calls internal/config, internal/bastion, internal/cluster, internal/client

internal/bastion/
  └── orchestrates tunneling, calls internal/tunnel, internal/client, internal/config

internal/tunnel/
  └── manages SSH tunnels, calls internal/pool

internal/pool/
  └── manages SSH connection pooling (standalone)

internal/cluster/
  └── cluster validation, calls internal/client

internal/client/
  └── OCI SDK wrapper (standalone)

internal/config/
  └── config types and YAML I/O, calls internal/state

internal/state/
  └── global state singleton (standalone)

pkg/utils/
  └── cross-platform path helpers (standalone)
```

### Key Components

**Connection Pool** (`internal/pool/`): Manages multiple SSH connections with concurrent use tracking. Each `TrackedSSHConnection` allows multiple simultaneous uses up to `maxConcurrent` before creating new connections.

**SSH Tunnel** (`internal/tunnel/`): Listens on a local port and forwards TCP connections through the SSH tunnel to a remote endpoint. Uses the connection pool for efficient SSH reuse.

**Bastion Types**: Two bastion modes are supported:
- `STANDARD`: Uses OCI Bastion service sessions with SSH port forwarding
- `INTERNAL`: Uses a jump box with the internal bastion load balancer

### Configuration

Config file location: `~/.tunatap/config.yaml`

Key config structures are in `internal/config/config.go`:
- `Config`: Root config with SSH settings, tenancies, and clusters
- `Cluster`: Per-cluster settings including region, bastion, and endpoints
- `ClusterEndpoint`: IP and port for cluster API endpoints

### Cross-Platform Considerations

All path handling uses `pkg/utils/paths.go` helpers which use `filepath.Join()` and `os.UserHomeDir()`. Never use hardcoded Unix paths like `/home` or `~/.ssh`.
